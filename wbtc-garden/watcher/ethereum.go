package watcher

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/catalogfi/cobi/wbtc-garden/blockchain"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/screener"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/ethereum"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/ethereum/typings/AtomicSwap"
	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
)

type EthereumWatcher struct {
	chain          model.Chain
	netConfig      model.NetworkConfig
	startBlock     uint64
	interval       time.Duration
	store          Store
	atomicSwapAddr common.Address
	client         *ethclient.Client
	ABI            *abi.ABI
	AtomincSwap    *AtomicSwap.AtomicSwap
	screener       screener.Screener
	logger         *zap.Logger
}

func NewEthereumWatcher(store Store, chain model.Chain, config model.Config, address common.Address, startBlock uint64, screener screener.Screener, logger *zap.Logger) (*EthereumWatcher, error) {
	client, err := blockchain.LoadClient(chain, config.Network)
	if err != nil {
		return nil, fmt.Errorf("failed to load client: %v", err)
	}

	ethClient, ok := client.(ethereum.Client)
	if !ok {
		return nil, fmt.Errorf("invalid client type: %T", client)
	}

	atomicSwap, err := AtomicSwap.NewAtomicSwap(address, ethClient.GetProvider())
	if err != nil {
		return nil, err
	}
	atomicSwapAbi, err := AtomicSwap.AtomicSwapMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return &EthereumWatcher{
		chain:          chain,
		netConfig:      config.Network[chain],
		interval:       10 * time.Second,
		store:          store,
		atomicSwapAddr: address,
		client:         ethClient.GetProvider(),
		startBlock:     startBlock,
		AtomincSwap:    atomicSwap,
		screener:       screener,
		ABI:            atomicSwapAbi,
		logger:         logger,
	}, nil
}

func (w *EthereumWatcher) Watch() {
	for {
		eventIds := [][]common.Hash{{
			w.ABI.Events["Initiated"].ID,
			w.ABI.Events["Redeemed"].ID,
			w.ABI.Events["Refunded"].ID,
		}}
		startBlock := w.startBlock

		currentBlock, err := w.client.BlockNumber(context.Background())
		if err != nil {
			w.logger.Error("failed to get current block number", zap.Error(err))
			return
		}

		logs, err := w.getLogs(startBlock, currentBlock, eventIds)
		if err != nil {
			w.logger.Error("failed to get logs", zap.Error(err))
			return
		}

		for _, log := range logs {
			switch log.Topics[0] {
			case eventIds[0][0]:
				if err := w.handleInitiate(log); err != nil {
					w.logger.Error("failed to initiate", zap.Error(err))
					return
				}
			case eventIds[0][1]:
				if err := w.handleRedeem(log); err != nil {
					w.logger.Error("failed to redeem", zap.Error(err))
					return
				}
			case eventIds[0][2]:
				if err := w.handleRefund(log); err != nil {
					w.logger.Error("failed to refund", zap.Error(err))
					return
				}
			}
		}

		// update confirmation status of unconfirmed initiates
		swaps, err := w.store.GetActiveSwaps(w.chain)
		if err != nil {
			w.logger.Error("failed to get unconfirmed swaps", zap.Error(err))
			return
		}
		if len(swaps) != 0 {
			for _, swap := range swaps {
				if currentBlock > swap.InitiateBlockNumber {
					confirmations := currentBlock - swap.InitiateBlockNumber
					if confirmations != swap.CurrentConfirmations {
						swap.CurrentConfirmations = confirmations
						if confirmations > swap.MinimumConfirmations {
							swap.CurrentConfirmations = swap.MinimumConfirmations
						}
						if err := w.store.UpdateSwap(&swap); err != nil {
							w.logger.Error("failed to update swap", zap.Error(err))
							return
						}
					}
				}
			}
		}

		w.startBlock = currentBlock
		time.Sleep(w.interval)
	}
}

func (w *EthereumWatcher) getLogs(fromBlock, toBlock uint64, eventIds [][]common.Hash) ([]types.Log, error) {
	midBlock := toBlock
	if w.netConfig.EventWindow < int64(fromBlock)-int64(toBlock) {
		midBlock = fromBlock + uint64(w.netConfig.EventWindow)
	}
	eventlogs := []types.Log{}
	for {
		query := geth.FilterQuery{
			FromBlock: big.NewInt(int64(fromBlock)),
			ToBlock:   big.NewInt(int64(midBlock)),
			Addresses: []common.Address{
				w.atomicSwapAddr,
			},
			Topics: eventIds,
		}
		logs, err := w.client.FilterLogs(context.Background(), query)
		if len(logs) > 0 {
			return logs, nil
		}
		if err != nil {
			return nil, err
		}
		eventlogs = append(eventlogs, logs...)
		if midBlock == toBlock {
			break
		}
		fromBlock = midBlock + 1
		midBlock = fromBlock + uint64(w.netConfig.EventWindow)
		if midBlock > toBlock {
			midBlock = toBlock
		}
	}
	return eventlogs, nil
}

func (w *EthereumWatcher) handleInitiate(log types.Log) error {
	swap, err := w.store.SwapByOCID(log.Topics[1].Hex())
	if err != nil {
		return err
	}

	cSwap, err := w.AtomincSwap.AtomicSwapOrders(nil, log.Topics[1])
	if err != nil {
		return err
	}

	isBlacklisted, err := w.screener.IsBlacklisted(map[string]model.Chain{cSwap.Initiator.Hex(): w.chain})
	if err != nil {
		return err
	}
	if isBlacklisted {
		return fmt.Errorf("blacklisted deposits detected")
	}

	amount, ok := new(big.Int).SetString(swap.Amount, 10)
	if !ok {
		return fmt.Errorf("invalid amount: %s", swap.Amount)
	}
	if cSwap.Amount.Cmp(amount) < 0 {
		return fmt.Errorf("insufficient amount: %s", swap.Amount)
	}
	if cSwap.Expiry.Cmp(big.NewInt(w.netConfig.Expiry)) == 0 {
		return fmt.Errorf("incorrect expiry: %s", swap.Amount)
	}
	swap.InitiateTxHash = log.TxHash.String()
	swap.InitiateBlockNumber = log.BlockNumber
	swap.OnChainIdentifier = log.Topics[1].Hex()

	return w.store.UpdateSwap(&swap)
}

func (w *EthereumWatcher) handleRedeem(log types.Log) error {
	swap, err := w.store.SwapByOCID(log.Topics[1].Hex())
	if err != nil {
		return err
	}
	eventValues := make(map[string]interface{})
	if err := w.ABI.UnpackIntoMap(eventValues, "Redeemed", log.Data); err != nil {
		return err
	}
	secret, ok := eventValues["secret"].([]byte)
	if !ok {
		return fmt.Errorf("invalid secret: %v", eventValues["secret"])
	}
	swap.Secret = hex.EncodeToString(secret)
	if swap.RedeemTxHash != "" {
		return nil
	}
	return w.store.UpdateSwap(&swap)
}

func (w *EthereumWatcher) handleRefund(log types.Log) error {
	swap, err := w.store.SwapByOCID(log.Topics[1].Hex())
	if err != nil {
		return err
	}
	if swap.RefundTxHash != "" {
		return nil
	}
	swap.RefundTxHash = log.TxHash.String()
	return w.store.UpdateSwap(&swap)
}

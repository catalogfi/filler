package executor

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/catalogfi/cobi/pkg/swap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/cobi/pkg/util"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
)

type EvmExecutor struct {
	logger  *zap.Logger
	wallets map[model.Chain]ethswap.Wallet
	clients map[model.Chain]*ethclient.Client
	storage Store
	dialer  util.WsClientDialer
	signer  string

	swaps map[model.Chain]chan ActionItem
	quit  chan struct{}
}

func NewEvmExecutor(logger *zap.Logger, wallets map[model.Chain]ethswap.Wallet, clients map[model.Chain]*ethclient.Client, storage Store, dialer util.WsClientDialer) *EvmExecutor {
	// Signer should be the same as the eth wallet address. We assume all evm wallets have the same address.
	signer := ""
	swaps := map[model.Chain]chan ActionItem{}
	for chain, wallet := range wallets {
		signer = wallet.Address().Hex()
		swaps[chain] = make(chan ActionItem, 16)
	}

	return &EvmExecutor{
		logger:  logger,
		wallets: wallets,
		clients: clients,
		storage: storage,
		dialer:  dialer,
		signer:  signer,

		quit: make(chan struct{}),
	}
}

func (ee *EvmExecutor) Start() {
	// Execute swaps
	for chain, swaps := range ee.swaps {
		chain := chain
		swaps := swaps
		go ee.chainWorker(chain, swaps)
	}

	go func() {
		for {
			ee.logger.Info(fmt.Sprintf("subscribing to orders of %v", ee.signer))
			client := ee.dialer()
			client.Subscribe(fmt.Sprintf("subscribe::%v", ee.signer))
			respChan := client.Listen()

		InnerLoop:
			for {
				select {
				case resp, ok := <-respChan:
					if !ok {
						break InnerLoop
					}

					switch response := resp.(type) {
					case rest.WebsocketError:
						break InnerLoop
					case rest.UpdatedOrders:
						for _, order := range response.Orders {
							if err := ee.processOrder(order); err != nil {
								ee.logger.Error("process order", zap.Error(err))
							}
						}
					}
				case <-ee.quit:
					return
				}
			}

			time.Sleep(5 * time.Second)
		}
	}()
}

func (ee *EvmExecutor) Stop() {
	if ee.quit != nil {
		close(ee.quit)
		ee.quit = nil
	}
}

func (ee *EvmExecutor) processOrder(order model.Order) error {
	if order.Status == model.Filled {
		order.FollowerAtomicSwap.SecretHash = order.SecretHash  // this is not populated by the orderbook
		order.InitiatorAtomicSwap.SecretHash = order.SecretHash // this is not populated by the orderbook

		// We're the taker
		if order.Taker == ee.signer {
			if order.InitiatorAtomicSwap.Status == model.Initiated &&
				order.FollowerAtomicSwap.Status == model.NotStarted {
				ee.execute(swap.ActionInitiate, order.FollowerAtomicSwap)
			} else if order.InitiatorAtomicSwap.Status == model.Initiated &&
				order.FollowerAtomicSwap.Status == model.Redeemed {
				if order.FollowerAtomicSwap.Secret == "" {
					return fmt.Errorf("missing secret")
				}
				order.InitiatorAtomicSwap.Secret = order.FollowerAtomicSwap.Secret
				ee.execute(swap.ActionRedeem, order.InitiatorAtomicSwap)
			} else if order.FollowerAtomicSwap.Status == model.Expired {
				ee.execute(swap.ActionRefund, order.FollowerAtomicSwap)
			}
		}
	}
	return nil
}

func (ee *EvmExecutor) execute(action swap.Action, atomicSwap *model.AtomicSwap) {
	swapChain, ok := ee.swaps[atomicSwap.Chain]
	if !ok {
		// Skip execution since the chain is not supported
		return
	}

	swapChain <- ActionItem{
		Action: action,
		Swap:   atomicSwap,
	}
}

func (ee *EvmExecutor) chainWorker(chain model.Chain, swaps chan ActionItem) {
	for item := range swaps {
		// Check if we have done the same action before
		done, err := ee.storage.CheckAction(item.Action, item.Swap.ID)
		if err != nil {
			ee.logger.Error("failed storing action", zap.Error(err))
			continue
		}
		if done {
			continue
		}

		ethSwap, err := ethswap.FromAtomicSwap(item.Swap)
		if err != nil {
			ee.logger.Error("parse swap", zap.Error(err))
			continue
		}

		// Execute the swap action
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			wallet := ee.wallets[chain]
			client := ee.clients[chain]
			var txHash string
			switch item.Action {
			case swap.ActionInitiate:
				initiated, err := ethSwap.Initiated(ctx, client)
				if err != nil {
					ee.logger.Error("check swap initiated", zap.Error(err))
					return
				}
				if initiated {
					return
				}
				txHash, err = wallet.Initiate(ctx, ethSwap)
			case swap.ActionRedeem:
				var secret []byte
				secret, err = hex.DecodeString(item.Swap.Secret)
				if err != nil {
					ee.logger.Error("decode secret", zap.Error(err))
					return
				}
				txHash, err = wallet.Redeem(ctx, ethSwap, secret)
			case swap.ActionRefund:
				txHash, err = wallet.Refund(ctx, ethSwap)
			default:
				return
			}
			ee.logger.Info("Execution done", zap.String("chain", string(chain)), zap.String("hash", txHash))

			// Store the action we have done and make sure we're not doing it again
			if err := ee.storage.RecordAction(item.Action, item.Swap.ID); err != nil {
				ee.logger.Error("failed storing action", zap.Error(err))
			}
		}()
	}
}

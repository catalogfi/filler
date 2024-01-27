package executor

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/catalogfi/cobi/pkg/swap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/cobi/pkg/util"
	"github.com/catalogfi/orderbook/model"
	"github.com/catalogfi/orderbook/rest"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"go.uber.org/zap"
)

type RetriableError struct {
	Err error
}

func (re RetriableError) Error() string {
	return re.Err.Error()
}

func NewRetriableError(err error) error {
	return RetriableError{err}
}

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
		signer = strings.ToLower(wallet.Address().Hex())
		swaps[chain] = make(chan ActionItem, 16)
	}

	return &EvmExecutor{
		logger:  logger,
		wallets: wallets,
		clients: clients,
		storage: storage,
		dialer:  dialer,
		signer:  signer,

		swaps: swaps,
		quit:  make(chan struct{}),
	}
}

func (ee *EvmExecutor) Start() {
	// Spin up a worker for each of the evm chain to execute swaps
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
				(order.FollowerAtomicSwap.Status == model.Redeemed ||
					order.FollowerAtomicSwap.Status == model.RedeemDetected) {
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
		ethSwap, err := ethswap.FromAtomicSwap(item.Swap)
		if err != nil {
			ee.logger.Error("parse swap", zap.Error(err))
			continue
		}
		log.Printf("%v swap, id = %v", item.Action, hex.EncodeToString(ethSwap.ID[:]))

		// Execute the swap action
		err = func() error {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()

			wallet := ee.wallets[chain]
			client := ee.clients[chain]
			var txHash common.Hash
			switch item.Action {
			case swap.ActionInitiate:
				var initiated bool
				initiated, err = ethSwap.Initiated(ctx, client)
				if err != nil {
					return NewRetriableError(err)
				}
				if initiated {
					ee.logger.Debug("⚠️ skip swap initiation", zap.String("chain", string(chain)), zap.Uint("swap", item.Swap.ID))
					return nil
				}
				txHash, err = wallet.Initiate(ctx, ethSwap)
			case swap.ActionRedeem:
				var secret []byte
				secret, err = hex.DecodeString(item.Swap.Secret)
				if err != nil {
					return err
				}
				txHash, err = wallet.Redeem(ctx, ethSwap, secret)
			case swap.ActionRefund:
				var expired bool
				expired, err = ethSwap.Expired(ctx, wallet.Client())
				if err != nil {
					log.Printf("checking expired %v", err)
					return NewRetriableError(err)
				}
				if !expired {
					log.Printf("swap not expired %v", item.Swap.ID)
					return NewRetriableError(fmt.Errorf("swap not expired"))
				}
				txHash, err = wallet.Refund(ctx, ethSwap)
			default:
				return nil
			}
			if err != nil {
				return NewRetriableError(err)
			}

			ee.logger.Info("✅ [Execution]", zap.String("chain", string(chain)), zap.String("hash", txHash.Hex()), zap.Uint("swap", item.Swap.ID))
			return nil
		}()

		if err != nil {
			// Retry after 30 seconds if it's a RetriableError
			var re RetriableError
			if errors.As(err, &re) {
				go func(item ActionItem) {
					time.Sleep(time.Minute)
					swaps <- item
				}(item)
			}
			ee.logger.Error("❌ [Execution]", zap.String("chain", string(chain)), zap.Error(err), zap.Uint("swap", item.Swap.ID), zap.String("action", string(item.Action)))
		}
	}
}

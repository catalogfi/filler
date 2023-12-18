package executor

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/orderbook/model"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/zap"
)

func (e *executor) StartEthExecutor(ctx context.Context) chan SwapMsg {
	e.logger.With(zap.String("ethereum executor", string(e.options.ETHChain))).Info("starting executor")
	swapChan := make(chan SwapMsg)
	go func() {
		defer e.chainWg.Done()
		for {
			select {
			case swap := <-swapChan:
				e.executeEthSwap(swap)
			case <-ctx.Done():
				e.logger.With(zap.String("ethereum executor", string(e.options.ETHChain))).Info("stopping executor")
				return
			}
		}
	}()
	return swapChan
}

func (e *executor) executeEthSwap(atomicSwap SwapMsg) {
	context, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := e.logger.With(zap.String("ethereum executor", string(e.options.ETHChain)), zap.Uint64("order-id", atomicSwap.OrderId))
	logger.Info("executing eth swap")
	status, err := e.store.Status(atomicSwap.Swap.SecretHash)
	if err != nil {
		logger.Error("order not found", zap.Error(err))
		return
	}

	ethSwap, err := e.getEthSwap(atomicSwap)
	if err != nil {
		logger.Error("failed to get eth swap", zap.Error(err))
		return
	}
	walletAddr := e.ethWallet.Address()

	if walletAddr == ethSwap.Initiator {
		switch atomicSwap.Swap.Status {
		case model.NotStarted:
			if (atomicSwap.Type == Initiator && status >= store.InitiatorInitiated) || (atomicSwap.Type == Follower && status >= store.FollowerInitiated) {
				return
			}
			if atomicSwap.Type == Follower && atomicSwap.CounterSwapStatus != model.Initiated {
				return
			}
			txHash, err := e.ethWallet.Initiate(context, &ethSwap)
			if err != nil {
				logger.Error("failed to initiate", zap.Error(err))
				var failedStatus store.Status
				if atomicSwap.Type == Initiator {
					failedStatus = store.InitiatorFailedToInitiate
				} else {
					failedStatus = store.FollowerFailedToInitiate
				}
				dbErr := e.store.UpdateOrderStatus(atomicSwap.Swap.SecretHash, failedStatus, err)
				if dbErr != nil {
					logger.Info("failed to update order status", zap.Error(dbErr))
				}
				return
			} else {
				var successStatus store.Status
				if atomicSwap.Type == Initiator {
					successStatus = store.InitiatorInitiated
				} else {
					successStatus = store.FollowerInitiated
				}
				e.store.UpdateOrderStatus(atomicSwap.Swap.SecretHash, successStatus, err)
				e.store.UpdateTxHash(atomicSwap.Swap.SecretHash, store.Initiated, txHash)
				logger.Info("initiate tx hash", zap.String("tx-hash", txHash))
			}
		case model.Expired:
			if (atomicSwap.Type == Initiator && status >= store.InitiatorRefunded) || (atomicSwap.Type == Follower && status >= store.FollowerRefunded) {
				return
			}
			txHash, err := e.ethWallet.Refund(context, &ethSwap)
			if err != nil {
				logger.Error("failed to refund", zap.Error(err))
				var failedStatus store.Status
				if atomicSwap.Type == Initiator {
					failedStatus = store.InitiatorFailedToRefund
				} else {
					failedStatus = store.FollowerFailedToRefund
				}
				dbErr := e.store.UpdateOrderStatus(atomicSwap.Swap.SecretHash, failedStatus, err)
				if dbErr != nil {
					logger.Info("failed to update order status", zap.Error(dbErr))
				}
				return
			} else {
				// TODO : combine these two calls in store
				var successStatus store.Status
				if atomicSwap.Type == Initiator {
					successStatus = store.InitiatorRefunded
				} else {
					successStatus = store.FollowerRefunded
				}
				e.store.UpdateOrderStatus(atomicSwap.Swap.SecretHash, successStatus, err)
				e.store.UpdateTxHash(atomicSwap.Swap.SecretHash, store.Refunded, txHash)
				logger.Info("refund tx hash", zap.String("tx-hash", txHash))
			}
		}
	} else if walletAddr == ethSwap.Redeemer {
		switch atomicSwap.Swap.Status {
		case model.Initiated:
			if (atomicSwap.Type == Initiator && status >= store.InitiatorRedeemed) || (atomicSwap.Type == Follower && status >= store.FollowerRedeemed) {
				return
			}
			if atomicSwap.CounterSwapStatus != model.Initiated {
				return
			}
			var secret []byte
			if atomicSwap.Type == Initiator {

				secretStr, err := e.store.Secret(atomicSwap.Swap.SecretHash)
				if err != nil {
					logger.Error("failed to get secret", zap.Error(err))
					return
				}
				secret, err = hex.DecodeString(secretStr)
				if err != nil {
					logger.Error("failed to decode secret", zap.Error(err))
					return
				}
			} else {
				secret, err = hex.DecodeString(atomicSwap.Swap.Secret)
				if err != nil {
					logger.Error("failed to decode secret", zap.Error(err))
					return
				}
			}
			txHash, err := e.ethWallet.Redeem(context, &ethSwap, secret)
			if err != nil {
				logger.Error("failed to redeem", zap.Error(err))
				var failedStatus store.Status
				if atomicSwap.Type == Initiator {
					failedStatus = store.InitiatorFailedToRedeem
				} else {
					failedStatus = store.FollowerFailedToRedeem
				}
				dbErr := e.store.UpdateOrderStatus(atomicSwap.Swap.SecretHash, failedStatus, err)
				if dbErr != nil {
					logger.Info("failed to update order status", zap.Error(dbErr))
				}
				return
			} else {
				var successStatus store.Status
				if atomicSwap.Type == Initiator {
					successStatus = store.InitiatorRedeemed
				} else {
					successStatus = store.FollowerRedeemed
				}
				e.store.UpdateOrderStatus(atomicSwap.Swap.SecretHash, successStatus, err)
				e.store.UpdateTxHash(atomicSwap.Swap.SecretHash, store.Redeemed, txHash)
				logger.Info("redeem tx hash", zap.String("tx-hash", txHash))
			}
		}
	}

}

func (e *executor) getEthSwap(atomicSwap SwapMsg) (ethswap.Swap, error) {
	waitBlocks, ok := new(big.Int).SetString(atomicSwap.Swap.Timelock, 10)
	if !ok {
		return ethswap.Swap{}, fmt.Errorf("failed to decode timelock")
	}
	amount, ok := new(big.Int).SetString(atomicSwap.Swap.Amount, 10)
	if !ok {
		return ethswap.Swap{}, fmt.Errorf("failed to decode amount")
	}
	// todo : check if address is hex
	initiatorAddr := common.HexToAddress(atomicSwap.Swap.InitiatorAddress)

	redeemerAddr := common.HexToAddress(atomicSwap.Swap.RedeemerAddress)

	contractAddr := common.HexToAddress(string(atomicSwap.Swap.Asset))

	ethSwap, err := ethswap.NewSwap(initiatorAddr, redeemerAddr, contractAddr, common.HexToHash(atomicSwap.Swap.SecretHash), amount, waitBlocks)
	if err != nil {
		return ethswap.Swap{}, fmt.Errorf("failed to decode initiator address,err :%v", err)
	}
	return *ethSwap, err
}

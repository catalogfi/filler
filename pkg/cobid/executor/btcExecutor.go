package executor

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/orderbook/model"
	"go.uber.org/zap"
)

func (b *executor) StartBtcExecutor(ctx context.Context) chan SwapMsg {
	b.logger.With(zap.String("ethereum executor", string(b.options.BTCChain))).Info("starting executor")
	swapChan := make(chan SwapMsg)
	go func() {
		defer b.chainWg.Done()
		for {
			select {
			case swap := <-swapChan:
				b.executeBtcSwap(swap)
			case <-ctx.Done():
				b.logger.With(zap.String("bitcoin executor", string(b.options.BTCChain))).Info("stopping executor")
				return
			}
		}
	}()
	return swapChan
}

func (b *executor) executeBtcSwap(atomicSwap SwapMsg) {
	context := context.Background()
	logger := b.logger.With(zap.String("bitcoin executor", string(b.options.BTCChain)), zap.Uint64("order-id", atomicSwap.OrderId))
	logger.Info("executing btc swap")
	status, err := b.store.Status(atomicSwap.Swap.SecretHash)
	if err != nil {
		logger.Error("order not found", zap.Error(err))
		return
	}

	btcSwap, err := b.getBTCSwap(atomicSwap)
	if err != nil {
		logger.Error("failed to get btc swap", zap.Error(err))
		return
	}

	walletAddr := b.btcWallet.Address().EncodeAddress()

	if btcSwap.IsInitiator(walletAddr) {
		switch atomicSwap.Swap.Status {
		case model.NotStarted:
			if status == store.InitiatorInitiated || status == store.InitiatorFailedToInitiate {
				return
			}
			if atomicSwap.Type == Follower && atomicSwap.CounterSwapStatus != model.Initiated {
				return
			}
			txHash, err := b.btcWallet.Initiate(context, btcSwap)
			if err != nil {
				logger.Error("failed to initiate", zap.Error(err))
				dbErr := b.store.UpdateOrderStatus(atomicSwap.Swap.SecretHash, store.InitiatorFailedToInitiate, err)
				if dbErr != nil {
					logger.Info("failed to update order status", zap.Error(dbErr))
				}
				return
			} else {
				b.store.UpdateOrderStatus(atomicSwap.Swap.SecretHash, store.InitiatorInitiated, err)
				b.store.UpdateTxHash(atomicSwap.Swap.SecretHash, store.Initiated, txHash)
				logger.Info("initiate tx hash", zap.String("tx-hash", txHash))
			}
		case model.Expired:
			if status == store.InitiatorRefunded || status == store.InitiatorFailedToRefund {
				return
			}
			txHash, err := b.btcWallet.Refund(context, btcSwap, walletAddr)
			if err != nil {
				logger.Error("failed to refund", zap.Error(err))
				dbErr := b.store.UpdateOrderStatus(atomicSwap.Swap.SecretHash, store.InitiatorFailedToRefund, err)
				if dbErr != nil {
					logger.Info("failed to update order status", zap.Error(dbErr))
				}
				return
			} else {
				dbErr := b.store.UpdateOrderStatus(atomicSwap.Swap.SecretHash, store.InitiatorRefunded, err)
				if dbErr != nil {
					logger.Info("failed to update order status", zap.Error(dbErr))
				}
				dbErr = b.store.UpdateTxHash(atomicSwap.Swap.SecretHash, store.Refunded, txHash)
				if dbErr != nil {
					logger.Info("failed to update txHash", zap.Error(err))
				}
				logger.Info("refund tx hash", zap.String("tx-hash", txHash))
			}
		}
	} else if btcSwap.IsRedeemer(walletAddr) {
		switch atomicSwap.Swap.Status {
		case model.Initiated:
			if status == store.FollowerRedeemed || status == store.FollowerFailedToRedeem {
				return
			}
			var secret []byte
			if atomicSwap.Type == Initiator {
				if atomicSwap.CounterSwapStatus != model.Initiated {
					return
				}
				secretStr, err := b.store.Secret(atomicSwap.Swap.SecretHash)
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
			txHash, err := b.btcWallet.Redeem(context, btcSwap, secret, walletAddr)
			if err != nil {
				logger.Error("failed to redeem", zap.Error(err))
				dbErr := b.store.UpdateOrderStatus(atomicSwap.Swap.SecretHash, store.FollowerFailedToRedeem, err)
				if dbErr != nil {
					logger.Info("failed to update order status", zap.Error(dbErr))
				}
				return
			} else {
				b.store.UpdateOrderStatus(atomicSwap.Swap.SecretHash, store.FollowerRedeemed, err)
				b.store.UpdateTxHash(atomicSwap.Swap.SecretHash, store.Redeemed, txHash)
				logger.Info("redeem tx hash", zap.String("tx-hash", txHash))
			}
		}
	}

}

func (b *executor) getBTCSwap(atomicSwap SwapMsg) (btcswap.Swap, error) {
	secretHash, err := hex.DecodeString(atomicSwap.Swap.SecretHash)
	if err != nil {
		return btcswap.Swap{}, fmt.Errorf("failed to decode secretHash,err:%v", err)
	}
	waitBlocks, err := strconv.ParseInt(atomicSwap.Swap.Timelock, 10, 64)
	if err != nil {
		return btcswap.Swap{}, fmt.Errorf("failed to decode timelock,err:%v", err)
	}
	amount, err := strconv.ParseInt(atomicSwap.Swap.Amount, 10, 64)
	if err != nil {
		return btcswap.Swap{}, fmt.Errorf("failed to decode amount,err:%v", err)

	}
	initiatorAddr, err := btcutil.DecodeAddress(atomicSwap.Swap.InitiatorAddress, atomicSwap.Swap.Chain.Params())
	if err != nil {
		return btcswap.Swap{}, fmt.Errorf("failed to decode initiator address,err:%v", err)
	}
	redeemerAddr, err := btcutil.DecodeAddress(atomicSwap.Swap.RedeemerAddress, atomicSwap.Swap.Chain.Params())
	if err != nil {
		return btcswap.Swap{}, fmt.Errorf("failed to decode redeemer address,err:%v", err)
	}
	btcSwap, err := btcswap.NewSwap(atomicSwap.Swap.Chain.Params(), initiatorAddr, redeemerAddr, amount, secretHash, waitBlocks)
	if err != nil {
		return btcswap.Swap{}, fmt.Errorf("failed to decode initiator address,err:%v", err)
	}
	return btcSwap, nil
}

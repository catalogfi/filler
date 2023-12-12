package executor

import (
	"context"
	"encoding/hex"
	"strconv"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/orderbook/model"
	"go.uber.org/zap"
)

// type btcExecutor struct {
// 	btcWallet btcswap.Wallet
// 	store     store.Store
// 	logger    *zap.Logger
// 	quit      chan struct{}
// 	wg        *sync.WaitGroup
// }

// type BTCExecutor interface {
// 	Start() chan SwapMsg
// 	Done()
// }

// func NewBtcExecutor(
// 	btcWallet btcswap.Wallet,
// 	store store.Store,
// 	logger *zap.Logger,
// 	quit chan struct{},
// 	wg *sync.WaitGroup) BTCExecutor {
// 	return &btcExecutor{
// 		btcWallet: btcWallet,
// 		store:     store,
// 		logger:    logger,
// 		quit:      quit,
// 		wg:        wg,
// 	}

// }

//	func (b *btcExecutor) Done() {
//		b.quit <- struct{}{}
//	}
func (b *executor) StartBtcExecutor(ctx context.Context) (swapChan chan SwapMsg) {
	defer b.wg.Done()
	go func() {
		for {
			select {
			case swap := <-swapChan:
				b.executeBtcSwap(swap.Orderid, swap.Swap)
			case <-ctx.Done():
				return
			}
		}
	}()
	return swapChan
}

func (b *executor) executeBtcSwap(orderID uint64, swap model.AtomicSwap) {
	context := context.Background()
	logger := b.logger.With(zap.Uint64("order-id", orderID))
	status, err := b.store.Status(swap.SecretHash)
	if err != nil {
		logger.Error("order not found", zap.Error(err))
		return
	}

	secret, err := hex.DecodeString(swap.Secret)
	if err != nil {
		logger.Error("failed to decode secret", zap.Error(err))
		return
	}
	secretHash, err := hex.DecodeString(swap.SecretHash)
	if err != nil {
		logger.Error("failed to decode secretHash", zap.Error(err))
		return
	}
	waitBlocks, err := strconv.ParseInt(swap.Timelock, 10, 64)
	if err != nil {
		logger.Error("failed to decode timelock", zap.Error(err))
		return
	}
	amount, err := strconv.ParseInt(swap.Amount, 10, 64)
	if err != nil {
		logger.Error("failed to decode amount", zap.Error(err))
		return
	}
	initiatorAddr, err := btcutil.DecodeAddress(swap.InitiatorAddress, swap.Chain.Params())
	if err != nil {
		logger.Error("failed to decode initiator address", zap.Error(err))
		return
	}
	redeemerAddr, err := btcutil.DecodeAddress(swap.RedeemerAddress, swap.Chain.Params())
	if err != nil {
		logger.Error("failed to decode redeemer address", zap.Error(err))
		return
	}
	btcSwap, err := btcswap.NewSwap(swap.Chain.Params(), initiatorAddr, redeemerAddr, amount, secretHash, waitBlocks)
	if err != nil {
		logger.Error("failed to decode initiator address", zap.Error(err))
		return
	}
	walletAddr := b.btcWallet.Address().EncodeAddress()

	if btcSwap.IsInitiator(walletAddr) {
		switch swap.Status {
		case model.NotStarted:
			if status == store.InitiatorInitiated || status == store.InitiatorFailedToInitiate {
				return
			}
			txHash, err := b.btcWallet.Initiate(context, btcSwap)
			if err != nil {
				b.store.UpdateOrderStatus(swap.SecretHash, store.InitiatorFailedToInitiate, err)
				return
			} else {
				b.store.UpdateOrderStatus(swap.SecretHash, store.InitiatorInitiated, err)
				b.store.UpdateTxHash(swap.SecretHash, store.Initiated, txHash)
			}
		case model.Expired:
			if status == store.InitiatorRefunded || status == store.InitiatorFailedToRefund {
				return
			}
			txHash, err := b.btcWallet.Refund(context, btcSwap, walletAddr)
			if err != nil {
				b.store.UpdateOrderStatus(swap.SecretHash, store.InitiatorFailedToRefund, err)
				return
			} else {
				b.store.UpdateOrderStatus(swap.SecretHash, store.InitiatorRefunded, err)
				b.store.UpdateTxHash(swap.SecretHash, store.Refunded, txHash)
			}
		}
	} else if btcSwap.IsRedeemer(walletAddr) {
		switch swap.Status {
		case model.Initiated:
			if status == store.FollowerRedeemed || status == store.FollowerFailedToRedeem {
				return
			}
			txHash, err := b.btcWallet.Redeem(context, btcSwap, secret, walletAddr)
			if err != nil {
				b.store.UpdateOrderStatus(swap.SecretHash, store.FollowerFailedToRedeem, err)
				return
			} else {
				b.store.UpdateOrderStatus(swap.SecretHash, store.FollowerRedeemed, err)
				b.store.UpdateTxHash(swap.SecretHash, store.Redeemed, txHash)
			}
		}
	}

}

package executor

import (
	"context"
	"encoding/hex"
	"math/big"

	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/orderbook/model"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/zap"
)

func (e *executor) StartEthExecutor(ctx context.Context) (swapChan chan SwapMsg) {
	e.logger.With(zap.String("ethereum executor", string(e.options.ETHChain))).Info("starting executor")
	go func() {
		defer e.chainWg.Done()
		for {
			select {
			case swap := <-swapChan:
				e.executeEthSwap(swap.Orderid, swap.Swap)
			case <-ctx.Done():
				e.logger.With(zap.String("ethereum executor", string(e.options.ETHChain))).Info("stopping executor")
				return
			}
		}
	}()
	return swapChan
}

func (e *executor) executeEthSwap(orderID uint64, swap model.AtomicSwap) {
	context := context.Background()
	logger := e.logger.With(zap.String("ethereum executor", string(e.options.ETHChain)), zap.Uint64("order-id", orderID))
	status, err := e.store.Status(swap.SecretHash)
	if err != nil {
		logger.Error("order not found", zap.Error(err))
		return
	}

	secret, err := hex.DecodeString(swap.Secret)
	if err != nil {
		logger.Error("failed to decode secret", zap.Error(err))
		return
	}
	waitBlocks, ok := new(big.Int).SetString(swap.Timelock, 10)
	if !ok {
		logger.Error("failed to decode timelock", zap.Error(err))
		return
	}
	amount, ok := new(big.Int).SetString(swap.Amount, 10)
	if !ok {
		logger.Error("failed to decode amount", zap.Error(err))
		return
	}
	// todo : check if address is hex
	initiatorAddr := common.HexToAddress(swap.InitiatorAddress)

	redeemerAddr := common.HexToAddress(swap.RedeemerAddress)

	contractAddr := common.HexToAddress(string(swap.Asset))

	ethSwap, err := ethswap.NewSwap(initiatorAddr, redeemerAddr, contractAddr, common.HexToHash(swap.SecretHash), amount, waitBlocks)
	if err != nil {
		logger.Error("failed to decode initiator address", zap.Error(err))
		return
	}
	walletAddr := e.ethWallet.Address()

	if walletAddr == initiatorAddr {
		switch swap.Status {
		case model.NotStarted:
			if status == store.InitiatorInitiated || status == store.InitiatorFailedToInitiate {
				return
			}
			txHash, err := e.ethWallet.Initiate(context, ethSwap)
			if err != nil {
				e.store.UpdateOrderStatus(swap.SecretHash, store.InitiatorFailedToInitiate, err)
			} else {
				e.store.UpdateOrderStatus(swap.SecretHash, store.InitiatorInitiated, err)
				e.store.UpdateTxHash(swap.SecretHash, store.Initiated, txHash)
			}
		case model.Expired:
			if status == store.InitiatorRefunded || status == store.InitiatorFailedToRefund {
				return
			}
			txHash, err := e.ethWallet.Refund(context, ethSwap)
			if err != nil {
				e.store.UpdateOrderStatus(swap.SecretHash, store.InitiatorFailedToRefund, err)
			} else {
				e.store.UpdateOrderStatus(swap.SecretHash, store.InitiatorRefunded, err)
				e.store.UpdateTxHash(swap.SecretHash, store.Refunded, txHash)
			}
		}
	} else if walletAddr == redeemerAddr {
		switch swap.Status {
		case model.Initiated:
			if status == store.FollowerRedeemed || status == store.FollowerFailedToRedeem {
				return
			}
			txHash, err := e.ethWallet.Redeem(context, ethSwap, secret)
			if err != nil {
				e.store.UpdateOrderStatus(swap.SecretHash, store.FollowerFailedToRedeem, err)
			} else {
				e.store.UpdateOrderStatus(swap.SecretHash, store.FollowerRedeemed, err)
				e.store.UpdateTxHash(swap.SecretHash, store.Redeemed, txHash)
			}
		}
	}

}

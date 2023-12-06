package executor

import (
	"context"
	"encoding/hex"
	"math/big"
	"sync"

	"github.com/catalogfi/cobi/pkg/store"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
	"github.com/catalogfi/orderbook/model"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/zap"
)

type ethExecutor struct {
	ethWallet ethswap.Wallet
	store     store.Store
	logger    *zap.Logger
	quit      chan struct{}
	wg        *sync.WaitGroup
}

type EthExecutor interface {
	Start(account uint32, isIw bool)
	Done()
}

func NewEthExecutor(
	ethWallet ethswap.Wallet,
	store store.Store,
	logger *zap.Logger,
	quit chan struct{},
	wg *sync.WaitGroup) BTCExecutor {
	return &ethExecutor{
		ethWallet: ethWallet,
		store:     store,
		logger:    logger,
		quit:      quit,
		wg:        wg,
	}

}

func (b *ethExecutor) Done() {
	b.quit <- struct{}{}
}

func (b *ethExecutor) Start() chan SwapMsg {
	defer b.wg.Done()
	var swapChan chan SwapMsg
	go func() {
		for {
			select {
			case swap := <-swapChan:
				b.executeSwap(swap.Orderid, swap.Swap)
			case <-b.quit:
				b.logger.Info("stopping executor")
				return
			}
		}
	}()
	return swapChan
}

func (b *ethExecutor) executeSwap(orderID uint64, swap model.AtomicSwap) {
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
	walletAddr := b.ethWallet.Address()

	if walletAddr == initiatorAddr {
		switch swap.Status {
		case model.NotStarted:
			if status == store.InitiatorInitiated || status == store.InitiatorFailedToInitiate {
				return
			}
			txHash, err := b.ethWallet.Initiate(context, ethSwap)
			if err != nil {
				b.store.UpdateOrderStatus(swap.SecretHash, store.InitiatorFailedToInitiate, err)
			} else {
				b.store.UpdateOrderStatus(swap.SecretHash, store.InitiatorInitiated, err)
				b.store.UpdateTxHash(swap.SecretHash, store.Initiated, txHash)
			}
		case model.Expired:
			if status == store.InitiatorRefunded || status == store.InitiatorFailedToRefund {
				return
			}
			txHash, err := b.ethWallet.Refund(context, ethSwap)
			if err != nil {
				b.store.UpdateOrderStatus(swap.SecretHash, store.InitiatorFailedToRefund, err)
			} else {
				b.store.UpdateOrderStatus(swap.SecretHash, store.InitiatorRefunded, err)
				b.store.UpdateTxHash(swap.SecretHash, store.Refunded, txHash)
			}
		}
	} else if walletAddr == redeemerAddr {
		switch swap.Status {
		case model.Initiated:
			if status == store.FollowerRedeemed || status == store.FollowerFailedToRedeem {
				return
			}
			txHash, err := b.ethWallet.Redeem(context, ethSwap, secret)
			if err != nil {
				b.store.UpdateOrderStatus(swap.SecretHash, store.FollowerFailedToRedeem, err)
			} else {
				b.store.UpdateOrderStatus(swap.SecretHash, store.FollowerRedeemed, err)
				b.store.UpdateTxHash(swap.SecretHash, store.Redeemed, txHash)
			}
		}
	}

}

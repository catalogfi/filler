package ethereum

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"fmt"
	"math/big"
	"time"

	"github.com/catalogfi/cobi/wbtc-garden/swapper"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/ethereum/typings/AtomicSwap"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

const MaxQueryBlockRange = 2000

type initiatorSwap struct {
	orderID          [32]byte
	initiator        *ecdsa.PrivateKey
	initiatorAddr    common.Address
	redeemerAddr     common.Address
	lastCheckedBlock *big.Int
	expiry           *big.Int
	atomicSwapAddr   common.Address
	secretHash       []byte
	client           Client
	amount           *big.Int
	tokenAddr        common.Address
	watcher          swapper.Watcher
}
type redeemerSwap struct {
	orderID          [32]byte
	redeemer         *ecdsa.PrivateKey
	lastCheckedBlock *big.Int
	expiry           *big.Int
	atomicSwapAddr   common.Address
	tokenAddr        common.Address
	amount           *big.Int
	secretHash       []byte
	client           Client
	watcher          swapper.Watcher
}

func NewInitiatorSwap(initiator *ecdsa.PrivateKey, redeemerAddr, atomicSwapAddr common.Address, secretHash []byte, expiry, minConfirmations, amount *big.Int, client Client, eventWindow int64) (swapper.InitiatorSwap, error) {

	initiatorAddr := crypto.PubkeyToAddress(initiator.PublicKey)
	orderId := sha256.Sum256(append(secretHash, initiatorAddr.Hash().Bytes()...))

	latestCheckedBlock := new(big.Int).Sub(expiry, big.NewInt(12000))
	if latestCheckedBlock.Cmp(big.NewInt(0)) == -1 {
		latestCheckedBlock = big.NewInt(0)
	}

	watcher, err := NewWatcher(atomicSwapAddr, secretHash, orderId[:], expiry, minConfirmations, amount, client, eventWindow)
	if err != nil {
		return &initiatorSwap{}, err
	}
	tokenAddr, err := client.GetTokenAddress(atomicSwapAddr)
	if err != nil {
		return &initiatorSwap{}, err
	}
	return &initiatorSwap{
		orderID:          orderId,
		initiator:        initiator,
		watcher:          watcher,
		initiatorAddr:    initiatorAddr,
		expiry:           expiry,
		atomicSwapAddr:   atomicSwapAddr,
		client:           client,
		amount:           amount,
		tokenAddr:        tokenAddr,
		redeemerAddr:     redeemerAddr,
		lastCheckedBlock: latestCheckedBlock,
		secretHash:       secretHash,
	}, nil
}

func (initiatorSwap *initiatorSwap) Initiate() (string, error) {
	return initiatorSwap.client.InitiateAtomicSwap(initiatorSwap.atomicSwapAddr, initiatorSwap.initiator, initiatorSwap.redeemerAddr, initiatorSwap.tokenAddr, initiatorSwap.expiry, initiatorSwap.amount, initiatorSwap.secretHash)
}

func (initiatorSwap *initiatorSwap) Expired() (bool, error) {
	return initiatorSwap.watcher.Expired()
}

func (initiatorSwap *initiatorSwap) WaitForRedeem() ([]byte, string, error) {
	for {
		redeemed, secret, txHash, err := initiatorSwap.IsRedeemed()
		if err != nil {
			fmt.Println("failed to check redeemed status", err)
			time.Sleep(5 * time.Second)
			continue
		}
		if redeemed {
			return secret, txHash, err
		}
		time.Sleep(5 * time.Second)
	}
}

func (initiatorSwap *initiatorSwap) IsRedeemed() (bool, []byte, string, error) {
	return initiatorSwap.watcher.IsRedeemed()
}

func (initiatorSwap *initiatorSwap) Refund() (string, error) {

	// Initialise the transactor
	transactor, err := initiatorSwap.client.GetTransactOpts(initiatorSwap.initiator)
	if err != nil {
		return "", err
	}

	tx, err := initiatorSwap.client.RefundAtomicSwap(initiatorSwap.atomicSwapAddr, transactor, initiatorSwap.tokenAddr, initiatorSwap.secretHash)
	if err != nil {
		return "", err
	}
	return tx, nil
}

func NewRedeemerSwap(redeemer *ecdsa.PrivateKey, initiatorAddr, atomicSwapAddr common.Address, secretHash []byte, expiry, amount, minConfirmations *big.Int, client Client, eventWindow int64) (swapper.RedeemerSwap, error) {
	orderId := sha256.Sum256(append(secretHash, initiatorAddr.Hash().Bytes()...))
	watcher, err := NewWatcher(atomicSwapAddr, secretHash, orderId[:], expiry, minConfirmations, amount, client, eventWindow)
	if err != nil {
		return &redeemerSwap{}, err
	}

	tokenAddr, err := client.GetTokenAddress(atomicSwapAddr)
	if err != nil {
		return &redeemerSwap{}, err
	}

	lastCheckedBlock := new(big.Int).Sub(expiry, big.NewInt(12000))
	return &redeemerSwap{
		orderID:          orderId,
		redeemer:         redeemer,
		watcher:          watcher,
		lastCheckedBlock: lastCheckedBlock,
		expiry:           expiry,
		atomicSwapAddr:   atomicSwapAddr,
		tokenAddr:        tokenAddr,
		amount:           amount,
		client:           client,
		secretHash:       secretHash,
	}, nil
}

func (redeemerSwap *redeemerSwap) Redeem(secret []byte) (string, error) {
	transactor, err := redeemerSwap.client.GetTransactOpts(redeemerSwap.redeemer)
	if err != nil {
		return "", err
	}
	return redeemerSwap.client.RedeemAtomicSwap(redeemerSwap.atomicSwapAddr, transactor, redeemerSwap.tokenAddr, redeemerSwap.orderID, secret)
}

func (redeemerSwap *redeemerSwap) IsInitiated() (bool, []string, uint64, error) {
	return redeemerSwap.watcher.IsInitiated()
}

func (redeemerSwap *redeemerSwap) WaitForInitiate() ([]string, error) {
	for {
		initiated, txHash, _, err := redeemerSwap.IsInitiated()
		if initiated {
			return txHash, nil
		}
		if err != nil {
			fmt.Println("failed to check initiated status", err)
		}
		time.Sleep(5 * time.Second)
	}
}

type watcher struct {
	client           Client
	atomicSwapAddr   common.Address
	lastCheckedBlock *big.Int
	amount           *big.Int
	expiry           *big.Int
	secretHash       []byte
	orderId          []byte
	minConfirmations *big.Int
	initiatedBlock   *big.Int
	eventWindow      *big.Int
}

func NewWatcher(atomicSwapAddr common.Address, secretHash, orderId []byte, expiry, minConfirmations, amount *big.Int, client Client, eventWindow int64) (swapper.Watcher, error) {
	latestCheckedBlock := new(big.Int).Sub(expiry, big.NewInt(12000))
	if latestCheckedBlock.Cmp(big.NewInt(0)) == -1 {
		latestCheckedBlock = big.NewInt(0)
	}
	return &watcher{
		client:           client,
		atomicSwapAddr:   atomicSwapAddr,
		lastCheckedBlock: latestCheckedBlock,
		expiry:           expiry,
		amount:           amount,
		secretHash:       secretHash,
		minConfirmations: minConfirmations,
		orderId:          orderId,
		eventWindow:      big.NewInt(eventWindow),
	}, nil
}

func (watcher *watcher) Expired() (bool, error) {
	initiated, _, _, _ := watcher.IsInitiated()
	if !initiated {
		return false, nil
	}
	currentBlock, err := watcher.client.GetCurrentBlock()
	if err != nil {
		return false, err
	}
	if currentBlock > watcher.initiatedBlock.Uint64()+watcher.expiry.Uint64() {
		return true, nil
	} else {
		return false, nil
	}
}

func (watcher *watcher) IsInitiated() (bool, []string, uint64, error) {
	currBlock, err := watcher.client.GetCurrentBlock()
	if err != nil {
		return false, []string{}, 0, err
	}
	currentBlock := big.NewInt(int64(currBlock))
	// if currentBlock.Int64() > watcher.lastCheckedBlock.Int64()+MaxQueryBlockRange {
	// 	currentBlock = big.NewInt(0).Add(watcher.lastCheckedBlock, big.NewInt(MaxQueryBlockRange))
	// }

	atomicSwapAbi, err := AtomicSwap.AtomicSwapMetaData.GetAbi()
	if err != nil {
		return false, []string{}, 0, err
	}

	initiatedEvent := atomicSwapAbi.Events["Initiated"]
	eventIds := [][]common.Hash{{initiatedEvent.ID}, {common.BytesToHash(watcher.orderId)}, {common.BytesToHash(watcher.secretHash)}}
	logs, err := watcher.checkLogs(currentBlock, eventIds)
	if len(logs) == 0 {
		return false, []string{}, 0, err
	}

	vLog := logs[0]

	isFinal, progress, err := watcher.client.IsFinal(vLog.TxHash.Hex(), watcher.minConfirmations.Uint64())
	if err != nil {
		return false, []string{}, 0, err
	}

	if !isFinal {
		return false, []string{}, progress, nil
	}

	watcher.initiatedBlock = new(big.Int).SetUint64(vLog.BlockNumber)
	return true, []string{vLog.TxHash.Hex()}, watcher.minConfirmations.Uint64(), nil
}

func (watcher *watcher) IsRedeemed() (bool, []byte, string, error) {
	currBlock, err := watcher.client.GetCurrentBlock()
	if err != nil {
		return false, nil, "", err
	}
	currentBlock := big.NewInt(int64(currBlock))
	// if currentBlock.Int64() > watcher.lastCheckedBlock.Int64()+MaxQueryBlockRange {
	// 	currentBlock = big.NewInt(0).Add(watcher.lastCheckedBlock, big.NewInt(MaxQueryBlockRange))
	// }

	atomicSwapAbi, err := AtomicSwap.AtomicSwapMetaData.GetAbi()
	if err != nil {
		return false, nil, "", err
	}

	redeemedEvent := atomicSwapAbi.Events["Redeemed"]
	eventIds := [][]common.Hash{{redeemedEvent.ID}, {common.BytesToHash(watcher.orderId)}, {common.BytesToHash(watcher.secretHash)}}
	logs, err := watcher.checkLogs(currentBlock, eventIds)
	if len(logs) == 0 {
		return false, nil, "", err
	}

	if len(logs) == 0 {
		// Update the last checked block height
		// newLastCheckedBlock := big.NewInt(0).Sub(currentBlock, watcher.minConfirmations)
		// if newLastCheckedBlock.Cmp(watcher.lastCheckedBlock) == 1 {
		// 	watcher.lastCheckedBlock = currentBlock
		// }
		return false, nil, "", err
	}

	vLog := logs[0]

	val, err := redeemedEvent.Inputs.Unpack(vLog.Data)
	if err != nil {
		return false, nil, "", err
	}

	return true, []byte(val[0].([]uint8)), vLog.TxHash.Hex(), nil
}

func (watcher *watcher) IsRefunded() (bool, string, error) {
	currBlock, err := watcher.client.GetCurrentBlock()
	if err != nil {
		return false, "", err
	}
	currentBlock := big.NewInt(int64(currBlock))
	// if currentBlock.Int64() > watcher.lastCheckedBlock.Int64()+MaxQueryBlockRange {
	// 	currentBlock = big.NewInt(0).Add(watcher.lastCheckedBlock, big.NewInt(MaxQueryBlockRange))
	// }

	atomicSwapAbi, err := AtomicSwap.AtomicSwapMetaData.GetAbi()
	if err != nil {
		return false, "", err
	}

	refundedEvent := atomicSwapAbi.Events["Refunded"]
	eventIds := [][]common.Hash{{refundedEvent.ID}, {common.BytesToHash(watcher.orderId)}}
	logs, err := watcher.checkLogs(currentBlock, eventIds)
	if len(logs) == 0 {
		return false, "", err
	}
	if err != nil {
		return false, "", err
	}

	if len(logs) == 0 {
		// Update the last checked block height
		// newLastCheckedBlock := big.NewInt(0).Sub(currentBlock, watcher.minConfirmations)
		// if newLastCheckedBlock.Cmp(watcher.lastCheckedBlock) == 1 {
		// 	watcher.lastCheckedBlock = currentBlock
		// }
		return false, "", err
	}
	return true, logs[0].TxHash.Hex(), nil
}

func (watcher *watcher) checkLogs(maxBlock *big.Int, eventIds [][]common.Hash) ([]types.Log, error) {
	leastWindow := new(big.Int).Sub(maxBlock, watcher.eventWindow)

	for maxBlock.Cmp(leastWindow) >= 0 {
		query := ethereum.FilterQuery{
			FromBlock: new(big.Int).Sub(maxBlock, big.NewInt(MaxQueryBlockRange)),
			ToBlock:   maxBlock,
			Addresses: []common.Address{
				watcher.atomicSwapAddr,
			},
			Topics: eventIds,
		}
		logs, err := watcher.client.GetProvider().FilterLogs(context.Background(), query)
		if len(logs) > 0 {
			return logs, nil
		}
		if err != nil {
			return nil, err
		}
		maxBlock = maxBlock.Sub(maxBlock, big.NewInt(MaxQueryBlockRange))
	}
	return nil, nil
}

package ethereum

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/catalogfi/cobi/pkg/swapper"
	"github.com/catalogfi/cobi/pkg/swapper/ethereum/typings/AtomicSwap"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

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
	atomicSwap       *AtomicSwap.AtomicSwap
	// IiwRpc           string
}

func NewWatcher(atomicSwapAddr common.Address, secretHash, orderId []byte, expiry, minConfirmations, amount *big.Int, client Client, eventWindow int64) (swapper.Watcher, error) {
	latestCheckedBlock := new(big.Int).Sub(expiry, big.NewInt(12000))
	if latestCheckedBlock.Cmp(big.NewInt(0)) == -1 {
		latestCheckedBlock = big.NewInt(0)
	}

	atomicSwapInstance, _ := AtomicSwap.NewAtomicSwap(atomicSwapAddr, client.GetProvider())
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
		atomicSwap:       atomicSwapInstance,
	}, nil
}

func (watcher *watcher) Expired() (bool, error) {
	initiated, txHash, _, _, _ := watcher.IsInitiated()
	if !initiated {
		return false, nil
	}
	currentBlock, err := watcher.client.GetCurrentBlock()
	if err != nil {
		return false, err
	}
	height, _, _, err := watcher.Status(txHash)
	if err != nil {
		return false, err
	}
	if currentBlock > height+watcher.expiry.Uint64() {
		return true, nil
	} else {
		return false, nil
	}
}

func (watcher *watcher) Status(txHash string) (uint64, uint64, bool, error) {
	txHashes := strings.Split(txHash, ",")
	if len(txHashes) == 0 {
		return 0, 0, false, fmt.Errorf("empty initiate txhash list")
	}
	blockHeight, conf, err := watcher.client.GetConfirmations(txHashes[0])
	if err != nil {
		return 0, 0, false, fmt.Errorf("failed to get confirmations: %w", err)
	}
	if len(txHashes) > 1 {
		for _, txHash := range txHashes[1:] {
			nextBlockHeight, nextConf, err := watcher.client.GetConfirmations(txHash)
			if err != nil {
				return 0, 0, false, fmt.Errorf("failed to get confirmations: %w", err)
			}
			if nextBlockHeight < blockHeight {
				blockHeight = nextBlockHeight
			}
			if nextConf < conf {
				conf = nextConf
			}
		}
	}
	return blockHeight, conf, false, err
}

func (watcher *watcher) IsDetected() (bool, string, string, error) {
	atomicSwapAbi, err := AtomicSwap.AtomicSwapMetaData.GetAbi()
	if err != nil {
		return false, "", "", err
	}

	currBlock, err := watcher.client.GetCurrentBlock()
	if err != nil {
		return false, "", "", err
	}
	currentBlock := big.NewInt(int64(currBlock))
	initiatedEvent := atomicSwapAbi.Events["Initiated"]
	eventIds := [][]common.Hash{{initiatedEvent.ID}, {common.BytesToHash(watcher.orderId)}, {common.BytesToHash(watcher.secretHash)}}
	logs, err := watcher.checkLogs(currentBlock, eventIds)
	if len(logs) == 0 {
		return false, "", "", err
	}

	vLog := logs[0]

	values, err := atomicSwapAbi.Unpack("Initiated", vLog.Data)
	if err != nil {
		return false, "", "", fmt.Errorf("failed to unpack Initiated event data: %v", err)
	}

	val, ok := values[1].(*big.Int)
	if !ok {
		return false, "", "", fmt.Errorf("unable to decode amount from Initiated event data")
	}

	order, err := watcher.atomicSwap.AtomicSwapOrders(nil, common.BytesToHash(watcher.orderId))
	if err != nil {
		return false, "", "", err
	}
	if order.Expiry.Cmp(watcher.expiry) != 0 {
		return false, "", "", fmt.Errorf("inititate with wrong timelock")
	}

	if val.Cmp(watcher.amount) < 0 {
		return false, "", "", fmt.Errorf("initiated with lower than expected amount")
	}

	return true, vLog.TxHash.Hex(), val.String(), nil
}

func (watcher *watcher) IsInitiated() (bool, string, map[string]model.Chain, uint64, error) {
	currBlock, err := watcher.client.GetCurrentBlock()
	if err != nil {
		return false, "", nil, 0, err
	}
	currentBlock := big.NewInt(int64(currBlock))
	// if currentBlock.Int64() > watcher.lastCheckedBlock.Int64()+MaxQueryBlockRange {
	// 	currentBlock = big.NewInt(0).Add(watcher.lastCheckedBlock, big.NewInt(MaxQueryBlockRange))
	// }

	atomicSwapAbi, err := AtomicSwap.AtomicSwapMetaData.GetAbi()
	if err != nil {
		return false, "", nil, 0, err
	}

	initiatedEvent := atomicSwapAbi.Events["Initiated"]
	eventIds := [][]common.Hash{{initiatedEvent.ID}, {common.BytesToHash(watcher.orderId)}, {common.BytesToHash(watcher.secretHash)}}
	logs, err := watcher.checkLogs(currentBlock, eventIds)
	if len(logs) == 0 {
		return false, "", nil, 0, err
	}

	vLog := logs[0]

	isFinal, progress, err := watcher.client.IsFinal(vLog.TxHash.Hex(), watcher.minConfirmations.Uint64())
	if err != nil {
		return false, "", nil, 0, err
	}

	if !isFinal {
		return false, "", nil, progress, nil
	}

	senders := map[string]model.Chain{}
	if watcher.client.ChainID().Int64() == 1 {
		tx, _, err := watcher.client.GetProvider().TransactionByHash(context.Background(), vLog.TxHash)
		if err != nil {
			return false, "", nil, 0, err
		}
		signer := types.LatestSignerForChainID(watcher.client.ChainID())
		from, err := signer.Sender(tx)
		if err != nil {
			return false, "", nil, 0, err
		}
		senders[from.Hex()] = model.Ethereum
	}

	watcher.initiatedBlock = new(big.Int).SetUint64(vLog.BlockNumber)
	return true, vLog.TxHash.Hex(), senders, watcher.minConfirmations.Uint64(), nil
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

func (watcher *watcher) IsInstantWallet(txHash string) (bool, error) {
	return false, nil
}

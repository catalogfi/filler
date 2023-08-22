package bitcoin

import (
	"encoding/hex"
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/catalogfi/cobi/wbtc-garden/swapper"
)

// Watcher implements the `swapper.Watcher` interface. It watches a particular HTLC contract address and detect state
// change by querying the bitcoin network. People can use this to check what kind of the state the atomic swap is at.
// It assumes the HTLC contract address is disposable and will only be used once.
type watcher struct {
	client           Client
	scriptAddr       btcutil.Address
	amount           uint64
	minConfirmations uint64
	waitBlocks       int64
	initiatedBlock   uint64
	initiatedTxs     []string
}

func NewWatcher(scriptAddr btcutil.Address, waitBlocks int64, minConfirmations, amount uint64, client Client) (swapper.Watcher, error) {
	return &watcher{
		scriptAddr:       scriptAddr,
		amount:           amount,
		waitBlocks:       waitBlocks,
		minConfirmations: minConfirmations,
		client:           client,
	}, nil
}

func (w *watcher) Expired() (bool, error) {
	// Check if the swap has been initiated
	initiated, _, _, err := w.IsInitiated()
	if err != nil {
		return false, err
	}
	if !initiated {
		return false, nil
	}

	// Check the number of blocks has been passed since the initiation
	latest, err := w.client.GetTipBlockHeight()
	if err != nil {
		return false, err
	}
	diff := latest - w.initiatedBlock + 1
	return diff >= uint64(w.waitBlocks), nil
}

func (w *watcher) IsInitiated() (bool, []string, uint64, error) {
	if w.initiatedBlock != 0 && w.initiatedTxs != nil {
		return true, w.initiatedTxs, w.minConfirmations, nil
	}

	// Fetch all utxos
	utxos, bal, err := w.client.GetUTXOs(w.scriptAddr, 0)
	if err != nil {
		return false, nil, 0, fmt.Errorf("failed to get UTXOs: %w", err)
	}

	// Check all utxos are confirmed and greater than or equal to the required amount
	if bal >= w.amount && len(utxos) > 0 {
		latest, err := w.client.GetTipBlockHeight()
		if err != nil {
			return false, nil, 0, fmt.Errorf("failed to get tip block: %w", err)
		}
		txHashes := make([]string, len(utxos))
		lastConfirmedTxBlock := uint64(0)
		for i, utxo := range utxos {
			confirmation := latest - utxo.Status.BlockHeight + 1
			if utxo.Status == nil || !utxo.Status.Confirmed || confirmation < w.minConfirmations {
				if confirmation < w.minConfirmations && utxo.Status.BlockHeight > 0 {
					return false, nil, confirmation, nil
				}
				return false, nil, 0, nil
			}
			txHashes[i] = utxo.TxID
			if utxo.Status.BlockHeight > lastConfirmedTxBlock {
				lastConfirmedTxBlock = utxo.Status.BlockHeight
			}
		}

		// Cache the result
		w.initiatedBlock = lastConfirmedTxBlock
		w.initiatedTxs = txHashes

		return true, txHashes, w.minConfirmations, nil
	}
	return false, nil, 0, nil
}

// IsRedeemed checks if the secret has been revealed on-chain.
func (w *watcher) IsRedeemed() (bool, []byte, string, error) {
	// witness, tx, err := w.client.GetSpendingWitness(w.scriptAddr)
	// if err != nil {
	// 	return false, nil, "", fmt.Errorf("failed to get UTXOs: %w", err)
	// }
	// if len(witness) == 5 {
	// 	// Check if the redeem tx is confirmed
	// 	// latest, err := w.client.GetTipBlockHeight()
	// 	// if err != nil {
	// 	// 	return false, nil, "", fmt.Errorf("failed to get tip block: %w", err)
	// 	// }
	// 	// if !tx.Status.Confirmed || latest-tx.Status.BlockHeight+1 < w.minConfirmations {
	// 	// 	return false, nil, "", nil
	// 	// }

	// 	// inputs are [
	// 	// 0 : sig,
	// 	// 1 : spender.PubKey().SerializeCompressed(),
	// 	// 2 : secret,
	// 	// 3 :[]byte{0x1},
	// 	// 4 : script
	// 	// ]
	// 	secretBytes, err := hex.DecodeString(witness[2])
	// 	if err != nil {
	// 		return false, nil, "", fmt.Errorf("failed to decode secret: %w", err)
	// 	}

	// 	return true, secretBytes, tx.TxID, nil
	// }
	// return false, nil, "", nil
	witness, tx, err := w.client.GetSpendingWitness(w.scriptAddr)
	if err != nil {
		return false, nil, "", fmt.Errorf("failed to get UTXOs: %w", err)
	}
	if len(witness) == 5 {
		// fmt.Println("Redeemed:", witness)
		// inputs are [ 0 : sig, 1 : spender.PubKey().SerializeCompressed(),2 : secret, 3 :[]byte{0x1}, script]
		secretString := witness[2]
		secretBytes := make([]byte, hex.DecodedLen(len(secretString)))
		_, err := hex.Decode(secretBytes, []byte(secretString))
		if err != nil {
			return false, nil, "", fmt.Errorf("failed to decode secret: %w", err)
		}
		return true, secretBytes, tx.TxID, nil
	}
	return false, nil, "", nil
}

// IsRefunded checks if there's any refund tx with the script address. It will return `true` even if the script
// address is not fully refunded.
func (w *watcher) IsRefunded() (bool, string, error) {
	// witness, tx, err := w.client.GetSpendingWitness(w.scriptAddr)
	// if err != nil {
	// 	return false, "", fmt.Errorf("failed to get UTXOs: %w", err)
	// }

	// // Check if the refund tx is confirmed
	// latest, err := w.client.GetTipBlockHeight()
	// if err != nil {
	// 	return false, "", fmt.Errorf("failed to get tip block: %w", err)
	// }
	// if !tx.Status.Confirmed || latest-tx.Status.BlockHeight+1 < w.minConfirmations {
	// 	return false, "", nil
	// }

	// // inputs are [
	// // 0 : sig,
	// // 1 : spender.PubKey().SerializeCompressed(),
	// // 2 :[]byte{},
	// // script
	// // ]
	// return len(witness) == 4, tx.TxID, nil
	_, bal, err := w.client.GetUTXOs(w.scriptAddr, 0)
	if err != nil {
		return false, "", fmt.Errorf("failed to get UTXOs: %w", err)
	}
	witness, tx, err := w.client.GetSpendingWitness(w.scriptAddr)
	if err != nil {
		return false, "", fmt.Errorf("failed to get UTXOs: %w", err)
	}
	if len(witness) == 4 && bal == 0 {
		fmt.Println("Refunded:", witness)
		// inputs are [ 0 : sig, 1 : spender.PubKey().SerializeCompressed(), 2 :[]byte{}, script]
		return true, tx.TxID, nil

	}
	return false, "", nil
}

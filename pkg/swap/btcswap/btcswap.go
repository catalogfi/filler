package btcswap

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/blockchain/btc"
)

type Swap struct {
	network    *chaincfg.Params
	amount     int64
	secret     []byte
	secretHash []byte
	waitBlock  int64
	address    btcutil.Address
	initiator  btcutil.Address
	redeemer   btcutil.Address
	script     []byte
}

func NewSwap(network *chaincfg.Params, initiatorAddr, redeemer btcutil.Address, amount int64, secretHash []byte, waitBlock int64) (Swap, error) {
	htlc, err := btc.HtlcScript(initiatorAddr.ScriptAddress(), redeemer.ScriptAddress(), secretHash, waitBlock)
	if err != nil {
		return Swap{}, err
	}
	addr, err := btc.P2wshAddress(htlc, network)
	if err != nil {
		return Swap{}, err
	}

	return Swap{
		network:    network,
		amount:     amount,
		secretHash: secretHash,
		waitBlock:  waitBlock,
		address:    addr,
		initiator:  initiatorAddr,
		redeemer:   redeemer,
		script:     htlc,
	}, nil
}

// Initiated returns if the swap has been initiated. It will also return an uint64 which is the block height of the last
// confirmed initiated tx. The swap doesn't have an idea about block confirmations. It will let the caller decide if the
// swap initiation has reached enough confirmation.
func (swap *Swap) Initiated(ctx context.Context, client btc.IndexerClient) (bool, uint64, error) {

}

func (swap *Swap) Initiators(ctx context.Context, client btc.IndexerClient) ([]string, error) {
	// Fetch all utxos
	utxos, err := client.GetUTXOs(ctx, swap.address)
	if err != nil {
		return nil, fmt.Errorf("failed to get UTXOs: %w", err)
	}

	// Check we have enough confirmed utxos (total amount >= required amount)
	total, confirmedBlock := int64(0), uint64(0)
	txhashes := make([]string, 0, len(utxos))
	for _, utxo := range utxos {
		if utxo.Status != nil && utxo.Status.Confirmed {
			if utxo.Status.BlockHeight > confirmedBlock {
				confirmedBlock = utxo.Status.BlockHeight
			}
		}
	}
	if total >= swap.amount {
		txSendersMap := map[string]bool{}
		for _, hash := range txhashes {
			rawTx, err := client.GetTx(ctx, hash)
			if err != nil {
				return nil, err
			}
			for _, vin := range rawTx.VINs {
				txSendersMap[vin.Prevout.ScriptPubKeyAddress] = true
			}
		}

		// Convert it to a slice
		txSenders := make([]string, len(txSendersMap))
		for sender := range txSendersMap {
			txSenders = append(txSenders, sender)
		}
		return txSenders, nil
	}
	return nil, nil
}

func (swap *Swap) Redeemed(ctx context.Context, client btc.IndexerClient) (bool, []byte, error) {
	if len(swap.secret) != 0 {
		return true, swap.secret, nil
	}

	txs, err := client.GetAddressTxs(ctx, swap.address)
	if err != nil {
		return false, nil, err
	}
	for _, tx := range txs {
		for _, vin := range tx.VINs {
			if vin.Prevout.ScriptPubKeyAddress == swap.address.EncodeAddress() {
				if len(*vin.Witness) == 5 {
					// witness format
					// [
					//   0 : sig,
					//   1 : spender's public key,
					//   2 : secret,
					//   3 : []byte{0x1},
					//   4 : script
					// ]
					secretString := (*vin.Witness)[2]
					secretBytes := make([]byte, hex.DecodedLen(len(secretString)))
					_, err := hex.Decode(secretBytes, []byte(secretString))
					if err != nil {
						return false, nil, err
					}

					swap.secret = secretBytes
					return true, swap.secret, nil
				}
			}
		}
	}
	return false, nil, nil
}

func (swap *Swap) Expired(ctx context.Context, client btc.IndexerClient) (bool, error) {
	// Check if swap has been redeemed
	redeemed, _, err := swap.Redeemed(ctx, client)
	if err != nil {
		return false, err
	}
	if redeemed {
		return false, fmt.Errorf("swap has been redeemed")
	}

	// Check if swap has been initiated
	initiated, initiatedBlock, err := swap.Initiated(ctx, client)
	if err != nil {
		return false, err
	}
	if !initiated {
		return false, fmt.Errorf("swap not initiated")
	}

	// Get the number of blocks has been passed since the initiation
	current, err := client.GetTipBlockHeight(ctx)
	if err != nil {
		return false, err
	}
	return current-initiatedBlock+1 >= uint64(swap.waitBlock), nil
}

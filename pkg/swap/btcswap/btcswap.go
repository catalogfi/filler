package btcswap

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/catalogfi/blockchain/btc"
)

type Swap struct {
	network *chaincfg.Params
	// client     btc.IndexerClient
	amount     int64
	secretHash []byte
	// secret  *string
	waitBlock int64
	address   btcutil.Address
	initiator btcutil.Address
	redeemer  btcutil.Address
	script    []byte

	// After initiation
	initiationLock *sync.Mutex
	initiatedBlock uint64
	initiatedTx    string // currently support one tx
	initiatedAddrs []string

	// After redeeming
	redeemLock *sync.Mutex
	secret     []byte
}

func NewSwap(network *chaincfg.Params, client btc.IndexerClient, initiatorAddr, redeemer btcutil.Address, amount int64, secretHash []byte, waitBlock int64) (Swap, error) {
	htlc, err := btc.HtlcScript(initiatorAddr.ScriptAddress(), redeemer.ScriptAddress(), secretHash, waitBlock)
	if err != nil {
		return Swap{}, err
	}
	addr, err := btc.P2wshAddress(htlc, network)
	if err != nil {
		return Swap{}, err
	}

	return Swap{
		network:        network,
		client:         client,
		amount:         amount,
		secretHash:     secretHash,
		waitBlock:      waitBlock,
		address:        addr,
		initiator:      initiatorAddr,
		redeemer:       redeemer,
		script:         htlc,
		initiationLock: new(sync.Mutex),
		redeemLock:     new(sync.Mutex),
	}, nil
}

func (swap *Swap) Initiated(ctx context.Context) (bool, uint64, error) {
	swap.initiationLock.Lock()
	defer swap.initiationLock.Unlock()

	if len(swap.initiatedTx) != 0 && swap.initiatedBlock != 0 {
		return true, swap.initiatedBlock, nil
	}

	// Fetch all utxos
	utxos, err := swap.client.GetUTXOs(ctx, swap.address)
	if err != nil {
		return false, 0, fmt.Errorf("failed to get UTXOs: %w", err)
	}

	// Check we have enough confirmed utxos (total amount >= required amount)
	for _, utxo := range utxos {
		if utxo.Status != nil && utxo.Amount >= swap.amount {
			swap.initiatedTx = utxo.TxID
			if !utxo.Status.Confirmed {
				return true, 0, nil
			}
			swap.initiatedBlock = utxo.Status.BlockHeight
			return true, utxo.Status.BlockHeight, nil
		}
	}

	return false, 0, nil
}

func (swap *Swap) Initiators(ctx context.Context) ([]string, error) {
	swap.initiationLock.Lock()
	defer swap.initiationLock.Unlock()

	// Check swap is initiated
	if len(swap.initiatedTx) == 0 {
		return nil, nil
	}
	// Return previously cached result
	if len(swap.initiatedAddrs) != 0 {
		return swap.initiatedAddrs, nil
	}

	// Collect the senders
	txSendersMap := map[string]bool{}
	rawTx, err := swap.client.GetTx(ctx, swap.initiatedTx)
	if err != nil {
		return nil, err
	}
	for _, vin := range rawTx.VINs {
		txSendersMap[vin.Prevout.ScriptPubKeyAddress] = true
	}

	// Convert it to a slice
	txSenders := make([]string, len(txSendersMap))
	for sender := range txSendersMap {
		txSenders = append(txSenders, sender)
	}
	swap.initiatedAddrs = txSenders
	return txSenders, nil
}

func (swap *Swap) Initiate(ctx context.Context, key *btcec.PrivateKey, feeRate int) (string, error) {
	if swap.initiatedTx != "" {
		return "", fmt.Errorf("swap already initiated")
	}

	fromAddr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(key.PubKey().SerializeCompressed()), swap.network)
	if err != nil {
		return "", err
	}
	fromScript, err := txscript.PayToAddrScript(fromAddr)
	if err != nil {
		return "", err
	}

	// Get all utxos
	utxos, err := swap.client.GetUTXOs(ctx, fromAddr)
	if err != nil {
		return "", err
	}

	// Build the tx which transfer funds to the swap address
	recipients := []btc.Recipient{
		{
			To:     swap.address.EncodeAddress(),
			Amount: swap.amount,
		},
	}
	tx, err := btc.BuildTransaction(feeRate, swap.network, btc.NewRawInputs(), utxos, recipients, btc.P2wpkhUpdater, fromAddr)
	if err != nil {
		return "", err
	}

	// Sign the inputs
	fetcher := txscript.NewMultiPrevOutFetcher(nil)
	for _, utxo := range utxos {
		hash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return "", err
		}
		fetcher.AddPrevOut(wire.OutPoint{
			Hash:  *hash,
			Index: utxo.Vout,
		}, wire.NewTxOut(utxo.Amount, fromScript))
	}
	for i, utxo := range tx.TxIn {
		sigHashes := txscript.NewTxSigHashes(tx, fetcher)
		txOut := fetcher.FetchPrevOutput(utxo.PreviousOutPoint)
		witness, err := txscript.WitnessSignature(tx, sigHashes, i, txOut.Value, fromScript, txscript.SigHashAll, key, true)
		if err != nil {
			return "", err
		}
		tx.TxIn[i].Witness = witness
	}

	// Submit the transaction and cache the result
	if err := swap.client.SubmitTx(ctx, tx); err != nil {
		return "", err
	}

	swap.initiationLock.Lock()
	defer swap.initiationLock.Unlock()
	swap.initiatedTx = tx.TxHash().String()
	return swap.initiatedTx, nil
}

func (swap *Swap) Redeemed(ctx context.Context) (bool, []byte, error) {
	swap.redeemLock.Lock()
	defer swap.redeemLock.Unlock()

	if len(swap.secret) != 0 {
		return true, swap.secret, nil
	}

	txs, err := swap.client.GetAddressTxs(ctx, swap.address)
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

func (swap *Swap) Redeem(ctx context.Context, key *btcec.PrivateKey, secret []byte, feeRate int, target string) (string, error) {
	// Check if the swap has been initiated
	initiated, _, err := swap.Initiated(ctx)
	if err != nil {
		return "", err
	}
	if !initiated {
		return "", fmt.Errorf("swap not initiated")
	}

	// Build the transaction to redeem the funds
	utxos, err := swap.client.GetUTXOs(ctx, swap.address)
	if err != nil {
		return "", err
	}
	rawInputs := btc.RawInputs{
		VIN:        utxos,
		BaseSize:   0,
		SegwitSize: len(utxos) * btc.RedeemHtlcRedeemSigScriptSize(len(secret)),
	}
	recipient := []btc.Recipient{
		{
			To:     target,
			Amount: 0,
		},
	}
	tx, err := btc.BuildTransaction(feeRate, swap.network, rawInputs, nil, recipient, nil, nil)
	if err != nil {
		return "", err
	}

	// Sign the inputs
	fromScript, err := txscript.PayToAddrScript(swap.address)
	if err != nil {
		return "", err
	}
	fetcher := txscript.NewMultiPrevOutFetcher(nil)
	for _, utxo := range utxos {
		hash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return "", err
		}
		fetcher.AddPrevOut(wire.OutPoint{
			Hash:  *hash,
			Index: utxo.Vout,
		}, wire.NewTxOut(utxo.Amount, fromScript))
	}

	for i, utxo := range tx.TxIn {
		txOut := fetcher.FetchPrevOutput(utxo.PreviousOutPoint)
		sig, err := txscript.RawTxInWitnessSignature(tx, txscript.NewTxSigHashes(tx, fetcher), i, txOut.Value, swap.script, txscript.SigHashAll, key)
		if err != nil {
			return "", err
		}
		tx.TxIn[i].Witness = btc.HtlcWitness(swap.script, key.PubKey().SerializeCompressed(), sig, secret)
	}

	if err := swap.client.SubmitTx(ctx, tx); err != nil {
		return "", err
	}

	swap.redeemLock.Lock()
	defer swap.redeemLock.Unlock()
	swap.secret = secret
	return tx.TxHash().String(), nil
}

func (swap *Swap) Expired(ctx context.Context) (bool, error) {
	// Check if swap has been Redeemed
	initiated, _, err := swap.Initiated(ctx)
	if err != nil {
		return false, err
	}
	if !initiated {
		return false, fmt.Errorf("swap not initiated")
	}

	// Check if swap has been redeemed
	redeemed, _, err := swap.Redeemed(ctx)
	if err != nil {
		return false, err
	}
	if redeemed {
		return false, fmt.Errorf("swap has been redeemed")
	}

	// Get the number of blocks has been passed since the initiation
	current, err := swap.client.GetTipBlockHeight(ctx)
	if err != nil {
		return false, err
	}
	return current-swap.initiatedBlock+1 >= uint64(swap.waitBlock), nil
}

func (swap *Swap) Refund(ctx context.Context, key *btcec.PrivateKey, feeRate int, target string) error {
	// Check if the swap has been initiated
	expired, err := swap.Expired(ctx)
	if err != nil {
		return err
	}
	if !expired {
		return fmt.Errorf("swap not expired")
	}

	// Build the transaction for refunding
	utxos, err := swap.client.GetUTXOs(ctx, swap.address)
	if err != nil {
		return err
	}
	rawInputs := btc.RawInputs{
		VIN:        utxos,
		BaseSize:   0,
		SegwitSize: len(utxos) * btc.RedeemHtlcRefundSigScriptSize,
	}
	recipients := []btc.Recipient{
		{
			To:     target,
			Amount: 0,
		},
	}
	tx, err := btc.BuildTransaction(feeRate, swap.network, rawInputs, nil, recipients, nil, nil)
	if err != nil {
		return err
	}

	// Sign the inputs
	fromScript, err := txscript.PayToAddrScript(swap.address)
	if err != nil {
		return err
	}
	fetcher := txscript.NewMultiPrevOutFetcher(nil)
	for _, utxo := range utxos {
		hash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return err
		}
		fetcher.AddPrevOut(wire.OutPoint{
			Hash:  *hash,
			Index: utxo.Vout,
		}, wire.NewTxOut(utxo.Amount, fromScript))
	}
	for i := range tx.TxIn {
		tx.TxIn[i].Sequence = uint32(swap.waitBlock)
	}
	for i, utxo := range tx.TxIn {
		txOut := fetcher.FetchPrevOutput(utxo.PreviousOutPoint)
		sig, err := txscript.RawTxInWitnessSignature(tx, txscript.NewTxSigHashes(tx, fetcher), i, txOut.Value, swap.script, txscript.SigHashAll, key)
		if err != nil {
			return err
		}
		tx.TxIn[i].Witness = btc.HtlcWitness(swap.script, key.PubKey().SerializeCompressed(), sig, nil)
	}

	return swap.client.SubmitTx(ctx, tx)
}

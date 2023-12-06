package btcswap

import (
	"context"
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/catalogfi/blockchain/btc"
)

// todo :  have instant wallet integration
type Wallet interface {
	Address() btcutil.Address

	Balance(ctx context.Context, pending bool) (int64, error)

	Initiate(ctx context.Context, swap Swap) (string, error)

	Redeem(ctx context.Context, swap Swap, secret []byte, target string) (string, error)

	Refund(ctx context.Context, swap Swap, target string) (string, error)
}

type wallet struct {
	mu           *sync.Mutex
	opts         Options
	client       btc.IndexerClient
	feeEstimator btc.FeeEstimator
	key          *btcec.PrivateKey
	address      btcutil.Address
}

func NewWallet(opts Options, client btc.IndexerClient, key *btcec.PrivateKey, estimator btc.FeeEstimator) (Wallet, error) {
	addr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(key.PubKey().SerializeCompressed()), opts.Network)
	if err != nil {
		return nil, fmt.Errorf("fail to get wallet address")
	}
	return &wallet{
		mu:           new(sync.Mutex),
		opts:         opts,
		client:       client,
		feeEstimator: estimator,
		key:          key,
		address:      addr,
	}, nil
}

func (wallet *wallet) Address() btcutil.Address {
	return wallet.address
}

func (wallet *wallet) Balance(ctx context.Context, pending bool) (int64, error) {
	utxos, err := wallet.client.GetUTXOs(ctx, wallet.address)
	if err != nil {
		return 0, err
	}
	total := int64(0)
	for _, utxo := range utxos {
		total += utxo.Amount
	}
	return total, nil
}

func (wallet *wallet) Initiate(ctx context.Context, swap Swap) (string, error) {
	if swap.Network.Name != wallet.opts.Network.Name {
		return "", fmt.Errorf("wrong network")
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	// Get all utxos
	utxos, err := wallet.client.GetUTXOs(ctx, wallet.address)
	if err != nil {
		return "", err
	}
	feeRate, err := wallet.feeRate()
	if err != nil {
		return "", err
	}

	// Build the tx which transfer funds to the swap address
	recipients := []btc.Recipient{
		{
			To:     swap.Address.EncodeAddress(),
			Amount: swap.Amount,
		},
	}
	fromScript, err := txscript.PayToAddrScript(wallet.address)
	if err != nil {
		return "", err
	}
	tx, err := btc.BuildTransaction(feeRate, swap.Network, btc.NewRawInputs(), utxos, recipients, btc.P2wpkhUpdater, wallet.address)
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
		witness, err := txscript.WitnessSignature(tx, sigHashes, i, txOut.Value, fromScript, txscript.SigHashAll, wallet.key, true)
		if err != nil {
			return "", err
		}
		tx.TxIn[i].Witness = witness
	}

	// Submit the transaction and cache the result
	if err := wallet.client.SubmitTx(ctx, tx); err != nil {
		return "", err
	}
	return tx.TxHash().String(), nil
}

func (wallet *wallet) Redeem(ctx context.Context, swap Swap, secret []byte, target string) (string, error) {
	if swap.Network.Name != wallet.opts.Network.Name {
		return "", fmt.Errorf("wrong network")
	}

	// Check the swap is initialised before redeeming
	utxos, err := wallet.client.GetUTXOs(ctx, swap.Address)
	if err != nil {
		return "", err
	}
	if len(utxos) == 0 {
		return "", fmt.Errorf("swap not initialised")
	}

	// Build the transaction to redeem the funds
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
	feeRate, err := wallet.feeRate()
	if err != nil {
		return "", err
	}
	tx, err := btc.BuildTransaction(feeRate, swap.Network, rawInputs, nil, recipient, nil, nil)
	if err != nil {
		return "", err
	}

	// Sign the inputs
	fromScript, err := txscript.PayToAddrScript(swap.Address)
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
		sig, err := txscript.RawTxInWitnessSignature(tx, txscript.NewTxSigHashes(tx, fetcher), i, txOut.Value, swap.Script, txscript.SigHashAll, wallet.key)
		if err != nil {
			return "", err
		}
		tx.TxIn[i].Witness = btc.HtlcWitness(swap.Script, wallet.key.PubKey().SerializeCompressed(), sig, secret)
	}

	// Submit the tx
	if err := wallet.client.SubmitTx(ctx, tx); err != nil {
		return "", err
	}
	return tx.TxHash().String(), nil
}

func (wallet *wallet) Refund(ctx context.Context, swap Swap, target string) (string, error) {
	if swap.Network.Name != wallet.opts.Network.Name {
		return "", fmt.Errorf("wrong network")
	}

	// Check if the swap has been initiated
	expired, err := swap.Expired(ctx, wallet.client)
	if err != nil {
		return "", err
	}
	if !expired {
		return "", fmt.Errorf("swap not expired")
	}

	// Build the transaction for refunding
	utxos, err := wallet.client.GetUTXOs(ctx, swap.Address)
	if err != nil {
		return "", err
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
	feeRate, err := wallet.feeRate()
	if err != nil {
		return "", err
	}
	tx, err := btc.BuildTransaction(feeRate, swap.Network, rawInputs, nil, recipients, nil, nil)
	if err != nil {
		return "", err
	}

	// Sign the inputs
	fromScript, err := txscript.PayToAddrScript(swap.Address)
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
	for i := range tx.TxIn {
		tx.TxIn[i].Sequence = uint32(swap.WaitBlock)
	}
	for i, utxo := range tx.TxIn {
		txOut := fetcher.FetchPrevOutput(utxo.PreviousOutPoint)
		sig, err := txscript.RawTxInWitnessSignature(tx, txscript.NewTxSigHashes(tx, fetcher), i, txOut.Value, swap.Script, txscript.SigHashAll, wallet.key)
		if err != nil {
			return "", err
		}
		tx.TxIn[i].Witness = btc.HtlcWitness(swap.Script, wallet.key.PubKey().SerializeCompressed(), sig, nil)
	}

	return "", wallet.client.SubmitTx(ctx, tx)
}

func (wallet *wallet) feeRate() (int, error) {
	feeRates, err := wallet.feeEstimator.FeeSuggestion()
	if err != nil {
		return 0, err
	}

	switch wallet.opts.FeeTier {
	case "minimum":
		return feeRates.Minimum, nil
	case "economy":
		return feeRates.Economy, nil
	case "low":
		return feeRates.Low, nil
	case "medium":
		return feeRates.Medium, nil
	case "high":
		return feeRates.High, nil
	default:
		return feeRates.High, nil
	}
}

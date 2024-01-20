package btcswap

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/cobi/pkg/swap"
)

type ActionItem struct {
	Action     swap.Action
	AtomicSwap Swap
	Secret     []byte
}

type OptionRBF struct {
	PreviousFee     int                   `json:"previous_fee"`
	PreviousFeeRate int                   `json:"previous_fee_rate"`
	PreviousTxIns   map[string]struct{}   `json:"previous_tx_ins"`
	PreviousUtxos   []btc.UTXO            `json:"previous_utxos"`
	PreviousSpend   map[string][]btc.UTXO `json:"previous_spend"`
}

type Wallet interface {
	Address() btcutil.Address

	Balance(ctx context.Context) (int64, error)

	Indexer() btc.IndexerClient

	BatchExecute(ctx context.Context, actions []ActionItem, rbf *OptionRBF) (*wire.MsgTx, *OptionRBF, error)

	Initiate(ctx context.Context, swap Swap) (string, error)

	Redeem(ctx context.Context, swap Swap, secret []byte, target string) (string, error)

	Refund(ctx context.Context, swap Swap, target string) (string, error)
}

type wallet struct {
	mu           *sync.RWMutex
	opts         Options
	client       btc.IndexerClient
	feeEstimator btc.FeeEstimator
	key          *btcec.PrivateKey
	address      btcutil.Address
}

func NewWallet(opts Options, client btc.IndexerClient, key *btcec.PrivateKey, estimator btc.FeeEstimator) (Wallet, error) {
	addr, err := btc.PublicKeyAddress(key.PubKey(), opts.Network, opts.AddressType)
	if err != nil {
		return nil, fmt.Errorf("fail to parse wallet address, %v", err)
	}

	return &wallet{
		mu:           new(sync.RWMutex),
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

func (wallet *wallet) Balance(ctx context.Context) (int64, error) {
	wallet.mu.RLock()
	defer wallet.mu.RUnlock()

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

func (wallet *wallet) Indexer() btc.IndexerClient {
	return wallet.client
}

func (wallet *wallet) BatchExecute(ctx context.Context, actions []ActionItem, rbf *OptionRBF) (*wire.MsgTx, *OptionRBF, error) {
	if len(actions) == 0 {
		return nil, nil, nil
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	scriptUtxos := map[string][]btc.UTXO{}
	recipients := make([]btc.Recipient, 0, len(actions))
	rawInputs := btc.NewRawInputs()
	utxoOrigin := map[int]ActionItem{}
	fetcher := txscript.NewMultiPrevOutFetcher(nil)
	walletScript, err := txscript.PayToAddrScript(wallet.address)
	if err != nil {
		return nil, nil, err
	}

	for _, action := range actions {
		if action.AtomicSwap.Network.Name != wallet.opts.Network.Name {
			return nil, nil, fmt.Errorf("wrong network")
		}

		switch action.Action {
		case swap.ActionInitiate:
			log.Print("add initiate ", action.AtomicSwap.Address.EncodeAddress())
			recipient := btc.Recipient{
				To:     action.AtomicSwap.Address.EncodeAddress(),
				Amount: action.AtomicSwap.Amount,
			}
			recipients = append(recipients, recipient)
		case swap.ActionRedeem:
			log.Print("add redeem ", action.AtomicSwap.Address)

			var utxos []btc.UTXO
			if rbf != nil {
				var ok bool
				utxos, ok = rbf.PreviousSpend[string(action.AtomicSwap.SecretHash)]
				if !ok {
					utxos, err = wallet.client.GetUTXOs(ctx, action.AtomicSwap.Address)
					if err != nil {
						return nil, nil, err
					}
				}
			} else {
				utxos, err = wallet.client.GetUTXOs(ctx, action.AtomicSwap.Address)
				if err != nil {
					return nil, nil, err
				}
			}

			if len(utxos) == 0 {
				return nil, nil, btc.ErrTxInputsMissingOrSpent
			}

			// Mark these utxo as redeeming, so we know how to sign them later.
			fromScript, err := txscript.PayToAddrScript(action.AtomicSwap.Address)
			if err != nil {
				return nil, nil, err
			}
			for i, utxo := range utxos {
				utxoOrigin[len(rawInputs.VIN)+i] = action
				hash, err := chainhash.NewHashFromStr(utxo.TxID)
				if err != nil {
					return nil, nil, err
				}
				fetcher.AddPrevOut(wire.OutPoint{
					Hash:  *hash,
					Index: utxo.Vout,
				}, wire.NewTxOut(utxo.Amount, fromScript))
			}
			scriptUtxos[string(action.AtomicSwap.SecretHash)] = utxos
			rawInputs.VIN = append(rawInputs.VIN, utxos...)
			rawInputs.SegwitSize += len(utxos) * btc.RedeemHtlcRedeemSigScriptSize(len(action.Secret))
		case swap.ActionRefund:
			log.Print("add refund ", action.AtomicSwap.Address)
			var utxos []btc.UTXO
			if rbf != nil {
				var ok bool
				utxos, ok = rbf.PreviousSpend[string(action.AtomicSwap.SecretHash)]
				if !ok {
					utxos, err = wallet.client.GetUTXOs(ctx, action.AtomicSwap.Address)
					if err != nil {
						return nil, nil, err
					}
				}
			} else {
				utxos, err = wallet.client.GetUTXOs(ctx, action.AtomicSwap.Address)
				if err != nil {
					return nil, nil, err
				}
			}

			if len(utxos) == 0 {
				return nil, nil, btc.ErrTxInputsMissingOrSpent
			}

			// expired, err := action.AtomicSwap.Expired(ctx, wallet.client)
			// if err != nil {
			// 	return nil, nil, err
			// }
			// if !expired {
			// 	return nil, nil, fmt.Errorf("swap not expired")
			// }

			// Mark these utxo as refunding, so we know how to sign them later.
			fromScript, err := txscript.PayToAddrScript(action.AtomicSwap.Address)
			if err != nil {
				return nil, nil, err
			}
			for i, utxo := range utxos {
				utxoOrigin[len(rawInputs.VIN)+i] = action
				hash, err := chainhash.NewHashFromStr(utxo.TxID)
				if err != nil {
					return nil, nil, err
				}
				fetcher.AddPrevOut(wire.OutPoint{
					Hash:  *hash,
					Index: utxo.Vout,
				}, wire.NewTxOut(utxo.Amount, fromScript))
			}
			rawInputs.VIN = append(rawInputs.VIN, utxos...)
			rawInputs.SegwitSize += len(utxos) * btc.RedeemHtlcRefundSigScriptSize
			scriptUtxos[string(action.AtomicSwap.SecretHash)] = utxos
		default:
			return nil, nil, fmt.Errorf("unknown action = %v", action.Action)
		}
	}

	// Estimate the fee and considering RBF
	feeRate, err := wallet.feeRate()
	if err != nil {
		return nil, nil, err
	}

	// Build the tx
	var utxos []btc.UTXO
	if rbf != nil {
		utxos = rbf.PreviousUtxos
	} else {
		utxos, err = wallet.client.GetUTXOs(ctx, wallet.address)
		if err != nil {
			return nil, nil, err
		}
	}

	for _, utxo := range utxos {
		hash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return nil, nil, err
		}
		fetcher.AddPrevOut(wire.OutPoint{
			Hash:  *hash,
			Index: utxo.Vout,
		}, wire.NewTxOut(utxo.Amount, walletScript))
	}
	if rbf != nil {
		if feeRate < rbf.PreviousFeeRate+wallet.opts.MinRelayFee {
			feeRate = rbf.PreviousFeeRate + wallet.opts.MinRelayFee
		}
	}

	tx, err := btc.BuildRbfTransaction(feeRate, wallet.opts.Network, rawInputs, utxos, recipients, btc.P2wpkhUpdater, wallet.address)
	if err != nil {
		return nil, nil, err
	}

	if rbf != nil {
		// We need to make sure the rbf tx has some input from the replaced one
		hasInput := false
		for _, in := range tx.TxIn {
			if _, ok := rbf.PreviousTxIns[in.PreviousOutPoint.String()]; ok {
				hasInput = true
				break
			}
		}

		if !hasInput {
			return nil, nil, fmt.Errorf("rbf tx has different inputs, %w", btc.ErrTxInputsMissingOrSpent)
		}
	}

	// Update the sequence before signing
	txIns := map[string]struct{}{}
	for i := range tx.TxIn {
		txIns[tx.TxIn[i].PreviousOutPoint.String()] = struct{}{}
		actionItem, ok := utxoOrigin[i]
		if !ok {
			continue
		}
		if actionItem.Action == swap.ActionRefund {
			tx.TxIn[i].Sequence = uint32(actionItem.AtomicSwap.WaitBlock)
		}
	}

	// Sign the transaction
	signTx := func(transaction *wire.MsgTx) error {
		for i, utxo := range transaction.TxIn {
			// Either redeem or refund a HTLC
			if i < len(utxoOrigin) {
				txOut := fetcher.FetchPrevOutput(utxo.PreviousOutPoint)
				actionItem := utxoOrigin[i]
				sig, err := txscript.RawTxInWitnessSignature(transaction, txscript.NewTxSigHashes(transaction, fetcher), i, txOut.Value, actionItem.AtomicSwap.Script, txscript.SigHashAll, wallet.key)
				if err != nil {
					return err
				}
				if actionItem.Action == swap.ActionRedeem {
					transaction.TxIn[i].Witness = btc.HtlcWitness(actionItem.AtomicSwap.Script, wallet.key.PubKey().SerializeCompressed(), sig, actionItem.Secret)
				} else if actionItem.Action == swap.ActionRefund {
					transaction.TxIn[i].Witness = btc.HtlcWitness(actionItem.AtomicSwap.Script, wallet.key.PubKey().SerializeCompressed(), sig, nil)
				}
			} else {
				sigHashes := txscript.NewTxSigHashes(transaction, fetcher)
				txOut := fetcher.FetchPrevOutput(utxo.PreviousOutPoint)
				witness, err := txscript.WitnessSignature(transaction, sigHashes, i, txOut.Value, walletScript, txscript.SigHashAll, wallet.key, true)
				if err != nil {
					return err
				}
				transaction.TxIn[i].Witness = witness
			}
		}
		return nil
	}
	if err := signTx(tx); err != nil {
		return nil, nil, fmt.Errorf("failed to sign tx, %v", err)
	}

	// Make sure we meet the rbf fee restriction
	if rbf != nil {
		for {
			vsize := btc.TxVirtualSize(tx)
			if btc.TotalFee(tx, fetcher) >= rbf.PreviousFee+vsize*wallet.opts.MinRelayFee {
				break
			}
			feeRate += 1

			// Build and sign again

			tx, err = btc.BuildRbfTransaction(feeRate, wallet.opts.Network, rawInputs, utxos, recipients, btc.P2wpkhUpdater, wallet.address)
			if err != nil {
				return nil, nil, err
			}

			if err := signTx(tx); err != nil {
				return nil, nil, fmt.Errorf("failed to sign tx, %v", err)
			}
		}
	}

	// Submit the transaction
	if err := wallet.client.SubmitTx(ctx, tx); err != nil {
		return nil, nil, err
	}

	return tx, &OptionRBF{
		PreviousFee:     btc.TotalFee(tx, fetcher),
		PreviousFeeRate: feeRate,
		PreviousTxIns:   txIns,
		PreviousUtxos:   utxos,
		PreviousSpend:   scriptUtxos,
	}, nil
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
	wallet.mu.Lock()
	defer wallet.mu.Unlock()

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

	// Submit the tx
	if err := wallet.client.SubmitTx(ctx, tx); err != nil {
		return "", err
	}
	return tx.TxHash().String(), nil
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

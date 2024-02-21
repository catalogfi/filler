package btcswap

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"sync"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet/txsizes"
	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/cobi/pkg/swap"
)

type ActionItem struct {
	Action     swap.Action
	AtomicSwap Swap
	Secret     []byte
}

func UtxoKey(utxo btc.UTXO) string {
	return fmt.Sprintf("%v-%v", utxo.TxID, utxo.Vout)
}

var (
	DefaultSigType    = 0
	SigTypeRedeemHTLC = 1
	SigTypeRefundHTLC = 2
	SigTypeP2WPKH     = 3
)

type OptionRBF struct {
	PrevRawInputs btc.RawInputs   `json:"prev_raw_inputs"`   // raw tx inputs of the previous tx
	PrevRecipient []btc.Recipient `json:"prev_recipient"`    // recipients of the previous tx
	PrevFeeRate   int             `json:"previous_fee_rate"` // fee rate of the previous tx
	PrevFee       int             `json:"previous_fee"`      // total fee amount of the previous tx

	PrevSigType     map[string]int    `json:"prev_sig_type"`     // a map links the utxo to how it should be signed
	PrevSigScript   map[string][]byte `json:"prev_sig_script"`   // a map links the utxo to its script
	PrevSigSecret   map[string][]byte `json:"prev_sig_secret"`   // a map links the utxo to the unlocking secret for it
	PrevSigSequence map[string]uint32 `json:"prev_sig_sequence"` // a map links the refund utxo to its timelock

	FirstInputs []btc.UTXO `json:"first_inputs"` // inputs of the first tx, so we can check if the following tx has intersection
	FirstUtxos  []btc.UTXO `json:"first_utxos"`  // available utxo list to make up amount difference
}

func CopyRBF(opts OptionRBF) OptionRBF {
	newOptions := OptionRBF{
		PrevRawInputs: btc.RawInputs{
			VIN:        make([]btc.UTXO, len(opts.PrevRawInputs.VIN)),
			BaseSize:   opts.PrevRawInputs.BaseSize,
			SegwitSize: opts.PrevRawInputs.SegwitSize,
		},
		PrevRecipient: make([]btc.Recipient, len(opts.PrevRecipient)),
		PrevFeeRate:   opts.PrevFeeRate,
		PrevFee:       opts.PrevFee,

		PrevSigType:     map[string]int{},
		PrevSigScript:   map[string][]byte{},
		PrevSigSecret:   map[string][]byte{},
		PrevSigSequence: map[string]uint32{},

		FirstInputs: make([]btc.UTXO, len(opts.FirstInputs)),
		FirstUtxos:  make([]btc.UTXO, len(opts.FirstUtxos)),
	}
	for i, utxo := range opts.PrevRawInputs.VIN {
		newOptions.PrevRawInputs.VIN[i] = utxo
	}
	for i, recipient := range opts.PrevRecipient {
		newOptions.PrevRecipient[i] = recipient
	}
	for key, utxoType := range opts.PrevSigType {
		newOptions.PrevSigType[key] = utxoType
	}
	for key, script := range opts.PrevSigScript {
		newOptions.PrevSigScript[key] = script
	}
	for key, secret := range opts.PrevSigSecret {
		newOptions.PrevSigSecret[key] = secret
	}
	for key, sequence := range opts.PrevSigSequence {
		newOptions.PrevSigSequence[key] = sequence
	}
	for i, utxo := range opts.FirstInputs {
		newOptions.FirstInputs[i] = utxo
	}
	for i, utxo := range opts.FirstUtxos {
		newOptions.FirstUtxos[i] = utxo
	}

	return newOptions
}

type Wallet interface {
	Address() btcutil.Address

	Balance(ctx context.Context) (int64, error)

	Indexer() btc.IndexerClient

	BatchExecute(ctx context.Context, actions []ActionItem) (string, error)

	ExecuteRbf(ctx context.Context, actions []ActionItem, rbf OptionRBF) (string, OptionRBF, error)

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

func (wallet *wallet) BatchExecute(ctx context.Context, actions []ActionItem) (string, error) {
	if len(actions) == 0 {
		return "", nil
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	recipients := make([]btc.Recipient, 0, len(actions))
	rawInputs := btc.NewRawInputs()
	utxoOrigin := map[int]ActionItem{}
	fetcher := txscript.NewMultiPrevOutFetcher(nil)
	walletScript, err := txscript.PayToAddrScript(wallet.address)
	if err != nil {
		return "", err
	}
	for _, action := range actions {
		if action.AtomicSwap.Network.Name != wallet.opts.Network.Name {
			return "", fmt.Errorf("wrong network")
		}

		switch action.Action {
		case swap.ActionInitiate:
			recipient := btc.Recipient{
				To:     action.AtomicSwap.Address.EncodeAddress(),
				Amount: action.AtomicSwap.Amount,
			}
			recipients = append(recipients, recipient)
		case swap.ActionRedeem:
			// Check the swap is initialised before redeeming
			utxos, err := wallet.client.GetUTXOs(ctx, action.AtomicSwap.Address)
			if err != nil {
				return "", err
			}
			if len(utxos) == 0 {
				return "", fmt.Errorf("swap (%v) not initialised", action.AtomicSwap.Address)
			}

			// Mark these utxo as redeeming, so we know how to sign them later.
			fromScript, err := txscript.PayToAddrScript(action.AtomicSwap.Address)
			if err != nil {
				return "", err
			}
			for i, utxo := range utxos {
				utxoOrigin[len(rawInputs.VIN)+i] = action
				hash, err := chainhash.NewHashFromStr(utxo.TxID)
				if err != nil {
					return "", err
				}
				fetcher.AddPrevOut(wire.OutPoint{
					Hash:  *hash,
					Index: utxo.Vout,
				}, wire.NewTxOut(utxo.Amount, fromScript))
			}
			rawInputs.VIN = append(rawInputs.VIN, utxos...)
			rawInputs.SegwitSize += len(utxos) * btc.RedeemHtlcRedeemSigScriptSize(len(action.Secret))
		case swap.ActionRefund:
			expired, err := action.AtomicSwap.Expired(ctx, wallet.client)
			if err != nil {
				return "", err
			}
			if !expired {
				return "", fmt.Errorf("swap not expired")
			}

			// Mark these utxo as refunding, so we know how to sign them later.
			utxos, err := wallet.client.GetUTXOs(ctx, action.AtomicSwap.Address)
			if err != nil {
				return "", err
			}
			fromScript, err := txscript.PayToAddrScript(action.AtomicSwap.Address)
			if err != nil {
				return "", err
			}
			for i, utxo := range utxos {
				utxoOrigin[len(rawInputs.VIN)+i] = action
				hash, err := chainhash.NewHashFromStr(utxo.TxID)
				if err != nil {
					return "", err
				}
				fetcher.AddPrevOut(wire.OutPoint{
					Hash:  *hash,
					Index: utxo.Vout,
				}, wire.NewTxOut(utxo.Amount, fromScript))
			}
			rawInputs.VIN = append(rawInputs.VIN, utxos...)
			rawInputs.SegwitSize += len(utxos) * btc.RedeemHtlcRefundSigScriptSize
		default:
			return "", fmt.Errorf("unknown action = %v", action.Action)
		}
	}

	// Build the transaction
	feeRate, err := wallet.feeRate()
	if err != nil {
		return "", err
	}
	utxos, err := wallet.client.GetUTXOs(ctx, wallet.address)
	if err != nil {
		return "", err
	}
	for _, utxo := range utxos {
		hash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return "", err
		}
		fetcher.AddPrevOut(wire.OutPoint{
			Hash:  *hash,
			Index: utxo.Vout,
		}, wire.NewTxOut(utxo.Amount, walletScript))
	}
	tx, err := btc.BuildTransaction(wallet.opts.Network, feeRate, rawInputs, utxos, btc.P2wpkhUpdater, recipients, wallet.address)
	if err != nil {
		return "", err
	}

	// Update the sequence before signing
	for i := range tx.TxIn {
		actionItem, ok := utxoOrigin[i]
		if !ok {
			continue
		}
		if actionItem.Action == swap.ActionRefund {
			tx.TxIn[i].Sequence = uint32(actionItem.AtomicSwap.WaitBlock)
		}
	}

	// Sign the transaction
	for i, utxo := range tx.TxIn {
		// Either redeem or refund a HTLC
		if i < len(utxoOrigin) {
			txOut := fetcher.FetchPrevOutput(utxo.PreviousOutPoint)
			actionItem := utxoOrigin[i]
			sig, err := txscript.RawTxInWitnessSignature(tx, txscript.NewTxSigHashes(tx, fetcher), i, txOut.Value, actionItem.AtomicSwap.Script, txscript.SigHashAll, wallet.key)
			if err != nil {
				return "", err
			}
			if actionItem.Action == swap.ActionRedeem {
				tx.TxIn[i].Witness = btc.HtlcWitness(actionItem.AtomicSwap.Script, wallet.key.PubKey().SerializeCompressed(), sig, actionItem.Secret)
			} else if actionItem.Action == swap.ActionRefund {
				tx.TxIn[i].Witness = btc.HtlcWitness(actionItem.AtomicSwap.Script, wallet.key.PubKey().SerializeCompressed(), sig, nil)
			}
		} else {
			sigHashes := txscript.NewTxSigHashes(tx, fetcher)
			txOut := fetcher.FetchPrevOutput(utxo.PreviousOutPoint)
			witness, err := txscript.WitnessSignature(tx, sigHashes, i, txOut.Value, walletScript, txscript.SigHashAll, wallet.key, true)
			if err != nil {
				return "", err
			}
			tx.TxIn[i].Witness = witness
		}
	}

	// Submit the transaction
	if err := wallet.client.SubmitTx(ctx, tx); err != nil {
		return "", err
	}
	return tx.TxHash().String(), nil
}

func (wallet *wallet) ExecuteRbf(ctx context.Context, actions []ActionItem, rbf OptionRBF) (string, OptionRBF, error) {
	if len(actions) == 0 {
		return "", rbf, nil
	}

	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	rbfIsNil := rbf.PrevFee == 0 && len(rbf.FirstInputs) == 0
	newRbf := CopyRBF(rbf)
	fetcher := txscript.NewMultiPrevOutFetcher(nil)
	walletScript, err := txscript.PayToAddrScript(wallet.address)
	if err != nil {
		return "", rbf, err
	}

	for _, input := range rbf.PrevRawInputs.VIN {
		key := fmt.Sprintf("%v-%v", input.TxID, input.Vout)
		hash, err := chainhash.NewHashFromStr(input.TxID)
		if err != nil {
			return "", rbf, err
		}
		fetcher.AddPrevOut(wire.OutPoint{
			Hash:  *hash,
			Index: input.Vout,
		}, wire.NewTxOut(input.Amount, rbf.PrevSigScript[key]))
	}

	// Add new actions to the inputs and outputs
	for _, action := range actions {
		if action.AtomicSwap.Network.Name != wallet.opts.Network.Name {
			return "", rbf, fmt.Errorf("wrong network")
		}

		switch action.Action {
		case swap.ActionInitiate:
			recipient := btc.Recipient{
				To:     action.AtomicSwap.Address.EncodeAddress(),
				Amount: action.AtomicSwap.Amount,
			}
			newRbf.PrevRecipient = append(newRbf.PrevRecipient, recipient)
		case swap.ActionRedeem:
			utxos, err := wallet.client.GetUTXOs(ctx, action.AtomicSwap.Address)
			if err != nil {
				return "", rbf, err
			}
			utxos = wallet.removeUnconfirmedUtxo(utxos)

			if len(utxos) == 0 {
				return "", rbf, btc.ErrTxInputsMissingOrSpent
			}

			// Mark these utxo as redeeming, so we know how to sign them later.
			fromScript, err := txscript.PayToAddrScript(action.AtomicSwap.Address)
			if err != nil {
				return "", rbf, err
			}
			for _, utxo := range utxos {
				hash, err := chainhash.NewHashFromStr(utxo.TxID)
				if err != nil {
					return "", rbf, err
				}
				fetcher.AddPrevOut(wire.OutPoint{
					Hash:  *hash,
					Index: utxo.Vout,
				}, wire.NewTxOut(utxo.Amount, fromScript))
				newRbf.PrevSigType[UtxoKey(utxo)] = SigTypeRedeemHTLC
				newRbf.PrevSigScript[UtxoKey(utxo)] = action.AtomicSwap.Script
				newRbf.PrevSigSecret[UtxoKey(utxo)] = action.Secret
			}
			newRbf.PrevRawInputs.VIN = append(newRbf.PrevRawInputs.VIN, utxos...)
			newRbf.PrevRawInputs.SegwitSize += len(utxos) * btc.RedeemHtlcRedeemSigScriptSize(len(action.Secret))
		case swap.ActionRefund:
			utxos, err := wallet.client.GetUTXOs(ctx, action.AtomicSwap.Address)
			if err != nil {
				return "", rbf, err
			}
			utxos = wallet.removeUnconfirmedUtxo(utxos)
			if len(utxos) == 0 {
				return "", rbf, btc.ErrTxInputsMissingOrSpent
			}

			// Mark these utxo as refunding, so we know how to sign them later.
			fromScript, err := txscript.PayToAddrScript(action.AtomicSwap.Address)
			if err != nil {
				return "", rbf, err
			}
			for _, utxo := range utxos {
				hash, err := chainhash.NewHashFromStr(utxo.TxID)
				if err != nil {
					return "", rbf, err
				}
				fetcher.AddPrevOut(wire.OutPoint{
					Hash:  *hash,
					Index: utxo.Vout,
				}, wire.NewTxOut(utxo.Amount, fromScript))
				newRbf.PrevSigType[UtxoKey(utxo)] = SigTypeRefundHTLC
				newRbf.PrevSigScript[UtxoKey(utxo)] = action.AtomicSwap.Script
				newRbf.PrevSigSequence[UtxoKey(utxo)] = uint32(action.AtomicSwap.WaitBlock)
			}

			newRbf.PrevRawInputs.VIN = append(newRbf.PrevRawInputs.VIN, utxos...)
			newRbf.PrevRawInputs.SegwitSize += len(utxos) * btc.RedeemHtlcRefundSigScriptSize
		default:
			return "", rbf, fmt.Errorf("unknown action = %v", action.Action)
		}
	}

	// Estimate the fee and considering RBF
	feeRate, err := wallet.feeRate()
	if err != nil {
		return "", rbf, err
	}

	// Fetch utxos
	var utxos []btc.UTXO
	if rbfIsNil {
		utxos, err = wallet.client.GetUTXOs(ctx, wallet.address)
		if err != nil {
			return "", rbf, err
		}
		utxos = wallet.removeUnconfirmedUtxo(utxos)
	} else {
		utxos = rbf.FirstUtxos
	}

	// Add to the fetcher for future signing
	for _, utxo := range utxos {
		hash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return "", rbf, err
		}
		fetcher.AddPrevOut(wire.OutPoint{
			Hash:  *hash,
			Index: utxo.Vout,
		}, wire.NewTxOut(utxo.Amount, walletScript))
	}

	// Update the fee rate if it's lower than (prevFeeRate + minRelayFee)
	if !rbfIsNil {
		if feeRate < rbf.PrevFeeRate+wallet.opts.MinRelayFee {
			feeRate = rbf.PrevFeeRate + wallet.opts.MinRelayFee
		}
	}

	// Build tx
	tx, err := btc.BuildRbfTransaction(wallet.opts.Network, feeRate, newRbf.PrevRawInputs, utxos, btc.P2wpkhUpdater, newRbf.PrevRecipient, wallet.address)
	if err != nil {
		return "", rbf, err
	}

	// Make sure the fee meet the rbf requirement
	if !rbfIsNil {
		for {
			// Estimate the tx size (rawInput.SegwitSize + P2WPKH segwit size * number of cobi utxos)
			extraSegSize := txsizes.RedeemP2WPKHInputWitnessWeight * (len(tx.TxIn) - len(newRbf.PrevRawInputs.VIN))
			vsize := btc.EstimateVirtualSize(tx, 0, newRbf.PrevRawInputs.SegwitSize+extraSegSize)
			if btc.TotalFee(tx, fetcher) >= rbf.PrevFee+vsize*wallet.opts.MinRelayFee {
				break
			}
			feeRate += 1

			// Build and sign again
			tx, err = btc.BuildRbfTransaction(wallet.opts.Network, feeRate, newRbf.PrevRawInputs, utxos, btc.P2wpkhUpdater, newRbf.PrevRecipient, wallet.address)
			if err != nil {
				return "", rbf, err
			}
		}
	}
	log.Printf("fee rate after adjustment = %v", feeRate)

	// Make sure one of the input from the first tx still exit in this transaction to prevent double initiation.
	if !rbfIsNil {
		hasOneOfTheInput := false
	Loop:
		for _, utxo := range rbf.FirstInputs {
			for _, input := range tx.TxIn {
				if utxo.TxID == input.PreviousOutPoint.Hash.String() && utxo.Vout == input.PreviousOutPoint.Index {
					hasOneOfTheInput = true
					break Loop
				}
			}
		}
		if !hasOneOfTheInput {
			return "", rbf, btc.ErrTxInputsMissingOrSpent
		}
	}

	// Set the sequence for refund utxo
	for i, in := range tx.TxIn {
		key := fmt.Sprintf("%v-%v", in.PreviousOutPoint.Hash.String(), in.PreviousOutPoint.Index)
		if newRbf.PrevSigType[key] == SigTypeRefundHTLC {
			sequence, ok := newRbf.PrevSigSequence[key]
			if !ok {
				return "", rbf, fmt.Errorf("missing sequence for %v", key)
			}
			tx.TxIn[i].Sequence = sequence
		}
	}

	// Sign the tx
	for i, in := range tx.TxIn {
		key := fmt.Sprintf("%v-%v", in.PreviousOutPoint.Hash.String(), in.PreviousOutPoint.Index)
		switch newRbf.PrevSigType[key] {
		case DefaultSigType, SigTypeP2WPKH:
			sigHashes := txscript.NewTxSigHashes(tx, fetcher)
			txOut := fetcher.FetchPrevOutput(in.PreviousOutPoint)
			witness, err := txscript.WitnessSignature(tx, sigHashes, i, txOut.Value, walletScript, txscript.SigHashAll, wallet.key, true)
			if err != nil {
				return "", rbf, err
			}
			tx.TxIn[i].Witness = witness
		case SigTypeRedeemHTLC:
			txOut := fetcher.FetchPrevOutput(in.PreviousOutPoint)
			script, ok := newRbf.PrevSigScript[key]
			if !ok {
				return "", rbf, fmt.Errorf("missing sig script for %v", key)
			}
			sig, err := txscript.RawTxInWitnessSignature(tx, txscript.NewTxSigHashes(tx, fetcher), i, txOut.Value, script, txscript.SigHashAll, wallet.key)
			if err != nil {
				return "", rbf, err
			}
			secret, ok := newRbf.PrevSigSecret[key]
			if !ok {
				return "", rbf, fmt.Errorf("missing sig secret for %v", key)
			}
			tx.TxIn[i].Witness = btc.HtlcWitness(script, wallet.key.PubKey().SerializeCompressed(), sig, secret)
		case SigTypeRefundHTLC:
			txOut := fetcher.FetchPrevOutput(in.PreviousOutPoint)
			script, ok := newRbf.PrevSigScript[key]
			if !ok {
				return "", rbf, fmt.Errorf("missing sig script for %v", key)
			}
			sig, err := txscript.RawTxInWitnessSignature(tx, txscript.NewTxSigHashes(tx, fetcher), i, txOut.Value, script, txscript.SigHashAll, wallet.key)
			if err != nil {
				return "", rbf, err
			}
			tx.TxIn[i].Witness = btc.HtlcWitness(script, wallet.key.PubKey().SerializeCompressed(), sig, nil)
		}
	}

	buffer := bytes.NewBuffer([]byte{})
	if err := tx.Serialize(buffer); err != nil {
		return "", rbf, err
	}
	log.Print("raw ", hex.EncodeToString(buffer.Bytes()))

	// Submit the transaction
	if err := wallet.client.SubmitTx(ctx, tx); err != nil {
		return "", rbf, err
	}

	// Update the rbf option for next tx
	newRbf.PrevFeeRate = feeRate
	newRbf.PrevFee = btc.TotalFee(tx, fetcher)
	if rbfIsNil {
		newRbf.FirstInputs = make([]btc.UTXO, len(tx.TxIn))
		for i, in := range tx.TxIn {
			txOut := fetcher.FetchPrevOutput(in.PreviousOutPoint)
			newRbf.FirstInputs[i] = btc.UTXO{
				TxID:   in.PreviousOutPoint.Hash.String(),
				Vout:   in.PreviousOutPoint.Index,
				Amount: txOut.Value,
			}
		}
		newRbf.FirstUtxos = utxos
	}
	// When we only have initiates in the first tx. We need to add those utxos from ourselves as the raw inputs.
	// And remove then from the available utxos list.
	if len(newRbf.PrevRawInputs.VIN) == 0 {
		used := len(tx.TxIn) - len(newRbf.PrevRawInputs.VIN)
		for _, in := range tx.TxIn {
			txOut := fetcher.FetchPrevOutput(in.PreviousOutPoint)
			newRbf.PrevRawInputs.VIN = append(newRbf.PrevRawInputs.VIN, btc.UTXO{
				TxID:   in.PreviousOutPoint.Hash.String(),
				Vout:   in.PreviousOutPoint.Index,
				Amount: txOut.Value,
			})
		}
		newRbf.FirstUtxos = utxos[used:]
		newRbf.PrevRawInputs.SegwitSize = len(tx.TxIn) * txsizes.RedeemP2WPKHInputWitnessWeight
	}

	return tx.TxHash().String(), newRbf, nil
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
	tx, err := btc.BuildTransaction(swap.Network, feeRate, btc.NewRawInputs(), utxos, btc.P2wpkhUpdater, recipients, wallet.address)
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
	feeRate, err := wallet.feeRate()
	if err != nil {
		return "", err
	}
	targetAddr, err := btcutil.DecodeAddress(target, wallet.opts.Network)
	if err != nil {
		return "", err
	}
	tx, err := btc.BuildTransaction(swap.Network, feeRate, rawInputs, nil, nil, nil, targetAddr)
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
	targetAddr, err := btcutil.DecodeAddress(target, wallet.opts.Network)
	if err != nil {
		return "", err
	}
	feeRate, err := wallet.feeRate()
	if err != nil {
		return "", err
	}
	tx, err := btc.BuildTransaction(swap.Network, feeRate, rawInputs, nil, nil, nil, targetAddr)
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

func (wallet *wallet) removeUnconfirmedUtxo(utxos []btc.UTXO) []btc.UTXO {
	confirmedUtxos := make([]btc.UTXO, 0, len(utxos))
	for _, utxo := range utxos {
		if utxo.Status != nil && utxo.Status.Confirmed {
			confirmedUtxos = append(confirmedUtxos, utxo)
		}
	}
	return confirmedUtxos
}

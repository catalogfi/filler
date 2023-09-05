package bitcoin

import (
	"crypto/sha256"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

const (
	BTC_VERSION = 2

	// DustAmount is the minimum amount sats node will accept for an UTXO.
	DustAmount = 546
)

type UTXO struct {
	Amount uint64  `json:"value"`
	TxID   string  `json:"txid"`
	Vout   uint32  `json:"vout"`
	Status *Status `json:"status"`
}

type UTXOs []UTXO

type Client interface {
	GetSpendingWitness(address btcutil.Address) ([]string, Transaction, error)
	GetTipBlockHeight() (uint64, error)
	GetConfirmations(txHash string) (uint64, uint64, error)
	GetUTXOs(address btcutil.Address, amount uint64) (UTXOs, uint64, error)
	Send(to btcutil.Address, amount uint64, from *btcec.PrivateKey) (string, error)
	GetTx(txid string) (Transaction, error)
	Spend(script []byte, scriptSig wire.TxWitness, spender *btcec.PrivateKey, waitBlocks uint) (string, error)
	Net() *chaincfg.Params
	CalculateTransferFee(nInputs, nOutputs int, txVersion int32) (uint64, error)
	CalculateRedeemFee() (uint64, error)
	GetFeeRates() (FeeRates, error)
}

type client struct {
	indexer Indexer
	net     *chaincfg.Params
}

func NewClient(indexer Indexer, net *chaincfg.Params) Client {
	return &client{indexer: indexer, net: net}
}

func (client *client) Net() *chaincfg.Params {
	return client.net
}

func (client *client) GetTipBlockHeight() (uint64, error) {
	return client.indexer.GetTipBlockHeight()
}

func (client *client) GetTx(txid string) (Transaction, error) {
	return client.indexer.GetTx(txid)
}

func (client *client) GetConfirmations(txHash string) (uint64, uint64, error) {
	if len(txHash) > 2 && txHash[:2] == "0x" {
		txHash = txHash[2:]
	}

	status, err := client.indexer.GetTx(txHash)
	if err != nil {
		return 0, 0, err
	}
	if status.Status.Confirmed {
		tip, err := client.indexer.GetTipBlockHeight()
		if err != nil {
			return 0, 0, nil
		}
		return status.Status.BlockHeight, tip - status.Status.BlockHeight, nil
	}
	return 0, 0, nil
}

func (client *client) GetSpendingWitness(address btcutil.Address) ([]string, Transaction, error) {
	return client.indexer.GetSpendingWitness(address)
}

func (client *client) GetUTXOs(address btcutil.Address, amount uint64) (UTXOs, uint64, error) {
	return client.indexer.GetUTXOs(address, amount)
}

func (client *client) Send(to btcutil.Address, amount uint64, from *btcec.PrivateKey) (string, error) {
	tx := wire.NewMsgTx(BTC_VERSION)

	fromAddr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(from.PubKey().SerializeCompressed()), client.Net())
	if err != nil {
		return "", fmt.Errorf("failed to create address from private key: %w", err)
	}

	utxosWithoutFee, _, err := client.GetUTXOs(fromAddr, amount)
	if err != nil {
		return "", fmt.Errorf("failed to get UTXOs: %w", err)
	}

	fee, err := client.CalculateTransferFee(len(utxosWithoutFee), 2, tx.Version)
	if err != nil {
		return "", fmt.Errorf("failed to calculate fee: %w", err)
	}

	utxosWihFee, selectedAmount, err := client.GetUTXOs(fromAddr, amount+fee)
	if err != nil {
		return "", fmt.Errorf("failed to get UTXOs: %w", err)
	}
	for _, utxo := range utxosWihFee {
		txid, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return "", fmt.Errorf("failed to parse txid in the utxo: %w", err)
		}
		tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(txid, utxo.Vout), nil, nil))
	}

	toScript, err := txscript.PayToAddrScript(to)
	if err != nil {
		return "", fmt.Errorf("failed to create script for address: %w", err)
	}

	fromScript, err := txscript.PayToAddrScript(fromAddr)
	if err != nil {
		return "", fmt.Errorf("failed to create script for address: %w", err)
	}

	tx.AddTxOut(wire.NewTxOut(int64(amount), toScript))
	if int64(selectedAmount-amount-fee) > DustAmount {
		tx.AddTxOut(wire.NewTxOut(int64(selectedAmount-amount-fee), fromScript))
	}

	for i, utxo := range utxosWihFee {
		fetcher := txscript.NewCannedPrevOutputFetcher(fromScript, int64(utxo.Amount))
		if err != nil {
			return "", err
		}

		sigHashes := txscript.NewTxSigHashes(tx, fetcher)
		witness, err := txscript.WitnessSignature(tx, sigHashes, i, int64(utxo.Amount), fromScript, txscript.SigHashAll, from, true)
		if err != nil {
			return "", err
		}
		tx.TxIn[i].Witness = witness
	}

	return client.indexer.SubmitTx(tx)
}

func (client *client) Spend(script []byte, redeemScript wire.TxWitness, spender *btcec.PrivateKey, waitBlocks uint) (string, error) {
	tx := wire.NewMsgTx(BTC_VERSION)

	scriptWitnessProgram := sha256.Sum256(script)
	scriptAddr, err := btcutil.NewAddressWitnessScriptHash(scriptWitnessProgram[:], client.Net())
	if err != nil {
		return "", fmt.Errorf("failed to create script address: %w", err)
	}
	utxos, balance, err := client.GetUTXOs(scriptAddr, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get UTXOs: %w", err)
	}
	amounts := make([]uint64, len(utxos))

	for i, utxo := range utxos {
		txid, err := chainhash.NewHashFromStr(utxos[i].TxID)
		if err != nil {
			return "", fmt.Errorf("failed to parse txid in the utxo: %w", err)
		}
		amounts[i] = utxos[i].Amount
		tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(txid, utxo.Vout), nil, nil))
	}

	spenderAddr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(spender.PubKey().SerializeCompressed()), client.Net())
	if err != nil {
		return "", fmt.Errorf("failed to create address from private key: %w", err)
	}
	spenderToScript, err := txscript.PayToAddrScript(spenderAddr)
	if err != nil {
		return "", fmt.Errorf("failed to create script for address: %w", err)
	}

	fee, err := client.CalculateRedeemFee()
	if err != nil {
		return "", fmt.Errorf("failed to calculate fee: %w", err)
	}
	if balance-fee <= DustAmount {
		return "", fmt.Errorf("balance too low")
	}
	tx.AddTxOut(wire.NewTxOut(int64(balance-fee), spenderToScript))

	for i := range tx.TxIn {
		fetcher := txscript.NewCannedPrevOutputFetcher(script, int64(amounts[i]))
		if waitBlocks > 0 {
			tx.TxIn[i].Sequence = uint32(waitBlocks) + 1
		}
		sigHashes := txscript.NewTxSigHashes(tx, fetcher)
		sig, err := txscript.RawTxInWitnessSignature(tx, sigHashes, i, int64(amounts[i]), script, txscript.SigHashAll, spender)
		if err != nil {
			return "", err
		}
		tx.TxIn[i].Witness = append(wire.TxWitness{sig}, redeemScript...)
		tx.TxIn[i].Witness = append(tx.TxIn[i].Witness, wire.TxWitness{script}...)
	}
	return client.indexer.SubmitTx(tx)
}

// CalculateFee estimates the fees of the bitcoin tx with given number of inputs and outputs.
func (client *client) CalculateTransferFee(nInputs, nOutputs int, txVersion int32) (uint64, error) {
	feeRates, err := client.GetFeeRates()
	if err != nil {
		return 0, err
	}
	if feeRates.FastestFee < 2 {
		feeRates.FastestFee = 2
	}
	switch txVersion {
	case 1:
		// inputs + 1 to account for input that might be used for fee
		// but if fee is already accounted in the selected utxos it will just lead to a slighty speedy transaction
		return uint64((nInputs+1)*148+nOutputs*34+10) * (uint64(feeRates.FastestFee)), nil
	case 2:
		return uint64(nInputs*68+nOutputs*31+10) * uint64(feeRates.FastestFee), nil
	}
	return 0, fmt.Errorf("tx type not supported")

}

func (client *client) CalculateRedeemFee() (uint64, error) {
	feeRates, err := client.GetFeeRates()
	if err != nil {
		return 0, err
	}
	if feeRates.FastestFee < 2 {
		feeRates.FastestFee = 2
	}
	// 141.5 is size in vbytes for the redeem transaction
	return 150 * uint64(feeRates.FastestFee) * 2, nil
}

func (client *client) GetFeeRates() (FeeRates, error) {
	return client.indexer.GetFeeRates()
}

type FeeRates struct {
	FastestFee  int `json:"fastestFee"`
	HalfHourFee int `json:"halfHourFee"`
	HourFee     int `json:"hourFee"`
	MinimumFee  int `json:"minimumFee"`
	EconomyFee  int `json:"economyFee"`
}

type Transaction struct {
	TxID   string `json:"txid"`
	VINs   []VIN  `json:"vin"`
	Status Status `json:"status"`
}

type VIN struct {
	TxID         string    `json:"txid"`
	Vout         int       `json:"vout"`
	Prevout      Prevout   `json:"prevout"`
	ScriptSigAsm string    `json:"scriptsig_asm"`
	Witness      *[]string `json:"witness" `
}

type Prevout struct {
	ScriptPubKeyType    string `json:"scriptpubkey_type"`
	ScriptPubKey        string `json:"scriptpubkey"`
	ScriptPubKeyAddress string `json:"scriptpubkey_address"`
}

type Status struct {
	Confirmed   bool   `json:"confirmed"`
	BlockHeight uint64 `json:"block_height"`
}

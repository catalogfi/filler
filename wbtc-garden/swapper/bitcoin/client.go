package bitcoin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

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
	GetUTXOs(address btcutil.Address, amount uint64) (UTXOs, uint64, error)
	Send(to btcutil.Address, amount uint64, from *btcec.PrivateKey) (string, error)
	Spend(script []byte, scriptSig wire.TxWitness, spender *btcec.PrivateKey, waitBlocks uint) (string, error)
	Net() *chaincfg.Params
}

type client struct {
	url string
	net *chaincfg.Params
}

func NewClient(url string, net *chaincfg.Params) Client {
	return &client{url: url, net: net}
}

func (client *client) Net() *chaincfg.Params {
	return client.net
}

func (client *client) GetTipBlockHeight() (uint64, error) {
	resp, err := http.Get(fmt.Sprintf("%s/blocks/tip/height", client.url))
	if err != nil {
		return 0, fmt.Errorf("failed to get transaction: %w", err)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %w", err)
	}
	return strconv.ParseUint(string(data), 10, 64)
}

func (client *client) GetSpendingWitness(address btcutil.Address) ([]string, Transaction, error) {
	resp, err := http.Get(fmt.Sprintf("%s/address/%s/txs", client.url, address.EncodeAddress()))
	if err != nil {
		return []string{}, Transaction{}, fmt.Errorf("failed to get transactions: %w", err)
	}
	var txs []Transaction
	if err := json.NewDecoder(resp.Body).Decode(&txs); err != nil {
		return []string{}, Transaction{}, fmt.Errorf("failed to decode transactions: %w", err)
	}
	for _, tx := range txs {
		for _, vin := range tx.VINs {
			if vin.Prevout.ScriptPubKeyAddress == address.EncodeAddress() {
				return *vin.Witness, tx, nil
			}
		}
	}
	return []string{}, Transaction{}, nil
}

func (client *client) GetUTXOs(address btcutil.Address, amount uint64) (UTXOs, uint64, error) {
	resp, err := http.Get(fmt.Sprintf("%s/address/%s/utxo", client.url, address.EncodeAddress()))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get UTXOs: %w", err)
	}
	utxos := UTXOs{}

	if err := json.NewDecoder(resp.Body).Decode(&utxos); err != nil {
		return nil, 0, fmt.Errorf("failed to decode UTXOs: %w", err)
	}

	var balance uint64
	for _, utxo := range utxos {
		balance += utxo.Amount
	}

	if amount == 0 {
		return utxos, balance, nil
	}
	if balance < amount {
		return nil, 0, fmt.Errorf("insufficient balance in %s", address.EncodeAddress())
	}

	var selected UTXOs
	var total uint64
	for _, utxo := range utxos {
		if total >= amount {
			break
		}
		selected = append(selected, utxo)
		total += utxo.Amount
	}
	return selected, total, nil
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

	return client.SubmitTx(tx)
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
	return client.SubmitTx(tx)
}

func (client *client) SubmitTx(tx *wire.MsgTx) (string, error) {
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}

	resp, err := http.Post(fmt.Sprintf("%s/tx", client.url), "application/text", bytes.NewBufferString(hex.EncodeToString(buf.Bytes())))
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read transaction id: %w", err)
	}
	
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to send transaction: %s", data)
	}

	return string(data), nil
}

func (client *client) GetFeeRates() (FeeRates, error) {
	var feeRates FeeRates
	resp, err := http.Get("https://mempool.space/api/v1/fees/recommended")
	if err != nil {
		return FeeRates{}, fmt.Errorf("failed to get fee rates: %w", err)
	}
	err = json.NewDecoder(resp.Body).Decode(&feeRates)
	if err != nil {
		return FeeRates{}, fmt.Errorf("failed to unmarshal response body: %w", err)
	}
	return feeRates, nil
}

// CalculateFee estimates the fees of the bitcoin tx with given number of inputs and outputs.
func (client *client) CalculateTransferFee(nInputs, nOutputs int, txVersion int32) (uint64, error) {
	feeRates, err := client.GetFeeRates()
	if err != nil {
		return 0, err
	}
	switch txVersion {
	case 1:
		// inputs + 1 to account for input that might be used for fee
		// but if fee is already accounted in the selected utxos it will just lead to a slighty speedy transaction
		return uint64((nInputs+1)*148+nOutputs*34+10) * (uint64(feeRates.HalfHourFee)), nil
	case 2:
		return uint64(nInputs*68+nOutputs*31+10) * uint64(feeRates.HalfHourFee), nil
	}
	return 0, fmt.Errorf("tx type not supported")

}


func (client *client) CalculateRedeemFee() (uint64, error) {
	feeRates, err := client.GetFeeRates()
	if err != nil {
		return 0, err
	}
	// 141.5 is size in vbytes for the redeem transaction
	return 150 * uint64(feeRates.FastestFee), nil
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

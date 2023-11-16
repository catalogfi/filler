package bitcoin

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
)

type mempool struct {
	url string
}

type Indexer interface {
	GetSpendingWitness(address btcutil.Address) ([]string, Transaction, error)
	GetTipBlockHeight() (uint64, error)
	GetTx(txid string) (Transaction, error)
	GetUTXOs(address btcutil.Address, amount uint64) (UTXOs, uint64, error)
	SubmitTx(tx *wire.MsgTx) (string, error)
	GetFeeRates() (FeeRates, error)
}

type indexer struct {
	indexers []Indexer
}

func NewMultiIndexer(indexers ...Indexer) (Indexer, error) {
	if len(indexers) == 0 {
		return nil, fmt.Errorf("need atleast one indexer")
	}
	return &indexer{indexers: indexers}, nil
}

func (indexer *indexer) GetSpendingWitness(address btcutil.Address) ([]string, Transaction, error) {
	var err error
	for _, indexer := range indexer.indexers {
		witness, tx, ierr := indexer.GetSpendingWitness(address)
		if ierr == nil {
			return witness, tx, ierr
		}
		err = ierr
	}
	return []string{}, Transaction{}, err
}

func (indexer *indexer) GetTipBlockHeight() (uint64, error) {
	var err error
	for _, indexer := range indexer.indexers {
		height, ierr := indexer.GetTipBlockHeight()
		if ierr == nil {
			return height, ierr
		}
		err = ierr
	}
	return 0, err
}

func (indexer *indexer) GetUTXOs(address btcutil.Address, amount uint64) (UTXOs, uint64, error) {
	var err error

	for _, indexer := range indexer.indexers {
		utxos, balance, ierr := indexer.GetUTXOs(address, amount)
		if ierr == nil {
			return utxos, balance, ierr
		}
		err = ierr
	}
	return nil, 0, err
}

func (indexer *indexer) GetTx(txid string) (Transaction, error) {
	var err error
	for _, indexer := range indexer.indexers {
		txHash, ierr := indexer.GetTx(txid)
		if ierr == nil {
			return txHash, ierr
		}
		err = ierr
	}
	return Transaction{}, err
}

func (indexer *indexer) SubmitTx(tx *wire.MsgTx) (string, error) {
	var err error
	for _, indexer := range indexer.indexers {
		txHash, ierr := indexer.SubmitTx(tx)
		if ierr == nil {
			return txHash, ierr
		}
		err = ierr
	}
	return "", err
}

func (indexer *indexer) GetFeeRates() (FeeRates, error) {
	var err error
	for _, indexer := range indexer.indexers {
		feeRates, ierr := indexer.GetFeeRates()
		if ierr == nil {
			return feeRates, ierr
		}
		err = ierr
	}
	return FeeRates{}, err
}

func NewMempool(url string) Indexer {
	return &mempool{url: url}
}

func (mempool *mempool) GetTipBlockHeight() (uint64, error) {
	resp, err := http.Get(fmt.Sprintf("%s/blocks/tip/height", mempool.url))
	if err != nil {
		return 0, fmt.Errorf("failed to get transaction: %w", err)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %w", err)
	}
	return strconv.ParseUint(string(data), 10, 64)
}

func (mempool *mempool) GetTx(txid string) (Transaction, error) {
	resp, err := http.Get(fmt.Sprintf("%s/tx/%s", mempool.url, txid))
	if err != nil {
		return Transaction{}, fmt.Errorf("failed to get transactions: %w", err)
	}
	var tx Transaction
	if err := json.NewDecoder(resp.Body).Decode(&tx); err != nil {
		return Transaction{}, fmt.Errorf("failed to decode transactions: %w", err)
	}
	return tx, nil
}

func (mempool *mempool) GetUTXOs(address btcutil.Address, amount uint64) (UTXOs, uint64, error) {
	resp, err := http.Get(fmt.Sprintf("%s/address/%s/utxo", mempool.url, address.EncodeAddress()))
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

func (mempool *mempool) GetSpendingWitness(address btcutil.Address) ([]string, Transaction, error) {
	resp, err := http.Get(fmt.Sprintf("%s/address/%s/txs", mempool.url, address.EncodeAddress()))
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

func (mempool *mempool) SubmitTx(tx *wire.MsgTx) (string, error) {
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}

	resp, err := http.Post(fmt.Sprintf("%s/tx", mempool.url), "application/text", bytes.NewBufferString(hex.EncodeToString(buf.Bytes())))
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

func (mempool *mempool) GetFeeRates() (FeeRates, error) {
	var feeRates FeeRates
	resp, err := http.Get(fmt.Sprintf("%s/fees/recommended", mempool.url))
	if err != nil {
		return FeeRates{}, fmt.Errorf("failed to get fee rates: %w", err)
	}
	err = json.NewDecoder(resp.Body).Decode(&feeRates)
	if err != nil {
		return FeeRates{}, fmt.Errorf("failed to unmarshal response body: %w", err)
	}
	return feeRates, nil
}

type blockstream struct {
	url string
}

func NewBlockstream(url string) Indexer {
	return &blockstream{url: url}
}

func (blockstream *blockstream) GetTipBlockHeight() (uint64, error) {
	resp, err := http.Get(fmt.Sprintf("%s/blocks/tip/height", blockstream.url))
	if err != nil {
		return 0, fmt.Errorf("failed to get transaction: %w", err)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %w", err)
	}
	return strconv.ParseUint(string(data), 10, 64)
}

func (blockstream *blockstream) GetTx(txid string) (Transaction, error) {
	resp, err := http.Get(fmt.Sprintf("%s/tx/%s", blockstream.url, txid))
	if err != nil {
		return Transaction{}, fmt.Errorf("failed to get transactions: %w", err)
	}
	var tx Transaction
	if err := json.NewDecoder(resp.Body).Decode(&tx); err != nil {
		return Transaction{}, fmt.Errorf("failed to decode transactions: %w", err)
	}
	return tx, nil
}

func (blockstream *blockstream) GetUTXOs(address btcutil.Address, amount uint64) (UTXOs, uint64, error) {
	resp, err := http.Get(fmt.Sprintf("%s/address/%s/utxo", blockstream.url, address.EncodeAddress()))
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

func (blockstream *blockstream) GetSpendingWitness(address btcutil.Address) ([]string, Transaction, error) {
	resp, err := http.Get(fmt.Sprintf("%s/address/%s/txs", blockstream.url, address.EncodeAddress()))
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

func (blockstream *blockstream) SubmitTx(tx *wire.MsgTx) (string, error) {
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}

	resp, err := http.Post(fmt.Sprintf("%s/tx", blockstream.url), "application/text", bytes.NewBufferString(hex.EncodeToString(buf.Bytes())))
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

func (blockstream *blockstream) GetFeeRates() (FeeRates, error) {
	resp, err := http.Get(fmt.Sprintf("%s/fee-estimates", blockstream.url))
	if err != nil {
		return FeeRates{}, fmt.Errorf("failed to get fee estimates: %w", err)
	}
	fees := map[string]float64{}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return FeeRates{}, fmt.Errorf("failed to read fee estimates: %w", err)
	}
	if err := json.Unmarshal(data, &fees); err != nil {
		return FeeRates{}, fmt.Errorf("failed to unmarshal fee estimates: %w", err)
	}
	if len(fees) == 0 {
		return FeeRates{
			FastestFee:  2,
			HalfHourFee: 2,
			HourFee:     2,
			MinimumFee:  2,
			EconomyFee:  2,
		}, nil
	}

	return FeeRates{
		FastestFee:  int(math.Ceil(fees["1"])),
		HalfHourFee: int(math.Ceil(fees["3"])),
		HourFee:     int(math.Ceil(fees["6"])),
		MinimumFee:  int(math.Ceil(fees["144"])),
		EconomyFee:  int(math.Ceil(fees["504"])),
	}, nil
}

package bitcoin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/wallet/txsizes"
	"github.com/tyler-smith/go-bip32"
)

type instantClient struct {
	url           string
	indexerClient Client
	store         Store
	code          uint32
}

type InstantClient interface {
	Client
	GetStore() Store
	GetInstantWalletAddress(from *btcec.PrivateKey) (string, error)
	FundInstanstWallet(from *btcec.PrivateKey, amount int64) (string, error)
	Deposit(ctx context.Context, amount int64, revokeSecretHash string, from *btcec.PrivateKey) (string, error)
}

func randomBytes(n int) ([]byte, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return []byte{}, err
	}
	return bytes, nil
}

// getStore
func (client *instantClient) GetStore() Store {
	return client.store
}

func (client *instantClient) GetInstantWalletAddress(from *btcec.PrivateKey) (string, error) {
	masterKey, _ := bip32.NewMasterKey(from.Serialize())
	var err error
	client.code, err = client.getCode(masterKey)
	if err != nil {
		return "", err
	}

	wallet, err := client.getInstantWalletDetails(masterKey, client.code)
	if err != nil {
		return "", err
	}
	return *wallet.WalletAddress, nil
}

func InstantWalletWrapper(url string, store Store, client Client) InstantClient {
	return &instantClient{url: url, indexerClient: client, store: store}
}

func (client *instantClient) GetFeeRates() (FeeRates, error) {
	return client.indexerClient.GetFeeRates()
}

func (client *instantClient) Net() *chaincfg.Params {
	return client.indexerClient.Net()
}

func (client *instantClient) GetTx(txid string) (Transaction, error) {
	return client.indexerClient.GetTx(txid)
}

func (client *instantClient) CalculateTransferFee(nInputs, nOutputs int, txType int32) (uint64, error) {
	return client.indexerClient.CalculateTransferFee(nInputs, nOutputs, txType)
}

func (client *instantClient) CalculateRedeemFee() (uint64, error) {
	return client.indexerClient.CalculateRedeemFee()
}
func (client *instantClient) CalculateIWRedeemFee(recipients []Recipient) uint64 {
	feeRates, err := client.GetFeeRates()
	if err != nil {
		return 1500
	}
	size, err := EstimateRedeemTxSize(recipients)
	if err != nil {
		return 1500
	}
	if feeRates.FastestFee < 2 {
		feeRates.FastestFee = 2
	}
	return uint64(feeRates.FastestFee) * uint64(size) * 2
}
func (client *instantClient) CalculateIWRefundFee() uint64 {
	feeRates, err := client.GetFeeRates()
	if err != nil {
		return 10 * uint64(EstimateRefundTxSize())
	}
	if feeRates.FastestFee < 2 {
		feeRates.FastestFee = 2
	}
	return uint64(feeRates.FastestFee) * uint64(EstimateRefundTxSize())
}
func EstimateRefundTxSize() int {
	baseSize := 8 +
		wire.VarIntSerializeSize(1) + // 1 input
		wire.VarIntSerializeSize(1) + // 1 output
		32 + // input txid
		4 + // input vout
		1 + // sigScript
		4 + // input sequence
		8 + // output amount
		1 + // pkScript size
		34 // p2wsk size

	swSize := 1 + 1 + // marker + flag
		1 + 4*1 + // stack number and each stack size
		72 + 72 + 106 // 2 * signature + script
	vsizeFloat := (float64(baseSize)*4 + float64(swSize)) / 4
	return int(math.Ceil(vsizeFloat))
}

func EstimateRedeemTxSize(recipients []Recipient) (int, error) {
	baseSize := 8 +
		wire.VarIntSerializeSize(1) + // 1 input
		32 + // input txid
		4 + // input vout
		1 + // sigScript
		4 // input sequence

	// Adding outputs size
	outputs := make([]*wire.TxOut, len(recipients))
	for i := range recipients {
		payScript, err := txscript.PayToAddrScript(recipients[i].To)
		if err != nil {
			return 0, err
		}
		outputs[i] = wire.NewTxOut(recipients[i].Amount, payScript)
	}
	size := txsizes.SumOutputSerializeSizes(outputs)
	baseSize += size

	swSize := 1 + 1 + // marker + flag
		1 + 4*1 + // stack number and each stack size
		72 + 72 + 106 // 2 * signature + script
	vsizeFloat := (float64(baseSize)*4 + float64(swSize)) / 4
	return int(math.Ceil(vsizeFloat)), nil
}

func (client *instantClient) GetTipBlockHeight() (uint64, error) {
	return client.indexerClient.GetTipBlockHeight()
}

func (client *instantClient) GetUTXOs(address btcutil.Address, amount uint64) (UTXOs, uint64, error) {
	return client.indexerClient.GetUTXOs(address, amount)
}

func (client *instantClient) GetSpendingWitness(address btcutil.Address) ([]string, Transaction, error) {
	return client.indexerClient.GetSpendingWitness(address)
}
func (client *instantClient) GetConfirmations(txHash string) (uint64, uint64, error) {
	return client.indexerClient.GetConfirmations(txHash)
}

func (client *instantClient) Send(to btcutil.Address, amount uint64, from *btcec.PrivateKey) (string, error) {
	// fmt.Println(to)
	masterKey, _ := bip32.NewMasterKey(from.Serialize())
	pubkey := masterKey.PublicKey()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	recipient := Recipient{To: to, Amount: int64(amount)}
	newIwSecret, err := randomBytes(32)
	if err != nil {
		return "", err
	}
	newIwSecretHash := sha256.Sum256([]byte(hex.EncodeToString(newIwSecret)))
	client.code, _ = client.getCode(masterKey)
	currentSecret, _ := client.store.Secret(pubkey.String(), client.code)
	txhash, err := client.Transfer(ctx, []Recipient{recipient}, currentSecret, hex.EncodeToString(newIwSecretHash[:]), masterKey, from)
	if err != nil {
		return "", fmt.Errorf("failed to transfer: %w", err)
	}
	err = client.store.PutSecret(pubkey.String(), hex.EncodeToString(newIwSecret), Created, client.code)
	if err != nil {
		return "", fmt.Errorf("failed to put secret: %w", err)
	}
	err = client.store.PutStatus(pubkey.String(), client.code, RefundTxGenerated)
	if err != nil {
		return "", fmt.Errorf("failed to put status: %w", err)
	}
	return txhash, nil
}

// Spends an atomic swap script using segwit witness
// if the balance of present instant wallet is zero or doesnt exist
// the btc is spent to next instant wallet
// or the balance in current instant wallet is combined iwth atomic swap
// and sent to next instant wallet
func (client *instantClient) Spend(script []byte, redeemScript wire.TxWitness, from *btcec.PrivateKey, waitBlocks uint) (string, error) {
	tx := wire.NewMsgTx(BTC_VERSION)
	masterKey, _ := bip32.NewMasterKey(from.Serialize())
	pubkey := masterKey.PublicKey()
	client.code, _ = client.getCode(masterKey)
	scriptWitnessProgram := sha256.Sum256(script)
	scriptAddr, err := btcutil.NewAddressWitnessScriptHash(scriptWitnessProgram[:], client.Net())
	if err != nil {
		return "", fmt.Errorf("failed to create script address: %w", err)
	}
	utxos, balance, err := client.GetUTXOs(scriptAddr, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get UTXOs: %w", err)
	}
	var inputs []*wire.TxIn
	amounts := make([]uint64, len(utxos))
	for i, utxo := range utxos {
		txid, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return "", fmt.Errorf("failed to parse txid in the utxo: %w", err)
		}
		amounts[i] = utxos[i].Amount
		tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(txid, utxo.Vout), nil, nil))
		inputs = append(inputs, wire.NewTxIn(wire.NewOutPoint(txid, utxo.Vout), nil, nil))
	}

	wallet, _ := client.getInstantWalletDetails(masterKey, client.code)
	walletAddr, err := btcutil.DecodeAddress(*wallet.WalletAddress, client.Net())
	if err != nil {
		return "", fmt.Errorf("failed to decode wallet address: %w", err)
	}
	_, balanceOfWallet, err := client.GetUTXOs(walletAddr, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get utxos: %w", err)
	}
	newSecret, _ := randomBytes(32)
	if err != nil {
		return "", fmt.Errorf("error generating secret: %w", err)
	}
	newSecretHash := sha256.Sum256([]byte(hex.EncodeToString(newSecret)))
	var txHash string
	if balanceOfWallet == 0 {
		//create input = redeem atomic swap
		//create output = IW1 pkaddress
		InstantWallet, err := client.getInstantWalletDetails(masterKey, client.code)
		if err != nil {
			return "", fmt.Errorf("failed to create new instant wallet: %w", err)
		}
		spenderAddr, err := btcutil.DecodeAddress(*InstantWallet.WalletAddress, client.Net())
		if err != nil {
			return "", fmt.Errorf("failed to decode btcutil address from instant wallet address: %w", err)
		}
		spenderToScript, err := txscript.PayToAddrScript(spenderAddr)
		if err != nil {
			return "", fmt.Errorf("failed to create script for address: %w", err)
		}
		FEE, err := client.CalculateRedeemFee()
		if err != nil {
			return "", fmt.Errorf("failed to calculate fee: %w", err)
		}
		if balance < FEE {
			return "", fmt.Errorf("balance is not enough to pay fee balance:%d , fee:%d", balance, FEE)
		}
		tx.AddTxOut(wire.NewTxOut(int64(balance-FEE), spenderToScript))
		for i := range tx.TxIn {
			fetcher := txscript.NewCannedPrevOutputFetcher(script, int64(amounts[i]))
			if waitBlocks > 0 {
				tx.TxIn[i].Sequence = uint32(waitBlocks) + 1
			}
			sigHashes := txscript.NewTxSigHashes(tx, fetcher)
			sig, err := txscript.RawTxInWitnessSignature(tx, sigHashes, i, int64(amounts[i]), script, txscript.SigHashAll, from)
			if err != nil {
				return "", err
			}
			tx.TxIn[i].Witness = append(wire.TxWitness{sig}, redeemScript...)
			tx.TxIn[i].Witness = append(tx.TxIn[i].Witness, wire.TxWitness{script}...)
		}
		outIndex := uint32(len(tx.TxOut) - 1)

		refundFee := client.CalculateIWRefundFee()

		_, err = client.createRefundSignature(&CreateRefundSignatureRequest{
			WalletAddress:    *wallet.WalletAddress,
			RevokeSecretHash: hex.EncodeToString(newSecretHash[:]),
			FundingTxID:      tx.TxHash().String(),
			FundingTxIndex:   &outIndex,
			Amount:           int64(balance) - int64(FEE),
			RefundFee:        int64(refundFee),
		})
		if err != nil {
			return "", err
		}
		wallet, _ := client.getInstantWalletDetails(masterKey, client.code)

		// verify system generated refund signature
		if err := smartVerifyRefundSig(client.instantWalletKey(masterKey, client.code), wallet, client.Net()); err != nil {
			return "", err

		}
		txHash, err = client.SubmitTx(tx)
		if err != nil {
			return "", err
		}
		client.code++
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		FEE, err := client.CalculateRedeemFee()
		if err != nil {
			return "", fmt.Errorf("failed to calculate fee: %w", err)
		}
		currentSecret, err := client.store.Secret(pubkey.String(), client.code)
		if err != nil {
			return "", fmt.Errorf("failed to get current secret: %w", err)
		}
		redeemTx, err := client.GetRedeemTx(ctx, inputs, balance, 0, FEE, currentSecret, hex.EncodeToString(newSecretHash[:]), masterKey, nil)
		if err != nil {
			// panic(err)
			return "", err
		}
		for i := 0; i < len(inputs); i++ {
			fetcher := txscript.NewCannedPrevOutputFetcher(script, int64(amounts[i]))
			if waitBlocks > 0 {
				redeemTx.TxIn[i+1].Sequence = uint32(waitBlocks) + 1
			}
			sigHashes := txscript.NewTxSigHashes(redeemTx, fetcher)
			sig, err := txscript.RawTxInWitnessSignature(redeemTx, sigHashes, i+1, int64(amounts[i]), script, txscript.SigHashAll, from)
			if err != nil {
				return "", err
			}
			redeemTx.TxIn[i+1].Witness = append(wire.TxWitness{sig}, redeemScript...)
			redeemTx.TxIn[i+1].Witness = append(redeemTx.TxIn[i+1].Witness, wire.TxWitness{script}...)
		}
		txHash, err = client.SubmitTx(redeemTx)
		if err != nil {
			return "", err
		}
		client.store.PutStatus(pubkey.String(), client.code-1, Redeemed)
		client.code++

	}
	client.store.PutSecret(pubkey.String(), hex.EncodeToString(newSecret), RefundTxGenerated, client.code)
	return txHash, nil
}

func (client *instantClient) FundInstanstWallet(from *btcec.PrivateKey, amount int64) (string, error) {
	masterKey, _ := bip32.NewMasterKey(from.Serialize())
	pubkey := masterKey.PublicKey()
	// return "", nil
	code, _ := client.getCode(masterKey)
	client.code = code
	wallet, err := client.getInstantWalletDetails(masterKey, client.code)
	if err != nil {
		return "", err
	}
	newSecret, _ := randomBytes(32)
	if err != nil {
		return "", err
	}
	nextSecretHash := sha256.Sum256([]byte(hex.EncodeToString(newSecret)))
	walletAddr, err := btcutil.DecodeAddress(*wallet.WalletAddress, client.Net())
	if err != nil {
		return "", err
	}
	_, balance, err := client.GetUTXOs(walletAddr, 0)
	if err != nil {
		return "", err
	}
	wallet, err = client.getInstantWalletDetails(masterKey, client.code)
	if err != nil {
		return "", err
	}
	var txHash string
	if (code == 0 || wallet.FundingTxID == nil) && balance == 0 {

		txHash, err = client.Deposit(context.Background(), amount, hex.EncodeToString(nextSecretHash[:]), from)
		if err != nil {
			return "", fmt.Errorf("error depositing to instant wallet: %w", err)
		}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		fromAddr, _ := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(from.PubKey().SerializeCompressed()), client.Net())
		utxos, total, err := client.GetUTXOs(fromAddr, uint64(amount))
		if err != nil {
			return "", err
		}

		masterScript, _ := txscript.PayToAddrScript(fromAddr)

		var inputs []*wire.TxIn

		for _, utxo := range utxos {
			hash, err := chainhash.NewHashFromStr(utxo.TxID)
			if err != nil {
				return "", err
			}
			inputs = append(inputs, wire.NewTxIn(wire.NewOutPoint(hash, uint32(utxo.Vout)), nil, nil))

		}
		// newSecretHash := sha256.Sum256(newSecret)
		FEE, err := client.CalculateTransferFee(len(inputs)+1, 2, 2)
		// FEE, err := client.indexerClient.CalculateRedeemFee()
		if err != nil {
			return "", fmt.Errorf("failed to calculate fee: %w", err)
		}
		currentSecret, err := client.store.Secret(pubkey.String(), client.code)
		if err != nil {
			return "", fmt.Errorf("failed to get current secret: %w", err)
		}
		// fromScript, err := txscript.PayToAddrScript(fromAddr)
		if err != nil {
			return "", fmt.Errorf("failed to create script for address: %w", err)
		}
		// fmt.Println(inputs, balance, total-uint64(amount), FEE, currentSecret, hex.EncodeToString(newSecretHash[:]), masterKey, masterScript)
		redeemTx, err := client.GetRedeemTx(ctx, inputs, uint64(amount), total-uint64(amount), FEE, currentSecret, hex.EncodeToString(nextSecretHash[:]), masterKey, masterScript)
		if err != nil {
			// panic(err)
			return "", err
		}

		for i, utxo := range utxos {

			fetcher := txscript.NewCannedPrevOutputFetcher(masterScript, int64(utxo.Amount))
			if err != nil {
				return "", err
			}

			sigHashes := txscript.NewTxSigHashes(redeemTx, fetcher)
			witness, err := txscript.WitnessSignature(redeemTx, sigHashes, i+1, int64(utxo.Amount), masterScript, txscript.SigHashAll, from, true)
			if err != nil {
				return "", err
			}
			redeemTx.TxIn[i+1].Witness = witness
		}
		txHash, err = client.SubmitTx(redeemTx)
		if err != nil {
			return "", err
		}
		client.store.PutStatus(pubkey.String(), client.code-1, Redeemed)
		client.code++

	}
	client.store.PutSecret(pubkey.String(), hex.EncodeToString(newSecret), RefundTxGenerated, client.code)
	return txHash, nil
}

package bitcoin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/catalogfi/blockchain/btc"
	"github.com/catalogfi/guardian"
	"github.com/tyler-smith/go-bip32"
)

type instantClient struct {
	indexerClient Client
	store         Store
	instantWallet guardian.BitcoinWallet
}

type InstantClient interface {
	Client
	GetStore() Store
	GetInstantWalletAddress() string
	FundInstantWallet(from *btcec.PrivateKey, amount int64) (string, error)
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

func (client *instantClient) GetInstantWalletAddress() string {
	return client.instantWallet.WalletAddress().String()
}

func InstantWalletWrapper(store Store, client Client, iw guardian.BitcoinWallet) InstantClient {

	return &instantClient{indexerClient: client, store: store, instantWallet: iw}
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
func (client *instantClient) SubmitTx(tx *wire.MsgTx) (string, error) {
	return client.indexerClient.SubmitTx(tx)
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
	masterKey, err := bip32.NewMasterKey(from.Serialize())
	if err != nil {
		return "", fmt.Errorf("failed to generate key ,error : %v", err)
	}
	pubkey := masterKey.PublicKey()
	iw, err := client.instantWallet.GetInstantWallet()
	if err != nil {
		return "", err
	}
	secret, err := client.store.GetSecret(iw.WalletAddress)
	if err != nil {
		return "", fmt.Errorf("wallet not found in store, deposit to initiate the wallet, error :%v", err)
	}
	recipients := []btc.Recipient{
		{
			To:     to.EncodeAddress(),
			Amount: int64(amount),
		},
	}
	newSecret, err := randomBytes(32)
	if err != nil {
		return "", err
	}
	newSecretHash := sha256.Sum256(newSecret)
	newSecretHashString := hex.EncodeToString(newSecretHash[:])

	txHash, err := client.instantWallet.Spend(context.Background(), recipients, secret, &newSecretHashString, true)
	if err != nil {
		return "", fmt.Errorf("failed to send transaction , error : %v", err)
	}
	err = client.store.PutSecret(pubkey.String(), hex.EncodeToString(newSecret), Redeemed, iw.WalletAddress)
	if err != nil {
		return "", err
	}

	return txHash, nil

}

// Spends an atomic swap script using segwit witness
// if the balance of present instant wallet is zero or doesnt exist
// the btc is spent to next instant wallet
// or the balance in current instant wallet is combined iwth atomic swap
// and sent to next instant wallet
func (client *instantClient) Spend(script []byte, redeemScript wire.TxWitness, from *btcec.PrivateKey, waitBlocks uint) (string, error) {
	scriptWitnessProgram := sha256.Sum256(script)
	scriptAddr, err := btcutil.NewAddressWitnessScriptHash(scriptWitnessProgram[:], client.Net())
	if err != nil {
		return "", fmt.Errorf("failed to create script address: %w", err)
	}

	newSecret, err := randomBytes(32)
	if err != nil {
		return "", err
	}
	newSecretHash := sha256.Sum256(newSecret)
	newSecretHashString := hex.EncodeToString(newSecretHash[:])

	txHash, err := client.instantWallet.RedeemAndDeposit(context.Background(), newSecretHashString, scriptAddr, script, redeemScript, waitBlocks)
	if err != nil {
		return "", err
	}

	iw, err := client.instantWallet.GetInstantWallet()
	if err != nil {
		return "", err
	}
	masterKey, err := bip32.NewMasterKey(from.Serialize())
	if err != nil {
		return "", fmt.Errorf("failed to generate key ,error : %v", err)
	}
	pubkey := masterKey.PublicKey()

	err = client.store.PutSecret(pubkey.String(), hex.EncodeToString(newSecret), RefundTxGenerated, iw.WalletAddress)
	if err != nil {
		return "", err
	}

	return txHash, nil

}

func (client *instantClient) FundInstantWallet(from *btcec.PrivateKey, amount int64) (string, error) {
	masterKey, err := bip32.NewMasterKey(from.Serialize())
	if err != nil {
		return "", fmt.Errorf("failed to generate key ,error : %v", err)
	}
	pubkey := masterKey.PublicKey()
	newSecret, err := randomBytes(32)
	if err != nil {
		return "", err
	}
	newSecretHash := sha256.Sum256(newSecret)
	newSecretHashString := hex.EncodeToString(newSecretHash[:])

	txHash, err := client.instantWallet.Deposit(context.Background(), int64(amount), newSecretHashString)
	if err != nil {
		return "", fmt.Errorf("failed to deposit , error : %v", err)
	}

	iw, err := client.instantWallet.GetInstantWallet()
	if err != nil {
		return "", err
	}
	err = client.store.PutSecret(pubkey.String(), hex.EncodeToString(newSecret), RefundTxGenerated, iw.WalletAddress)
	if err != nil {
		return "", err
	}

	return txHash, nil

}

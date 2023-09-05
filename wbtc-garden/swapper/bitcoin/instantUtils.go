package bitcoin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/tyler-smith/go-bip32"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const DUST = 1000

type Recipient struct {
	To     btcutil.Address `json:"to"`
	Amount int64           `json:"amount"`
}

func (r *Recipient) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		To     string `json:"to"`
		Amount int64  `json:"amount"`
	}{
		To:     r.To.EncodeAddress(),
		Amount: r.Amount,
	})
}

func (r *Recipient) UnmarshalJSON(data []byte) error {
	aux := &struct {
		To     string `json:"to"`
		Amount int64  `json:"amount"`
	}{}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	var (
		toAddr btcutil.Address
		found  bool
		err    error
	)
	for _, network := range []*chaincfg.Params{&chaincfg.RegressionNetParams, &chaincfg.MainNetParams, &chaincfg.TestNet3Params} {
		toAddr, err = btcutil.DecodeAddress(string(aux.To), network)
		if err == nil {
			if toAddr.IsForNet(network) {
				found = true
				break
			}
		}
	}
	if !found {
		return fmt.Errorf("cannot parse address")
	}
	r.To = toAddr
	r.Amount = aux.Amount
	return nil
}
func (client *instantClient) GetRedeemTx(ctx context.Context, asTxIns []*wire.TxIn, amount, change, fee uint64, revokerSecret, nextRevokerSecretHash string, masterKey *bip32.Key, fromScript []byte) (*wire.MsgTx, error) {
	wallet, err := client.getInstantWalletDetails(masterKey, client.code)
	if err != nil {
		return nil, err
	}
	walletAddr, err := btcutil.DecodeAddress(*wallet.WalletAddress, client.Net())
	if err != nil {
		return nil, err
	}
	_, balance, err := client.GetUTXOs(walletAddr, 0)
	if err != nil {
		return nil, err
	}
	newInstantWallet, err := client.getInstantWalletDetails(masterKey, client.code+1)
	if err != nil {
		return nil, err
	}

	nextWalletAddr, err := btcutil.DecodeAddress(*newInstantWallet.WalletAddress, client.Net())
	if err != nil {
		return nil, err
	}
	var recipientsWithoutFee []Recipient
	recipientsWithoutFee = append(recipientsWithoutFee, Recipient{To: nextWalletAddr, Amount: int64(balance)})
	redeemFee := client.CalculateIWRedeemFee(recipientsWithoutFee)
	if err != nil {
		return nil, err
	}
	if balance+amount < uint64(fee+redeemFee) {
		return nil, fmt.Errorf("insufficient balance for fee")
	}

	payScript, err := txscript.PayToAddrScript(nextWalletAddr)
	if err != nil {
		return nil, err
	}

	txID, err := chainhash.NewHashFromStr(*wallet.FundingTxID)
	if err != nil {
		return nil, err
	}
	var recipients []Recipient
	recipients = append(recipients, Recipient{To: nextWalletAddr, Amount: int64(balance - redeemFee - fee)})
	redeemTx := wire.NewMsgTx(2)
	redeemTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(txID, *wallet.FundingTxIndex), nil, nil))
	for _, asTxIn := range asTxIns {
		redeemTx.AddTxIn(asTxIn)
	}
	var outAmt int64
	if change > DUST {
		outAmt = int64(balance + amount - fee - redeemFee)
		redeemTx.AddTxOut(wire.NewTxOut(int64(balance+amount-fee-redeemFee), payScript))
		redeemTx.AddTxOut(wire.NewTxOut(int64(change), fromScript))
	} else {
		outAmt = int64(balance + change + amount - fee - redeemFee)
		redeemTx.AddTxOut(wire.NewTxOut(int64(balance+change+amount-fee-redeemFee), payScript))
	}
	var buf bytes.Buffer
	redeemTx.Serialize(&buf)

	_, err = client.setupRedeemTransactionFromHex(&SetupRedeemTransactionFromHexRequest{
		WalletAddress: *wallet.WalletAddress,
		Recipients:    recipients,
		Fees:          int64(fee + redeemFee),
		RedeemTxHex:   hex.EncodeToString(buf.Bytes()),
	})
	if err != nil {
		return nil, err
	}

	wallet, _ = client.getInstantWalletDetails(masterKey, client.code)

	redeemTx, sig, err := smartGenerateCombinedRedeemTxSig(client.instantWalletKey(masterKey, client.code), wallet, redeemTx, client.Net())
	if err != nil {
		return nil, err
	}
	fundingTxIndex := uint32(0)

	refundFee := client.CalculateIWRefundFee()

	_, err = client.createRefundSignature(&CreateRefundSignatureRequest{
		WalletAddress:    nextWalletAddr.String(),
		RevokeSecretHash: nextRevokerSecretHash,
		FundingTxID:      redeemTx.TxHash().String(),
		FundingTxIndex:   &fundingTxIndex,
		Amount:           outAmt,
		RefundFee:        int64(refundFee),
	})
	if err != nil {
		return nil, err
	}
	newWallet, err := client.getInstantWalletDetails(masterKey, client.code+1)
	if err != nil {
		return nil, err
	}
	nextInstantWallet := client.instantWalletKey(masterKey, client.code+1)

	// verify system generated refund signature
	if err := smartVerifyRefundSig(nextInstantWallet, newWallet, client.Net()); err != nil {
		return nil, err
	}

	// broadcast tx manually for assurance
	systemPubKey, err := hex.DecodeString(*wallet.SystemPubKey)
	if err != nil {
		return nil, err
	}
	redeemDetails, err := GetRedeemTxDetails(wallet)
	if err != nil {
		return nil, err
	}
	systemSig, err := hex.DecodeString(redeemDetails.SystemSignature)
	if err != nil {
		return nil, err
	}
	if err = SetMultisigWitness(redeemTx, client.instantWalletKey(masterKey, client.code).PubKey().SerializeCompressed(), sig, systemPubKey, systemSig, systemPubKey); err != nil {
		return nil, err
	}

	return redeemTx, nil

}

func (client *instantClient) Transfer(ctx context.Context, recipients []Recipient, revokerSecret, nextRevokerSecretHash string, masterKey *bip32.Key, from *secp256k1.PrivateKey) (string, error) {
	wallet, err := client.getInstantWalletDetails(masterKey, client.code)
	if err != nil {
		return "", err
	}

	walletAddr, err := btcutil.DecodeAddress(*wallet.WalletAddress, client.Net())
	if err != nil {
		return "", err
	}
	sendAmount := int64(0)
	for _, recipient := range recipients {
		sendAmount += recipient.Amount
	}
	var refundAddr btcutil.Address
	_, balance, _ := client.GetUTXOs(walletAddr, 0)
	// recipients * 2 for change and fee
	fee := client.CalculateIWRedeemFee(recipients) * 2
	if err != nil {
		return "", fmt.Errorf("failed to calculate fee with %v", err)
	}
	if balance < uint64(sendAmount)+fee {
		return "", fmt.Errorf("insufficient balance "+strconv.FormatInt(int64(balance), 10)+" for send amount "+strconv.FormatInt(sendAmount, 10)+" and fee "+strconv.FormatInt(int64(fee), 10), walletAddr)
	}
	if refundAmount := int64(balance) - sendAmount - int64(fee); refundAmount > 100 {
		newInstantWallet, err := client.getInstantWalletDetails(masterKey, client.code+1)
		if err != nil {
			return "", err
		}

		refundAddr, err = btcutil.DecodeAddress(*newInstantWallet.WalletAddress, client.Net())
		if err != nil {
			return "", err
		}

		recipients = append(recipients, Recipient{To: refundAddr, Amount: int64(refundAmount)})
	}

	resp, err := client.setupRedeemTransaction(&SetupRedeemTransactionRequest{
		WalletAddress: *wallet.WalletAddress,
		Recipients:    recipients,
		Fees:          int64(fee),
	})
	if err != nil {
		return "", err
	}

	wallet.RedeemTxDetails = resp.RedeemTxDetails

	wallet, _ = client.getInstantWalletDetails(masterKey, client.code)
	// verify system generated redeem signature
	if err := smartVerifyRedeemSig(client.instantWalletKey(masterKey, client.code), wallet, client.Net()); err != nil {
		return "", err
	}

	redeemTx, sig, err := smartGenerateRedeemSig(client.instantWalletKey(masterKey, client.code), wallet, recipients, client.Net())
	if err != nil {
		return "", err
	}
	// fmt.Println("recipients", recipients, "fee", fee, "balance", balance, "sendAmount", sendAmount, "redeeemTx", *redeemTx.TxIn[0], len(redeemTx.TxIn), "sig", sig)
	if balance > uint64(sendAmount)+fee {
		outIndex := uint32(len(redeemTx.TxOut) - 1)
		// fmt.Println("outIndex", outIndex)
		refundFee := client.CalculateIWRefundFee()
		_, err := client.createRefundSignature(&CreateRefundSignatureRequest{
			WalletAddress:    refundAddr.String(),
			RevokeSecretHash: nextRevokerSecretHash,
			FundingTxID:      redeemTx.TxHash().String(),
			FundingTxIndex:   &outIndex,
			Amount:           int64(balance - uint64(sendAmount) - fee),
			RefundFee:        int64(refundFee),
		})
		if err != nil {
			return "", err
		}
		newWallet, err := client.getInstantWalletDetails(masterKey, client.code+1)
		if err != nil {
			return "", err
		}
		nextInstantWallet := client.instantWalletKey(masterKey, client.code+1)

		// verify system generated refund signature
		if err := smartVerifyRefundSig(nextInstantWallet, newWallet, client.Net()); err != nil {
			return "", err
		}
	}

	// enables server to broadcast transaction on users behalf in a more guaranteed manner
	if _, err = client.finalizeRedeemTransaction(&FinalizeRedeemTransactionRequest{
		WalletAddress: *wallet.WalletAddress,
		UserSignature: hex.EncodeToString(sig),
		RevokedSecret: revokerSecret,
	}); err != nil {
		return "", err
	}

	// broadcast tx manually for assurance
	systemPubKey, err := hex.DecodeString(*wallet.SystemPubKey)
	if err != nil {
		return "", err
	}
	redeemDetails, err := GetRedeemTxDetails(wallet)
	if err != nil {
		return "", err
	}
	systemSig, err := hex.DecodeString(redeemDetails.SystemSignature)
	if err != nil {
		return "", err
	}
	if err = SetMultisigWitness(redeemTx, client.instantWalletKey(masterKey, client.code).PubKey().SerializeCompressed(), sig, systemPubKey, systemSig, systemPubKey); err != nil {
		return "", err
	}

	fmt.Println("sending redeem tx", zap.Any("tx hash", redeemTx.TxHash().String()))

	if _, err = client.SubmitTx(redeemTx); err != nil {
		fmt.Println("failed to manually submit redeem tx, server will retry broadcast soon", zap.Any("error", err))
	}

	client.code++
	return redeemTx.TxHash().String(), nil
}

func (client *instantClient) SubmitTx(tx *wire.MsgTx) (string, error) {
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return "", fmt.Errorf("failed to serialize transaction: %w", err)
	}

	resp, err := http.Post(fmt.Sprintf("%s/tx", "https://mempool.space/testnet/api"), "application/text", bytes.NewBufferString(hex.EncodeToString(buf.Bytes())))
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}

	// data1, err1 := io.ReadAll(resp.Body)
	// fmt.Println(string(data1), err1)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to send transaction: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read transaction id: %w", err)
	}
	return string(data), nil
}

type BTCInstantWallet struct {
	gorm.Model
	WalletAddress        *string `gorm:"uniqueIndex;not null"`
	UserPubKey           *string `gorm:"uniqueIndex;not null"`
	SystemPrivateKey     *string `gorm:"unique;not null" json:"-"`
	SystemPubKey         *string `gorm:"unique;not null"`
	RevokeSecretHash     *string
	Amount               *uint64
	FundingTxID          *string `gorm:"unique"`
	FundingTxIndex       *uint32
	FundingTxStatus      int64
	FundingTxBlockNumber int64
	RedeemTxDetails      datatypes.JSON
	RefundTxDetails      datatypes.JSON
}

type CreateInstantWalletRequest struct {
	UserPubKey string `json:"userPubKey" binding:"required"`
}

type CreateRefundSignatureRequest struct {
	WalletAddress    string  `json:"walletAddress" binding:"required"`
	RevokeSecretHash string  `json:"revokeSecretHash" binding:"required"`
	FundingTxID      string  `json:"fundingTxID" binding:"required"`
	FundingTxIndex   *uint32 `json:"fundingTxIndex" binding:"required"`
	Amount           int64   `json:"amount" binding:"required"`
	RefundFee        int64   `json:"fee" binding:"required"`
}

type CreateInstantWalletResponse struct {
	BTCInstantWallet
	RefundSignature string `json:"refundSignature"`
	RefundFee       uint32 `json:"refundFee"`
}

type SetupRedeemTransactionRequest struct {
	WalletAddress string      `json:"walletAddress" binding:"required"`
	Recipients    []Recipient `json:"recipients" binding:"required"`
	Fees          int64       `json:"fees" binding:"required"`
}

type SetupRedeemTransactionFromHexRequest struct {
	WalletAddress string      `json:"walletAddress" binding:"required"`
	RedeemTxHex   string      `json:"redeemTxHex" binding:"required"`
	Recipients    []Recipient `json:"recipients" binding:"required"`
	Fees          int64       `json:"fees" binding:"required"`
}

type FinalizeRedeemTransactionRequest struct {
	WalletAddress string `json:"walletAddress" binding:"required"`
	UserSignature string `json:"userSignature" binding:"required"`
	RevokedSecret string `json:"revokedSecret" binding:"required"`
}

type GetInstantWalletRequest struct {
	WalletAddress string `json:"walletAddress"`
	UserPubkey    string `json:"userPubkey"`
}

func (client *instantClient) getInstantWalletDetails(masterKey *bip32.Key, code uint32) (*BTCInstantWallet, error) {
	resp, err := client.getInstantWallet(&GetInstantWalletRequest{
		UserPubkey: hex.EncodeToString(client.instantWalletKey(masterKey, code).PubKey().SerializeCompressed()),
	})
	if err != nil && strings.Contains(err.Error(), "wallet not found") {
		resp, err = client.createInstantWallet(&CreateInstantWalletRequest{
			UserPubKey: hex.EncodeToString(client.instantWalletKey(masterKey, code).PubKey().SerializeCompressed()),
		})
	}
	return resp, err
}

func (client *instantClient) createInstantWallet(req *CreateInstantWalletRequest) (*BTCInstantWallet, error) {
	return client.submitRequest(req, "/createInstantWallet")
}

func (client *instantClient) getInstantWallet(req *GetInstantWalletRequest) (*BTCInstantWallet, error) {
	return client.submitRequest(req, "/getInstantWallet")
}

func (client *instantClient) createRefundSignature(req *CreateRefundSignatureRequest) (*BTCInstantWallet, error) {
	return client.submitRequest(req, "/createRefundSignature")
}

func (client *instantClient) setupRedeemTransaction(req *SetupRedeemTransactionRequest) (*BTCInstantWallet, error) {
	return client.submitRequest(req, "/setupRedeemTransaction")
}
func (client *instantClient) setupRedeemTransactionFromHex(req *SetupRedeemTransactionFromHexRequest) (*BTCInstantWallet, error) {
	return client.submitRequest(req, "/setupRedeemTransactionFromHex")
}

func (client *instantClient) finalizeRedeemTransaction(req *FinalizeRedeemTransactionRequest) (*BTCInstantWallet, error) {
	return client.submitRequest(req, "/finalizeRedeemTransaction")
}

func (client *instantClient) submitRequest(req interface{}, endpoint string) (*BTCInstantWallet, error) {
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(req); err != nil {
		return nil, err
	}

	resp, err := http.Post(client.url+endpoint, "application/json", buf)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errObj := struct {
			Error string `json:"error"`
		}{}

		if err := json.NewDecoder(resp.Body).Decode(&errObj); err != nil {
			errMsg, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read the error message %v", err)
			}
			return nil, fmt.Errorf("failed to decode the error %v %v %v", string(errMsg), endpoint, resp.Body)
		}
		return nil, fmt.Errorf("request failed with status %s %v", endpoint, errObj.Error)
	}

	response := struct {
		Message *BTCInstantWallet `json:"message"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	return response.Message, err
}
func (client *instantClient) instantWalletKey(key *bip32.Key, code uint32) *btcec.PrivateKey {
	instantKey, err := key.NewChildKey(code)
	if err != nil {
		panic(err)
	}
	instantWalletKey, _ := btcec.PrivKeyFromBytes(instantKey.Key)
	return instantWalletKey
}
func (client *instantClient) getCode(key *bip32.Key) (uint32, error) {
	// get the latest instant wallet client and return that code
	code := uint32(0)
	for {
		instantKey, err := key.NewChildKey(code)
		if err != nil {
			panic(err)
		}
		instantWalletKey, _ := btcec.PrivKeyFromBytes(instantKey.Key)

		iw, err := client.getInstantWallet(&GetInstantWalletRequest{
			UserPubkey: hex.EncodeToString(instantWalletKey.PubKey().SerializeCompressed()),
		})
		if err != nil {
			if strings.Contains(fmt.Sprint(err), "wallet not found") {
				return code, nil
			} else {
				panic(err)
			}
		}
		if value := string(iw.RedeemTxDetails); value == "null" {
			return code, nil
		}
		code++
	}
}

func smartSetupTx(utxos UTXOs, fromAddress, toAddress btcutil.Address, amount, totalBalance int64, fee int64) (*wire.MsgTx, error) {
	tx := wire.NewMsgTx(2)
	for _, utxo := range utxos {
		hash, err := chainhash.NewHashFromStr(utxo.TxID)
		if err != nil {
			return nil, err
		}
		txIn := wire.NewTxIn(wire.NewOutPoint(hash, uint32(utxo.Vout)), nil, nil)
		tx.AddTxIn(txIn)
	}

	fromScript, err := txscript.PayToAddrScript(fromAddress)
	if err != nil {
		return nil, err
	}

	toScript, err := txscript.PayToAddrScript(toAddress)
	if err != nil {
		return nil, err
	}
	tx.AddTxOut(wire.NewTxOut(amount, toScript))
	if totalBalance-amount-fee > 0 {
		tx.AddTxOut(wire.NewTxOut(totalBalance-amount-fee, fromScript))
	}

	return tx, nil
}

func smartVerifyRefundSig(userPrivKey *btcec.PrivateKey, instantWallet *BTCInstantWallet, network *chaincfg.Params) error {
	systemPubKey, err := hex.DecodeString(*instantWallet.SystemPubKey)
	if err != nil {
		return err
	}
	script, _, err := GetInstantWalletAddress(userPrivKey.PubKey().SerializeCompressed(), systemPubKey, systemPubKey, network)
	if err != nil {
		return err
	}

	refundDetails, err := GetRefundTxDetails(instantWallet)
	if err != nil {
		return err
	}

	secretHash, err := hex.DecodeString(*instantWallet.RevokeSecretHash)
	if err != nil {
		return err
	}

	systemSig, err := hex.DecodeString(refundDetails.SystemSignature)
	if err != nil {
		return err
	}
	_, refundTx, err := BuildRefundTx(userPrivKey.PubKey().SerializeCompressed(), systemPubKey, secretHash, refundDetails.WaitBlocks, refundDetails.Fees, UTXO{
		TxID:   *instantWallet.FundingTxID,
		Vout:   *instantWallet.FundingTxIndex,
		Amount: *instantWallet.Amount,
	}, network)
	if err != nil {
		return err
	}

	if refundTx.TxHash().String() != refundDetails.TxHash {
		return fmt.Errorf("refund tx hash genereated locally does not match refund tx hash from system")
	}

	fetcher := txscript.NewCannedPrevOutputFetcher(script, int64(*instantWallet.Amount))

	hash, err := txscript.CalcWitnessSigHash(script, txscript.NewTxSigHashes(refundTx, fetcher), txscript.SigHashAll, refundTx, 0, int64(*instantWallet.Amount))
	if err != nil {
		return err
	}

	signature, err := ecdsa.ParseDERSignature(systemSig)
	if err != nil {
		return err
	}

	parsedSystemPubkey, err := btcec.ParsePubKey(systemPubKey)
	if err != nil {
		return err
	}
	if !signature.Verify(hash, parsedSystemPubkey) {
		return fmt.Errorf("invalid refund signature from system for given details")
	}
	return nil
}

func smartVerifyRedeemSig(userPrivKey *btcec.PrivateKey, instantWallet *BTCInstantWallet, network *chaincfg.Params) error {
	systemPubKey, err := hex.DecodeString(*instantWallet.SystemPubKey)
	if err != nil {
		return err
	}
	script, _, err := GetInstantWalletAddress(userPrivKey.PubKey().SerializeCompressed(), systemPubKey, systemPubKey, network)
	if err != nil {
		return err
	}

	redeemTxDetails, err := GetRedeemTxDetails(instantWallet)
	if err != nil {
		return err
	}

	systemSig, err := hex.DecodeString(redeemTxDetails.SystemSignature)
	if err != nil {
		return err
	}
	redeemTx, err := BuildRedeemOrRefundSpendTx(redeemTxDetails.Fees, UTXO{
		TxID:   *instantWallet.FundingTxID,
		Vout:   *instantWallet.FundingTxIndex,
		Amount: uint64(*instantWallet.Amount),
	}, redeemTxDetails.Recipients, network)
	if err != nil {
		return err
	}

	if redeemTx.TxHash().String() != redeemTxDetails.TxHash {
		return fmt.Errorf("redeem tx hash genereated locally does not match redeem tx hash from system")
	}

	fetcher := txscript.NewCannedPrevOutputFetcher(script, int64(*instantWallet.Amount))

	hash, err := txscript.CalcWitnessSigHash(script, txscript.NewTxSigHashes(redeemTx, fetcher), txscript.SigHashAll, redeemTx, 0, int64(*instantWallet.Amount))
	if err != nil {
		return err
	}

	signature, err := ecdsa.ParseDERSignature(systemSig)
	if err != nil {
		return err
	}

	parsedSystemPubkey, err := btcec.ParsePubKey(systemPubKey)
	if err != nil {
		return err
	}
	if !signature.Verify(hash, parsedSystemPubkey) {
		return fmt.Errorf("invalid redeem signature from system for given details")
	}
	return nil
}
func smartGenerateCombinedRedeemTxSig(userPrivKey *btcec.PrivateKey, instantWallet *BTCInstantWallet, redeemTx *wire.MsgTx, network *chaincfg.Params) (*wire.MsgTx, []byte, error) {
	systemPubKey, err := hex.DecodeString(*instantWallet.SystemPubKey)
	if err != nil {
		return nil, nil, err
	}
	script, _, err := GetInstantWalletAddress(userPrivKey.PubKey().SerializeCompressed(), systemPubKey, systemPubKey, network)
	if err != nil {
		return nil, nil, err
	}
	redeemTxDetails, err := GetRedeemTxDetails(instantWallet)
	if err != nil {
		return nil, nil, err
	}
	if redeemTx.TxHash().String() != redeemTxDetails.TxHash {
		return nil, nil, fmt.Errorf("redeem tx hash genereated locally does not match redeem tx hash from system")
	}

	fetcher := txscript.NewCannedPrevOutputFetcher(script, int64(*instantWallet.Amount))
	sig, err := txscript.RawTxInWitnessSignature(redeemTx, txscript.NewTxSigHashes(redeemTx, fetcher), 0, int64(*instantWallet.Amount), script, txscript.SigHashAll, userPrivKey)
	return redeemTx, sig, err
}

func smartGenerateRedeemSig(userPrivKey *btcec.PrivateKey, instantWallet *BTCInstantWallet, recipients []Recipient, network *chaincfg.Params) (*wire.MsgTx, []byte, error) {
	systemPubKey, err := hex.DecodeString(*instantWallet.SystemPubKey)
	if err != nil {
		return nil, nil, err
	}
	script, _, err := GetInstantWalletAddress(userPrivKey.PubKey().SerializeCompressed(), systemPubKey, systemPubKey, network)
	if err != nil {
		return nil, nil, err
	}

	redeemTxDetails, err := GetRedeemTxDetails(instantWallet)
	if err != nil {
		return nil, nil, err
	}

	redeemTx, err := BuildRedeemOrRefundSpendTx(redeemTxDetails.Fees, UTXO{
		TxID:   *instantWallet.FundingTxID,
		Vout:   *instantWallet.FundingTxIndex,
		Amount: uint64(*instantWallet.Amount),
	}, recipients, network)
	if err != nil {
		return nil, nil, err
	}

	if redeemTx.TxHash().String() != redeemTxDetails.TxHash {
		return nil, nil, fmt.Errorf("redeem tx hash genereated locally does not match redeem tx hash from system")
	}

	fetcher := txscript.NewCannedPrevOutputFetcher(script, int64(*instantWallet.Amount))
	sig, err := txscript.RawTxInWitnessSignature(redeemTx, txscript.NewTxSigHashes(redeemTx, fetcher), 0, int64(*instantWallet.Amount), script, txscript.SigHashAll, userPrivKey)
	return redeemTx, sig, err
}

func smartGenerateRefundTx(userPrivKey *btcec.PrivateKey, instantWallet *BTCInstantWallet, network *chaincfg.Params) (*wire.MsgTx, error) {
	systemPubKey, err := hex.DecodeString(*instantWallet.SystemPubKey)
	if err != nil {
		return nil, err
	}
	script, _, err := GetInstantWalletAddress(userPrivKey.PubKey().SerializeCompressed(), systemPubKey, systemPubKey, network)
	if err != nil {
		return nil, err
	}

	refundTxDetails, err := GetRefundTxDetails(instantWallet)
	if err != nil {
		return nil, err
	}

	secretHash, err := hex.DecodeString(*instantWallet.RevokeSecretHash)
	if err != nil {
		return nil, err
	}

	_, refundTx, err := BuildRefundTx(userPrivKey.PubKey().SerializeCompressed(), systemPubKey, secretHash[:], refundTxDetails.WaitBlocks, refundTxDetails.Fees, UTXO{
		TxID:   *instantWallet.FundingTxID,
		Vout:   *instantWallet.FundingTxIndex,
		Amount: *instantWallet.Amount,
	}, network)
	if err != nil {
		return nil, err
	}

	fetcher := txscript.NewCannedPrevOutputFetcher(script, int64(*instantWallet.Amount))
	userSig, err := txscript.RawTxInWitnessSignature(refundTx, txscript.NewTxSigHashes(refundTx, fetcher), 0, int64(*instantWallet.Amount), script, txscript.SigHashAll, userPrivKey)
	if err != nil {
		return nil, err
	}
	systemSig, err := hex.DecodeString(refundTxDetails.SystemSignature)
	if err != nil {
		return nil, err
	}

	if err = SetMultisigWitness(refundTx, userPrivKey.PubKey().SerializeCompressed(), userSig, systemPubKey, systemSig, systemPubKey); err != nil {
		return nil, err
	}

	log.Println("refund tx hash", refundTx.TxHash().String())

	return refundTx, nil
}

func smartGenerateRefundSpendTx(userPrivKey *btcec.PrivateKey, instantWallet *BTCInstantWallet, recipients []Recipient, fee int64, network *chaincfg.Params) (*wire.MsgTx, error) {
	systemPubKey, err := hex.DecodeString(*instantWallet.SystemPubKey)
	if err != nil {
		return nil, err
	}

	refundTxDetails, err := GetRefundTxDetails(instantWallet)
	if err != nil {
		return nil, err
	}

	secretHash, err := hex.DecodeString(*instantWallet.RevokeSecretHash)
	if err != nil {
		return nil, err
	}

	refundScript, refundTx, err := BuildRefundTx(userPrivKey.PubKey().SerializeCompressed(), systemPubKey, secretHash[:], refundTxDetails.WaitBlocks, refundTxDetails.Fees, UTXO{
		TxID:   *instantWallet.FundingTxID,
		Vout:   *instantWallet.FundingTxIndex,
		Amount: *instantWallet.Amount,
	}, network)
	if err != nil {
		return nil, err
	}
	if refundTx.TxHash().String() != refundTxDetails.TxHash {
		return nil, fmt.Errorf("refund tx hash genereated locally does not match refund tx hash from system")
	}

	refundSpendTx, err := BuildRedeemOrRefundSpendTx(fee, UTXO{
		TxID:   refundTx.TxHash().String(),
		Vout:   0,
		Amount: *instantWallet.Amount - uint64(refundTxDetails.Fees),
	}, recipients, network)
	if err != nil {
		return nil, err
	}

	refundSpendTx.TxIn[0].Sequence = uint32(refundTxDetails.WaitBlocks + 1)
	fetcher := txscript.NewCannedPrevOutputFetcher(refundScript, int64(*instantWallet.Amount)-refundTxDetails.Fees)
	refundSig, err := txscript.RawTxInWitnessSignature(refundSpendTx, txscript.NewTxSigHashes(refundSpendTx, fetcher), 0, int64(*instantWallet.Amount)-refundTxDetails.Fees, refundScript, txscript.SigHashAll, userPrivKey)
	if err != nil {
		return nil, err
	}

	if err = SetRefundSpendWitness(refundSpendTx, userPrivKey.PubKey().SerializeCompressed(), systemPubKey, refundSig, secretHash, nil, []byte{0x1}, refundTxDetails.WaitBlocks); err != nil {
		return nil, err
	}

	return refundSpendTx, nil
}

type BTCRedeemTxDetails struct {
	TxHash            string
	UserSignature     string
	SystemSignature   string
	Recipients        []Recipient
	Fees              int64
	Status            int64
	BroadcastAttempts int64
	RelayerMessage    string
	BlockNumber       int64
}

type BTCRefundTxDetails struct {
	TxHash          string
	SystemSignature string
	Fees            int64
	WaitBlocks      int64
	// RevealedRevokerSecret provided by the user when they want to do a redeem call
	RevealedRevokerSecret string
	// Status represents the status of refund tx,
	// will be not 0(pending) if user has sent the refund tx to the blockchain directly
	Status      int64
	BlockNumber int64

	// malicious refund recovery tx, only set when malicious refund is detected and is recovered
	RecoveryTxHash        string
	RecoveryStatus        int64
	RecoveryTxFee         int64
	RecoveryBlockNumber   int64
	RecoveryStatusMessage string
}

func GetRefundTxDetails(n *BTCInstantWallet) (*BTCRefundTxDetails, error) {
	details := BTCRefundTxDetails{}
	if n.RefundTxDetails == nil {
		return nil, nil
	}
	if err := json.Unmarshal(n.RefundTxDetails, &details); err != nil {
		return nil, err
	}
	return &details, nil
}

func GetRedeemTxDetails(n *BTCInstantWallet) (*BTCRedeemTxDetails, error) {
	details := BTCRedeemTxDetails{}
	if n.RedeemTxDetails == nil {
		return nil, nil
	}
	if err := json.Unmarshal(n.RedeemTxDetails, &details); err != nil {
		return nil, err
	}
	return &details, nil
}
func GetInstantWalletAddress(pubKeyA, pubKeyB, randomBytes []byte, network *chaincfg.Params) ([]byte, string, error) {
	instantWallet, err := InstantWalletScript(pubKeyA, pubKeyB, randomBytes)
	if err != nil {
		return nil, "", err
	}

	scriptHash := sha256.Sum256(instantWallet)
	instantWalletAddr, err := btcutil.NewAddressWitnessScriptHash((scriptHash)[:], network)
	if err != nil {
		return nil, "", err
	}
	return instantWallet, instantWalletAddr.EncodeAddress(), nil
}
func (client *instantClient) Deposit(ctx context.Context, amount int64, revokeSecretHash string, from *btcec.PrivateKey) (string, error) {
	masterKey, _ := bip32.NewMasterKey(from.Serialize())
	code, err := client.getCode(masterKey)
	if err != nil {
		return "", fmt.Errorf("failed to get code: %v", err)
	}
	wallet, err := client.getInstantWalletDetails(masterKey, client.code)
	if err != nil {
		return "", err
	}

	walletAddr, err := btcutil.DecodeAddress(*wallet.WalletAddress, client.Net())
	if err != nil {
		return "", err
	}
	fromAddr, _ := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(from.PubKey().SerializeCompressed()), client.Net())
	utxos, total, err := client.GetUTXOs(fromAddr, uint64(amount))
	if err != nil {
		return "", err
	}
	fee, err := client.CalculateTransferFee(len(utxos)+1, 2, 2)
	if err != nil {
		return "", err
	}

	masterScript, _ := txscript.PayToAddrScript(fromAddr)

	tx, err := smartSetupTx(utxos, fromAddr, walletAddr, amount, int64(total), int64(fee))
	if err != nil {
		return "", err
	}

	for i, utxo := range utxos {

		fetcher := txscript.NewCannedPrevOutputFetcher(masterScript, int64(utxo.Amount))
		if err != nil {
			return "", err
		}

		sigHashes := txscript.NewTxSigHashes(tx, fetcher)
		witness, err := txscript.WitnessSignature(tx, sigHashes, i, int64(utxo.Amount), masterScript, txscript.SigHashAll, from, true)
		if err != nil {
			return "", err
		}
		tx.TxIn[i].Witness = witness
	}
	// print serialized tx
	fundingTxIndex := uint32(0)
	refundFee := client.CalculateIWRefundFee()

	_, err = client.createRefundSignature(&CreateRefundSignatureRequest{
		WalletAddress:    *wallet.WalletAddress,
		RevokeSecretHash: revokeSecretHash,
		FundingTxID:      tx.TxHash().String(),
		FundingTxIndex:   &fundingTxIndex,
		Amount:           amount,
		RefundFee:        int64(refundFee),
	})
	if err != nil {
		return "", err
	}
	wallet, err = client.getInstantWalletDetails(masterKey, code)
	if err != nil {
		return "", err
	}
	nextInstantWallet := client.instantWalletKey(masterKey, code)

	// verify system generated refund signature
	if err := smartVerifyRefundSig(nextInstantWallet, wallet, client.Net()); err != nil {
		return "", err
	}

	fmt.Println("sending funding tx", revokeSecretHash, zap.Any("tx hash", tx.TxHash().String()))
	return client.SubmitTx(tx)
}

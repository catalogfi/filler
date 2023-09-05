package bitcoin

import (
	"crypto/sha256"
	"fmt"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

const (
	DefaultBTCVersion = 2
)

// InstantWalletScript generates instant wallet script
func InstantWalletScript(pubKeyA, pubKeyB, randomBytes []byte) ([]byte, error) {
	return txscript.NewScriptBuilder().
		AddData(randomBytes).
		AddOp(txscript.OP_DROP).
		AddOp(txscript.OP_2).
		AddData(pubKeyA).
		AddData(pubKeyB).
		AddOp(txscript.OP_2).
		AddOp(txscript.OP_CHECKMULTISIG).
		Script()
}

// RefundScript generates refund script
func RefundScript(ownerPub, revokerPub, refundSecretHash []byte, waitTime int64) ([]byte, error) {
	return txscript.NewScriptBuilder().
		AddOp(txscript.OP_IF).
		AddInt64(waitTime).
		AddOp(txscript.OP_CHECKSEQUENCEVERIFY).
		AddOp(txscript.OP_DROP).
		AddData(ownerPub).
		AddOp(txscript.OP_ELSE).
		AddOp(txscript.OP_SHA256).
		AddData(refundSecretHash).
		AddOp(txscript.OP_EQUALVERIFY).
		AddData(revokerPub).
		AddOp(txscript.OP_ENDIF).
		AddOp(txscript.OP_CHECKSIG).
		Script()
}

// BuildFundingTx builds the instant wallet funding tx, this is mostly used in tests
func BuildFundingTx(inputUtxos UTXOs, from btcutil.Address, utxoTotalValue, amount, fee int64, script []byte) (*wire.MsgTx, error) {
	fundTx := wire.NewMsgTx(DefaultBTCVersion)

	for _, inputUtxo := range inputUtxos {
		txID, err := chainhash.NewHashFromStr(inputUtxo.TxID)
		if err != nil {
			return nil, err
		}
		// the funding tx utxo details sent by user (the tx does not yet exist on chain)
		fundTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(txID, inputUtxo.Vout), nil, nil))
	}

	payScript, err := WitnessScriptHash(script)
	if err != nil {
		return nil, err
	}
	fromAddr, err := txscript.PayToAddrScript(from)
	if err != nil {
		return nil, err
	}

	fundTx.AddTxOut(wire.NewTxOut(amount, payScript))
	fundTx.AddTxOut(wire.NewTxOut(utxoTotalValue-amount-fee, fromAddr))
	return fundTx, nil
}

// BuildRefundTx builds the instant wallet refund tx paying to the RefundScript
func BuildRefundTx(ownerPub, revokerPub, refundSecretHash []byte, waitTime, fee int64, fundingUtxo UTXO, network *chaincfg.Params) ([]byte, *wire.MsgTx, error) {
	refundTx := wire.NewMsgTx(DefaultBTCVersion)
	txID, err := chainhash.NewHashFromStr(fundingUtxo.TxID)
	if err != nil {
		return nil, nil, err
	}
	// the funding tx utxo details sent by user (the tx does not yet exist on chain)
	refundTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(txID, fundingUtxo.Vout), nil, nil))

	refundScript, err := RefundScript(ownerPub, revokerPub, refundSecretHash, waitTime)
	if err != nil {
		return nil, nil, err
	}
	payScript, err := WitnessScriptHash(refundScript)
	if err != nil {
		return nil, nil, err
	}
	refundTx.AddTxOut(wire.NewTxOut(int64(fundingUtxo.Amount)-fee, payScript))
	return refundScript, refundTx, nil
}

// BuildRedeemOrRefundSpendTx builds a simple btc tx given the inputs and to address, used for redeem txs and refund spend txs
func BuildRedeemOrRefundSpendTx(fee int64, instantUtxo UTXO, recipients []Recipient, network *chaincfg.Params) (*wire.MsgTx, error) {
	redeemTx := wire.NewMsgTx(DefaultBTCVersion)
	txID, err := chainhash.NewHashFromStr(instantUtxo.TxID)
	if err != nil {
		return nil, err
	}
	// the funding tx utxo details sent by user (the tx does not yet exist on chain)
	redeemTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(txID, instantUtxo.Vout), nil, nil))

	total := int64(0)
	for i := range recipients {
		payScript, err := txscript.PayToAddrScript(recipients[i].To)
		if err != nil {
			return nil, err
		}
		redeemTx.AddTxOut(wire.NewTxOut(int64(recipients[i].Amount), payScript))
		total += int64(recipients[i].Amount)
	}
	if total+fee > int64(instantUtxo.Amount) {
		return nil, fmt.Errorf("total amount spent %v+fee(%v) > utxo amount(%v)", total, fee, instantUtxo.Amount)
	}
	return redeemTx, nil
}

// SetMultisigWitness used for setting the witness script for any txs using the instant wallet utxo(redeem, refund)
func SetMultisigWitness(multisigTx *wire.MsgTx, pubkeyA, sigA, pubKeyB, sigB, randomBytes []byte) error {
	// assumed that there's only 1 txIn (instant wallet utxo)

	instantWalletScript, err := InstantWalletScript(pubkeyA, pubKeyB, randomBytes)
	if err != nil {
		return err
	}
	witnessStack := wire.TxWitness(make([][]byte, 4))
	witnessStack[0] = nil
	witnessStack[1] = sigA
	witnessStack[2] = sigB
	witnessStack[3] = instantWalletScript

	multisigTx.TxIn[0].Witness = witnessStack
	return nil
}

// SetRefundSpendWitness used for setting witness signature for spending the refunded utxo from the refund script,
// has 2 possible paths either refund immediately through user secret or refund through timelock.
func SetRefundSpendWitness(refundSpend *wire.MsgTx, ownerPubkey, revokerPubkey, signature, refundSecretHash, refundSecret, op []byte, waitTime int64) error {
	// assumed that there's only 1 txIn (instant wallet utxo)
	refundScript, err := RefundScript(ownerPubkey, revokerPubkey, refundSecretHash, waitTime)
	if err != nil {
		return err
	}

	var witnessStack wire.TxWitness

	// else condition for instant revokation
	if op == nil {
		witnessStack = wire.TxWitness(make([][]byte, 4))
		witnessStack[0] = signature
		witnessStack[1] = refundSecret
		witnessStack[2] = nil
		witnessStack[3] = refundScript
	} else {
		// for users normal refund flow
		witnessStack = wire.TxWitness(make([][]byte, 3))
		witnessStack[0] = signature
		witnessStack[1] = []byte{0x1}
		witnessStack[2] = refundScript
	}
	refundSpend.TxIn[0].Witness = witnessStack
	return nil
}

func WitnessScriptHash(witnessScript []byte) ([]byte, error) {
	bldr := txscript.NewScriptBuilder()

	bldr.AddOp(txscript.OP_0)
	scriptHash := sha256.Sum256(witnessScript)
	bldr.AddData(scriptHash[:])
	return bldr.Script()
}

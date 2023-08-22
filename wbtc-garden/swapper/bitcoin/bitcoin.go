package bitcoin

import (
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// NewHTLCScript builds a bitcoin script following BIP-199 (https://github.com/bitcoin/bips/blob/master/bip-0199.mediawiki#summary)
func NewHTLCScript(initiatorAddress, redeemerAddress btcutil.Address, secretHash []byte, waitTime int64) ([]byte, error) {
	return txscript.NewScriptBuilder().
		AddOp(txscript.OP_IF).
		AddOp(txscript.OP_SHA256).
		AddData(secretHash).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_DUP).
		AddOp(txscript.OP_HASH160).
		AddData(redeemerAddress.ScriptAddress()).
		AddOp(txscript.OP_ELSE).
		AddInt64(waitTime).
		AddOp(txscript.OP_CHECKSEQUENCEVERIFY).
		AddOp(txscript.OP_DROP).
		AddOp(txscript.OP_DUP).
		AddOp(txscript.OP_HASH160).
		AddData(initiatorAddress.ScriptAddress()).
		AddOp(txscript.OP_ENDIF).
		AddOp(txscript.OP_EQUALVERIFY).
		AddOp(txscript.OP_CHECKSIG).
		Script()
}

func NewHTLCRedeemWitness(pubKey, secret []byte) wire.TxWitness {
	return wire.TxWitness{pubKey, secret, []byte{0x1}}
}

func NewHTLCRefundWitness(pubKey []byte) wire.TxWitness {
	return wire.TxWitness{pubKey, nil}
}

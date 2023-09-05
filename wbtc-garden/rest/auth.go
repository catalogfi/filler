package rest

import (
	"fmt"
	"strings"
	"time"

	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/rest/utils"
	"github.com/dgrijalva/jwt-go"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/spruceid/siwe-go"
)

type auth struct {
	config model.Network
}

type Claims struct {
	UserWallet string `json:"userWallet"`
	jwt.StandardClaims
}

func NewAuth(config model.Network) Auth {
	return &auth{
		config: config,
	}
}

func (a *auth) Verify(req model.VerifySiwe) (*jwt.Token, error) {
	parsedMessage, err := siwe.ParseMessage(req.Message)
	if err != nil {
		return nil, fmt.Errorf("Error parsing message: %w ", err)
	}

	valid, err := parsedMessage.ValidNow()
	if err != nil {
		return nil, fmt.Errorf("Error validating message: %w ", err)
	}
	if !valid {
		return nil, fmt.Errorf("Validating expired Token")
	}

	fromAddress, err := a.verifySignature(parsedMessage.String(), req.Signature, parsedMessage.GetAddress(), parsedMessage.GetChainID())

	if err != nil {
		return nil, fmt.Errorf("Error verifying message: %w ", err)
	}

	claims := &Claims{
		UserWallet: strings.ToLower(fromAddress.String()),
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(time.Hour * 24).Unix(), // Token expires in 24 hours
			IssuedAt:  time.Now().Unix(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token, nil

}

/*
verifies the whether the given signature is valid or not.

if the recoveredAddress from signature is user, then it is valid signature.
if not, then it is a smart contract and we directly call ERC1271 to verify the signature.
*/
func (a *auth) verifySignature(msg string, signature string, owner common.Address, chainId int) (*common.Address, error) {

	sigHash := utils.GetEIP191SigHash(msg)
	sigBytes, err := hexutil.Decode(signature)
	if err != nil {
		return nil, err
	}
	if sigBytes[64] != 27 && sigBytes[64] != 28 {
		return nil, fmt.Errorf("Invalid signature recovery byte")
	}
	sigBytes[64] -= 27
	pubkey, err := crypto.SigToPub(sigHash.Bytes(), sigBytes)
	if err != nil {
		return nil, err
	}
	addr := crypto.PubkeyToAddress(*pubkey)
	if addr != owner {
		sigBytes[64] += 27
		return utils.CheckERC1271Sig(sigHash, sigBytes, owner, chainId, a.config)
	}
	return &addr, nil

}

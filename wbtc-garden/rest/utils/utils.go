package utils

import (
	"context"
	"fmt"

	"github.com/catalogfi/cobi/wbtc-garden/model"
	ERC1271 "github.com/catalogfi/cobi/wbtc-garden/rest/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

func GetEthClientByChainId(chainId int, config model.Config) (*ethclient.Client, error) {
	switch chainId {
	case 1:
		return ethclient.Dial(config[model.Ethereum].RPC)
	case 11155111:
		return ethclient.Dial(config[model.EthereumSepolia].RPC)
	case 10:
		return ethclient.Dial(config[model.EthereumOptimism].RPC)
	default:
		return nil, fmt.Errorf("No RPC url found for chainId")

	}
}

/*
Prefixes the given message with the EIP191 prefix (x19Ethereum Signed Message:\n32) followed by message.
And then returns the Keccak256 hash of the prefixed message.
*/
func GetEIP191SigHash(msg string) common.Hash {
	// Ref: https://stackoverflow.com/questions/49085737/geth-ecrecover-invalid-signature-recovery-id
	utf8Btyes := []byte(msg)
	prefixedMsg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len([]byte(utf8Btyes)), utf8Btyes)
	return crypto.Keccak256Hash([]byte(prefixedMsg))
}

/*
Checks if the given signature is valid or not according to ERC1271.
*/
func CheckERC1271Sig(sigHash common.Hash, signature []byte, verifyingContract common.Address, chainId int, config model.Config) (*common.Address, error) {
	conn, err := GetEthClientByChainId(chainId, config)
	if err != nil {
		return nil, err
	}
	code, err := conn.CodeAt(context.Background(), verifyingContract, nil)
	if err != nil {
		return nil, err
	}
	if len(code) == 0 {
		return nil, fmt.Errorf("Invalid signature")
	}
	erc1271, err := ERC1271.NewERC1271(verifyingContract, conn)
	if err != nil {
		return nil, err
	}
	res, err := erc1271.IsValidSignature(nil, sigHash, signature)
	if err != nil {
		return nil, err
	}
	// 0x1626ba7e is the ERC1271 magic value
	if hexutil.Encode(res[:]) != "0x1626ba7e" {
		return nil, fmt.Errorf("Invalid signature")
	}
	return &verifyingContract, nil
}

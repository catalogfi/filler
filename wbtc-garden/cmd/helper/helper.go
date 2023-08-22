package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/catalogfi/cobi/wbtc-garden/model"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/ethereum"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/ethereum/typings/ERC20"
	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
	"go.uber.org/zap"
)

func getKeys(entropy []byte, chain model.Chain, user uint32, selector []uint32) ([]interface{}, error) {
	masterKey, err := bip32.NewMasterKey(entropy)
	if err != nil {
		return nil, fmt.Errorf("failed to create master key: %v", err)
	}

	var key *bip32.Key

	switch chain {
	case model.Bitcoin:
		key, err = masterKey.NewChildKey(0)
		if err != nil {
			return nil, fmt.Errorf("failed to create child key: %v", err)
		}
	case model.BitcoinTestnet, model.BitcoinRegtest:
		key, err = masterKey.NewChildKey(1)
		if err != nil {
			return nil, fmt.Errorf("failed to create child key: %v", err)
		}
	case model.Ethereum, model.EthereumLocalnet, model.EthereumSepolia, model.EthereumOptimism:
		key, err = masterKey.NewChildKey(60)
		if err != nil {
			return nil, fmt.Errorf("failed to create child key: %v", err)
		}
	default:
		return nil, fmt.Errorf("invalid chain: %s", chain)
	}

	key, err = key.NewChildKey(user)
	if err != nil {
		return nil, fmt.Errorf("failed to create child key: %v", err)
	}

	keys := make([]interface{}, len(selector))
	for i, sel := range selector {
		childKey, err := key.NewChildKey(sel)
		if err != nil {
			return nil, fmt.Errorf("failed to create %d child key: %v", sel, err)
		}

		if chain.IsBTC() {
			privKey, _ := btcec.PrivKeyFromBytes(childKey.PublicKey().Key)
			keys[i] = privKey
		} else if chain.IsEVM() {
			privKey, err := crypto.ToECDSA(childKey.Key)
			if err != nil {
				return nil, fmt.Errorf("failed to create private key: %v", err)
			}
			keys[i] = privKey
		} else {
			return nil, fmt.Errorf("unsupported chain: %s", chain)
		}
	}
	return keys, nil
}

func readMnemonic() ([]byte, error) {
	// homeDir, err := os.UserHomeDir()
	// if err != nil {
	// 	return nil, err
	// }

	if data, err := os.ReadFile("./cob/MNEMONIC"); err == nil {
		return bip39.EntropyFromMnemonic(string(data))
	}
	fmt.Println("Generating new mnemonic")
	entropy := [32]byte{}

	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, err
	}
	mnemonic, err := bip39.NewMnemonic(entropy[:])
	if err != nil {
		return nil, err
	}
	fmt.Println(mnemonic)

	file, err := os.Create("./cob/MNEMONIC")
	if err != nil {
		fmt.Println("error above", err)
		return nil, err
	}
	defer file.Close()

	_, err = file.WriteString(mnemonic)
	if err != nil {
		fmt.Println("error here", err)
		return nil, err
	}
	return entropy[:], nil
}

func watch() {

}

func main() {
	PRIVKEY_1 := "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	PRIVKEY_2 := "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"

	pk1, err := crypto.HexToECDSA(PRIVKEY_1)
	if err != nil {
		panic(err)
	}
	pk2, err := crypto.HexToECDSA(PRIVKEY_2)
	if err != nil {
		panic(err)
	}

	addr1 := crypto.PubkeyToAddress(pk1.PublicKey)
	addr2 := crypto.PubkeyToAddress(pk2.PublicKey)

	fmt.Println("addr1", addr1.Hex())
	fmt.Println("addr2", addr2.Hex())

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	client, err := ethereum.NewClient(logger, "http://localhost:8545")
	if err != nil {
		panic(err)
	}

	wbtc, err := ERC20.NewERC20(common.HexToAddress("0x401dDf5FD514c7C3AD5bFF8A70221ff7d091163F"), bind.ContractBackend(client.GetProvider()))
	if err != nil {
		panic(err)
	}

	currBlock, err := client.GetCurrentBlock()
	if err != nil {
		panic(err)
	}

	ops, err := bind.NewKeyedTransactorWithChainID(pk1, big.NewInt(1337))
	if err != nil {
		panic(err)
	}

	_, err = wbtc.Transfer(ops, addr2, big.NewInt(1000))
	if err != nil {
		panic(err)
	}

	for {
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		logs, err := client.GetProvider().FilterLogs(ctx, geth.FilterQuery{
			Addresses: []common.Address{common.HexToAddress("0x401dDf5FD514c7C3AD5bFF8A70221ff7d091163F")},
			FromBlock: big.NewInt(int64(currBlock - 10)),
			Topics:    [][]common.Hash{{common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")}, {}, {addr2.Hash()}},
		})
		if err != nil {
			panic(err)
		}
		if len(logs) == 0 {
			fmt.Println("no logs")
			time.Sleep(3 * time.Second)
			continue
		}
		fmt.Println("logs", logs)
	}

	// entropy, err := readMnemonic()
	// if err != nil {
	// 	panic(err)
	// }
	// alice, err := getKeys(entropy, model.BitcoinRegtest, 0, []uint32{0})
	// if err != nil {
	// 	panic(err)
	// }
	// bob, err := getKeys(entropy, model.BitcoinRegtest, 1, []uint32{0})
	// if err != nil {
	// 	panic(err)
	// }

	// aPK := alice[0].(*btcec.PrivateKey)
	// bPK := bob[0].(*btcec.PrivateKey)

	// fmt.Println("Alice's address:", aPK.ToECDSA().D.Text(16))
	// fmt.Println("Bob's address:", bPK.ToECDSA().D.Text(16))
}

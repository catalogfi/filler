package cobi

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/wbtc-garden/blockchain"
	"github.com/catalogfi/wbtc-garden/model"
	"github.com/catalogfi/wbtc-garden/swapper/bitcoin"
	"github.com/catalogfi/wbtc-garden/swapper/ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tyler-smith/go-bip32"
)

type Key struct {
	inner *bip32.Key
}

func (key *Key) Interface(chain model.Chain) (interface{}, error) {
	if chain.IsBTC() {
		return key.BtcKey(), nil
	} else {
		return key.ECDSA()
	}
}

func (key *Key) BtcKey() *btcec.PrivateKey {
	privKey, _ := btcec.PrivKeyFromBytes(key.inner.PublicKey().Key)
	return privKey
}

func (key *Key) ECDSA() (*ecdsa.PrivateKey, error) {
	return crypto.ToECDSA(key.inner.Key)
}

func (key *Key) Address(chain model.Chain) (string, error) {
	switch {
	case chain.IsBTC():
		params := getParams(chain)
		addr, err := key.P2pkhAddress(params)
		if err != nil {
			return "", err
		}
		return addr.EncodeAddress(), nil
	case chain.IsEVM():
		addr, err := key.EvmAddress()
		if err != nil {
			return "", err
		}
		return addr.Hex(), nil
	default:
		return "", fmt.Errorf("unsupport chain type %v", chain)
	}
}

func (key *Key) P2pkhAddress(network *chaincfg.Params) (btcutil.Address, error) {
	keyBytesHash := btcutil.Hash160(key.BtcKey().PubKey().SerializeCompressed())
	return btcutil.NewAddressPubKeyHash(keyBytesHash, network)
}

func (key *Key) EvmAddress() (common.Address, error) {
	ecdsaKey, err := key.ECDSA()
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(ecdsaKey.PublicKey), nil
}

func LoadKey(seed []byte, chain model.Chain, user, selector uint32) (*Key, error) {
	masterKey, err := bip32.NewMasterKey(seed)
	if err != nil {
		return nil, err
	}

	var index uint32
	switch chain {
	case model.Bitcoin:
		index = 0
	case model.BitcoinTestnet, model.BitcoinRegtest:
		index = 1
	case model.Ethereum, model.EthereumLocalnet, model.EthereumSepolia, model.EthereumOptimism:
		index = 60
	default:
		return nil, fmt.Errorf("invalid chain: %s", chain)
	}

	for _, idx := range append([]uint32{index}, user, selector) {
		masterKey, err = masterKey.NewChildKey(idx)
		if err != nil {
			return nil, fmt.Errorf("failed to create child key: %v", err)
		}
	}
	return &Key{masterKey}, nil
}

type Keys struct {
	m map[[32]byte]*Key
}

func NewKeys() Keys {
	return Keys{
		m: map[[32]byte]*Key{},
	}
}

func (keys Keys) GetKey(seed []byte, chain model.Chain, user, selector uint32) (*Key, error) {
	digest := append(seed, []byte(fmt.Sprintf("%v_%v_%v", chain, user, selector))...)
	mapKey := sha256.Sum256(digest)
	value, ok := keys.m[mapKey]
	if !ok {
		var err error
		value, err = LoadKey(seed, chain, user, selector)
		if err != nil {
			return nil, err
		}
		keys.m[mapKey] = value
	}
	return value, nil
}

func getParams(chain model.Chain) *chaincfg.Params {
	switch chain {
	case model.Bitcoin:
		return &chaincfg.MainNetParams
	case model.BitcoinTestnet:
		return &chaincfg.TestNet3Params
	case model.BitcoinRegtest:
		return &chaincfg.RegressionNetParams
	default:
		panic("constraint violation: unknown chain")
	}
}

func getBalance(chain model.Chain, address string, config model.Config, asset model.Asset) (*big.Int, error) {
	client, err := blockchain.LoadClient(chain, config.RPC)
	if err != nil {
		return nil, fmt.Errorf("failed to load client: %v", err)
	}

	switch client := client.(type) {
	case bitcoin.Client:
		address, err := btcutil.DecodeAddress(address, client.Net())
		if err != nil {
			return nil, fmt.Errorf("failed to create address: %v", err)
		}
		_, balance, err := client.GetUTXOs(address, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to get UTXOs: %v", err)
		}
		return big.NewInt(int64(balance)), nil

	case ethereum.Client:
		address := common.HexToAddress(address)
		if asset == model.Primary {
			balance, err := client.GetProvider().BalanceAt(context.Background(), address, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get ETH balance: %v", err)
			}
			return balance, nil
		} else {
			balance, err := client.GetERC20Balance(common.HexToAddress(asset.SecondaryID()), address)
			if err != nil {
				return nil, fmt.Errorf("failed to get ERC20 balance: %v", err)
			}
			return balance, nil
		}
	default:
		return nil, fmt.Errorf("unsupported chain: %s", chain)
	}
}

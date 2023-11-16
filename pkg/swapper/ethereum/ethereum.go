package ethereum

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"fmt"
	"math/big"
	"time"

	"github.com/catalogfi/cobi/pkg/swapper"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const MaxQueryBlockRange = 2000

type initiatorSwap struct {
	orderID          [32]byte
	initiator        *ecdsa.PrivateKey
	initiatorAddr    common.Address
	redeemerAddr     common.Address
	lastCheckedBlock *big.Int
	expiry           *big.Int
	atomicSwapAddr   common.Address
	secretHash       []byte
	client           Client
	amount           *big.Int
	tokenAddr        common.Address
	watcher          swapper.Watcher
}
type redeemerSwap struct {
	orderID          [32]byte
	redeemer         *ecdsa.PrivateKey
	lastCheckedBlock *big.Int
	expiry           *big.Int
	atomicSwapAddr   common.Address
	tokenAddr        common.Address
	amount           *big.Int
	secretHash       []byte
	client           Client
	watcher          swapper.Watcher
}

func NewInitiatorSwap(initiator *ecdsa.PrivateKey, redeemerAddr, atomicSwapAddr common.Address, secretHash []byte, expiry, minConfirmations, amount *big.Int, client Client, eventWindow int64) (swapper.InitiatorSwap, error) {

	initiatorAddr := crypto.PubkeyToAddress(initiator.PublicKey)
	orderId := sha256.Sum256(append(secretHash, initiatorAddr.Hash().Bytes()...))

	latestCheckedBlock := new(big.Int).Sub(expiry, big.NewInt(12000))
	if latestCheckedBlock.Cmp(big.NewInt(0)) == -1 {
		latestCheckedBlock = big.NewInt(0)
	}

	watcher, err := NewWatcher(atomicSwapAddr, secretHash, orderId[:], expiry, minConfirmations, amount, client, eventWindow)
	if err != nil {
		return &initiatorSwap{}, err
	}
	tokenAddr, err := client.GetTokenAddress(atomicSwapAddr)
	if err != nil {
		return &initiatorSwap{}, err
	}
	return &initiatorSwap{
		orderID:          orderId,
		initiator:        initiator,
		watcher:          watcher,
		initiatorAddr:    initiatorAddr,
		expiry:           expiry,
		atomicSwapAddr:   atomicSwapAddr,
		client:           client,
		amount:           amount,
		tokenAddr:        tokenAddr,
		redeemerAddr:     redeemerAddr,
		lastCheckedBlock: latestCheckedBlock,
		secretHash:       secretHash,
	}, nil
}

func (initiatorSwap *initiatorSwap) Initiate() (string, error) {
	return initiatorSwap.client.InitiateAtomicSwap(initiatorSwap.atomicSwapAddr, initiatorSwap.initiator, initiatorSwap.redeemerAddr, initiatorSwap.tokenAddr, initiatorSwap.expiry, initiatorSwap.amount, initiatorSwap.secretHash)
}

func (initiatorSwap *initiatorSwap) Expired() (bool, error) {
	return initiatorSwap.watcher.Expired()
}

func (initiatorSwap *initiatorSwap) WaitForRedeem() ([]byte, string, error) {
	for {
		redeemed, secret, txHash, err := initiatorSwap.IsRedeemed()
		if err != nil {
			fmt.Println("failed to check redeemed status", err)
			time.Sleep(5 * time.Second)
			continue
		}
		if redeemed {
			return secret, txHash, err
		}
		time.Sleep(5 * time.Second)
	}
}

func (initiatorSwap *initiatorSwap) IsRedeemed() (bool, []byte, string, error) {
	return initiatorSwap.watcher.IsRedeemed()
}

func (initiatorSwap *initiatorSwap) Refund() (string, error) {

	// Initialise the transactor
	transactor, err := initiatorSwap.client.GetTransactOpts(initiatorSwap.initiator)
	if err != nil {
		return "", err
	}

	tx, err := initiatorSwap.client.RefundAtomicSwap(initiatorSwap.atomicSwapAddr, transactor, initiatorSwap.tokenAddr, initiatorSwap.orderID)
	if err != nil {
		return "", err
	}
	return tx, nil
}

func NewRedeemerSwap(redeemer *ecdsa.PrivateKey, initiatorAddr, atomicSwapAddr common.Address, secretHash []byte, expiry, amount, minConfirmations *big.Int, client Client, eventWindow int64) (swapper.RedeemerSwap, error) {
	orderId := sha256.Sum256(append(secretHash, initiatorAddr.Hash().Bytes()...))
	watcher, err := NewWatcher(atomicSwapAddr, secretHash, orderId[:], expiry, minConfirmations, amount, client, eventWindow)
	if err != nil {
		return &redeemerSwap{}, err
	}

	tokenAddr, err := client.GetTokenAddress(atomicSwapAddr)
	if err != nil {
		return &redeemerSwap{}, err
	}

	lastCheckedBlock := new(big.Int).Sub(expiry, big.NewInt(12000))
	return &redeemerSwap{
		orderID:          orderId,
		redeemer:         redeemer,
		watcher:          watcher,
		lastCheckedBlock: lastCheckedBlock,
		expiry:           expiry,
		atomicSwapAddr:   atomicSwapAddr,
		tokenAddr:        tokenAddr,
		amount:           amount,
		client:           client,
		secretHash:       secretHash,
	}, nil
}

func (redeemerSwap *redeemerSwap) Redeem(secret []byte) (string, error) {
	transactor, err := redeemerSwap.client.GetTransactOpts(redeemerSwap.redeemer)
	if err != nil {
		return "", err
	}
	return redeemerSwap.client.RedeemAtomicSwap(redeemerSwap.atomicSwapAddr, transactor, redeemerSwap.tokenAddr, redeemerSwap.orderID, secret)
}

func (redeemerSwap *redeemerSwap) IsInitiated() (bool, string, uint64, error) {
	initated, txhash, _, minConf, err := redeemerSwap.watcher.IsInitiated()
	return initated, txhash, minConf, err
}

func (redeemerSwap *redeemerSwap) WaitForInitiate() (string, error) {
	defer fmt.Println("Done WaitForInitiate")
	for {
		initiated, txHash, _, err := redeemerSwap.IsInitiated()
		if initiated {
			return txHash, nil
		}
		if err != nil {
			fmt.Println("failed to check initiated status", err)
		}
		time.Sleep(5 * time.Second)
	}
}

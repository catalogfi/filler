package ethswap

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/catalogfi/cobi/pkg/swap/ethswap/bindings"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Wallet interface {
	Address() common.Address

	Balance(ctx context.Context, pending bool) (*big.Int, error)

	TokenBalance(ctx context.Context, pending bool) (*big.Int, error)

	Initiate(ctx context.Context, swap Swap) (string, error)

	Redeem(ctx context.Context, swap Swap, secret []byte) (string, error)

	Refund(ctx context.Context, swap Swap) (string, error)
}

type wallet struct {
	options Options
	mu      *sync.Mutex
	key     *ecdsa.PrivateKey
	addr    common.Address
	client  *ethclient.Client
	swap    *bindings.AtomicSwap
	token   *bindings.ERC20
	nonce   uint64
}

func NewWallet(options Options, key *ecdsa.PrivateKey, client *ethclient.Client) (Wallet, error) {
	atomicSwap, err := bindings.NewAtomicSwap(options.SwapAddr, client)
	if err != nil {
		return nil, err
	}
	tokenAddr, err := atomicSwap.Token(&bind.CallOpts{})
	if err != nil {
		return nil, err
	}
	erc20, err := bindings.NewERC20(tokenAddr, client)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	if options.ChainID.Cmp(chainID) != 0 {
		return nil, fmt.Errorf("wrong chain ID, expect %v, got %v", options.ChainID, chainID)
	}

	nonce, err := client.PendingNonceAt(ctx, crypto.PubkeyToAddress(key.PublicKey))
	if err != nil {
		return nil, err
	}
	return &wallet{
		options: options,
		mu:      new(sync.Mutex),
		key:     key,
		addr:    crypto.PubkeyToAddress(key.PublicKey),
		client:  client,
		swap:    atomicSwap,
		token:   erc20,
		nonce:   nonce,
	}, nil
}

func (wallet *wallet) Address() common.Address {
	return wallet.addr
}

func (wallet *wallet) Balance(ctx context.Context, pending bool) (*big.Int, error) {
	// return the eth balance
	if pending {
		return wallet.client.PendingBalanceAt(ctx, wallet.addr)
	}
	return wallet.client.BalanceAt(ctx, wallet.addr, nil)
}

func (wallet *wallet) TokenBalance(ctx context.Context, pending bool) (*big.Int, error) {
	callOpts := &bind.CallOpts{
		Pending: pending,
		Context: ctx,
	}
	return wallet.token.BalanceOf(callOpts, wallet.addr)
}

func (wallet *wallet) Initiate(ctx context.Context, swap Swap) (string, error) {
	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	allowance, err := wallet.token.Allowance(&bind.CallOpts{}, swap.Initiator, wallet.options.SwapAddr)
	if err != nil {
		return "", err
	}
	transactor, err := bind.NewKeyedTransactorWithChainID(wallet.key, wallet.options.ChainID)
	if err != nil {
		return "", err
	}
	transactor.Nonce = big.NewInt(int64(wallet.nonce))

	// Approve the allowance if it's not enough
	if allowance.Cmp(swap.Amount) < 0 {
		approveTx, err := wallet.token.Approve(transactor, wallet.options.SwapAddr, swap.Amount)
		if err != nil {
			if strings.Contains(err.Error(), "nonce too low") {
				wallet.calibrateNonce()
			}
			return "", err
		}
		if _, err := bind.WaitMined(ctx, wallet.client, approveTx); err != nil {
			return "", err
		}
		wallet.nonce++
		transactor.Nonce = big.NewInt(int64(wallet.nonce))
	}

	// Initiate the atomic swap
	tx, err := wallet.swap.Initiate(transactor, swap.Redeemer, swap.Expiry, swap.Amount, swap.SecretHash)
	if err != nil {
		if strings.Contains(err.Error(), "nonce too low") {
			wallet.calibrateNonce()
		}
		return "", err
	}
	wallet.nonce++
	return tx.Hash().String(), nil
}

func (wallet *wallet) Redeem(ctx context.Context, swap Swap, secret []byte) (string, error) {
	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	transactor, err := bind.NewKeyedTransactorWithChainID(wallet.key, wallet.options.ChainID)
	if err != nil {
		return "", err
	}
	transactor.Nonce = big.NewInt(int64(wallet.nonce))

	tx, err := wallet.swap.Redeem(transactor, swap.ID, secret)
	if err != nil {
		if strings.Contains(err.Error(), "nonce too low") {
			wallet.calibrateNonce()
		}
		return "", err
	}
	wallet.nonce++
	return tx.Hash().String(), nil
}

func (wallet *wallet) Refund(ctx context.Context, swap Swap) (string, error) {
	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	transactor, err := bind.NewKeyedTransactorWithChainID(wallet.key, wallet.options.ChainID)
	if err != nil {
		return "", err
	}
	transactor.Nonce = big.NewInt(int64(wallet.nonce))

	tx, err := wallet.swap.Refund(transactor, swap.ID)
	if err != nil {
		if strings.Contains(err.Error(), "nonce too low") {
			wallet.calibrateNonce()
		}
		return "", err
	}
	wallet.nonce++
	return tx.Hash().String(), nil
}

func (wallet *wallet) calibrateNonce() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nonce, err := wallet.client.PendingNonceAt(ctx, wallet.addr)
	if err != nil {
		log.Print("failed to get nonce ", err)
		return
	}
	wallet.nonce = nonce
}

package ethswap

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
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

	Balance(ctx context.Context, tokenAddr *common.Address, pending bool) (*big.Int, error)

	Initiate(ctx context.Context, swap *Swap) (string, error)

	Redeem(ctx context.Context, swap *Swap, secret []byte) (string, error)

	Refund(ctx context.Context, swap *Swap) (string, error)
}

type wallet struct {
	options Options
	mu      *sync.Mutex
	key     *ecdsa.PrivateKey
	addr    common.Address
	client  *ethclient.Client
	swap    *bindings.AtomicSwap
	token   *bindings.ERC20
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
		return nil, fmt.Errorf("wrong chain ID")
	}

	return &wallet{
		options: options,
		mu:      new(sync.Mutex),
		key:     key,
		addr:    crypto.PubkeyToAddress(key.PublicKey),
		client:  client,
		swap:    atomicSwap,
		token:   erc20,
	}, nil
}

func (wallet *wallet) Address() common.Address {
	return wallet.addr
}

func (wallet *wallet) Balance(ctx context.Context, tokenAddr *common.Address, pending bool) (*big.Int, error) {
	if tokenAddr == nil {
		// return the eth balance
		if pending {
			return wallet.client.PendingBalanceAt(ctx, wallet.addr)
		}
		return wallet.client.BalanceAt(ctx, wallet.addr, nil)
	} else {
		// return the erc20 balance
		erc20, err := bindings.NewERC20(*tokenAddr, wallet.client)
		if err != nil {
			return nil, err
		}
		callOpts := &bind.CallOpts{
			Pending: pending,
			Context: ctx,
		}
		return erc20.BalanceOf(callOpts, wallet.addr)
	}
}

func (wallet *wallet) Initiate(ctx context.Context, swap *Swap) (string, error) {
	allowance, err := wallet.token.Allowance(&bind.CallOpts{}, swap.Initiator, wallet.options.SwapAddr)
	if err != nil {
		return "", err
	}
	transactor, err := bind.NewKeyedTransactorWithChainID(wallet.key, wallet.options.ChainID)
	if err != nil {
		return "", err
	}

	// Approve the allowance if it's not enough
	if allowance.Cmp(swap.Amount) < 0 {
		approveTx, err := wallet.token.Approve(transactor, wallet.options.SwapAddr, swap.Amount)
		if err != nil {
			return "", err
		}
		if _, err := bind.WaitMined(ctx, wallet.client, approveTx); err != nil {
			return "", err
		}
	}

	// Initiate the atomic swap
	tx, err := wallet.swap.Initiate(transactor, swap.Redeemer, swap.Expiry, swap.Amount, swap.SecretHash)
	if err != nil {
		return "", err
	}
	receipt, err := bind.WaitMined(ctx, wallet.client, tx)
	if err != nil {
		return "", err
	}
	return receipt.TxHash.String(), nil
}

func (wallet *wallet) Redeem(ctx context.Context, swap *Swap, secret []byte) (string, error) {
	transactor, err := bind.NewKeyedTransactorWithChainID(wallet.key, wallet.options.ChainID)
	if err != nil {
		return "", err
	}

	tx, err := wallet.swap.Redeem(transactor, swap.ID, secret)
	if err != nil {
		return "", err
	}
	receipt, err := bind.WaitMined(ctx, wallet.client, tx)
	if err != nil {
		return "", err
	}
	return receipt.TxHash.String(), nil
}

func (wallet *wallet) Refund(ctx context.Context, swap *Swap) (string, error) {
	transactor, err := bind.NewKeyedTransactorWithChainID(wallet.key, wallet.options.ChainID)
	if err != nil {
		return "", err
	}
	tx, err := wallet.swap.Refund(transactor, swap.ID)
	if err != nil {
		return "", err
	}
	receipt, err := bind.WaitMined(ctx, wallet.client, tx)
	if err != nil {
		return "", err
	}
	return receipt.TxHash.String(), nil
}

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

	SignMessage(message string) ([]byte, error)
}

type wallet struct {
	mu     *sync.Mutex
	key    *ecdsa.PrivateKey
	addr   common.Address
	client *ethclient.Client
	swap   *bindings.AtomicSwap
	token  *bindings.ERC20

	chainID   *big.Int
	tokenAddr common.Address
	swapAddr  common.Address
}

func NewWallet(key *ecdsa.PrivateKey, client *ethclient.Client, swapAddr common.Address) (Wallet, error) {
	atomicSwap, err := bindings.NewAtomicSwap(swapAddr, client)
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

	return &wallet{
		mu:        new(sync.Mutex),
		key:       key,
		addr:      crypto.PubkeyToAddress(key.PublicKey),
		client:    client,
		swap:      atomicSwap,
		token:     erc20,
		chainID:   chainID,
		tokenAddr: tokenAddr,
		swapAddr:  swapAddr,
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
	allowance, err := wallet.token.Allowance(&bind.CallOpts{}, swap.Initiator, wallet.swapAddr)
	if err != nil {
		return "", err
	}
	transactor, err := bind.NewKeyedTransactorWithChainID(wallet.key, wallet.chainID)
	if err != nil {
		return "", err
	}

	// Approve the allowance if it's not enough
	if allowance.Cmp(swap.Amount) < 0 {
		approveTx, err := wallet.token.Approve(transactor, wallet.swapAddr, swap.Amount)
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
	transactor, err := bind.NewKeyedTransactorWithChainID(wallet.key, wallet.chainID)
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
	transactor, err := bind.NewKeyedTransactorWithChainID(wallet.key, wallet.chainID)
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

func (wallet *wallet) SignMessage(message string) ([]byte, error) {

	sign := ToEIP191SignedMessageHash([]byte(message))
	signature, err := crypto.Sign(sign.Bytes(), wallet.key)

	if err != nil {
		return nil, err
	}

	signature[64] += 27
	return signature, nil
}

func ToEIP191SignedMessageHash(data []byte) common.Hash {
	msg := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(data), data)
	return crypto.Keccak256Hash([]byte(msg))
}

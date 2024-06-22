package ethswap

import (
	"context"
	"crypto/ecdsa"
	"fmt"
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

	// Address returns the address of the wallet
	Address() common.Address

	// Client returns the blockchain client.
	Client() *ethclient.Client

	// Balance returns the ETH balance of the wallet address
	Balance(ctx context.Context, pending bool) (*big.Int, error)

	// TokenBalance returns the token balance of the wallet address. Token is assumed an ERC-20 token and retrieved from
	// the swap contract.
	TokenBalance(ctx context.Context, pending bool) (*big.Int, error)

	// Initiate an atomic swap.
	Initiate(ctx context.Context, swap Swap) (common.Hash, error)

	// Redeem an atomic swap.
	Redeem(ctx context.Context, swap Swap, secret []byte) (common.Hash, error)

	// Refund an atomic swap.
	Refund(ctx context.Context, swap Swap) (common.Hash, error)
}

type wallet struct {
	options Options
	key     *ecdsa.PrivateKey
	client  *ethclient.Client

	mu    *sync.Mutex
	addr  common.Address
	swap  *bindings.AtomicSwap
	token *bindings.ERC20
	nonce uint64
}

func NewWallet(options Options, key *ecdsa.PrivateKey, client *ethclient.Client) (Wallet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	callOpts := &bind.CallOpts{Context: ctx}
	addr := crypto.PubkeyToAddress(key.PublicKey)

	// Make sure the chain ID matches our expectation, so we know we are on the right chain.
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return nil, err
	}
	if options.ChainID.Cmp(chainID) != 0 {
		return nil, fmt.Errorf("wrong chain ID, expect %v, got %v", options.ChainID, chainID)
	}

	// Get the token contract address from the swap contract and initialise the bindings.
	atomicSwap, err := bindings.NewAtomicSwap(options.SwapAddr, client)
	if err != nil {
		return nil, err
	}
	tokenAddr, err := atomicSwap.Token(callOpts)
	if err != nil {
		return nil, err
	}
	erc20, err := bindings.NewERC20(tokenAddr, client)
	if err != nil {
		return nil, err
	}

	wal := &wallet{
		options: options,
		key:     key,
		client:  client,

		mu:    new(sync.Mutex),
		addr:  addr,
		swap:  atomicSwap,
		token: erc20,
	}

	// Get the pending nonce, and we'll manually manage the nonce with the wallet.
	wal.nonce, err = client.PendingNonceAt(ctx, crypto.PubkeyToAddress(key.PublicKey))
	if err != nil {
		return nil, err
	}

	if err := wal.allowanceCheck(); err != nil {
		return nil, err
	}

	return wal, nil
}

func (wallet *wallet) Address() common.Address {
	return wallet.addr
}

func (wallet *wallet) Client() *ethclient.Client {
	return wallet.client
}

func (wallet *wallet) Balance(ctx context.Context, pending bool) (*big.Int, error) {
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

func (wallet *wallet) Initiate(ctx context.Context, swap Swap) (common.Hash, error) {
	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	transactor, err := wallet.transactor(ctx)
	if err != nil {
		return common.Hash{}, err
	}

	// Initiate the atomic swap
	tx, err := wallet.swap.Initiate(transactor, swap.Redeemer, swap.Expiry, swap.Amount, swap.SecretHash)
	if err != nil {
		if strings.Contains(err.Error(), "nonce too low") {
			if inErr := wallet.calibrateNonce(); inErr != nil {
				return common.Hash{}, fmt.Errorf("initiation failed = %v, reset nonce failed = %v", err, inErr)
			}
		}
		return common.Hash{}, err
	}
	wallet.nonce++
	return tx.Hash(), nil
}

func (wallet *wallet) Redeem(ctx context.Context, swap Swap, secret []byte) (common.Hash, error) {
	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	transactor, err := wallet.transactor(ctx)
	if err != nil {
		return common.Hash{}, err
	}

	tx, err := wallet.swap.Redeem(transactor, swap.ID, secret)
	if err != nil {
		if strings.Contains(err.Error(), "nonce too low") {
			if inErr := wallet.calibrateNonce(); inErr != nil {
				return common.Hash{}, fmt.Errorf("redeem failed = %v, reset nonce failed = %v", err, inErr)
			}
		}
		return common.Hash{}, err
	}
	wallet.nonce++
	return tx.Hash(), nil
}

func (wallet *wallet) Refund(ctx context.Context, swap Swap) (common.Hash, error) {
	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	transactor, err := wallet.transactor(ctx)
	if err != nil {
		return common.Hash{}, err
	}

	tx, err := wallet.swap.Refund(transactor, swap.ID)
	if err != nil {
		if strings.Contains(err.Error(), "nonce too low") {
			if inErr := wallet.calibrateNonce(); inErr != nil {
				return common.Hash{}, fmt.Errorf("refund failed = %v, reset nonce failed = %v", err, inErr)
			}
		}
		return common.Hash{}, err
	}
	wallet.nonce++
	return tx.Hash(), nil
}

func (wallet *wallet) allowanceCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	callOpts := &bind.CallOpts{Context: ctx}

	// Check we have enough allowance for the swap contract
	allowance, err := wallet.token.Allowance(callOpts, wallet.addr, wallet.options.SwapAddr)
	if err != nil {
		return err
	}
	totalSupply, err := wallet.token.TotalSupply(callOpts)
	if err != nil {
		return err
	}

	// Do a large approval when the allowance is low, we should only need to do this once.
	if allowance.Cmp(totalSupply) == -1 {
		transactor, err := wallet.transactor(ctx)
		if err != nil {
			return err
		}

		// Approve the max allowance
		data := make([]byte, 32)
		for i := 0; i < 32; i++ {
			data[i] = 0xff
		}
		max := big.NewInt(0).SetBytes(data)
		tx, err := wallet.token.Approve(transactor, wallet.options.SwapAddr, max)
		if err != nil {
			return err
		}

		// Wait for the tx to be mined and start
		receipt, err := bind.WaitMined(ctx, wallet.client, tx)
		if err != nil {
			return err
		}

		// Check if transaction has been reverted
		if receipt.Status == 0 {
			return fmt.Errorf("tx reverted, hash = %v", receipt.TxHash.Hex())
		}
	}
	return nil
}

func (wallet *wallet) calibrateNonce() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	nonce, err := wallet.client.PendingNonceAt(ctx, wallet.addr)
	if err != nil {
		return err
	}
	wallet.nonce = nonce
	return nil
}

func (wallet *wallet) transactor(ctx context.Context) (*bind.TransactOpts, error) {
	transactor, err := bind.NewKeyedTransactorWithChainID(wallet.key, wallet.options.ChainID)
	if err != nil {
		return nil, err
	}
	transactor.Nonce = big.NewInt(int64(wallet.nonce))
	transactor.Context = ctx
	return transactor, nil
}

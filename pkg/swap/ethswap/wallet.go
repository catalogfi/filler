package ethswap

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/catalogfi/blockchain/evm/bindings/contracts/htlc/gardenhtlc"
	"github.com/catalogfi/blockchain/evm/bindings/openzeppelin/contracts/token/ERC20/erc20"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type TransactFunc func(*bind.TransactOpts) (*types.Transaction, error)

type Wallet interface {

	// Address returns the address of the wallet
	Address() common.Address

	// Client returns the blockchain client.
	Client() *ethclient.Client

	// Balance returns the ETH balance of the wallet address
	Balance(ctx context.Context, pending bool) (*big.Int, error)

	// TokenBalance returns the token balance of the wallet address. Token is assumed an ERC-20 token and retrieved from
	// the HTLC contract.
	TokenBalance(ctx context.Context, pending bool) (*big.Int, error)

	// Initiate an atomic swap.
	Initiate(ctx context.Context, swap Swap) (*types.Transaction, error)

	// Redeem an atomic swap.
	Redeem(ctx context.Context, swap Swap, secret []byte) (*types.Transaction, error)

	// Refund an atomic swap.
	Refund(ctx context.Context, swap Swap) (*types.Transaction, error)
}

type wallet struct {
	options Options
	key     *ecdsa.PrivateKey
	client  *ethclient.Client

	mu           *sync.Mutex
	addr         common.Address
	htlc         *gardenhtlc.GardenHTLC
	token        *erc20.ERC20
	transactOpts *bind.TransactOpts
}

func NewWallet(options Options, key *ecdsa.PrivateKey, client *ethclient.Client) (Wallet, error) {
	ctx, cancel := context.WithTimeout(context.Background(), options.Timeout)
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

	// Initialise bindings.
	htlc, err := gardenhtlc.NewGardenHTLC(options.SwapAddr, client)
	if err != nil {
		return nil, err
	}
	tokenAddr, err := htlc.Token(callOpts)
	if err != nil {
		return nil, err
	}
	erc20, err := erc20.NewERC20(tokenAddr, client)
	if err != nil {
		return nil, err
	}

	// Initialise the transactor
	nonce, err := client.PendingNonceAt(ctx, crypto.PubkeyToAddress(key.PublicKey))
	if err != nil {
		return nil, err
	}
	transactor, err := bind.NewKeyedTransactorWithChainID(key, options.ChainID)
	if err != nil {
		return nil, err
	}
	transactor.Nonce = big.NewInt(int64(nonce))

	wal := &wallet{
		options: options,
		key:     key,
		client:  client,

		mu:           new(sync.Mutex),
		addr:         addr,
		htlc:         htlc,
		token:        erc20,
		transactOpts: transactor,
	}

	// Check token allowance against the token contract
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

func (wallet *wallet) Initiate(ctx context.Context, swap Swap) (*types.Transaction, error) {
	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	// Initiate the atomic swap
	f := func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return wallet.htlc.Initiate(opts, swap.Redeemer, swap.Expiry, swap.Amount, swap.SecretHash)
	}
	return wallet.transact(ctx, f)
}

func (wallet *wallet) Redeem(ctx context.Context, swap Swap, secret []byte) (*types.Transaction, error) {
	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	f := func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return wallet.htlc.Redeem(opts, swap.ID, secret)
	}
	return wallet.transact(ctx, f)
}

func (wallet *wallet) Refund(ctx context.Context, swap Swap) (*types.Transaction, error) {
	wallet.mu.Lock()
	defer wallet.mu.Unlock()

	f := func(opts *bind.TransactOpts) (*types.Transaction, error) {
		return wallet.htlc.Refund(opts, swap.ID)
	}
	return wallet.transact(ctx, f)
}

func (wallet *wallet) allowanceCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), wallet.options.Timeout*2)
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
		data := make([]byte, 32)
		for i := 0; i < 32; i++ {
			data[i] = 0xff
		}
		max := big.NewInt(0).SetBytes(data)
		f := func(opts *bind.TransactOpts) (*types.Transaction, error) {
			return wallet.token.Approve(opts, wallet.options.SwapAddr, max)
		}
		tx, err := wallet.transact(ctx, f)
		if err != nil {
			return err
		}

		// Wait for the tx to be mined and check receipt status
		receipt, err := bind.WaitMined(ctx, wallet.client, tx)
		if err != nil {
			return err
		}
		if receipt.Status == 0 {
			return fmt.Errorf("tx reverted, hash = %v", receipt.TxHash.Hex())
		}
	}
	return nil
}

func (wallet *wallet) transact(ctx context.Context, f TransactFunc) (*types.Transaction, error) {
	for {
		tx, err := f(wallet.transactOpts)
		if err != nil {
			// If nonce is incorrect
			if strings.Contains(err.Error(), "nonce too low") || strings.Contains(err.Error(), "tx doesn't have the correct nonce") {
				nonce, err := wallet.client.PendingNonceAt(ctx, wallet.addr)
				if err != nil {
					return nil, err
				}
				wallet.transactOpts.Nonce = big.NewInt(int64(nonce))
				continue
			}

			// Return other errors immediately without retrying
			return nil, err
		}
		wallet.transactOpts.Nonce = big.NewInt(wallet.transactOpts.Nonce.Int64() + 1)
		return tx, nil
	}
}

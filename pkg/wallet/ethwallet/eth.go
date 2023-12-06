package ethwallet

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"sync"

	"github.com/catalogfi/cobi/pkg/wallet/ethwallet/bindings"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Swap struct {
	Initiator  common.Address
	Redeemer   common.Address
	SecretHash common.Hash
	Client     *ethclient.Client
	Amount     *big.Int
	Expiry     *big.Int
	ID         [32]byte
	secret     []byte
}

type Wallet interface {
	Address() string

	Balance(ctx context.Context, tokenAddr *common.Address, pending bool) (*big.Int, error)

	Initiate(ctx context.Context, swap Swap) (string, error)

	Initiated(ctx context.Context, swap Swap) (bool, error)

	Initiator(ctx context.Context, swap Swap) (string, error)

	Redeem(ctx context.Context, swap Swap, secret []byte) (string, error)

	Redeemed(ctx context.Context, swap Swap) (bool, error)

	Secret(ctx context.Context, swap Swap) ([]byte, error)

	Refund(ctx context.Context, swap Swap) (string, error)

	Expired(ctx context.Context, swap Swap) (bool, error)
}

type wallet struct {
	mu     *sync.Mutex
	key    *ecdsa.PrivateKey
	addr   common.Address
	client *ethclient.Client
	swap   *bindings.AtomicSwap
	token  *bindings.ERC20

	chainID       *big.Int
	tokenAddr     common.Address
	swapAddr      common.Address
	watchInterval uint64
}

func New() Wallet {
	return &wallet{}
}

func (wallet *wallet) Address() string {
	return wallet.addr.Hex()
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

func (wallet *wallet) Initiate(ctx context.Context, swap Swap) (string, error) {
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
		if _, err := bind.WaitMined(ctx, swap.Client, approveTx); err != nil {
			return "", err
		}
	}

	// Initiate the atomic swap
	tx, err := wallet.swap.Initiate(transactor, swap.Redeemer, swap.Expiry, swap.Amount, swap.SecretHash)
	if err != nil {
		return "", err
	}
	receipt, err := bind.WaitMined(ctx, swap.Client, tx)
	if err != nil {
		return "", err
	}
	return receipt.TxHash.String(), nil
}

func (wallet *wallet) Initiated(ctx context.Context, swap Swap) (bool, error) {
	callOpts := &bind.CallOpts{Context: ctx}
	details, err := wallet.swap.AtomicSwapOrders(callOpts, swap.ID)
	if err != nil {
		return false, err
	}
	if details.InitiatedAt.Uint64() == 0 {
		return false, nil
	}
	return true, nil
}

func (wallet *wallet) Initiator(ctx context.Context, swap Swap) (string, error) {
	details, err := wallet.swap.AtomicSwapOrders(&bind.CallOpts{Context: ctx}, swap.ID)
	if err != nil {
		return "", err
	}
	if details.InitiatedAt == nil {
		return "", fmt.Errorf("swap not initiated")
	}
	return details.Initiator.Hex(), nil
}

func (wallet *wallet) Redeem(ctx context.Context, swap Swap, secret []byte) (string, error) {
	transactor, err := bind.NewKeyedTransactorWithChainID(wallet.key, wallet.chainID)
	if err != nil {
		return "", err
	}

	tx, err := wallet.swap.Redeem(transactor, swap.ID, secret)
	if err != nil {
		return "", err
	}
	receipt, err := bind.WaitMined(ctx, swap.Client, tx)
	if err != nil {
		return "", err
	}
	return receipt.TxHash.String(), nil
}

func (wallet *wallet) Redeemed(ctx context.Context, swap Swap) (bool, error) {
	// Check if the order has been fulfilled
	details, err := wallet.swap.AtomicSwapOrders(&bind.CallOpts{Context: ctx}, swap.ID)
	if err != nil {
		return false, err
	}
	return details.IsFulfilled, nil
}

func (wallet *wallet) Secret(ctx context.Context, swap Swap) ([]byte, error) {
	// Check if the swap has been fulfilled
	details, err := wallet.swap.AtomicSwapOrders(&bind.CallOpts{Context: ctx}, swap.ID)
	if err != nil {
		return nil, err
	}
	if !details.IsFulfilled {
		return nil, nil
	}

	// Filter the logs to get the secret
	start := details.InitiatedAt
	latestBlock, err := swap.Client.BlockByNumber(ctx, nil)
	if err != nil {
		return nil, err
	}
	latest := latestBlock.Number()
	if latest.Cmp(details.Expiry) == 1 {
		latest = details.Expiry
	}

	for start.Cmp(latest) == -1 {
		end := start.Uint64() + wallet.watchInterval
		if end > latest.Uint64() {
			end = latest.Uint64()
		}
		opts := bind.FilterOpts{
			Start:   start.Uint64(),
			End:     &end,
			Context: ctx,
		}
		iter, err := wallet.swap.FilterRedeemed(&opts, [][32]byte{swap.ID}, [][32]byte{swap.SecretHash})
		if err != nil {
			return nil, err
		}
		if iter.Error() != nil {
			return nil, iter.Error()
		}

		for iter.Next() {
			return iter.Event.Secret, nil
		}
		start = big.NewInt(int64(end))
	}

	return nil, fmt.Errorf("secret not found")
}

func (wallet *wallet) Refund(ctx context.Context, swap Swap) (string, error) {
	transactor, err := bind.NewKeyedTransactorWithChainID(wallet.key, wallet.chainID)
	if err != nil {
		return "", err
	}
	tx, err := wallet.swap.Refund(transactor, swap.ID)
	if err != nil {
		return "", err
	}
	receipt, err := bind.WaitMined(ctx, swap.Client, tx)
	if err != nil {
		return "", err
	}
	return receipt.TxHash.String(), nil
}

func (wallet *wallet) Expired(ctx context.Context, swap Swap) (bool, error) {
	details, err := wallet.swap.AtomicSwapOrders(&bind.CallOpts{Context: ctx}, swap.ID)
	if err != nil {
		return false, err
	}
	latest, err := swap.Client.BlockByNumber(ctx, nil)
	if err != nil {
		return false, err
	}
	return latest.Header().Number.Int64()-details.InitiatedAt.Int64() >= details.Expiry.Int64(), nil
}

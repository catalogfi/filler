package ethswap

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/big"

	"github.com/catalogfi/cobi/pkg/swap/ethswap/bindings"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Swap struct {
	ID         [32]byte
	Initiator  common.Address
	Redeemer   common.Address
	SecretHash common.Hash
	Amount     *big.Int
	Expiry     *big.Int
	Contract   common.Address

	// TODO : MIGHT WORTH CACHING SOME OF THE RESULTS
	secret []byte
}

func NewSwap(initiator, redeemer, contract common.Address, secretHash common.Hash, amount, expiry *big.Int) (*Swap, error) {
	id := sha256.Sum256(append(secretHash[:], common.BytesToHash(initiator.Bytes()).Bytes()...))

	return &Swap{
		ID:         id,
		Initiator:  initiator,
		Redeemer:   redeemer,
		SecretHash: secretHash,
		Amount:     amount,
		Expiry:     expiry,
		Contract:   contract,
	}, nil
}

func (swap *Swap) Initiated(ctx context.Context, client *ethclient.Client) (bool, error) {
	atomicSwap, err := bindings.NewAtomicSwap(swap.Contract, client)
	if err != nil {
		return false, err
	}
	details, err := atomicSwap.AtomicSwapOrders(&bind.CallOpts{Context: ctx}, swap.ID)
	if err != nil {
		return false, err
	}
	if details.InitiatedAt.Uint64() == 0 {
		return false, nil
	}
	return true, nil
}

func (swap *Swap) Redeemed(ctx context.Context, client *ethclient.Client) (bool, error) {
	if len(swap.secret) != 0 {
		return true, nil
	}

	// Check if the swap has been redeemed
	atomicSwap, err := bindings.NewAtomicSwap(swap.Contract, client)
	if err != nil {
		return false, err
	}
	details, err := atomicSwap.AtomicSwapOrders(&bind.CallOpts{Context: ctx}, swap.ID)
	if err != nil {
		return false, err
	}
	return details.IsFulfilled, err
}

func (swap *Swap) Secret(ctx context.Context, client *ethclient.Client, step uint64) ([]byte, error) {
	if len(swap.secret) != 0 {
		return swap.secret, nil
	}

	// Check if the swap has been redeemed
	atomicSwap, err := bindings.NewAtomicSwap(swap.Contract, client)
	if err != nil {
		return nil, err
	}
	details, err := atomicSwap.AtomicSwapOrders(&bind.CallOpts{Context: ctx}, swap.ID)
	if err != nil {
		return nil, err
	}
	if !details.IsFulfilled {
		return nil, fmt.Errorf("swap not redeemed")
	}

	start := details.InitiatedAt
	latestBlock, err := client.BlockByNumber(ctx, nil)
	if err != nil {
		return nil, err
	}
	latest := latestBlock.Number()
	if step == 0 {
		step = 500
	}
	expiry := big.NewInt(0).Add(details.InitiatedAt, details.Expiry)
	if latest.Cmp(expiry) == 1 {
		latest = expiry
	}

	for start.Cmp(latest) == -1 {
		end := start.Uint64() + step
		if end > latest.Uint64() {
			end = latest.Uint64()
		}
		opts := bind.FilterOpts{
			Start:   start.Uint64(),
			End:     &end,
			Context: ctx,
		}
		iter, err := atomicSwap.FilterRedeemed(&opts, [][32]byte{swap.ID}, [][32]byte{swap.SecretHash})
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

func (swap *Swap) Expired(ctx context.Context, client *ethclient.Client) (bool, error) {
	atomicSwap, err := bindings.NewAtomicSwap(swap.Contract, client)
	if err != nil {
		return false, err
	}

	details, err := atomicSwap.AtomicSwapOrders(&bind.CallOpts{Context: ctx}, swap.ID)
	if err != nil {
		return false, err
	}
	latest, err := client.BlockByNumber(ctx, nil)
	if err != nil {
		return false, err
	}
	return latest.Header().Number.Int64()-details.InitiatedAt.Int64() >= details.Expiry.Int64(), nil
}

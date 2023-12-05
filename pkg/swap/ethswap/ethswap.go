package ethswap

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"fmt"
	"math/big"
	"time"

	"github.com/catalogfi/cobi/pkg/swap/ethswap/bindings"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type Swap struct {
	Initiator       common.Address
	Redeemer        common.Address
	ContractAddress common.Address
	SecretHash      common.Hash
	Client          *ethclient.Client
	Amount          *big.Int
	Expiry          *big.Int

	ID         [32]byte
	atomicSwap *bindings.AtomicSwap
	token      *bindings.ERC20
	chainID    *big.Int
	secret     []byte
}

func NewSwap(initiator, redeemer, contract common.Address, client *ethclient.Client, secretHash common.Hash, amount, expiry *big.Int) (*Swap, error) {
	id := sha256.Sum256(append(secretHash[:], common.BytesToHash(initiator.Bytes()).Bytes()...))
	atomicSwap, err := bindings.NewAtomicSwap(contract, client)
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

	return &Swap{
		ID:              id,
		Initiator:       initiator,
		Redeemer:        redeemer,
		ContractAddress: contract,
		SecretHash:      secretHash,
		Client:          client,
		Amount:          amount,
		Expiry:          expiry,

		atomicSwap: atomicSwap,
		token:      erc20,
		chainID:    chainID,
	}, nil
}

func (swap *Swap) Initiated() (bool, error) {
	details, err := swap.atomicSwap.AtomicSwapOrders(&bind.CallOpts{}, swap.ID)
	if err != nil {
		return false, err
	}
	if details.InitiatedAt.Uint64() == 0 {
		return false, nil
	}
	return true, nil
}

func (swap *Swap) Initiate(ctx context.Context, key *ecdsa.PrivateKey) (string, error) {
	allowance, err := swap.token.Allowance(&bind.CallOpts{}, swap.Initiator, swap.ContractAddress)
	if err != nil {
		return "", err
	}
	transactor, err := bind.NewKeyedTransactorWithChainID(key, swap.chainID)
	if err != nil {
		return "", err
	}

	// Approve the allowance if it's not enough
	if allowance.Cmp(swap.Amount) < 0 {
		approveTx, err := swap.token.Approve(transactor, swap.ContractAddress, swap.Amount)
		if err != nil {
			return "", err
		}
		if _, err := bind.WaitMined(ctx, swap.Client, approveTx); err != nil {
			return "", err
		}
	}

	tx, err := swap.atomicSwap.Initiate(transactor, swap.Redeemer, swap.Expiry, swap.Amount, swap.SecretHash)
	if err != nil {
		return "", err
	}
	receipt, err := bind.WaitMined(ctx, swap.Client, tx)
	if err != nil {
		return "", err
	}
	return receipt.TxHash.String(), nil
}

func (swap *Swap) Redeem(ctx context.Context, key *ecdsa.PrivateKey, secret []byte) (string, error) {
	transactor, err := bind.NewKeyedTransactorWithChainID(key, swap.chainID)
	if err != nil {
		return "", err
	}

	tx, err := swap.atomicSwap.Redeem(transactor, swap.ID, secret)
	if err != nil {
		return "", err
	}
	receipt, err := bind.WaitMined(ctx, swap.Client, tx)
	if err != nil {
		return "", err
	}
	swap.secret = secret
	return receipt.TxHash.String(), nil
}

func (swap *Swap) Redeemed(ctx context.Context, step uint64) (bool, []byte, error) {
	if len(swap.secret) != 0 {
		return true, swap.secret, nil
	}
	details, err := swap.atomicSwap.AtomicSwapOrders(&bind.CallOpts{Context: ctx}, swap.ID)
	if err != nil {
		return false, nil, err
	}
	if !details.IsFulfilled {
		return false, nil, nil
	}

	start := details.InitiatedAt
	latest, err := swap.Client.BlockByNumber(ctx, nil)
	if err != nil {
		return false, nil, err
	}
	last := latest.Number()
	if step == 0 {
		step = 500
	}
	if last.Cmp(details.Expiry) == 1 {
		last = details.Expiry
	}

	for start.Cmp(last) == -1 {
		end := start.Uint64() + step
		opts := bind.FilterOpts{
			Start:   start.Uint64(),
			End:     &end,
			Context: ctx,
		}
		iter, err := swap.atomicSwap.FilterRedeemed(&opts, [][32]byte{swap.ID}, [][32]byte{swap.SecretHash})
		if err != nil {
			return false, nil, err
		}
		if iter.Error() != nil {
			return false, nil, iter.Error()
		}

		for iter.Next() {
			return true, iter.Event.Secret, nil
		}
		start = big.NewInt(int64(end))
	}

	return details.IsFulfilled, nil, fmt.Errorf("secret not found")
}

func (swap *Swap) Refund(ctx context.Context, key *ecdsa.PrivateKey) error {
	transactor, err := bind.NewKeyedTransactorWithChainID(key, swap.chainID)
	if err != nil {
		return err
	}
	tx, err := swap.atomicSwap.Refund(transactor, swap.ID)
	if err != nil {
		return err
	}
	_, err = bind.WaitMined(ctx, swap.Client, tx)
	return err
}

func (swap *Swap) Expired(ctx context.Context) (bool, error) {
	details, err := swap.atomicSwap.AtomicSwapOrders(&bind.CallOpts{Context: ctx}, swap.ID)
	if err != nil {
		return false, err
	}
	latest, err := swap.Client.BlockByNumber(ctx, nil)
	if err != nil {
		return false, err
	}
	return latest.Header().Number.Int64()-details.InitiatedAt.Int64() >= details.Expiry.Int64(), nil
}

package wallet

import (
	"context"

	"github.com/catalogfi/cobi/pkg/swap/btcswap"
	"github.com/catalogfi/cobi/pkg/swap/ethswap"
)

// 1. internally manage all utxos/nonces
// 2. manage the key
// 3. have instant wallet integration
// 4. should have a bitcoin client embeded
type BitcoinWallet interface {
	Balance(pending bool) (uint64, error)

	Address() string

	Initiate(ctx context.Context, swap btcswap.Swap) error

	Redeem(ctx context.Context, swap btcswap.Swap) error

	Refund(ctx context.Context, swap btcswap.Swap) error
}

type EthereumWallet interface {
	Address() string

	Balance(pending bool) (uint64, error)

	Initiate(ctx context.Context, swap ethswap.Swap) error

	Redeem(ctx context.Context, swap ethswap.Swap) error

	Refund(ctx context.Context, swap ethswap.Swap) error
}

type btcWallet struct {
	instantWalletOpts *config
}

func (wallet btcwallet) run(ctx context.Context) {
	if wallet.instantWalletOpts != nil {
		go func() {
			// watching deposit to master wallet and move it to instant address
		}()
	}
}

package swapper

import (
	"errors"
	"fmt"
)

var (
	ErrInitiateTimeout = errors.New("initiate timeout")
	ErrRedeemTimeout   = errors.New("redeem timeout")
)

type InitiatorSwap interface {
	Initiate() (string, error)
	WaitForRedeem() ([]byte, string, error)
	IsRedeemed() (bool, []byte, string, error)
	Refund() (string, error)
	Expired() (bool, error)
}

type RedeemerSwap interface {
	Redeem(secret []byte) (string, error)
	IsInitiated() (bool, []string, uint64, error)
	WaitForInitiate() ([]string, error)
}

type Watcher interface {
	Expired() (bool, error)
	IsInitiated() (bool, []string, uint64, error)
	IsRedeemed() (bool, []byte, string, error)
	IsRefunded() (bool, string, error)
}

func ExecuteAtomicSwapFirst(initiator InitiatorSwap, redeemer RedeemerSwap, secret []byte) error {
	if _, err := initiator.Initiate(); err != nil {
		return err
	}
	if _, err := redeemer.WaitForInitiate(); err != nil {
		if err == ErrInitiateTimeout {
			_, err = initiator.Refund()
			if err != nil {
				return err
			}
		}
		return err
	}
	if _, err := redeemer.Redeem(secret); err != nil {
		return err
	}
	return nil
}

func ExecuteAtomicSwapSecond(initiator InitiatorSwap, redeemer RedeemerSwap) error {
	fmt.Println("Waiting for Initiate on:", redeemer)
	if _, err := redeemer.WaitForInitiate(); err != nil {
		return err
	}
	fmt.Println("Initiating on:", redeemer)
	if _, err := initiator.Initiate(); err != nil {
		return err
	}
	secret, _, err := initiator.WaitForRedeem()
	if err != nil {
		return err
	}
	if secret != nil {
		if _, err := redeemer.Redeem(secret); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("failed to redeem : empty secret")
	}
	return nil
}

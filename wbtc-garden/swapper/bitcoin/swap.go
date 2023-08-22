package bitcoin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/catalogfi/cobi/wbtc-garden/swapper"
	"go.uber.org/zap"
)

type initiatorSwap struct {
	logger  *zap.Logger
	watcher swapper.Watcher
	client  Client

	initiator  *btcec.PrivateKey
	waitBlocks int64
	amount     uint64
	script     []byte
	scriptAddr btcutil.Address
}

// TODO : Naming is very confusing, Bob (Buy BTC/ Sell WBTC) needs to create a initiator swap for WBTC and a redeemer swap
// for btc ???
func NewInitiatorSwap(logger *zap.Logger, initiator *btcec.PrivateKey, redeemerAddr btcutil.Address, secretHash []byte, waitBlocks int64, minConfirmations, amount uint64, client Client) (swapper.InitiatorSwap, error) {
	initiatorAddr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(initiator.PubKey().SerializeCompressed()), client.Net())
	if err != nil {
		return nil, fmt.Errorf("failed to create initiator address: %w", err)
	}
	htlcScript, err := NewHTLCScript(initiatorAddr, redeemerAddr, secretHash, waitBlocks)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTLC script: %w", err)
	}

	witnessProgram := sha256.Sum256(htlcScript)
	scriptAddr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], client.Net())
	if err != nil {
		return nil, fmt.Errorf("failed to create script address: %w", err)
	}

	watcher, err := NewWatcher(scriptAddr, waitBlocks, minConfirmations, amount, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}
	childLogger := logger.With(zap.String("service", "initSwap"))
	return &initiatorSwap{
		logger:     childLogger,
		initiator:  initiator,
		script:     htlcScript,
		watcher:    watcher,
		scriptAddr: scriptAddr,
		amount:     amount,
		waitBlocks: waitBlocks,
		client:     client,
	}, nil
}

func (s *initiatorSwap) Initiate() (string, error) {
	txHash, err := s.client.Send(s.scriptAddr, s.amount, s.initiator)
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}
	s.logger.Info("Initiated", zap.String("txHash", txHash))
	return txHash, nil
}

func (s *initiatorSwap) Expired() (bool, error) {
	return s.watcher.Expired()
}

func (s *initiatorSwap) Refund() (string, error) {
	witness := NewHTLCRefundWitness(s.initiator.PubKey().SerializeCompressed())
	txHash, err := s.client.Spend(s.script, witness, s.initiator, uint(s.waitBlocks))
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}
	s.logger.Info("Refunded", zap.String("txHash", txHash))
	return txHash, nil
}

func (s *initiatorSwap) WaitForRedeem() ([]byte, string, error) {
	for {
		s.logger.Debug("Waiting for Redemption", zap.String("address", s.scriptAddr.EncodeAddress()))
		redeemed, secret, tx, err := s.IsRedeemed()
		if err != nil {
			time.Sleep(30 * time.Second)
			continue
		}

		if redeemed {
			s.logger.Info("Redeemed", zap.String("secret", hex.EncodeToString(secret)))
			return secret, tx, nil
		}
		time.Sleep(30 * time.Second)
	}
}

func (s *initiatorSwap) IsRedeemed() (bool, []byte, string, error) {
	return s.watcher.IsRedeemed()
}

type redeemerSwap struct {
	logger     *zap.Logger
	amount     uint64
	redeemer   *btcec.PrivateKey
	htlcScript []byte
	scriptAddr btcutil.Address
	watcher    swapper.Watcher
	client     Client
}

func NewRedeemerSwap(logger *zap.Logger, redeemer *btcec.PrivateKey, initiator btcutil.Address, secretHash []byte, waitBlocks int64, minConfirmations, amount uint64, client Client) (swapper.RedeemerSwap, error) {
	redeemerAddr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(redeemer.PubKey().SerializeCompressed()), client.Net())
	if err != nil {
		return nil, fmt.Errorf("failed to create redeemer address: %w", err)
	}

	htlcScript, err := NewHTLCScript(initiator, redeemerAddr, secretHash, waitBlocks)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTLC script: %w", err)
	}
	witnessProgram := sha256.Sum256(htlcScript)
	scriptAddr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], client.Net())
	if err != nil {
		return nil, fmt.Errorf("failed to create script address: %w", err)
	}

	watcher, err := NewWatcher(scriptAddr, waitBlocks, minConfirmations, amount, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}
	childLogger := logger.With(zap.String("service", "redeemSwap"))

	return &redeemerSwap{
		logger:     childLogger,
		redeemer:   redeemer,
		watcher:    watcher,
		htlcScript: htlcScript,
		scriptAddr: scriptAddr,
		amount:     amount,
		client:     client,
	}, nil
}

func (s *redeemerSwap) Redeem(secret []byte) (string, error) {
	script := NewHTLCRedeemWitness(s.redeemer.PubKey().SerializeCompressed(), secret)
	txHash, err := s.client.Spend(s.htlcScript, script, s.redeemer, 0)
	if err != nil {
		return "", fmt.Errorf("failed to send transaction: %w", err)
	}
	s.logger.Info("Redeemed", zap.String("txHash", txHash))
	return txHash, nil
}

func (s *redeemerSwap) WaitForInitiate() ([]string, error) {
	for {
		initiated, txHashes, _, err := s.IsInitiated()
		if initiated {
			return txHashes, nil
		}
		if err != nil {
			s.logger.Error("wait for initiation", zap.Error(err))
		}
		time.Sleep(30 * time.Second)
	}
}

func (s *redeemerSwap) IsInitiated() (bool, []string, uint64, error) {
	return s.watcher.IsInitiated()
}

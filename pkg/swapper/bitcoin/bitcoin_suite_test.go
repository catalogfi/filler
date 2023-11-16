package bitcoin_test

import (
	crand "crypto/rand"
	"encoding/hex"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBitcoin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bitcoin Suite")
}

var (
	PrivateKey1 = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"
	PrivateKey2 = "59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d"
)

func ParseKey(pkHex string, network *chaincfg.Params) (*btcec.PrivateKey, btcutil.Address, error) {
	pkBytes, err := hex.DecodeString(pkHex)
	if err != nil {
		return nil, nil, err
	}
	pk, _ := btcec.PrivKeyFromBytes(pkBytes)

	// addr, err := btcutil.NewAddressPubKeyHash(btcutil.Hash160(pk.PubKey().SerializeCompressed()), network)
	addr, err := btcutil.NewAddressWitnessPubKeyHash(btcutil.Hash160(pk.PubKey().SerializeCompressed()), network)
	if err != nil {
		return nil, nil, err
	}
	return pk, addr, nil
}

func NigiriFaucet(addr string) (string, error) {
	res, err := RunOutput("nigiri", "faucet", addr)
	if err != nil {
		return "", err
	}
	txid := strings.TrimSpace(strings.TrimPrefix(string(res), "txId:"))
	return txid, nil
}

func RunOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	return cmd.Output()
}

func RandomSecret() []byte {
	length := rand.Intn(32)
	data := make([]byte, length)

	_, err := crand.Read(data)
	if err != nil {
		panic(err)
	}
	return data
}

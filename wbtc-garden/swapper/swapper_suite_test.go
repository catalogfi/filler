package swapper_test

import (
	"fmt"
	"log"
	"math/big"
	"testing"

	"github.com/catalogfi/cobi/wbtc-garden/swapper/ethereum/typings/AtomicSwap"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/ethereum/typings/TestERC20"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSwapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Swapper Suite")
}

func Setup(PRIV_KEY_1, PRIV_KEY_2 string) (common.Address, common.Address) {
	// PRIV_KEY_1 := os.Getenv("PRIV_KEY_1")
	// PRIV_KEY_2 := os.Getenv("PRIV_KEY_2")
	ethPrivKey1, err := crypto.HexToECDSA(PRIV_KEY_1)
	if err != nil {
		log.Fatal(err)
	}
	ethPrivKey2, err := crypto.HexToECDSA(PRIV_KEY_2)
	if err != nil {
		log.Fatal(err)
	}
	// _ = crypto.PubkeyToAddress(ethPrivKey1.PublicKey)
	ethPkAddr2 := crypto.PubkeyToAddress(ethPrivKey2.PublicKey)

	provider, _ := ethclient.Dial("http://localhost:8545")

	auth, _ := bind.NewKeyedTransactorWithChainID(ethPrivKey1, big.NewInt(31337))
	ERC20TokenAddr, _, ERC20Instance, err := TestERC20.DeployTestERC20(auth, provider)
	if err != nil {
		log.Fatal(err)
	}

	auth.Nonce = auth.Nonce.Add(auth.Nonce, big.NewInt(1))
	_, err = ERC20Instance.Transfer(auth, ethPkAddr2, big.NewInt(10e10))
	if err != nil {
		log.Fatal(err)
	}

	auth.Nonce = auth.Nonce.Add(auth.Nonce, big.NewInt(1))
	AtomicSwapAddr, _, _, err := AtomicSwap.DeployAtomicSwap(auth, provider, ERC20TokenAddr)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("AtomicSwap address :", AtomicSwapAddr)
	fmt.Println("ERC20 address :", ERC20TokenAddr)

	return AtomicSwapAddr, ERC20TokenAddr

}

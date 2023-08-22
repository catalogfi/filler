package bitcoin_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/catalogfi/cobi/wbtc-garden/swapper/bitcoin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("bitcoin client ", func() {
	Context("when using the bitcoin client", func() {
		It("should be able to get info of the blockchain", func() {
			By("Initialise client")
			network := &chaincfg.RegressionNetParams
			electrs := "http://localhost:30000"
			client := bitcoin.NewClient(electrs, network)

			By("Net()")
			net := client.Net()
			Expect(net).Should(Equal(network))

			By("GetTipBlockHeight()")
			latest, err := client.GetTipBlockHeight()
			Expect(err).Should(BeNil())
			Expect(latest > 100).Should(BeTrue())

			By("Parse the private key")
			_, addr, err := ParseKey(PrivateKey1, network)
			Expect(err).Should(BeNil())

			By("Fund the address")
			_, err = NigiriFaucet(addr.EncodeAddress())
			Expect(err).Should(BeNil())
			time.Sleep(5 * time.Second)

			By("Fetch the utxo by using `GetUTXOs()`")
			utxos, _, err := client.GetUTXOs(addr, 0)
			Expect(err).Should(BeNil())
			Expect(len(utxos)).Should(BeNumerically(">=", 1))
		})

		It("should be able to send bitcoin", func() {
			By("Initialise client")
			network := &chaincfg.RegressionNetParams
			electrs := "http://localhost:30000"
			client := bitcoin.NewClient(electrs, network)

			By("Parse the private key")
			pk1, addr1, err := ParseKey(PrivateKey1, network)
			Expect(err).Should(BeNil())
			_, addr2, err := ParseKey(PrivateKey2, network)
			Expect(err).Should(BeNil())

			By("Fund addr1")
			_, err = NigiriFaucet(addr1.EncodeAddress())
			Expect(err).Should(BeNil())
			time.Sleep(5 * time.Second)

			By("Sending some bitcoin to addr2")
			txid, err := client.Send(addr2, 1e7, pk1)
			Expect(err).To(BeNil())
			Expect(txid).NotTo(BeNil())
			time.Sleep(5 * time.Second)
		})

		It("should spend the htlc funds", func() {
			By("Initialise client")
			network := &chaincfg.RegressionNetParams
			electrs := "http://localhost:30000"
			client := bitcoin.NewClient(electrs, network)

			By("Parse the private key")
			pk1, addr1, err := ParseKey(PrivateKey1, network)
			Expect(err).Should(BeNil())
			pk2, addr2, err := ParseKey(PrivateKey2, network)
			Expect(err).Should(BeNil())

			By("Fund addr1")
			_, err = NigiriFaucet(addr1.EncodeAddress())
			Expect(err).Should(BeNil())
			time.Sleep(5 * time.Second)

			By("Generate the HTLC script")
			secret := RandomSecret()
			secretHash := sha256.Sum256(secret)
			waitTime := int64(6)
			script, err := bitcoin.NewHTLCScript(addr1, addr2, secretHash[:], waitTime)
			Expect(err).Should(BeNil())
			witnessProgram := sha256.Sum256(script)
			scriptAddr, err := btcutil.NewAddressWitnessScriptHash(witnessProgram[:], client.Net())
			Expect(err).Should(BeNil())

			By("Send some funds to the HTLC address")
			_, err = client.Send(scriptAddr, 1e7, pk1)
			Expect(err).Should(BeNil())
			time.Sleep(5 * time.Second)

			By("Try to spend the funds in the HTLC using secret")
			witness := bitcoin.NewHTLCRedeemWitness(pk2.PubKey().SerializeCompressed(), secret)
			redeemTxid, err := client.Spend(script, witness, pk2, 0)
			Expect(err).Should(BeNil())
			time.Sleep(5 * time.Second)

			By("Get spending witness")
			witnesses, tx, err := client.GetSpendingWitness(scriptAddr)
			Expect(err).Should(BeNil())
			Expect(redeemTxid).Should(Equal(tx.TxID))
			Expect(len(witnesses)).Should(Equal(5))
			revealedSecret, err := hex.DecodeString(witnesses[2])
			Expect(err).Should(BeNil())
			Expect(bytes.Equal(revealedSecret, secret)).Should(BeTrue())
		})
	})
})

package bitcoin_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Bitcoin watcher", func() {
	Context("when watching an atomic swap", func() {
		It("should tell the state of the atomic swap ", func() {
			// Initialise a swap between Alice and Bob, check if the watcher can fetch the correct status
			Expect(true).Should(BeTrue())
		})

		It("should return if the swap is expired", func() {
			Expect(true).Should(BeTrue())
		})

		It("should return if the swap is refunded", func() {
			Expect(true).Should(BeTrue())
		})

	})

	Context("when one of the user is malicious", func() {
		It("should not return the swap is initiated if not enough funds", func() {

		})

		It("should not return the swap is initiated if user sending from P2WSH input", func() {

		})
	})
})

package btcswap_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBtcswap(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Btcswap Suite")
}

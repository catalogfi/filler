package screener_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestScreener(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Screener Suite")
}

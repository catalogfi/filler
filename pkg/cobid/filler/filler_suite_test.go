package filler_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFiller(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Filler Suite")
}

var _ = BeforeSuite(func() {
	_ = os.Remove("test.db")
})

var _ = AfterSuite(func() {
	_ = os.Remove("test.db")
})

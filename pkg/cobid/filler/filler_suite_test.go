package filler_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFiller(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Filler Suite")
}

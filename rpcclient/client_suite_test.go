package rpcclient_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRPCClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Rpc Client Suite")
}

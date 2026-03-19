package termwright_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTermwright(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Termwright Suite")
}

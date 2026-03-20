package mdutil_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMdutil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mdutil Suite")
}

package review_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestReview(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Review Suite")
}

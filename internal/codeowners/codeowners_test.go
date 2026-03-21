package codeowners_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jethrokuan/pry/internal/codeowners"
)

func TestCodeowners(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Codeowners Suite")
}

var _ = Describe("Codeowners", func() {
	var (
		tmpDir string
		coFile string
	)

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		coFile = filepath.Join(tmpDir, "CODEOWNERS")
	})

	writeFile := func(content string) {
		err := os.WriteFile(coFile, []byte(content), 0644)
		Expect(err).NotTo(HaveOccurred())
	}

	Describe("Parse", func() {
		It("parses basic rules", func() {
			writeFile("*.go @go-team\n/docs/ @docs-team\n")
			co, err := codeowners.Parse(coFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(co.Rules).To(HaveLen(2))
			Expect(co.Rules[0].Pattern).To(Equal("*.go"))
			Expect(co.Rules[0].Owners).To(Equal([]string{"@go-team"}))
		})

		It("skips comments and blank lines", func() {
			writeFile("# comment\n\n*.js @js-team\n")
			co, err := codeowners.Parse(coFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(co.Rules).To(HaveLen(1))
		})

		It("handles multiple owners", func() {
			writeFile("*.go @team-a @team-b @user1\n")
			co, err := codeowners.Parse(coFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(co.Rules[0].Owners).To(Equal([]string{"@team-a", "@team-b", "@user1"}))
		})
	})

	Describe("Owners", func() {
		It("returns last matching rule (last wins)", func() {
			writeFile("* @default\n*.go @go-team\n")
			co, err := codeowners.Parse(coFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(co.Owners("main.go")).To(Equal([]string{"@go-team"}))
			Expect(co.Owners("readme.md")).To(Equal([]string{"@default"}))
		})

		It("matches directory patterns", func() {
			writeFile("docs/ @docs-team\n")
			co, err := codeowners.Parse(coFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(co.Owners("docs/guide.md")).To(Equal([]string{"@docs-team"}))
			Expect(co.Owners("docs/api/ref.md")).To(Equal([]string{"@docs-team"}))
			Expect(co.Owners("src/main.go")).To(BeNil())
		})

		It("matches patterns with leading /", func() {
			writeFile("/build/ @build-team\n")
			co, err := codeowners.Parse(coFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(co.Owners("build/output.log")).To(Equal([]string{"@build-team"}))
		})

		It("matches ** patterns", func() {
			writeFile("src/**/*.go @go-team\n")
			co, err := codeowners.Parse(coFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(co.Owners("src/main.go")).To(Equal([]string{"@go-team"}))
			Expect(co.Owners("src/internal/deep/file.go")).To(Equal([]string{"@go-team"}))
			Expect(co.Owners("src/main.js")).To(BeNil())
		})
	})

	Describe("OwnedBy", func() {
		It("checks ownership case-insensitively", func() {
			writeFile("*.go @Org/Go-Team\n")
			co, err := codeowners.Parse(coFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(co.OwnedBy("main.go", "@org/go-team")).To(BeTrue())
			Expect(co.OwnedBy("main.go", "@Org/Go-Team")).To(BeTrue())
			Expect(co.OwnedBy("main.go", "@other")).To(BeFalse())
		})
	})

	Describe("OwnedByAny", func() {
		It("returns true when any candidate matches", func() {
			writeFile("*.go @go-team @backup-team\ndocs/ @docs-team\n")
			co, err := codeowners.Parse(coFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(co.OwnedByAny("main.go", []string{"@go-team"})).To(BeTrue())
			Expect(co.OwnedByAny("main.go", []string{"@other", "@backup-team"})).To(BeTrue())
			Expect(co.OwnedByAny("main.go", []string{"@docs-team"})).To(BeFalse())
		})

		It("matches case-insensitively", func() {
			writeFile("*.go @Org/Go-Team\n")
			co, err := codeowners.Parse(coFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(co.OwnedByAny("main.go", []string{"@org/go-team"})).To(BeTrue())
		})

		It("returns false when owners or candidates are empty", func() {
			writeFile("*.go @go-team\n")
			co, err := codeowners.Parse(coFile)
			Expect(err).NotTo(HaveOccurred())
			Expect(co.OwnedByAny("main.go", nil)).To(BeFalse())
			Expect(co.OwnedByAny("main.go", []string{})).To(BeFalse())
			Expect(co.OwnedByAny("unmatched.txt", []string{"@go-team"})).To(BeFalse())
		})
	})

	Describe("Find", func() {
		It("finds .github/CODEOWNERS", func() {
			ghDir := filepath.Join(tmpDir, ".github")
			Expect(os.MkdirAll(ghDir, 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(ghDir, "CODEOWNERS"), []byte("* @team\n"), 0644)).To(Succeed())

			co := codeowners.Find(tmpDir)
			Expect(co).NotTo(BeNil())
			Expect(co.Rules).To(HaveLen(1))
		})

		It("returns nil when no CODEOWNERS found", func() {
			co := codeowners.Find(tmpDir)
			Expect(co).To(BeNil())
		})
	})
})

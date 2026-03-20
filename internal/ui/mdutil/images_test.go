package mdutil_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jkuan/pr-review/internal/ui/mdutil"
)

var _ = Describe("ReplaceImages", func() {
	It("replaces standard markdown images", func() {
		input := "Check this: ![screenshot](https://example.com/img.png)"
		result := mdutil.ReplaceImages(input)
		Expect(result).To(Equal("Check this: [🖼 screenshot](https://example.com/img.png)"))
	})

	It("replaces images with no alt text", func() {
		input := "![](https://example.com/img.png)"
		result := mdutil.ReplaceImages(input)
		Expect(result).To(Equal("[🖼 image](https://example.com/img.png)"))
	})

	It("replaces images with title attribute", func() {
		input := `![alt](https://example.com/img.png "a title")`
		result := mdutil.ReplaceImages(input)
		Expect(result).To(Equal("[🖼 alt](https://example.com/img.png)"))
	})

	It("replaces linked images using the outer link URL", func() {
		input := "[![badge](https://img.shields.io/badge.svg)](https://example.com)"
		result := mdutil.ReplaceImages(input)
		Expect(result).To(Equal("[🖼 badge](https://example.com)"))
	})

	It("replaces HTML img tags", func() {
		input := `<img src="https://example.com/screenshot.png" alt="My Screenshot" />`
		result := mdutil.ReplaceImages(input)
		Expect(result).To(Equal("[🖼 My Screenshot](https://example.com/screenshot.png)"))
	})

	It("replaces HTML img tags without alt", func() {
		input := `<img src="https://example.com/img.png" />`
		result := mdutil.ReplaceImages(input)
		Expect(result).To(Equal("[🖼 image](https://example.com/img.png)"))
	})

	It("handles multiple images in one block", func() {
		input := "Before ![a](url1) middle ![b](url2) after"
		result := mdutil.ReplaceImages(input)
		Expect(result).To(Equal("Before [🖼 a](url1) middle [🖼 b](url2) after"))
	})

	It("strips p tags from GitHub HTML", func() {
		input := `<p><img src="https://example.com/img.png" alt="test" /></p>`
		result := mdutil.ReplaceImages(input)
		Expect(result).To(Equal("[🖼 test](https://example.com/img.png)"))
	})

	It("leaves text without images unchanged", func() {
		input := "Just a regular comment with [a link](url) and `code`."
		result := mdutil.ReplaceImages(input)
		Expect(result).To(Equal(input))
	})

	It("preserves surrounding markdown", func() {
		input := "## Header\n\nSome text\n\n![diagram](https://example.com/d.png)\n\nMore text"
		result := mdutil.ReplaceImages(input)
		Expect(result).To(Equal("## Header\n\nSome text\n\n[🖼 diagram](https://example.com/d.png)\n\nMore text"))
	})
})

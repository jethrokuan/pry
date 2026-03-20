// Package mdutil provides markdown pre-processing utilities.
package mdutil

import (
	"regexp"
	"strings"
)

// mdImage matches ![alt](url) and ![alt](url "title").
var mdImage = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)

// linkedImage matches [![alt](img-url)](link-url) — images wrapped in a link.
var linkedImage = regexp.MustCompile(`\[!\[([^\]]*)\]\([^)]+\)\]\(([^)]+)\)`)

// htmlImg matches <img ... src="url" ...> tags (self-closing or not).
var htmlImg = regexp.MustCompile(`<img\s[^>]*src=["']([^"']+)["'][^>]*/?>`)

// htmlImgAlt extracts the alt attribute from an img tag.
var htmlImgAlt = regexp.MustCompile(`alt=["']([^"']+)["']`)

// ReplaceImages pre-processes markdown text to convert image references
// into text placeholders with clickable links. This should be called
// before passing markdown to glamour for rendering.
//
// Transformations:
//   - [![alt](img)](link)  → [🖼 alt](link)
//   - ![alt](url)          → [🖼 alt](url)
//   - <img src="url" ...>  → [🖼 image](url)
func ReplaceImages(md string) string {
	// First handle linked images: [![alt](img-url)](link-url)
	// These should link to the outer URL, not the image URL.
	md = linkedImage.ReplaceAllStringFunc(md, func(match string) string {
		groups := linkedImage.FindStringSubmatch(match)
		alt := groups[1]
		linkURL := groups[2]
		if alt == "" {
			alt = "image"
		}
		return "[🖼 " + alt + "](" + linkURL + ")"
	})

	// Then handle standalone images: ![alt](url)
	md = mdImage.ReplaceAllStringFunc(md, func(match string) string {
		groups := mdImage.FindStringSubmatch(match)
		alt := groups[1]
		url := groups[2]
		if alt == "" {
			alt = "image"
		}
		return "[🖼 " + alt + "](" + url + ")"
	})

	// Handle HTML img tags
	md = htmlImg.ReplaceAllStringFunc(md, func(match string) string {
		srcGroups := htmlImg.FindStringSubmatch(match)
		url := srcGroups[1]

		alt := "image"
		if altGroups := htmlImgAlt.FindStringSubmatch(match); altGroups != nil {
			alt = altGroups[1]
		}

		return "[🖼 " + alt + "](" + url + ")"
	})

	// Clean up: GitHub sometimes wraps images in <p> tags
	md = strings.ReplaceAll(md, "<p>", "")
	md = strings.ReplaceAll(md, "</p>", "")

	return md
}

package site

import (
	"html"
	"strings"

	"github.com/nilszeilon/notesync/internal/markdown"
)

// ReplaceWikiLinks converts [[wiki-links]] to HTML anchor tags
// and ![[image]] embeds to <img> tags.
func ReplaceWikiLinks(content string) string {
	// First, replace image embeds ![[image.png]]
	content = markdown.ObsidianEmbedRe.ReplaceAllStringFunc(content, func(match string) string {
		inner := strings.TrimSpace(match[3 : len(match)-2]) // strip ![[  ]]

		// Handle ![[alt|image.png]] syntax
		alt := ""
		path := inner
		if idx := strings.Index(inner, "|"); idx != -1 {
			alt = strings.TrimSpace(inner[:idx])
			path = strings.TrimSpace(inner[idx+1:])
		}
		if alt == "" {
			alt = path
		}

		return `<img src="/images/` + html.EscapeString(path) + `" alt="` + html.EscapeString(alt) + `">`
	})

	// Then, replace note wikilinks [[link]]
	content = markdown.WikilinkRe.ReplaceAllStringFunc(content, func(match string) string {
		inner := match[2 : len(match)-2]
		display := inner
		target := inner

		if idx := strings.Index(inner, "|"); idx != -1 {
			display = inner[:idx]
			target = inner[idx+1:]
		}

		display = strings.TrimSpace(display)
		target = strings.TrimSpace(target)
		slug := markdown.Slugify(target)

		return `<a href="/` + slug + `">` + html.EscapeString(display) + `</a>`
	})

	return content
}

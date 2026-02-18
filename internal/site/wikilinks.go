package site

import (
	"regexp"
	"strings"
)

var wikilinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// ExtractWikiLinks returns all [[wiki-link]] targets from markdown content.
func ExtractWikiLinks(content string) []string {
	matches := wikilinkRe.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	var links []string
	for _, m := range matches {
		target := m[1]
		// Handle [[display|target]] syntax
		if idx := strings.Index(target, "|"); idx != -1 {
			target = target[idx+1:]
		}
		target = strings.TrimSpace(target)
		slug := Slugify(target)
		if !seen[slug] {
			seen[slug] = true
			links = append(links, slug)
		}
	}
	return links
}

// ReplaceWikiLinks converts [[wiki-links]] to HTML anchor tags.
func ReplaceWikiLinks(content string) string {
	return wikilinkRe.ReplaceAllStringFunc(content, func(match string) string {
		inner := match[2 : len(match)-2]
		display := inner
		target := inner

		if idx := strings.Index(inner, "|"); idx != -1 {
			display = inner[:idx]
			target = inner[idx+1:]
		}

		display = strings.TrimSpace(display)
		target = strings.TrimSpace(target)
		slug := Slugify(target)

		return `<a href="/` + slug + `">` + display + `</a>`
	})
}

// Slugify converts a note title to a URL-safe slug.
func Slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	// collapse multiple dashes
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	return s
}

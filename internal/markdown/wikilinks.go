package markdown

import (
	"regexp"
	"strings"
)

var WikilinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// ExtractWikiLinks returns all [[wiki-link]] targets from markdown content,
// excluding image embeds (![[image.png]]).
func ExtractWikiLinks(content string) []string {
	// Remove image embeds first so they aren't matched as wikilinks
	cleaned := ObsidianEmbedRe.ReplaceAllString(content, "")

	matches := WikilinkRe.FindAllStringSubmatch(cleaned, -1)
	seen := make(map[string]bool)
	var links []string
	for _, m := range matches {
		target := m[1]
		// Handle [[target|display]] syntax (Obsidian convention)
		if idx := strings.Index(target, "|"); idx != -1 {
			target = target[:idx]
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

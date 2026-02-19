package site

import (
	"html"
	"regexp"
	"strings"
)

var imageEmbedRe = regexp.MustCompile(`!\[\[([^\]]+)\]\]`)
var wikilinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

var imageExtSet = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".svg": true, ".webp": true,
}

// isImagePath returns true if the path has an image extension.
func isImagePath(path string) bool {
	ext := strings.ToLower(path)
	if idx := strings.LastIndex(ext, "."); idx != -1 {
		return imageExtSet[ext[idx:]]
	}
	return false
}

// ExtractWikiLinks returns all [[wiki-link]] targets from markdown content,
// excluding image embeds (![[image.png]]).
func ExtractWikiLinks(content string) []string {
	// Remove image embeds first so they aren't matched as wikilinks
	cleaned := imageEmbedRe.ReplaceAllString(content, "")

	matches := wikilinkRe.FindAllStringSubmatch(cleaned, -1)
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

// ReplaceWikiLinks converts [[wiki-links]] to HTML anchor tags
// and ![[image]] embeds to <img> tags.
func ReplaceWikiLinks(content string) string {
	// First, replace image embeds ![[image.png]]
	content = imageEmbedRe.ReplaceAllStringFunc(content, func(match string) string {
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
	content = wikilinkRe.ReplaceAllStringFunc(content, func(match string) string {
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

		return `<a href="/` + slug + `">` + html.EscapeString(display) + `</a>`
	})

	return content
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

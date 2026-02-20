package sync

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	obsidianImageRe = regexp.MustCompile(`!\[\[([^\]]+)\]\]`)
	mdImageRe       = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)
)

type frontmatter struct {
	Publish bool `yaml:"publish"`
}

// isPublished reads a markdown file and returns true if its YAML frontmatter
// contains publish: true.
func isPublished(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)

	// First line must be "---"
	if !scanner.Scan() || strings.TrimSpace(scanner.Text()) != "---" {
		return false
	}

	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return false
	}

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(strings.Join(lines, "\n")), &fm); err != nil {
		return false
	}
	return fm.Publish
}

// isImage returns true if the file extension is a supported image format.
func isImage(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp":
		return true
	}
	return false
}

// isMd returns true if the file has a .md extension.
func isMd(path string) bool {
	return strings.ToLower(filepath.Ext(path)) == ".md"
}

// extractImageRefs returns image filenames referenced in markdown content
// via Obsidian embeds ![[image.png]] and standard markdown ![alt](image.png).
func extractImageRefs(content string) []string {
	seen := make(map[string]bool)
	var refs []string

	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || !isImage(name) {
			return
		}
		// Normalise to basename — Obsidian always references by filename.
		base := filepath.Base(name)
		if !seen[base] {
			seen[base] = true
			refs = append(refs, base)
		}
	}

	// ![[image.png]] or ![[alt|image.png]]
	for _, m := range obsidianImageRe.FindAllStringSubmatch(content, -1) {
		inner := m[1]
		if idx := strings.Index(inner, "|"); idx != -1 {
			inner = inner[idx+1:]
		}
		add(inner)
	}

	// ![alt](image.png)
	for _, m := range mdImageRe.FindAllStringSubmatch(content, -1) {
		add(m[1])
	}

	return refs
}

// collectPublishedImageRefs walks dir and returns the set of image basenames
// referenced by published markdown files.
func collectPublishedImageRefs(dir string) map[string]bool {
	refs := make(map[string]bool)
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !isMd(path) {
			return nil
		}
		if !isPublished(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, img := range extractImageRefs(string(data)) {
			refs[img] = true
		}
		return nil
	})
	return refs
}

package sync

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
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

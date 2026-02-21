package markdown

import (
	"bufio"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Frontmatter struct {
	Title   string `yaml:"title"`
	Publish bool   `yaml:"publish"`
	Date    string `yaml:"date"`
}

// ParseFrontmatter splits markdown content into YAML frontmatter and body.
func ParseFrontmatter(content string) (Frontmatter, string) {
	var fm Frontmatter
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return fm, content
	}

	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		return fm, content
	}

	fmBlock := rest[:endIdx]
	body := rest[endIdx+4:] // skip \n---

	_ = yaml.Unmarshal([]byte(fmBlock), &fm)
	return fm, strings.TrimSpace(body)
}

// IsPublished reads a markdown file and returns true if its YAML frontmatter
// contains publish: true.
func IsPublished(path string) bool {
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

	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(strings.Join(lines, "\n")), &fm); err != nil {
		return false
	}
	return fm.Publish
}

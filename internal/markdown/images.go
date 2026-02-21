package markdown

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nilszeilon/notesync/internal/fileutil"
)

var (
	ObsidianEmbedRe = regexp.MustCompile(`!\[\[([^\]]+)\]\]`)
	mdImageRe       = regexp.MustCompile(`!\[[^\]]*\]\(([^)]+)\)`)
)

// ExtractImageRefs returns image filenames referenced in markdown content
// via Obsidian embeds ![[image.png]] and standard markdown ![alt](image.png).
func ExtractImageRefs(content string) []string {
	seen := make(map[string]bool)
	var refs []string

	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || !fileutil.IsImage(name) {
			return
		}
		base := filepath.Base(name)
		if !seen[base] {
			seen[base] = true
			refs = append(refs, base)
		}
	}

	// ![[image.png]] or ![[alt|image.png]]
	for _, m := range ObsidianEmbedRe.FindAllStringSubmatch(content, -1) {
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

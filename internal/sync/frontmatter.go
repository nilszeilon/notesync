package sync

import (
	"os"
	"path/filepath"

	"github.com/nilszeilon/notesync/internal/fileutil"
	"github.com/nilszeilon/notesync/internal/markdown"
)

// collectPublishedImageRefs walks dir and returns the set of image basenames
// referenced by published markdown files.
func collectPublishedImageRefs(dir string) map[string]bool {
	refs := make(map[string]bool)
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !fileutil.IsMd(path) {
			return nil
		}
		if !markdown.IsPublished(path) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, img := range markdown.ExtractImageRefs(string(data)) {
			refs[img] = true
		}
		return nil
	})
	return refs
}

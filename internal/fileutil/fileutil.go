package fileutil

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ImageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".svg": true, ".webp": true,
}

var SyncExts = map[string]bool{
	".md": true, ".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".svg": true, ".webp": true,
}

func IsImage(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ImageExts[ext]
}

func IsMd(path string) bool {
	return strings.ToLower(filepath.Ext(path)) == ".md"
}

func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

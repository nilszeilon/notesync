package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type FileInfo struct {
	Path string `json:"path"`
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

type Storage struct {
	mu      sync.RWMutex
	dataDir string
}

func New(dataDir string) (*Storage, error) {
	absDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve data dir: %w", err)
	}
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &Storage{dataDir: absDir}, nil
}

func (s *Storage) DataDir() string {
	return s.dataDir
}

// safePath resolves relPath under dataDir and verifies it doesn't escape.
func (s *Storage) safePath(relPath string) (string, error) {
	cleaned := filepath.Clean(relPath)
	full := filepath.Join(s.dataDir, cleaned)
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	if !strings.HasPrefix(abs, s.dataDir+string(filepath.Separator)) && abs != s.dataDir {
		return "", fmt.Errorf("path escapes data directory: %s", relPath)
	}
	return abs, nil
}

func (s *Storage) Put(relPath string, r io.Reader) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fullPath, err := s.safePath(relPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	// Write to temp file then rename for atomicity
	tmp, err := os.CreateTemp(filepath.Dir(fullPath), ".notesync-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("sync file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close file: %w", err)
	}

	if err := os.Rename(tmpPath, fullPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename file: %w", err)
	}
	return nil
}

func (s *Storage) Delete(relPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fullPath, err := s.safePath(relPath)
	if err != nil {
		return err
	}

	return os.Remove(fullPath)
}

func (s *Storage) List() ([]FileInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	files := []FileInfo{}
	err := filepath.Walk(s.dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(s.dataDir, path)
		if err != nil {
			return err
		}

		hash, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("hash %s: %w", relPath, err)
		}

		files = append(files, FileInfo{
			Path: relPath,
			Hash: hash,
			Size: info.Size(),
		})
		return nil
	})
	return files, err
}

func (s *Storage) FullPath(relPath string) (string, error) {
	return s.safePath(relPath)
}

func hashFile(path string) (string, error) {
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

func HashReader(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

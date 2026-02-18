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
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &Storage{dataDir: dataDir}, nil
}

func (s *Storage) DataDir() string {
	return s.dataDir
}

func (s *Storage) Put(relPath string, r io.Reader) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	relPath = filepath.Clean(relPath)
	if strings.Contains(relPath, "..") {
		return fmt.Errorf("invalid path: %s", relPath)
	}

	fullPath := filepath.Join(s.dataDir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func (s *Storage) Delete(relPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	relPath = filepath.Clean(relPath)
	if strings.Contains(relPath, "..") {
		return fmt.Errorf("invalid path: %s", relPath)
	}

	fullPath := filepath.Join(s.dataDir, relPath)
	return os.Remove(fullPath)
}

func (s *Storage) List() ([]FileInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var files []FileInfo
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

func (s *Storage) FullPath(relPath string) string {
	return filepath.Join(s.dataDir, filepath.Clean(relPath))
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

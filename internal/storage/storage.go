package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const TombstoneTTL = 30 * 24 * time.Hour

type Tombstone struct {
	Path      string    `json:"path"`
	DeletedAt time.Time `json:"deleted_at"`
}

type FileInfo struct {
	Path    string    `json:"path"`
	Hash    string    `json:"hash"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
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

	if err := os.Remove(fullPath); err != nil {
		return err
	}

	// Remove empty parent directories up to dataDir
	dir := filepath.Dir(fullPath)
	for dir != s.dataDir {
		if err := os.Remove(dir); err != nil {
			break // not empty or other error, stop
		}
		dir = filepath.Dir(dir)
	}
	return nil
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
			Path:    relPath,
			Hash:    hash,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
		return nil
	})
	return files, err
}

func (s *Storage) Get(relPath string) (io.ReadCloser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fullPath, err := s.safePath(relPath)
	if err != nil {
		return nil, err
	}
	return os.Open(fullPath)
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

// --- Tombstone CRUD ---

func (s *Storage) tombstonePath() string {
	return filepath.Join(s.dataDir, ".tombstones.json")
}

func (s *Storage) loadTombstones() ([]Tombstone, error) {
	data, err := os.ReadFile(s.tombstonePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ts []Tombstone
	if err := json.Unmarshal(data, &ts); err != nil {
		return nil, err
	}
	return ts, nil
}

func (s *Storage) saveTombstones(ts []Tombstone) error {
	data, err := json.Marshal(ts)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.tombstonePath()), ".tombstones-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, s.tombstonePath())
}

func (s *Storage) AddTombstone(relPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ts, err := s.loadTombstones()
	if err != nil {
		return fmt.Errorf("load tombstones: %w", err)
	}

	now := time.Now()
	found := false
	for i, t := range ts {
		if t.Path == relPath {
			ts[i].DeletedAt = now
			found = true
			break
		}
	}
	if !found {
		ts = append(ts, Tombstone{Path: relPath, DeletedAt: now})
	}

	return s.saveTombstones(ts)
}

func (s *Storage) ListTombstones() ([]Tombstone, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ts, err := s.loadTombstones()
	if err != nil {
		return nil, fmt.Errorf("load tombstones: %w", err)
	}

	cutoff := time.Now().Add(-TombstoneTTL)
	active := make([]Tombstone, 0, len(ts))
	for _, t := range ts {
		if t.DeletedAt.After(cutoff) {
			active = append(active, t)
		}
	}

	// Prune expired by saving only active
	if len(active) != len(ts) {
		if err := s.saveTombstones(active); err != nil {
			return nil, fmt.Errorf("prune tombstones: %w", err)
		}
	}

	return active, nil
}

func (s *Storage) RemoveTombstone(relPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ts, err := s.loadTombstones()
	if err != nil {
		return fmt.Errorf("load tombstones: %w", err)
	}

	filtered := make([]Tombstone, 0, len(ts))
	for _, t := range ts {
		if t.Path != relPath {
			filtered = append(filtered, t)
		}
	}

	if len(filtered) != len(ts) {
		return s.saveTombstones(filtered)
	}
	return nil
}

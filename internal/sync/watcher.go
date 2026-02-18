package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nilszeilon/notesync/internal/storage"
)

var syncExts = map[string]bool{
	".md": true, ".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".svg": true, ".webp": true,
}

type Watcher struct {
	dir    string
	client *Client
}

func NewWatcher(dir string, client *Client) *Watcher {
	return &Watcher{dir: dir, client: client}
}

// FullSync compares local files with remote and uploads diffs.
func (w *Watcher) FullSync() error {
	remote, err := w.client.ListRemote()
	if err != nil {
		return fmt.Errorf("list remote: %w", err)
	}

	remoteMap := make(map[string]storage.FileInfo)
	for _, f := range remote {
		remoteMap[f.Path] = f
	}

	localFiles := make(map[string]bool)

	// Walk local dir, upload new/changed files
	err = filepath.Walk(w.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !syncExts[ext] {
			return nil
		}

		relPath, _ := filepath.Rel(w.dir, path)
		localFiles[relPath] = true

		localHash, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("hash local file %s: %w", relPath, err)
		}

		rf, exists := remoteMap[relPath]
		if !exists || rf.Hash != localHash {
			log.Printf("uploading: %s", relPath)
			if err := w.client.Upload(relPath, path); err != nil {
				return fmt.Errorf("upload %s: %w", relPath, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Delete remote files that don't exist locally
	for _, rf := range remote {
		if !localFiles[rf.Path] {
			log.Printf("deleting remote: %s", rf.Path)
			if err := w.client.Delete(rf.Path); err != nil {
				log.Printf("delete remote %s: %v", rf.Path, err)
			}
		}
	}

	return nil
}

// Watch starts watching for file changes and syncs them.
func (w *Watcher) Watch() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()

	// Add all directories recursively
	err = filepath.Walk(w.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("add watch paths: %w", err)
	}

	log.Printf("watching %s for changes...", w.dir)

	// Debounce events
	debounce := make(map[string]time.Time)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(event.Name))
			if !syncExts[ext] {
				continue
			}

			// Debounce: skip if we processed this file very recently
			if last, ok := debounce[event.Name]; ok && time.Since(last) < 500*time.Millisecond {
				continue
			}
			debounce[event.Name] = time.Now()

			relPath, err := filepath.Rel(w.dir, event.Name)
			if err != nil {
				log.Printf("rel path error: %v", err)
				continue
			}

			switch {
			case event.Op&(fsnotify.Create|fsnotify.Write) != 0:
				// Small delay to let the file finish writing
				time.Sleep(100 * time.Millisecond)
				if _, err := os.Stat(event.Name); err != nil {
					continue // file was deleted quickly
				}
				log.Printf("syncing: %s", relPath)
				if err := w.client.Upload(relPath, event.Name); err != nil {
					log.Printf("upload error: %v", err)
				}

			case event.Op&(fsnotify.Remove|fsnotify.Rename) != 0:
				// Editors often save via rename; wait briefly then check if file reappeared
				time.Sleep(200 * time.Millisecond)
				if _, err := os.Stat(event.Name); err == nil {
					// File still exists (editor rename-save), treat as update
					log.Printf("syncing: %s", relPath)
					if err := w.client.Upload(relPath, event.Name); err != nil {
						log.Printf("upload error: %v", err)
					}
				} else {
					log.Printf("deleting: %s", relPath)
					if err := w.client.Delete(relPath); err != nil {
						log.Printf("delete error: %v", err)
					}
				}
			}

			// Also watch new directories
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					watcher.Add(event.Name)
				}
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			log.Printf("watcher error: %v", err)
		}
	}
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

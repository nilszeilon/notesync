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
	dir           string
	client        *Client
	publishClient *Client
}

func NewWatcher(dir string, client *Client, publishClient *Client) *Watcher {
	return &Watcher{dir: dir, client: client, publishClient: publishClient}
}

// FullSync compares local files with remote and uploads diffs.
func (w *Watcher) FullSync() error {
	// Sync all files to private client
	if w.client != nil {
		if err := w.fullSyncClient(w.client, nil); err != nil {
			return fmt.Errorf("full sync (private): %w", err)
		}
	}

	// Sync published files + images to publish client
	if w.publishClient != nil {
		shouldSync := func(relPath, absPath string) bool {
			return isImage(relPath) || (isMd(relPath) && isPublished(absPath))
		}
		if err := w.fullSyncClient(w.publishClient, shouldSync); err != nil {
			return fmt.Errorf("full sync (publish): %w", err)
		}
	}

	return nil
}

// fullSyncClient syncs files to a single client. If filter is nil, all syncable
// files are synced. If filter is set, only files where filter returns true are
// synced; the rest are deleted from remote.
func (w *Watcher) fullSyncClient(c *Client, filter func(relPath, absPath string) bool) error {
	remote, err := c.ListRemote()
	if err != nil {
		return fmt.Errorf("list remote: %w", err)
	}

	remoteMap := make(map[string]storage.FileInfo)
	for _, f := range remote {
		remoteMap[f.Path] = f
	}

	localFiles := make(map[string]bool)

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

		if filter != nil && !filter(relPath, path) {
			return nil
		}

		localFiles[relPath] = true

		localHash, err := hashFile(path)
		if err != nil {
			return fmt.Errorf("hash local file %s: %w", relPath, err)
		}

		rf, exists := remoteMap[relPath]
		if !exists || rf.Hash != localHash {
			log.Printf("uploading: %s", relPath)
			if err := c.Upload(relPath, path); err != nil {
				return fmt.Errorf("upload %s: %w", relPath, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Delete remote files that don't exist locally (or don't pass the filter)
	for _, rf := range remote {
		if !localFiles[rf.Path] {
			log.Printf("deleting remote: %s", rf.Path)
			if err := c.Delete(rf.Path); err != nil {
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
				w.handleWrite(relPath, event.Name)

			case event.Op&(fsnotify.Remove|fsnotify.Rename) != 0:
				// Editors often save via rename; wait briefly then check if file reappeared
				time.Sleep(200 * time.Millisecond)
				if _, err := os.Stat(event.Name); err == nil {
					// File still exists (editor rename-save), treat as update
					w.handleWrite(relPath, event.Name)
				} else {
					w.handleDelete(relPath)
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

func (w *Watcher) handleWrite(relPath, absPath string) {
	// Always upload to private client
	if w.client != nil {
		log.Printf("syncing: %s", relPath)
		if err := w.client.Upload(relPath, absPath); err != nil {
			log.Printf("upload error: %v", err)
		}
	}

	// Publish client: upload if published or image, delete if unpublished md
	if w.publishClient != nil {
		if isImage(relPath) || (isMd(relPath) && isPublished(absPath)) {
			log.Printf("syncing (publish): %s", relPath)
			if err := w.publishClient.Upload(relPath, absPath); err != nil {
				log.Printf("publish upload error: %v", err)
			}
		} else if isMd(relPath) {
			// Markdown file that is not published — remove from publish server
			log.Printf("removing unpublished from publish server: %s", relPath)
			if err := w.publishClient.Delete(relPath); err != nil {
				log.Printf("publish delete error: %v", err)
			}
		}
	}
}

func (w *Watcher) handleDelete(relPath string) {
	if w.client != nil {
		log.Printf("deleting: %s", relPath)
		if err := w.client.Delete(relPath); err != nil {
			log.Printf("delete error: %v", err)
		}
	}
	if w.publishClient != nil {
		log.Printf("deleting (publish): %s", relPath)
		if err := w.publishClient.Delete(relPath); err != nil {
			log.Printf("publish delete error: %v", err)
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

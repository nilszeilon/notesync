package sync

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nilszeilon/notesync/internal/fileutil"
	"github.com/nilszeilon/notesync/internal/markdown"
	"github.com/nilszeilon/notesync/internal/storage"
)

type Watcher struct {
	dir           string
	client        *Client
	publishClient *Client
	pushOnly      bool
	pollInterval  time.Duration
}

func NewWatcher(dir string, client *Client, publishClient *Client, pushOnly bool, pollInterval time.Duration) *Watcher {
	return &Watcher{dir: dir, client: client, publishClient: publishClient, pushOnly: pushOnly, pollInterval: pollInterval}
}

// FullSync compares local files with remote and uploads diffs.
func (w *Watcher) FullSync() error {
	// Sync all files to private client
	if w.client != nil {
		if err := w.fullSyncClient(w.client, nil); err != nil {
			return fmt.Errorf("full sync (private): %w", err)
		}
	}

	// Sync published files + referenced images to publish client
	if w.publishClient != nil {
		referencedImages := collectPublishedImageRefs(w.dir)
		shouldSync := func(relPath, absPath string) bool {
			if fileutil.IsMd(relPath) {
				return markdown.IsPublished(absPath)
			}
			if fileutil.IsImage(relPath) {
				return referencedImages[filepath.Base(relPath)]
			}
			return false
		}
		if err := w.fullSyncClient(w.publishClient, shouldSync); err != nil {
			return fmt.Errorf("full sync (publish): %w", err)
		}
	}

	return nil
}

// fullSyncClient syncs files with a single client. If filter is nil (private
// client), sync is bidirectional: local files are pushed, remote-only files are
// pulled, and conflicts are resolved by most recent modification time. If filter
// is set (publish client), sync is one-way push with remote deletions for files
// that no longer pass the filter.
func (w *Watcher) fullSyncClient(c *Client, filter func(relPath, absPath string) bool) error {
	remote, err := c.ListRemote()
	if err != nil {
		return fmt.Errorf("list remote: %w", err)
	}

	remoteMap := make(map[string]storage.FileInfo)
	for _, f := range remote {
		remoteMap[f.Path] = f
	}

	// For private client, fetch tombstones to handle remote deletions
	var tombstoneMap map[string]storage.Tombstone
	if filter == nil {
		tombstones, err := c.ListTombstones()
		if err != nil {
			log.Printf("warning: failed to list tombstones: %v", err)
		} else {
			tombstoneMap = make(map[string]storage.Tombstone, len(tombstones))
			for _, t := range tombstones {
				tombstoneMap[t.Path] = t
			}
		}
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
		if !fileutil.SyncExts[ext] {
			return nil
		}

		relPath, _ := filepath.Rel(w.dir, path)

		if filter != nil && !filter(relPath, path) {
			return nil
		}

		localFiles[relPath] = true

		localHash, err := fileutil.HashFile(path)
		if err != nil {
			return fmt.Errorf("hash local file %s: %w", relPath, err)
		}

		rf, exists := remoteMap[relPath]
		if !exists {
			// Not on remote — check tombstones for private client
			if filter == nil && tombstoneMap != nil {
				if ts, hasTombstone := tombstoneMap[relPath]; hasTombstone {
					if ts.DeletedAt.After(info.ModTime()) {
						// Deleted remotely after local modtime — delete locally
						log.Printf("deleting (tombstone): %s", relPath)
						if err := os.Remove(path); err != nil {
							log.Printf("delete local %s: %v", relPath, err)
						}
						// Remove empty parent directories up to sync dir
						dir := filepath.Dir(path)
						for dir != w.dir {
							if err := os.Remove(dir); err != nil {
								break
							}
							dir = filepath.Dir(dir)
						}
						return nil
					}
					// Local file recreated after deletion — upload
					log.Printf("uploading (recreated after tombstone): %s", relPath)
					if err := c.Upload(relPath, path); err != nil {
						return fmt.Errorf("upload %s: %w", relPath, err)
					}
					return nil
				}
			}
			// No tombstone — new file, upload
			log.Printf("uploading: %s", relPath)
			if err := c.Upload(relPath, path); err != nil {
				return fmt.Errorf("upload %s: %w", relPath, err)
			}
		} else if rf.Hash != localHash {
			if filter != nil {
				// Publish client: always upload local
				log.Printf("uploading: %s", relPath)
				if err := c.Upload(relPath, path); err != nil {
					return fmt.Errorf("upload %s: %w", relPath, err)
				}
			} else {
				// Private client: resolve conflict by modtime
				localModTime := info.ModTime()
				if localModTime.After(rf.ModTime) {
					log.Printf("uploading (local newer): %s", relPath)
					if err := c.Upload(relPath, path); err != nil {
						return fmt.Errorf("upload %s: %w", relPath, err)
					}
				} else {
					log.Printf("downloading (remote newer): %s", relPath)
					localPath := filepath.Join(w.dir, relPath)
					if err := c.Download(relPath, localPath); err != nil {
						return fmt.Errorf("download %s: %w", relPath, err)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if filter != nil {
		// Publish client: delete remote files that don't exist locally (or don't pass filter)
		for _, rf := range remote {
			if !localFiles[rf.Path] {
				log.Printf("deleting remote: %s", rf.Path)
				if err := c.Delete(rf.Path); err != nil {
					log.Printf("delete remote %s: %v", rf.Path, err)
				}
			}
		}
	} else if !w.pushOnly {
		// Private client: download remote files not present locally
		for _, rf := range remote {
			if !localFiles[rf.Path] {
				ext := strings.ToLower(filepath.Ext(rf.Path))
				if !fileutil.SyncExts[ext] {
					continue
				}
				log.Printf("downloading (new remote): %s", rf.Path)
				localPath := filepath.Join(w.dir, rf.Path)
				if err := c.Download(rf.Path, localPath); err != nil {
					log.Printf("download %s: %v", rf.Path, err)
				}
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

	// Periodic remote poll for changes from other clients
	var pollChan <-chan time.Time
	if w.pollInterval > 0 && !w.pushOnly {
		ticker := time.NewTicker(w.pollInterval)
		defer ticker.Stop()
		pollChan = ticker.C
		log.Printf("polling remote every %s for new files", w.pollInterval)
	}

	// Debounce events
	debounce := make(map[string]time.Time)

	for {
		select {
		case <-pollChan:
			log.Println("polling remote for changes...")
			if err := w.FullSync(); err != nil {
				log.Printf("poll sync error: %v", err)
			}

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(event.Name))
			isSyncable := fileutil.SyncExts[ext]

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
				if !isSyncable {
					continue
				}
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
					if isSyncable {
						w.handleWrite(relPath, event.Name)
					}
				} else if isSyncable {
					w.handleDelete(relPath)
				} else {
					// No extension or non-syncable — likely a directory deletion.
					// Delete all remote files under this prefix.
					w.handleDirDelete(relPath)
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

	// Publish client: upload if published md (+ its images), or referenced image
	if w.publishClient != nil {
		if fileutil.IsMd(relPath) && markdown.IsPublished(absPath) {
			log.Printf("syncing (publish): %s", relPath)
			if err := w.publishClient.Upload(relPath, absPath); err != nil {
				log.Printf("publish upload error: %v", err)
			}
			// Also sync any images referenced by this published file
			w.syncReferencedImages(absPath)
		} else if fileutil.IsMd(relPath) {
			// Markdown file that is not published — remove from publish server
			log.Printf("removing unpublished from publish server: %s", relPath)
			if err := w.publishClient.Delete(relPath); err != nil {
				log.Printf("publish delete error: %v", err)
			}
		} else if fileutil.IsImage(relPath) {
			// Image changed — upload only if referenced by any published file
			refs := collectPublishedImageRefs(w.dir)
			if refs[filepath.Base(relPath)] {
				log.Printf("syncing (publish, referenced image): %s", relPath)
				if err := w.publishClient.Upload(relPath, absPath); err != nil {
					log.Printf("publish upload error: %v", err)
				}
			}
		}
	}
}

// syncReferencedImages reads a published markdown file, finds its image
// references, and uploads any matching local images to the publish server.
func (w *Watcher) syncReferencedImages(absPath string) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return
	}
	refs := markdown.ExtractImageRefs(string(data))
	if len(refs) == 0 {
		return
	}
	refSet := make(map[string]bool, len(refs))
	for _, r := range refs {
		refSet[r] = true
	}
	// Walk the sync dir for matching images
	filepath.Walk(w.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !fileutil.IsImage(path) {
			return nil
		}
		if !refSet[filepath.Base(path)] {
			return nil
		}
		relPath, _ := filepath.Rel(w.dir, path)
		log.Printf("syncing (publish, image for published note): %s", relPath)
		if err := w.publishClient.Upload(relPath, path); err != nil {
			log.Printf("publish image upload error: %v", err)
		}
		return nil
	})
}

func (w *Watcher) handleDirDelete(relPrefix string) {
	deleteFromClient := func(c *Client, label string) {
		remote, err := c.ListRemote()
		if err != nil {
			log.Printf("dir delete list (%s): %v", label, err)
			return
		}
		prefix := relPrefix + "/"
		for _, rf := range remote {
			if rf.Path == relPrefix || strings.HasPrefix(rf.Path, prefix) {
				log.Printf("deleting (%s dir removal): %s", label, rf.Path)
				if err := c.Delete(rf.Path); err != nil {
					log.Printf("delete %s (%s): %v", rf.Path, label, err)
				}
			}
		}
	}
	if w.client != nil {
		deleteFromClient(w.client, "private")
	}
	if w.publishClient != nil {
		deleteFromClient(w.publishClient, "publish")
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


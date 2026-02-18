package main

import (
	"flag"
	"log"
	"os"

	"github.com/nilszeilon/notesync/internal/sync"
)

func main() {
	dir := flag.String("dir", ".", "local notes directory to watch")
	server := flag.String("server", "http://localhost:8080", "server URL")
	flag.Parse()

	token := os.Getenv("NOTESYNC_TOKEN")

	client := sync.NewClient(*server, token)
	watcher := sync.NewWatcher(*dir, client)

	// Full sync on startup
	log.Println("performing full sync...")
	if err := watcher.FullSync(); err != nil {
		log.Fatalf("full sync failed: %v", err)
	}
	log.Println("full sync complete")

	// Watch for changes
	if err := watcher.Watch(); err != nil {
		log.Fatalf("watcher error: %v", err)
	}
}

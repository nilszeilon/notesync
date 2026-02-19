package main

import (
	"flag"
	"log"
	"os"

	"github.com/nilszeilon/notesync/internal/sync"
)

func main() {
	dir := flag.String("dir", ".", "local notes directory to watch")
	server := flag.String("server", "", "private server URL (syncs all files)")
	publishServer := flag.String("publish-server", "", "publish server URL (syncs published files only)")
	pushOnly := flag.Bool("push-only", false, "only push local files, don't download new remote files (still syncs updates to existing local files)")
	flag.Parse()

	if *server == "" && *publishServer == "" {
		log.Fatal("at least one of -server or -publish-server must be set")
	}

	var client *sync.Client
	if *server != "" {
		token := os.Getenv("NOTESYNC_TOKEN")
		client = sync.NewClient(*server, token)
	}

	var publishClient *sync.Client
	if *publishServer != "" {
		publishToken := os.Getenv("NOTESYNC_PUBLISH_TOKEN")
		publishClient = sync.NewClient(*publishServer, publishToken)
	}

	watcher := sync.NewWatcher(*dir, client, publishClient, *pushOnly)

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

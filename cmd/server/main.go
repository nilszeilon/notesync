package main

import (
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	notesync "github.com/nilszeilon/notesync"
	"github.com/nilszeilon/notesync/internal/api"
	"github.com/nilszeilon/notesync/internal/site"
	"github.com/nilszeilon/notesync/internal/storage"
)

func main() {
	port := flag.String("port", "8080", "server port")
	dataDir := flag.String("data", "./data", "data directory for stored files")
	siteDir := flag.String("site", "./_site", "output directory for generated site")
	flag.Parse()

	// Load embedded templates
	templateSub, err := fs.Sub(notesync.TemplateFS, "templates")
	if err != nil {
		log.Fatalf("template fs: %v", err)
	}
	if err := site.LoadTemplates(templateSub); err != nil {
		log.Fatalf("load templates: %v", err)
	}

	// Token from env
	token := os.Getenv("NOTESYNC_TOKEN")
	if token == "" {
		log.Println("warning: NOTESYNC_TOKEN not set, API is unauthenticated")
	}

	// Initialize storage
	store, err := storage.New(*dataDir)
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}

	// Initialize site builder
	absDataDir, _ := filepath.Abs(*dataDir)
	absSiteDir, _ := filepath.Abs(*siteDir)
	builder := site.NewBuilder(absDataDir, absSiteDir)

	// Initial site build
	if err := builder.Build(); err != nil {
		log.Printf("initial site build: %v", err)
	}

	// Set up HTTP routes
	mux := http.NewServeMux()

	// API routes
	handler := api.NewHandler(store, builder, token)
	handler.RegisterRoutes(mux)

	// Static site serving
	mux.Handle("/", http.FileServer(http.Dir(absSiteDir)))

	addr := ":" + *port
	log.Printf("server starting on %s", addr)
	log.Printf("data dir: %s", absDataDir)
	log.Printf("site dir: %s", absSiteDir)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

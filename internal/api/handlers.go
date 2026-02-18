package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/nilszeilon/notesync/internal/site"
	"github.com/nilszeilon/notesync/internal/storage"
)

type Handler struct {
	store   *storage.Storage
	builder *site.Builder
	token   string
}

func NewHandler(store *storage.Storage, builder *site.Builder, token string) *Handler {
	return &Handler{store: store, builder: builder, token: token}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/files/", h.authMiddleware(h.handleFiles))
	mux.HandleFunc("/api/files", h.authMiddleware(h.handleListFiles))
}

func (h *Handler) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.token != "" {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || auth[7:] != h.token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func (h *Handler) handleListFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	files, err := h.store.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func (h *Handler) handleFiles(w http.ResponseWriter, r *http.Request) {
	filePath := strings.TrimPrefix(r.URL.Path, "/api/files/")
	if filePath == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodPut:
		if err := h.store.Put(filePath, r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		h.rebuild()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))

	case http.MethodDelete:
		if err := h.store.Delete(filePath); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		h.rebuild()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) rebuild() {
	if err := h.builder.Build(); err != nil {
		log.Printf("site build error: %v", err)
	} else {
		log.Println("site rebuilt successfully")
	}
}

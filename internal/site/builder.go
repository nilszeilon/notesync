package site

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
	"gopkg.in/yaml.v3"
)

type Frontmatter struct {
	Title   string `yaml:"title"`
	Publish bool   `yaml:"publish"`
	Date    string `yaml:"date"`
}

type Note struct {
	Frontmatter
	Slug     string
	Body     string // markdown body without frontmatter
	FilePath string // relative path in storage
	ModTime  time.Time
}

type Builder struct {
	mu      sync.Mutex
	dataDir string
	outDir  string
	md      goldmark.Markdown
}

func NewBuilder(dataDir, outDir string) *Builder {
	return &Builder{
		dataDir: dataDir,
		outDir:  outDir,
		md: goldmark.New(
			goldmark.WithRendererOptions(
				html.WithUnsafe(),
			),
		),
	}
}

func (b *Builder) Build() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Clean output directory contents (but not the dir itself, which may be a mount point)
	entries, err := os.ReadDir(b.outDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read output dir: %w", err)
	}
	for _, e := range entries {
		os.RemoveAll(filepath.Join(b.outDir, e.Name()))
	}
	if err := os.MkdirAll(b.outDir, 0755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	// Collect all notes
	notes, err := b.collectNotes()
	if err != nil {
		return fmt.Errorf("collect notes: %w", err)
	}

	// Filter to published notes, separate out index note
	var published []Note
	var indexNote *Note
	for _, n := range notes {
		if n.Slug == "index" {
			n := n // copy
			indexNote = &n
			continue
		}
		if n.Publish {
			published = append(published, n)
		}
	}

	// Sort by date descending
	sort.Slice(published, func(i, j int) bool {
		di := published[i].parsedDate()
		dj := published[j].parsedDate()
		return di.After(dj)
	})

	// Build backlink index (include index note if present)
	allPublished := published
	if indexNote != nil {
		allPublished = append(allPublished, *indexNote)
	}
	backlinks := buildBacklinks(allPublished)

	// Build slug->note index
	slugIndex := make(map[string]Note)
	for _, n := range allPublished {
		slugIndex[n.Slug] = n
	}

	// Generate note pages
	for _, n := range published {
		if err := b.buildNotePage(n, backlinks[n.Slug], slugIndex, b.outDir); err != nil {
			return fmt.Errorf("build page %s: %w", n.Slug, err)
		}
	}

	// Generate index page: use index.md if it exists, otherwise auto-generate listing
	if indexNote != nil {
		if err := b.buildIndexFromNote(*indexNote, backlinks[indexNote.Slug], slugIndex); err != nil {
			return fmt.Errorf("build index from note: %w", err)
		}
	} else {
		if err := b.buildIndex(published); err != nil {
			return fmt.Errorf("build index: %w", err)
		}
	}

	// Copy style.css
	if err := os.WriteFile(filepath.Join(b.outDir, "style.css"), StyleCSS, 0644); err != nil {
		return fmt.Errorf("write css: %w", err)
	}

	// Copy images
	if err := b.copyImages(); err != nil {
		return fmt.Errorf("copy images: %w", err)
	}

	// Generate search index JSON
	if err := b.buildSearchIndex(published); err != nil {
		return fmt.Errorf("build search index: %w", err)
	}

	return nil
}

func (b *Builder) collectNotes() ([]Note, error) {
	var notes []Note
	err := filepath.Walk(b.dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		relPath, _ := filepath.Rel(b.dataDir, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		fm, body := parseFrontmatter(string(data))
		// Preserve folder structure in slug: "projects/foo.md" → "projects/foo"
		slugBase := strings.TrimSuffix(relPath, filepath.Ext(relPath))
		parts := strings.Split(filepath.ToSlash(slugBase), "/")
		for i, p := range parts {
			parts[i] = Slugify(p)
		}
		slug := strings.Join(parts, "/")

		if fm.Title == "" {
			fm.Title = strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
		}

		notes = append(notes, Note{
			Frontmatter: fm,
			Slug:        slug,
			Body:        body,
			FilePath:    relPath,
			ModTime:     info.ModTime(),
		})
		return nil
	})
	return notes, err
}

func parseFrontmatter(content string) (Frontmatter, string) {
	var fm Frontmatter
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return fm, content
	}

	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx == -1 {
		return fm, content
	}

	fmBlock := rest[:endIdx]
	body := rest[endIdx+4:] // skip \n---

	_ = yaml.Unmarshal([]byte(fmBlock), &fm)
	return fm, strings.TrimSpace(body)
}

func (n Note) parsedDate() time.Time {
	if n.Date != "" {
		t, err := time.Parse("2006-01-02", n.Date)
		if err == nil {
			return t
		}
	}
	return n.ModTime
}

func (n Note) dateString() string {
	d := n.parsedDate()
	return d.Format("2006-01-02")
}

func buildBacklinks(notes []Note) map[string][]string {
	// slug -> list of slugs that link to it
	backlinks := make(map[string][]string)
	for _, n := range notes {
		links := ExtractWikiLinks(n.Body)
		for _, target := range links {
			backlinks[target] = append(backlinks[target], n.Slug)
		}
	}
	return backlinks
}

func (b *Builder) buildNotePage(n Note, backlinkSlugs []string, slugIndex map[string]Note, notesDir string) error {
	// Convert wikilinks in markdown before rendering
	bodyWithLinks := ReplaceWikiLinks(n.Body)

	// Render markdown to HTML
	var htmlBuf bytes.Buffer
	if err := b.md.Convert([]byte(bodyWithLinks), &htmlBuf); err != nil {
		return err
	}

	// Build backlink summaries
	var backlinks []NoteSummary
	seen := make(map[string]bool)
	for _, slug := range backlinkSlugs {
		if seen[slug] || slug == n.Slug {
			continue
		}
		seen[slug] = true
		if linked, ok := slugIndex[slug]; ok {
			backlinks = append(backlinks, NoteSummary{
				Title: linked.Title,
				Slug:  linked.Slug,
			})
		}
	}

	data := PageData{
		Title:     n.Title,
		DateStr:   n.dateString(),
		Content:   template.HTML(htmlBuf.String()),
		Backlinks: backlinks,
	}

	pageDir := filepath.Join(notesDir, n.Slug)
	if err := os.MkdirAll(pageDir, 0755); err != nil {
		return err
	}
	outPath := filepath.Join(pageDir, "index.html")
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return Templates.ExecuteTemplate(f, "page.html", data)
}

func (b *Builder) buildIndexFromNote(n Note, backlinkSlugs []string, slugIndex map[string]Note) error {
	bodyWithLinks := ReplaceWikiLinks(n.Body)

	var htmlBuf bytes.Buffer
	if err := b.md.Convert([]byte(bodyWithLinks), &htmlBuf); err != nil {
		return err
	}

	var backlinks []NoteSummary
	seen := make(map[string]bool)
	for _, slug := range backlinkSlugs {
		if seen[slug] || slug == n.Slug {
			continue
		}
		seen[slug] = true
		if linked, ok := slugIndex[slug]; ok {
			backlinks = append(backlinks, NoteSummary{
				Title: linked.Title,
				Slug:  linked.Slug,
			})
		}
	}

	data := PageData{
		Title:     n.Title,
		DateStr:   n.dateString(),
		Content:   template.HTML(htmlBuf.String()),
		Backlinks: backlinks,
	}

	f, err := os.Create(filepath.Join(b.outDir, "index.html"))
	if err != nil {
		return err
	}
	defer f.Close()

	return Templates.ExecuteTemplate(f, "page.html", data)
}

func (b *Builder) buildIndex(notes []Note) error {
	var summaries []NoteSummary
	for _, n := range notes {
		summaries = append(summaries, NoteSummary{
			Title:   n.Title,
			Slug:    n.Slug,
			DateStr: n.dateString(),
		})
	}

	data := IndexData{Notes: summaries}

	f, err := os.Create(filepath.Join(b.outDir, "index.html"))
	if err != nil {
		return err
	}
	defer f.Close()

	return Templates.ExecuteTemplate(f, "index.html", data)
}

type searchEntry struct {
	Title string `json:"title"`
	Slug  string `json:"slug"`
	Date  string `json:"date"`
}

func (b *Builder) buildSearchIndex(notes []Note) error {
	var entries []searchEntry
	for _, n := range notes {
		entries = append(entries, searchEntry{
			Title: n.Title,
			Slug:  n.Slug,
			Date:  n.dateString(),
		})
	}
	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(b.outDir, "search.json"), data, 0644)
}

var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true,
	".gif": true, ".svg": true, ".webp": true,
}

func (b *Builder) copyImages() error {
	return filepath.Walk(b.dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !imageExts[ext] {
			return nil
		}

		relPath, _ := filepath.Rel(b.dataDir, path)
		destPath := filepath.Join(b.outDir, "images", relPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()

		dst, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer dst.Close()

		_, err = io.Copy(dst, src)
		return err
	})
}

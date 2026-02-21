package site

import (
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
)

var DefaultTemplates *template.Template
var DefaultStyleCSS []byte

func LoadTemplates(fsys fs.FS) error {
	var err error
	DefaultTemplates, err = template.ParseFS(fsys, "*.html")
	if err != nil {
		return err
	}

	DefaultStyleCSS, err = fs.ReadFile(fsys, "style.css")
	return err
}

// loadUserTemplates checks {dataDir}/templates/ for user overrides.
// Returns templates and CSS to use for this build.
func loadUserTemplates(dataDir string) (*template.Template, []byte) {
	tmplDir := filepath.Join(dataDir, "templates")

	tmpl := DefaultTemplates
	css := DefaultStyleCSS

	// Try loading user HTML templates
	if page, err := os.ReadFile(filepath.Join(tmplDir, "page.html")); err == nil {
		if idx, err := os.ReadFile(filepath.Join(tmplDir, "index.html")); err == nil {
			if t, err := template.New("page.html").Parse(string(page)); err == nil {
				if t, err = t.New("index.html").Parse(string(idx)); err == nil {
					tmpl = t
				}
			}
		} else {
			// Only page.html override — parse it, copy index from defaults
			if t, err := template.Must(DefaultTemplates.Clone()).New("page.html").Parse(string(page)); err == nil {
				tmpl = t
			}
		}
	} else if idx, err := os.ReadFile(filepath.Join(tmplDir, "index.html")); err == nil {
		// Only index.html override
		if t, err := template.Must(DefaultTemplates.Clone()).New("index.html").Parse(string(idx)); err == nil {
			tmpl = t
		}
	}

	// Try loading user CSS
	if data, err := os.ReadFile(filepath.Join(tmplDir, "style.css")); err == nil {
		css = data
	}

	return tmpl, css
}

type IndexData struct {
	Notes []NoteSummary
}

type NoteSummary struct {
	Title   string
	Slug    string
	DateStr string
}

type PageData struct {
	Title     string
	DateStr   string
	Content   template.HTML
	Backlinks []NoteSummary
}

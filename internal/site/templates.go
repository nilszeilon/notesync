package site

import (
	"html/template"
	"io/fs"
)

var Templates *template.Template
var StyleCSS []byte

func LoadTemplates(fsys fs.FS) error {
	var err error
	Templates, err = template.ParseFS(fsys, "*.html")
	if err != nil {
		return err
	}

	StyleCSS, err = fs.ReadFile(fsys, "style.css")
	return err
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

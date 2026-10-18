package main

import (
	_ "embed"
	"net/http"
)

//go:embed console.html
var indexHTML string

//go:embed advanced.html
var advancedHTML string

func handleIndex(w http.ResponseWriter, r *http.Request) {
	html := indexHTML
	if r.URL.Path == "/advanced" {
		html = advancedHTML
	} else if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

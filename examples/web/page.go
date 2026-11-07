package main

import (
	_ "embed"
	"io"
	"net/http"
)

//go:embed index.html
var html string

func page(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, html)
}

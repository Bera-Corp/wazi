package controlplane

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets/*
var brandAssets embed.FS

func registerBrandAssets(mux *http.ServeMux) {
	assets, err := fs.Sub(brandAssets, "assets")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assets))))
}

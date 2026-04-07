package control

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed webh5_assets/*
var webH5Assets embed.FS

func NewWebH5Handler() http.Handler {
	assetRoot, err := fs.Sub(webH5Assets, "webh5_assets")
	if err != nil {
		panic(err)
	}

	return http.FileServer(http.FS(assetRoot))
}

package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"waypoint/internal/webassets"
)

func Handler() http.Handler {
	assets, err := fs.Sub(webassets.Assets, "dist")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/readyz", readyz(assets))
	mux.Handle("/", spa(assets))
	return mux
}

type statusResponse struct {
	Status string `json:"status"`
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

func readyz(assets fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		if err := requiredAssetsPresent(assets); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, statusResponse{Status: "not-ready"})
			return
		}
		writeJSON(w, http.StatusOK, statusResponse{Status: "ready"})
	}
}

func requiredAssetsPresent(assets fs.FS) error {
	for _, name := range []string{"index.html", "assets/waypoint.js", "assets/waypoint.css"} {
		if _, err := fs.Stat(assets, name); err != nil {
			return err
		}
	}
	return nil
}

func spa(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		relative := strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(r.URL.Path, "/")), "/")
		if relative == "" || relative == "." {
			relative = "index.html"
		}

		if relative == "index.html" {
			fileServer.ServeHTTP(w, r)
			return
		}

		if strings.Contains(path.Base(relative), ".") {
			if exists(assets, relative) {
				fileServer.ServeHTTP(w, r)
				return
			}
			http.NotFound(w, r)
			return
		}

		routed := r.Clone(r.Context())
		routed.URL.Path = "/index.html"
		fileServer.ServeHTTP(w, routed)
	})
}

func exists(assets fs.FS, name string) bool {
	_, err := fs.Stat(assets, name)
	return err == nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}


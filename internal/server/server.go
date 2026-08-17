package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"waypoint/internal/webassets"
)

func Handler() http.Handler {
	return handler(nil)
}

func HandlerWithDB(db *sql.DB) http.Handler {
	return handler(db)
}

func handler(db *sql.DB) http.Handler {
	assets, err := fs.Sub(webassets.Assets, "dist")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/readyz", readyz(assets, db))
	store := newEvidenceStore()
	mux.HandleFunc("/api/v1/captures", captureHandler(db, store, "rest"))
	mux.HandleFunc("/captures", captureHandler(db, store, "rest"))
	mux.HandleFunc("/api/v1/mcp/capture", captureHandler(db, store, "mcp"))
	mux.HandleFunc("/mcp/capture", captureHandler(db, store, "mcp"))
	mux.HandleFunc("/api/v1/mcp/status", mcpStatusHandler(db))
	mux.HandleFunc("/mcp/status", mcpStatusHandler(db))
	mux.HandleFunc("/api/v1/out-of-band/review", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/out-of-band/review", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/api/v1/out-of-band/reviews", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/out-of-band/reviews", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/api/v1/out-of-band/claims/review", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/out-of-band/claims/review", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/api/v1/claims/review", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/claims/review", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/api/v1/mcp/review", outOfBandReviewHandler(db, "mcp"))
	mux.HandleFunc("/mcp/review", outOfBandReviewHandler(db, "mcp"))
	mux.HandleFunc("/api/v1/entities", entityReadHandler(db))
	mux.HandleFunc("/api/v1/entities/", entityReadHandler(db))
	mux.HandleFunc("/api/v1/entities/merge", mergeEntityHandler(db))
	mux.HandleFunc("/api/v1/entities/split", splitEntityHandler(db))
	mux.HandleFunc("/api/v1/evidence", evidenceHandler(db, store))
	mux.HandleFunc("/api/v1/evidence/", evidenceHandler(db, store))
	mux.HandleFunc("/evidence", evidenceHandler(db, store))
	mux.HandleFunc("/evidence/", evidenceHandler(db, store))
	mux.HandleFunc("/api/v1/findings", findingsHandler(db))
	mux.HandleFunc("/api/v1/findings/", findingsHandler(db))
	mux.HandleFunc("/api/v1/audit-events", auditEventsHandler(db))
	mux.HandleFunc("/events", auditEventsStreamHandler(db))
	report := reportHandler(db, store)
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reportJSONRoute.MatchString(r.URL.Path) || reportPDFRoute.MatchString(r.URL.Path) {
			report.ServeHTTP(w, r)
			return
		}
		spa(assets).ServeHTTP(w, r)
	}))
	return mux
}

type statusResponse struct {
	Status string `json:"status"`
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, statusResponse{Status: "ok"})
}

type dbPinger interface {
	PingContext(context.Context) error
}

func readyz(assets fs.FS, db dbPinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := requiredAssetsPresent(assets); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, statusResponse{Status: "not-ready"})
			return
		}
		if db == nil {
			writeJSON(w, http.StatusServiceUnavailable, statusResponse{Status: "not-ready"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
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
			serveIndex(w, r, assets)
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

		serveIndex(w, r, assets)
	})
}

func serveIndex(w http.ResponseWriter, r *http.Request, assets fs.FS) {
	data, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}

	http.ServeContent(w, r, "index.html", time.Unix(0, 0), bytes.NewReader(data))
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

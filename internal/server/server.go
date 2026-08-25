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

	"waypoint/internal/egresspolicy"
	"waypoint/internal/webassets"
)

type RuntimeState struct {
	Egress egresspolicy.State `json:"egress"`
}

func Handler() http.Handler {
	return handler(nil, RuntimeState{})
}

func HandlerWithDB(db *sql.DB) http.Handler {
	return handler(db, RuntimeState{})
}

func HandlerWithDBAndRuntime(db *sql.DB, runtime RuntimeState) http.Handler {
	return handler(db, runtime)
}

func handler(db *sql.DB, runtime RuntimeState) http.Handler {
	assets, err := fs.Sub(webassets.Assets, "dist")
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/readyz", readyz(assets, db))
	store := newEvidenceStore()
	exports := newExportManagerWithRuntime(db, store, runtime)
	go exports.recoverOutstanding(context.Background())
	mux.HandleFunc("/api/v1/captures", captureHandler(db, store, "rest"))
	mux.HandleFunc("/captures", captureHandler(db, store, "rest"))
	mux.HandleFunc("/mcp", mcpHandler(db, store))
	mux.HandleFunc("/api/v1/out-of-band-claims", outOfBandClaimsHandler(db, "rest"))
	mux.HandleFunc("/api/v1/out-of-band-claims/", outOfBandClaimResourceHandler(db, "rest"))
	mux.HandleFunc("/out-of-band-claims", outOfBandClaimsHandler(db, "rest"))
	mux.HandleFunc("/out-of-band-claims/", outOfBandClaimResourceHandler(db, "rest"))
	mux.HandleFunc("/api/v1/actors", actorHandler(db))
	mux.HandleFunc("/api/v1/actors/", actorResourceHandler(db))
	mux.HandleFunc("/actors", actorHandler(db))
	mux.HandleFunc("/actors/", actorResourceHandler(db))
	mux.HandleFunc("/api/v1/out-of-band/review", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/out-of-band/review", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/api/v1/out-of-band/reviews", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/out-of-band/reviews", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/api/v1/out-of-band-claims/review", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/out-of-band-claims/review", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/api/v1/out-of-band-claims/reviews", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/out-of-band-claims/reviews", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/api/v1/out-of-band/claims/review", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/out-of-band/claims/review", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/api/v1/claims/review", outOfBandReviewHandler(db, "rest"))
	mux.HandleFunc("/claims/review", outOfBandReviewHandler(db, "rest"))
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
	mux.HandleFunc("/api/v1/actions", actionsHandler(db, false))
	mux.HandleFunc("/actions", actionsHandler(db, false))
	mux.HandleFunc("/api/v1/actions/needs-plugin", actionsHandler(db, true))
	mux.HandleFunc("/actions/needs-plugin", actionsHandler(db, true))
	mux.HandleFunc("/api/v1/audit-events", auditEventsHandler(db))
	mux.HandleFunc("/events", auditEventsStreamHandler(db))
	mux.HandleFunc("/api/v1/exports", exportHandler(db, store, exports))
	mux.HandleFunc("/api/v1/exports/", exportHandler(db, store, exports))
	mux.HandleFunc("/exports", exportHandler(db, store, exports))
	mux.HandleFunc("/exports/", exportHandler(db, store, exports))
	mux.HandleFunc("/api/v1/export-receipts/", exportHandler(db, store, exports))
	mux.HandleFunc("/export-receipts/", exportHandler(db, store, exports))
	mux.HandleFunc("/api/v1/teardown-authorizations", exportHandler(db, store, exports))
	mux.HandleFunc("/api/v1/teardown-authorizations/", exportHandler(db, store, exports))
	mux.HandleFunc("/teardown-authorizations", exportHandler(db, store, exports))
	mux.HandleFunc("/teardown-authorizations/", exportHandler(db, store, exports))
	mux.HandleFunc("/api/v1/runtime", runtimeHandler(runtime))
	mux.HandleFunc("/runtime", runtimeHandler(runtime))
	report := reportHandlerWithRuntime(db, store, runtime)
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

func runtimeHandler(runtime RuntimeState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, runtime)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

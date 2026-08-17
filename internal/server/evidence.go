package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const evidenceContractVersion = "1.0.0"

var evidenceRoutePrefixes = []string{"/api/v1/evidence", "/evidence"}

type evidenceRecord struct {
	ID           string
	EngagementID string
	ActionID     string
	Role         string
	MediaType    string
	ByteLength   int64
	SHA256       string
	CreatedAt    time.Time
	StorageKey   string
}

type evidenceResponse struct {
	ContractVersion string    `json:"contractVersion"`
	ID              string    `json:"id"`
	EngagementID    string    `json:"engagementId"`
	ActionID        string    `json:"actionId"`
	Role            string    `json:"role"`
	MediaType       string    `json:"mediaType"`
	ByteLength      int64     `json:"byteLength"`
	SHA256          string    `json:"sha256"`
	CreatedAt       time.Time `json:"createdAt"`
	ContentPath     string    `json:"contentPath"`
}

func evidenceHandler(db *sql.DB, store *evidenceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil || store == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: requestIDFromHeader(r.Header.Get("X-Request-ID")), Retryable: true, Detail: "evidence retrieval is unavailable"})
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{evidenceContractVersion}})
			return
		}
		token, err := bearerToken(r.Header.Get("Authorization"))
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUnauthorized), Status: http.StatusUnauthorized, Code: "unauthenticated", RequestID: reqID, Retryable: false, Detail: err.Error()})
			return
		}
		actor, err := lookupActor(r.Context(), db, token)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUnauthorized), Status: http.StatusUnauthorized, Code: "unauthenticated", RequestID: reqID, Retryable: false, Detail: "invalid actor credential"})
			return
		}
		if err := store.ensureReady(r.Context(), db); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: reqID, Retryable: true, Detail: "evidence storage is unavailable"})
			return
		}

		trimmed := evidenceRequestPath(r.URL.Path)
		if trimmed == "" {
			http.NotFound(w, r)
			return
		}
		parts := strings.Split(trimmed, "/")
		if len(parts) == 0 || parts[0] == "" {
			http.NotFound(w, r)
			return
		}
		evidenceID := parts[0]
		if !isUUID(evidenceID) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "evidence id must be a UUID."})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		record, err := loadEvidenceRecord(ctx, db, actor.EngagementID, evidenceID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeProblem(w, captureProblem{Type: "https://docs.waypoint.security/problems/resource-not-found", Title: "That trail record is not available", Status: http.StatusNotFound, Code: "resource_not_found", RequestID: reqID, Retryable: false, Detail: "The resource does not exist in this engagement or is not available to this actor."})
				return
			}
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load evidence failed"})
			return
		}

		if len(parts) == 1 {
			writeJSONWithHeaders(w, http.StatusOK, evidenceResponseFromRecord(record), reqID)
			return
		}
		if len(parts) != 2 || parts[1] != "content" {
			http.NotFound(w, r)
			return
		}

		if err := serveEvidenceContent(w, r, store, record, reqID); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "resource_not_found", RequestID: reqID, Retryable: false, Detail: "The resource does not exist in this engagement or is not available to this actor."})
				return
			}
			if strings.Contains(err.Error(), "size mismatch") {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUnprocessableEntity), Status: http.StatusUnprocessableEntity, Code: "evidence_integrity_mismatch", RequestID: reqID, Retryable: false, Detail: "stored evidence bytes do not match the recorded metadata."})
				return
			}
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "stream evidence failed"})
		}
	}
}

func evidenceRequestPath(urlPath string) string {
	cleaned := path.Clean(urlPath)
	for _, prefix := range evidenceRoutePrefixes {
		if cleaned == prefix {
			return ""
		}
		if strings.HasPrefix(cleaned, prefix+"/") {
			return strings.TrimPrefix(cleaned, prefix+"/")
		}
	}
	return ""
}

func loadEvidenceRecord(ctx context.Context, db *sql.DB, engagementID, evidenceID string) (evidenceRecord, error) {
	var rec evidenceRecord
	err := db.QueryRowContext(ctx, `
		SELECT e.id::text, e.engagement_id::text, a.id::text, e.kind, e.media_type, e.byte_length, e.sha256, e.created_at, e.storage_key
		FROM evidence e
		JOIN LATERAL (
			SELECT id
			FROM action
			WHERE engagement_id = e.engagement_id AND (stdout_evidence_id = e.id OR stderr_evidence_id = e.id)
			ORDER BY started_at ASC, id ASC
			LIMIT 1
		) a ON true
		WHERE e.id = $1 AND e.engagement_id = $2`, evidenceID, engagementID).Scan(&rec.ID, &rec.EngagementID, &rec.ActionID, &rec.Role, &rec.MediaType, &rec.ByteLength, &rec.SHA256, &rec.CreatedAt, &rec.StorageKey)
	if err != nil {
		return evidenceRecord{}, err
	}
	if rec.Role != "stdout" && rec.Role != "stderr" && rec.Role != "screenshot" && rec.Role != "attachment" {
		return evidenceRecord{}, sql.ErrNoRows
	}
	return rec, nil
}

func evidenceResponseFromRecord(rec evidenceRecord) evidenceResponse {
	return evidenceResponse{
		ContractVersion: evidenceContractVersion,
		ID:              rec.ID,
		EngagementID:    rec.EngagementID,
		ActionID:        rec.ActionID,
		Role:            rec.Role,
		MediaType:       rec.MediaType,
		ByteLength:      rec.ByteLength,
		SHA256:          rec.SHA256,
		CreatedAt:       rec.CreatedAt.UTC(),
		ContentPath:     "/api/v1/evidence/" + rec.ID + "/content",
	}
}

func serveEvidenceContent(w http.ResponseWriter, r *http.Request, store *evidenceStore, record evidenceRecord, reqID string) error {
	fullPath, err := store.resolveStoragePath(record.StorageKey)
	if err != nil {
		return err
	}
	f, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() != record.ByteLength {
		return fmt.Errorf("size mismatch: stored evidence size %d does not match metadata %d", st.Size(), record.ByteLength)
	}

	mediaType := safeMediaType(record.MediaType)
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", record.ID))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Request-ID", reqID)
	w.Header().Set("Waypoint-Contract-Version", evidenceContractVersion)
	http.ServeContent(w, r, record.ID, record.CreatedAt.UTC(), f)
	return nil
}

func safeMediaType(v string) string {
	mediaType := strings.TrimSpace(v)
	if mediaType == "" {
		return "application/octet-stream"
	}
	typ, params, err := mime.ParseMediaType(mediaType)
	if err != nil || typ == "" {
		return "application/octet-stream"
	}
	formatted := mime.FormatMediaType(typ, params)
	if formatted == "" {
		return typ
	}
	return formatted
}

func (s *evidenceStore) resolveStoragePath(storageKey string) (string, error) {
	storageKey = strings.TrimSpace(storageKey)
	if storageKey == "" {
		return "", os.ErrNotExist
	}
	cleaned := filepath.Clean(filepath.FromSlash(storageKey))
	if cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return "", os.ErrNotExist
	}
	root := filepath.Clean(s.root)
	fullPath := filepath.Join(root, cleaned)
	rel, err := filepath.Rel(root, fullPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", os.ErrNotExist
	}
	return fullPath, nil
}

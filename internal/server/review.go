package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"strings"
	"time"

	dbutil "waypoint/internal/db"
)

const outOfBandReviewContractVersion = "1.0.0"

type outOfBandReviewRequest struct {
	ClaimID        string    `json:"claimId"`
	ClaimKind      string    `json:"claimKind"`
	SourceActionID string    `json:"sourceActionId"`
	Resolution     string    `json:"resolution"`
	ResolvedAt     time.Time `json:"resolvedAt"`
	Notes          string    `json:"notes,omitempty"`
}

type outOfBandReviewResponse struct {
	ContractVersion  string    `json:"contractVersion"`
	ClaimID          string    `json:"claimId"`
	AuditEventCursor string    `json:"auditEventCursor"`
	ResolvedAt       time.Time `json:"resolvedAt"`
	Idempotency      string    `json:"idempotency"`
}

func mcpStatusHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		if db == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: requestIDFromHeader(r.Header.Get("X-Request-ID")), Retryable: true, Detail: "mcp status is unavailable"})
			return
		}

		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{outOfBandReviewContractVersion}})
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

		writeJSONWithHeaders(w, http.StatusOK, map[string]any{
			"contractVersion": outOfBandReviewContractVersion,
			"status":          "ready",
			"actorId":         actor.ID,
			"engagementId":    actor.EngagementID,
			"tools":           []string{"capture", "status"},
		}, reqID)
	}
}

func outOfBandReviewHandler(db *sql.DB, originKind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if db == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: reqID, Retryable: true, Detail: "out-of-band review is unavailable"})
			return
		}
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{outOfBandReviewContractVersion}})
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

		body, err := ioReadAllLimited(r.Body, 1<<20)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "request body is invalid"})
			return
		}
		var req outOfBandReviewRequest
		if err := decodeStrictJSON(body, &req); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()})
			return
		}
		if !isUUID(req.ClaimID) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/claimId", Code: "invalid_uuid", Message: "claimId must be a UUID."}}})
			return
		}
		if !strings.EqualFold(req.ClaimID, r.Header.Get("Idempotency-Key")) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/claimId", Code: "idempotency_key_mismatch", Message: "Idempotency-Key must exactly match claimId."}}})
			return
		}
		if req.ClaimKind != "entity" && req.ClaimKind != "result" {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/claimKind", Code: "invalid_enum", Message: "claimKind must be entity or result."}}})
			return
		}
		if !isUUID(req.SourceActionID) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/sourceActionId", Code: "invalid_uuid", Message: "sourceActionId must be a UUID."}}})
			return
		}
		if req.Resolution != "linked" && req.Resolution != "dismissed" {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/resolution", Code: "invalid_enum", Message: "resolution must be linked or dismissed."}}})
			return
		}
		if req.ResolvedAt.IsZero() {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/resolvedAt", Code: "required", Message: "resolvedAt is required."}}})
			return
		}

		data := map[string]any{
			"claimId":        req.ClaimID,
			"claimKind":      req.ClaimKind,
			"sourceActionId": req.SourceActionID,
			"resolution":     req.Resolution,
			"resolvedAt":     req.ResolvedAt,
		}
		if req.Notes != "" {
			data["notes"] = req.Notes
		}
		fingerprintBytes, err := json.Marshal(data)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "request body is invalid"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "begin transaction failed"})
			return
		}
		defer tx.Rollback()

		if err := lockOutOfBandReviewScope(ctx, tx, actor.EngagementID, actor.ID, req.ClaimID); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "review lock failed"})
			return
		}

		var existingID int64
		var existingData string
		err = tx.QueryRowContext(ctx, `SELECT id, data::text FROM audit_event WHERE engagement_id = $1 AND type = 'out-of-band.resolved' AND subject_type = 'out_of_band_claim' AND subject_id = $2 ORDER BY id DESC LIMIT 1`, actor.EngagementID, req.ClaimID).Scan(&existingID, &existingData)
		if err == nil {
			if existingData == string(fingerprintBytes) {
				if err := tx.Commit(); err != nil {
					writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "commit replay failed"})
					return
				}
				writeJSONWithHeaders(w, http.StatusOK, outOfBandReviewResponse{ContractVersion: outOfBandReviewContractVersion, ClaimID: req.ClaimID, AuditEventCursor: eventCursor(existingID), ResolvedAt: req.ResolvedAt, Idempotency: "replayed"}, reqID)
				return
			}
			if err := tx.Commit(); err != nil {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "commit conflict failed"})
				return
			}
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "idempotency_conflict", RequestID: reqID, Retryable: false, ExistingActionID: eventCursor(existingID), Detail: "same claimId was previously reviewed with different payload"})
			return
		}
		if !errors.Is(err, sql.ErrNoRows) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load review state failed"})
			return
		}
		if ok, err := sourceActionCaptured(ctx, tx, actor.EngagementID, req.SourceActionID); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "validate source action failed"})
			return
		} else if !ok {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "invalid_source_capture", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/sourceActionId", Code: "not_found", Message: "sourceActionId must reference a captured action in this engagement."}}, Detail: "source action is not captured in this engagement"})
			return
		}

		eventID, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
			EngagementID:  actor.EngagementID,
			Type:          "out-of-band.resolved",
			Actor:         auditActorSnapshot(actor),
			Origin:        dbutil.AuditOrigin{Kind: originKind},
			Subject:       dbutil.AuditSubject{Type: "out_of_band_claim", ID: req.ClaimID, Revision: 1},
			RequestID:     reqID,
			CorrelationID: req.ClaimID,
			Data:          data,
		})
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: fmt.Sprintf("append out-of-band review event failed: %v", err)})
			return
		}
		if err := tx.Commit(); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "commit review failed"})
			return
		}
		writeJSONWithHeaders(w, http.StatusCreated, outOfBandReviewResponse{ContractVersion: outOfBandReviewContractVersion, ClaimID: req.ClaimID, AuditEventCursor: eventCursor(eventID), ResolvedAt: req.ResolvedAt, Idempotency: "created"}, reqID)
	}
}

func sourceActionCaptured(ctx context.Context, tx *sql.Tx, engagementID, actionID string) (bool, error) {
	var found int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM action WHERE engagement_id = $1 AND id = $2 LIMIT 1`, engagementID, actionID).Scan(&found); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func lockOutOfBandReviewScope(ctx context.Context, tx *sql.Tx, engagementID, actorID, claimID string) error {
	h := fnv.New64a()
	_, _ = h.Write([]byte(engagementID + "|" + actorID + "|" + claimID))
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(h.Sum64()))
	return err
}

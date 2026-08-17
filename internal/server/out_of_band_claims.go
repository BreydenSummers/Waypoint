package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	dbutil "waypoint/internal/db"
)

const outOfBandClaimContractVersion = "1.0.0"

const (
	outOfBandClaimTypeFlagged     = "out-of-band.flagged"
	outOfBandClaimTypeResolved    = "out-of-band.resolved"
	outOfBandClaimStatusPending   = "pending"
	outOfBandClaimStatusLinked    = "linked"
	outOfBandClaimStatusDismissed = "dismissed"
)

type outOfBandClaimCreateRequest struct {
	ClaimKind              string    `json:"claimKind"`
	ClaimedSubjectID       string    `json:"claimedSubjectId"`
	AssertedSourceActionID string    `json:"assertedSourceActionId,omitempty"`
	ObservedAt             time.Time `json:"observedAt"`
}

type outOfBandClaimResolutionRequest struct {
	Resolution       string `json:"resolution"`
	SourceActionID   string `json:"sourceActionId,omitempty"`
	Notes            string `json:"notes,omitempty"`
	ExpectedRevision int    `json:"expectedRevision"`
}

type outOfBandClaimPageResponse struct {
	ContractVersion string                 `json:"contractVersion"`
	Items           []outOfBandClaimItem   `json:"items"`
	Page            outOfBandClaimPageMeta `json:"page"`
}

type outOfBandClaimPageMeta struct {
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}

type outOfBandClaimItem struct {
	ContractVersion   string           `json:"contractVersion"`
	ID                string           `json:"id"`
	EngagementID      string           `json:"engagementId"`
	ClaimKind         string           `json:"claimKind"`
	ClaimedSubjectID  string           `json:"claimedSubjectId"`
	SourceActionID    *string          `json:"sourceActionId"`
	DetectionBoundary string           `json:"detectionBoundary"`
	Reason            string           `json:"reason"`
	Status            string           `json:"status"`
	ObservedAt        time.Time        `json:"observedAt"`
	ObservedBy        auditEventActor  `json:"observedBy"`
	ResolvedAt        *time.Time       `json:"resolvedAt,omitempty"`
	ResolvedBy        *auditEventActor `json:"resolvedBy,omitempty"`
	Notes             string           `json:"notes,omitempty"`
	Revision          int              `json:"revision"`
}

type outOfBandClaimEventRow struct {
	EventID    int64
	ClaimID    string
	Type       string
	Revision   int
	OccurredAt time.Time
	Actor      auditEventActor
	Data       json.RawMessage
}

type outOfBandClaimObservedData struct {
	ClaimID           string    `json:"claimId"`
	ClaimKind         string    `json:"claimKind"`
	ClaimedSubjectID  string    `json:"claimedSubjectId"`
	SourceActionID    *string   `json:"sourceActionId"`
	DetectionBoundary string    `json:"detectionBoundary"`
	Reason            string    `json:"reason"`
	ObservedAt        time.Time `json:"observedAt"`
}

type outOfBandClaimResolvedData struct {
	ClaimID        string    `json:"claimId"`
	ClaimKind      string    `json:"claimKind"`
	SourceActionID *string   `json:"sourceActionId"`
	Resolution     string    `json:"resolution"`
	ResolvedAt     time.Time `json:"resolvedAt"`
	Notes          string    `json:"notes,omitempty"`
}

func outOfBandClaimsHandler(db *sql.DB, originKind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if db == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: reqID, Retryable: true, Detail: "out-of-band claims are unavailable"})
			return
		}
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{outOfBandClaimContractVersion}})
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

		switch r.Method {
		case http.MethodGet:
			limit, pb := parseAuditLimit(r.URL.Query().Get("limit"))
			if pb != nil {
				pb.RequestID = reqID
				writeProblem(w, *pb)
				return
			}
			after, pb := parseAuditCursorParam(r.URL.Query().Get("after"))
			if pb != nil {
				pb.RequestID = reqID
				writeProblem(w, *pb)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			page, err := loadOutOfBandClaimPage(ctx, db, actor.EngagementID, after, limit)
			if err != nil {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load out-of-band claims failed"})
				return
			}
			writeJSONWithHeaders(w, http.StatusOK, page, reqID)
		case http.MethodPost:
			body, err := ioReadAllLimited(r.Body, 1<<20)
			if err != nil {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "request body is invalid"})
				return
			}
			var req outOfBandClaimCreateRequest
			if err := decodeStrictJSON(body, &req); err != nil {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()})
				return
			}
			if req.ClaimKind != "entity" && req.ClaimKind != "result" {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/claimKind", Code: "invalid_enum", Message: "claimKind must be entity or result."}}})
				return
			}
			if !isUUID(req.ClaimedSubjectID) {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/claimedSubjectId", Code: "invalid_uuid", Message: "claimedSubjectId must be a UUID."}}})
				return
			}
			if req.ObservedAt.IsZero() {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/observedAt", Code: "required", Message: "observedAt is required."}}})
				return
			}
			if strings.TrimSpace(req.AssertedSourceActionID) != "" && !isUUID(req.AssertedSourceActionID) {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/assertedSourceActionId", Code: "invalid_uuid", Message: "assertedSourceActionId must be a UUID."}}})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			claim, err := observeOutOfBandClaim(ctx, db, actor, originKind, reqID, req)
			if err != nil {
				if pb, ok := err.(*captureProblem); ok {
					writeProblem(w, *pb)
					return
				}
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "create out-of-band claim failed"})
				return
			}
			writeJSONWithHeaders(w, http.StatusCreated, claim, reqID)
		}
	}
}

func outOfBandClaimResourceHandler(db *sql.DB, originKind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/out-of-band-claims/", "/out-of-band-claims/":
			outOfBandClaimsHandler(db, originKind).ServeHTTP(w, r)
			return
		}
		relative := strings.TrimPrefix(r.URL.Path, "/")
		parts := strings.Split(relative, "/")
		if len(parts) >= 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "out-of-band-claims" {
			if len(parts) == 4 && parts[3] != "" {
				outOfBandClaimGetHandler(db, originKind, parts[3]).ServeHTTP(w, r)
				return
			}
			if len(parts) == 5 && parts[4] == "resolve" {
				outOfBandClaimResolvePathHandler(db, originKind, parts[3]).ServeHTTP(w, r)
				return
			}
		}
		if len(parts) >= 2 && parts[0] == "out-of-band-claims" {
			if len(parts) == 2 && parts[1] != "" {
				outOfBandClaimGetHandler(db, originKind, parts[1]).ServeHTTP(w, r)
				return
			}
			if len(parts) == 3 && parts[2] == "resolve" {
				outOfBandClaimResolvePathHandler(db, originKind, parts[1]).ServeHTTP(w, r)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func outOfBandClaimGetHandler(db *sql.DB, originKind, claimID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if db == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: reqID, Retryable: true, Detail: "out-of-band claims are unavailable"})
			return
		}
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{outOfBandClaimContractVersion}})
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
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		item, err := loadOutOfBandClaim(ctx, db, actor.EngagementID, claimID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "resource_not_found", RequestID: reqID, Retryable: false, Detail: "The resource does not exist in this engagement or is not available to this actor."})
				return
			}
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load out-of-band claim failed"})
			return
		}
		_ = originKind
		writeJSONWithHeaders(w, http.StatusOK, item, reqID)
	}
}

func outOfBandClaimResolvePathHandler(db *sql.DB, originKind, claimID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if db == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: reqID, Retryable: true, Detail: "out-of-band claims are unavailable"})
			return
		}
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{outOfBandClaimContractVersion}})
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
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		item, err := resolveOutOfBandClaim(ctx, db, actor, originKind, reqID, claimID, r)
		if err != nil {
			if pb, ok := err.(*captureProblem); ok {
				writeProblem(w, *pb)
				return
			}
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "resolve out-of-band claim failed"})
			return
		}
		writeJSONWithHeaders(w, http.StatusOK, item, reqID)
	}
}

func observeOutOfBandClaim(ctx context.Context, db *sql.DB, actor actorRecord, originKind, reqID string, req outOfBandClaimCreateRequest) (outOfBandClaimItem, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return outOfBandClaimItem{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "begin transaction failed"}
	}
	defer tx.Rollback()

	claimID := newUUID()
	observedSourceActionID := strings.TrimSpace(req.AssertedSourceActionID)
	var linked bool
	if observedSourceActionID != "" {
		if ok, err := sourceActionCaptured(ctx, tx, actor.EngagementID, observedSourceActionID); err == nil && ok {
			linked = true
		}
	}

	flaggedData := map[string]any{
		"claimId":           claimID,
		"claimKind":         req.ClaimKind,
		"claimedSubjectId":  req.ClaimedSubjectID,
		"sourceActionId":    nil,
		"detectionBoundary": "best_effort",
		"reason":            "missing_captured_source_action",
		"observedAt":        req.ObservedAt,
	}
	if linked {
		flaggedData["sourceActionId"] = observedSourceActionID
	}
	if _, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
		EngagementID:  actor.EngagementID,
		Type:          outOfBandClaimTypeFlagged,
		Actor:         auditActorSnapshot(actor),
		Origin:        dbutil.AuditOrigin{Kind: originKind},
		Subject:       dbutil.AuditSubject{Type: "out_of_band_claim", ID: claimID, Revision: 1},
		RequestID:     reqID,
		CorrelationID: claimID,
		Data:          flaggedData,
	}); err != nil {
		return outOfBandClaimItem{}, err
	}

	if linked {
		resolvedAt := req.ObservedAt
		resolvedData := map[string]any{
			"claimId":        claimID,
			"claimKind":      req.ClaimKind,
			"sourceActionId": observedSourceActionID,
			"resolution":     outOfBandClaimStatusLinked,
			"resolvedAt":     resolvedAt,
		}
		if _, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
			EngagementID:  actor.EngagementID,
			Type:          outOfBandClaimTypeResolved,
			Actor:         auditActorSnapshot(actor),
			Origin:        dbutil.AuditOrigin{Kind: originKind},
			Subject:       dbutil.AuditSubject{Type: "out_of_band_claim", ID: claimID, Revision: 2},
			RequestID:     reqID,
			CorrelationID: claimID,
			Data:          resolvedData,
		}); err != nil {
			return outOfBandClaimItem{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return outOfBandClaimItem{}, err
	}
	return loadOutOfBandClaim(ctx, db, actor.EngagementID, claimID)
}

func resolveOutOfBandClaim(ctx context.Context, db *sql.DB, actor actorRecord, originKind, reqID, claimID string, r *http.Request) (outOfBandClaimItem, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return outOfBandClaimItem{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "begin transaction failed"}
	}
	defer tx.Rollback()

	if err := lockOutOfBandReviewScope(ctx, tx, actor.EngagementID, actor.ID, claimID); err != nil {
		return outOfBandClaimItem{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "review lock failed"}
	}
	rows, err := loadOutOfBandClaimTimeline(ctx, tx, actor.EngagementID, claimID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return outOfBandClaimItem{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "resource_not_found", RequestID: reqID, Retryable: false, Detail: "The resource does not exist in this engagement or is not available to this actor."}
		}
		return outOfBandClaimItem{}, err
	}
	current, err := buildOutOfBandClaim(actor.EngagementID, rows)
	if err != nil {
		return outOfBandClaimItem{}, err
	}

	body, err := ioReadAllLimited(r.Body, 1<<20)
	if err != nil {
		return outOfBandClaimItem{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "request body is invalid"}
	}
	var req outOfBandClaimResolutionRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		return outOfBandClaimItem{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()}
	}
	if req.ExpectedRevision < 1 {
		return outOfBandClaimItem{}, badField("/expectedRevision", "invalid_range", "expectedRevision must be >= 1.")
	}
	if req.Resolution != outOfBandClaimStatusLinked && req.Resolution != outOfBandClaimStatusDismissed {
		return outOfBandClaimItem{}, badField("/resolution", "invalid_enum", "resolution must be linked or dismissed.")
	}
	if req.Resolution == outOfBandClaimStatusLinked && !isUUID(strings.TrimSpace(req.SourceActionID)) {
		return outOfBandClaimItem{}, badField("/sourceActionId", "invalid_uuid", "sourceActionId must be a UUID.")
	}
	if req.Resolution == outOfBandClaimStatusDismissed && strings.TrimSpace(req.SourceActionID) != "" {
		return outOfBandClaimItem{}, badField("/sourceActionId", "unexpected_field", "dismissed claims must not include sourceActionId.")
	}

	if current.Status == req.Resolution && claimResolutionMatches(current, req) {
		return current, nil
	}
	if current.Revision != req.ExpectedRevision {
		return outOfBandClaimItem{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusPreconditionFailed), Status: http.StatusPreconditionFailed, Code: "precondition_failed", RequestID: reqID, Retryable: false, Detail: "expectedRevision does not match the current claim revision."}
	}
	if req.Resolution == outOfBandClaimStatusLinked {
		if ok, err := sourceActionCaptured(ctx, tx, actor.EngagementID, req.SourceActionID); err != nil {
			return outOfBandClaimItem{}, err
		} else if !ok {
			return outOfBandClaimItem{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "invalid_source_capture", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/sourceActionId", Code: "not_found", Message: "sourceActionId must reference a captured action in this engagement."}}, Detail: "source action is not captured in this engagement"}
		}
	}
	resolvedAt := time.Now().UTC()
	resolvedData := map[string]any{
		"claimId":        claimID,
		"claimKind":      current.ClaimKind,
		"sourceActionId": nil,
		"resolution":     req.Resolution,
		"resolvedAt":     resolvedAt,
	}
	if req.Notes != "" {
		resolvedData["notes"] = req.Notes
	}
	if req.Resolution == outOfBandClaimStatusLinked {
		resolvedData["sourceActionId"] = req.SourceActionID
	}
	if _, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
		EngagementID:  actor.EngagementID,
		Type:          outOfBandClaimTypeResolved,
		Actor:         auditActorSnapshot(actor),
		Origin:        dbutil.AuditOrigin{Kind: originKind},
		Subject:       dbutil.AuditSubject{Type: "out_of_band_claim", ID: claimID, Revision: current.Revision + 1},
		RequestID:     reqID,
		CorrelationID: claimID,
		Data:          resolvedData,
	}); err != nil {
		return outOfBandClaimItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return outOfBandClaimItem{}, err
	}
	return loadOutOfBandClaim(ctx, db, actor.EngagementID, claimID)
}

func loadOutOfBandClaimPage(ctx context.Context, db *sql.DB, engagementID string, after *int64, limit int) (outOfBandClaimPageResponse, error) {
	args := []any{engagementID}
	filter := ""
	if after != nil {
		filter = "WHERE latest_event_id > $2"
		args = append(args, *after)
	}
	args = append(args, limit+1)
	query := fmt.Sprintf(`SELECT subject_id, latest_event_id FROM (
		SELECT DISTINCT ON (subject_id) subject_id, id AS latest_event_id
		FROM audit_event
		WHERE engagement_id = $1 AND subject_type = 'out_of_band_claim' AND type IN ('out-of-band.flagged', 'out-of-band.resolved')
		ORDER BY subject_id, subject_revision DESC, id DESC
	) claims %s ORDER BY latest_event_id ASC LIMIT $%d`, filter, len(args))
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return outOfBandClaimPageResponse{}, err
	}
	defer rows.Close()
	pageClaims := make([]struct {
		ClaimID       string
		LatestEventID int64
	}, 0, limit+1)
	for rows.Next() {
		var claimID string
		var latestEventID int64
		if err := rows.Scan(&claimID, &latestEventID); err != nil {
			return outOfBandClaimPageResponse{}, err
		}
		pageClaims = append(pageClaims, struct {
			ClaimID       string
			LatestEventID int64
		}{ClaimID: claimID, LatestEventID: latestEventID})
	}
	if err := rows.Err(); err != nil {
		return outOfBandClaimPageResponse{}, err
	}
	page := outOfBandClaimPageResponse{ContractVersion: outOfBandClaimContractVersion}
	if len(pageClaims) == 0 {
		return page, nil
	}
	hasMore := len(pageClaims) > limit
	if hasMore {
		pageClaims = pageClaims[:limit]
	}
	items := make([]outOfBandClaimItem, 0, len(pageClaims))
	for _, claim := range pageClaims {
		item, err := loadOutOfBandClaim(ctx, db, engagementID, claim.ClaimID)
		if err != nil {
			return outOfBandClaimPageResponse{}, err
		}
		items = append(items, item)
	}
	page.Items = items
	page.Page.HasMore = hasMore
	if hasMore {
		page.Page.NextCursor = eventCursor(pageClaims[len(pageClaims)-1].LatestEventID)
	}
	return page, nil
}

func loadOutOfBandClaim(ctx context.Context, db *sql.DB, engagementID, claimID string) (outOfBandClaimItem, error) {
	rows, err := loadOutOfBandClaimTimeline(ctx, db, engagementID, claimID)
	if err != nil {
		return outOfBandClaimItem{}, err
	}
	return buildOutOfBandClaim(engagementID, rows)
}

func loadOutOfBandClaimTimeline(ctx context.Context, q queryer, engagementID, claimID string) ([]outOfBandClaimEventRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, subject_id, type, subject_revision, occurred_at, actor_id, actor_kind, actor_handle, actor_role, actor_agent_name, actor_model, actor_version, actor_authorized_by, data
		FROM audit_event
		WHERE engagement_id = $1 AND subject_type = 'out_of_band_claim' AND subject_id = $2 AND type IN ('out-of-band.flagged', 'out-of-band.resolved')
		ORDER BY subject_revision ASC, id ASC`, engagementID, claimID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []outOfBandClaimEventRow
	for rows.Next() {
		var row outOfBandClaimEventRow
		var actorAuth sql.NullString
		if err := rows.Scan(&row.EventID, &row.ClaimID, &row.Type, &row.Revision, &row.OccurredAt, &row.Actor.ID, &row.Actor.Kind, &row.Actor.Handle, &row.Actor.Role, &row.Actor.AgentName, &row.Actor.Model, &row.Actor.Version, &actorAuth, &row.Data); err != nil {
			return nil, err
		}
		if actorAuth.Valid {
			row.Actor.AuthorizedBy = actorAuth.String
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, sql.ErrNoRows
	}
	return out, nil
}

func buildOutOfBandClaim(engagementID string, rows []outOfBandClaimEventRow) (outOfBandClaimItem, error) {
	if len(rows) == 0 {
		return outOfBandClaimItem{}, sql.ErrNoRows
	}
	var observed outOfBandClaimObservedData
	if err := json.Unmarshal(rows[0].Data, &observed); err != nil {
		return outOfBandClaimItem{}, err
	}
	item := outOfBandClaimItem{
		ContractVersion:   outOfBandClaimContractVersion,
		ID:                rows[0].ClaimID,
		EngagementID:      engagementID,
		ClaimKind:         observed.ClaimKind,
		ClaimedSubjectID:  observed.ClaimedSubjectID,
		SourceActionID:    observed.SourceActionID,
		DetectionBoundary: observed.DetectionBoundary,
		Reason:            observed.Reason,
		Status:            outOfBandClaimStatusPending,
		ObservedAt:        observed.ObservedAt,
		ObservedBy:        rows[0].Actor,
		Revision:          rows[0].Revision,
	}
	if item.DetectionBoundary == "" {
		item.DetectionBoundary = "best_effort"
	}
	if item.Reason == "" {
		item.Reason = "missing_captured_source_action"
	}
	for _, row := range rows[1:] {
		if row.Type != outOfBandClaimTypeResolved {
			continue
		}
		var resolved outOfBandClaimResolvedData
		if err := json.Unmarshal(row.Data, &resolved); err != nil {
			return outOfBandClaimItem{}, err
		}
		item.Revision = row.Revision
		item.Status = resolved.Resolution
		item.ResolvedAt = &resolved.ResolvedAt
		actor := row.Actor
		item.ResolvedBy = &actor
		item.Notes = resolved.Notes
		if resolved.Resolution == outOfBandClaimStatusLinked {
			item.SourceActionID = resolved.SourceActionID
		} else {
			item.SourceActionID = nil
		}
	}
	return item, nil
}

func claimResolutionMatches(current outOfBandClaimItem, req outOfBandClaimResolutionRequest) bool {
	if current.Status != req.Resolution {
		return false
	}
	if req.Resolution == outOfBandClaimStatusLinked {
		if current.SourceActionID == nil || *current.SourceActionID != req.SourceActionID {
			return false
		}
	}
	if req.Notes == "" {
		return current.Notes == ""
	}
	return current.Notes == req.Notes
}

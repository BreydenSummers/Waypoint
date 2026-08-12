package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	dbutil "waypoint/internal/db"
)

const findingContractVersion = "1.0.0"

type findingPromoteRequest struct {
	SourceActionID    string   `json:"sourceActionId"`
	Title             string   `json:"title"`
	Severity          string   `json:"severity"`
	AffectedEntityIDs []string `json:"affectedEntityIds,omitempty"`
	Remediation       string   `json:"remediation"`
	Status            string   `json:"status"`
}

type findingUpdateRequest struct {
	ExpectedRevision  *int      `json:"expectedRevision,omitempty"`
	Title             *string   `json:"title,omitempty"`
	Severity          *string   `json:"severity,omitempty"`
	AffectedEntityIDs *[]string `json:"affectedEntityIds,omitempty"`
	Remediation       *string   `json:"remediation,omitempty"`
	Status            *string   `json:"status,omitempty"`
}

type findingListResponse struct {
	ContractVersion string        `json:"contractVersion"`
	Items           []findingItem `json:"items"`
}

type findingRevisionsResponse struct {
	ContractVersion string           `json:"contractVersion"`
	Items           []auditEventItem `json:"items"`
}

type findingItem struct {
	ContractVersion   string    `json:"contractVersion"`
	ID                string    `json:"id"`
	EngagementID      string    `json:"engagementId"`
	Title             string    `json:"title"`
	Severity          string    `json:"severity"`
	AffectedEntityIDs []string  `json:"affectedEntityIds"`
	EvidenceActionIDs []string  `json:"evidenceActionIds"`
	Remediation       string    `json:"remediation"`
	Status            string    `json:"status"`
	PromotedBy        string    `json:"promotedBy"`
	PromotedAt        time.Time `json:"promotedAt"`
	Revision          int       `json:"revision"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type findingRow struct {
	ID                string
	EngagementID      string
	Title             string
	Severity          string
	AffectedEntityIDs []string
	EvidenceActionIDs []string
	Remediation       string
	Status            string
	PromotedBy        string
	PromotedAt        sql.NullTime
	Revision          int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type findingActionRow struct {
	ID    string
	Phase string
}

func findingsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: requestIDFromHeader(r.Header.Get("X-Request-ID")), Retryable: true, Detail: "finding operations are unavailable"})
			return
		}

		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{findingContractVersion}})
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

		trimmed := strings.TrimPrefix(path.Clean(r.URL.Path), "/api/v1/findings")
		trimmed = strings.TrimPrefix(trimmed, "/")
		if trimmed == "" || trimmed == "." || trimmed == "promote" {
			if r.Method == http.MethodGet && trimmed != "promote" {
				handleFindingList(w, r, db, actor, reqID)
				return
			}
			if r.Method == http.MethodPost {
				handleFindingPromotion(w, r, db, actor, reqID)
				return
			}
			w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		parts := strings.Split(trimmed, "/")
		if len(parts) == 1 {
			handleFindingItem(w, r, db, actor, reqID, parts[0])
			return
		}
		if len(parts) == 2 && parts[1] == "revisions" {
			handleFindingRevisions(w, r, db, actor, reqID, parts[0])
			return
		}

		http.NotFound(w, r)
	}
}

func handleFindingList(w http.ResponseWriter, r *http.Request, db *sql.DB, actor actorRecord, reqID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	items, err := loadFindingList(ctx, db, actor.EngagementID, strings.TrimSpace(r.URL.Query().Get("status")))
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load findings failed"})
		return
	}
	writeJSONWithHeaders(w, http.StatusOK, findingListResponse{ContractVersion: findingContractVersion, Items: items}, reqID)
}

func handleFindingItem(w http.ResponseWriter, r *http.Request, db *sql.DB, actor actorRecord, reqID, findingID string) {
	if !isUUID(findingID) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "finding id must be a UUID."})
		return
	}

	switch r.Method {
	case http.MethodGet:
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		row, err := loadFindingRow(ctx, db, actor.EngagementID, findingID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "finding not found"})
				return
			}
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load finding failed"})
			return
		}
		writeJSONWithHeaders(w, http.StatusOK, findingItemFromRow(row), reqID)
	case http.MethodPatch:
		if !isFindingOperator(actor) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusForbidden), Status: http.StatusForbidden, Code: "forbidden", RequestID: reqID, Retryable: false, Detail: "only an operator can revise a finding."})
			return
		}
		body, err := ioReadAllLimited(r.Body, 1<<20)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "request body is invalid"})
			return
		}
		var req findingUpdateRequest
		if err := decodeStrictJSON(body, &req); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()})
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
		resp, pb, err := applyFindingUpdate(ctx, tx, actor, findingID, req, reqID)
		if pb != nil {
			pb.RequestID = reqID
			writeProblem(w, *pb)
			return
		}
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: err.Error()})
			return
		}
		if err := tx.Commit(); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "commit finding revision failed"})
			return
		}
		writeJSONWithHeaders(w, http.StatusOK, resp, reqID)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPatch)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	}
}

func handleFindingRevisions(w http.ResponseWriter, r *http.Request, db *sql.DB, actor actorRecord, reqID, findingID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if !isUUID(findingID) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "finding id must be a UUID."})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if _, err := loadFindingRow(ctx, db, actor.EngagementID, findingID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "finding not found"})
			return
		}
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load finding revisions failed"})
		return
	}
	items, err := loadFindingRevisions(ctx, db, actor.EngagementID, findingID)
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load finding revisions failed"})
		return
	}
	writeJSONWithHeaders(w, http.StatusOK, findingRevisionsResponse{ContractVersion: findingContractVersion, Items: items}, reqID)
}

func handleFindingPromotion(w http.ResponseWriter, r *http.Request, db *sql.DB, actor actorRecord, reqID string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if !isFindingOperator(actor) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusForbidden), Status: http.StatusForbidden, Code: "forbidden", RequestID: reqID, Retryable: false, Detail: "only an operator can promote an attack to a finding."})
		return
	}
	body, err := ioReadAllLimited(r.Body, 1<<20)
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "request body is invalid"})
		return
	}
	var req findingPromoteRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()})
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
	resp, pb, err := applyFindingPromotion(ctx, tx, actor, req, reqID)
	if pb != nil {
		pb.RequestID = reqID
		writeProblem(w, *pb)
		return
	}
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: err.Error()})
		return
	}
	if err := tx.Commit(); err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "commit finding promotion failed"})
		return
	}
	writeJSONWithHeaders(w, http.StatusCreated, resp, reqID)
}

func applyFindingPromotion(ctx context.Context, tx *sql.Tx, actor actorRecord, req findingPromoteRequest, reqID string) (findingItem, *captureProblem, error) {
	if !isUUID(req.SourceActionID) {
		return findingItem{}, badField("/sourceActionId", "invalid_uuid", "sourceActionId must be a UUID."), nil
	}
	if strings.TrimSpace(req.Title) == "" {
		return findingItem{}, badField("/title", "missing_field", "title is required."), nil
	}
	if !isFindingSeverity(req.Severity) {
		return findingItem{}, badField("/severity", "invalid_enum", "severity must be one of info, low, medium, high, or critical."), nil
	}
	if strings.TrimSpace(req.Remediation) == "" {
		return findingItem{}, badField("/remediation", "missing_field", "remediation is required."), nil
	}
	if strings.TrimSpace(req.Status) == "" {
		return findingItem{}, badField("/status", "missing_field", "status is required."), nil
	}
	if pb := validateUUIDList(req.AffectedEntityIDs, "/affectedEntityIds"); pb != nil {
		return findingItem{}, pb, nil
	}

	action, pb, err := loadAttackAction(ctx, tx, actor.EngagementID, req.SourceActionID)
	if pb != nil || err != nil {
		return findingItem{}, pb, err
	}
	if action.Phase != "attacks" {
		return findingItem{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "finding_conflict", Retryable: false, Detail: "only attack actions can be promoted."}, nil
	}

	affected := normalizeUUIDs(req.AffectedEntityIDs)
	if len(affected) == 0 {
		derived, err := loadActionAffectedEntities(ctx, tx, actor.EngagementID, req.SourceActionID)
		if err != nil {
			return findingItem{}, nil, err
		}
		affected = derived
	}

	findingID := newUUID()
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO finding (id, engagement_id, title, severity, affected_entity_ids, evidence_action_ids, remediation, status, promoted_by, promoted_at, revision, created_at, updated_at) VALUES ($1, $2, $3, $4, $5::uuid[], $6::uuid[], $7, $8, $9, $10, 1, $10, $10)`, findingID, actor.EngagementID, strings.TrimSpace(req.Title), strings.TrimSpace(req.Severity), uuidArrayLiteral(affected), uuidArrayLiteral([]string{req.SourceActionID}), strings.TrimSpace(req.Remediation), strings.TrimSpace(req.Status), actor.ID, now); err != nil {
		return findingItem{}, nil, err
	}
	if err := appendFindingAuditEvent(ctx, tx, actor, reqID, "finding.promoted", findingID, 1, map[string]any{"sourceActionId": req.SourceActionID, "affectedEntityIds": affected, "evidenceActionIds": []string{req.SourceActionID}, "title": strings.TrimSpace(req.Title), "severity": strings.TrimSpace(req.Severity), "status": strings.TrimSpace(req.Status)}); err != nil {
		return findingItem{}, nil, err
	}
	row, err := loadFindingRow(ctx, tx, actor.EngagementID, findingID)
	if err != nil {
		return findingItem{}, nil, err
	}
	return findingItemFromRow(row), nil, nil
}

func applyFindingUpdate(ctx context.Context, tx *sql.Tx, actor actorRecord, findingID string, req findingUpdateRequest, reqID string) (findingItem, *captureProblem, error) {
	if req.ExpectedRevision == nil {
		return findingItem{}, badField("/expectedRevision", "missing_field", "expectedRevision is required."), nil
	}
	row, err := loadFindingRowForUpdate(ctx, tx, actor.EngagementID, findingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return findingItem{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", Retryable: false, Detail: "finding not found"}, nil
		}
		return findingItem{}, nil, err
	}
	if pb := checkExpectedRevision(req.ExpectedRevision, row.Revision, "/expectedRevision"); pb != nil {
		return findingItem{}, pb, nil
	}

	updated := row
	changed := false
	if req.Title != nil {
		if strings.TrimSpace(*req.Title) == "" {
			return findingItem{}, badField("/title", "missing_field", "title cannot be empty."), nil
		}
		trimmed := strings.TrimSpace(*req.Title)
		if trimmed != updated.Title {
			updated.Title = trimmed
			changed = true
		}
	}
	if req.Severity != nil {
		if !isFindingSeverity(*req.Severity) {
			return findingItem{}, badField("/severity", "invalid_enum", "severity must be one of info, low, medium, high, or critical."), nil
		}
		trimmed := strings.TrimSpace(*req.Severity)
		if trimmed != updated.Severity {
			updated.Severity = trimmed
			changed = true
		}
	}
	if req.Remediation != nil {
		if strings.TrimSpace(*req.Remediation) == "" {
			return findingItem{}, badField("/remediation", "missing_field", "remediation cannot be empty."), nil
		}
		trimmed := strings.TrimSpace(*req.Remediation)
		if trimmed != updated.Remediation {
			updated.Remediation = trimmed
			changed = true
		}
	}
	if req.Status != nil {
		if strings.TrimSpace(*req.Status) == "" {
			return findingItem{}, badField("/status", "missing_field", "status cannot be empty."), nil
		}
		trimmed := strings.TrimSpace(*req.Status)
		if trimmed != updated.Status {
			updated.Status = trimmed
			changed = true
		}
	}
	if req.AffectedEntityIDs != nil {
		if pb := validateUUIDList(*req.AffectedEntityIDs, "/affectedEntityIds"); pb != nil {
			return findingItem{}, pb, nil
		}
		affected := normalizeUUIDs(*req.AffectedEntityIDs)
		if !equalStringSlices(affected, updated.AffectedEntityIDs) {
			updated.AffectedEntityIDs = affected
			changed = true
		}
	}
	if !changed {
		return findingItem{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", Retryable: false, Detail: "at least one finding field must change."}, nil
	}

	updated.Revision = row.Revision + 1
	updated.UpdatedAt = time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `UPDATE finding SET title = $2, severity = $3, affected_entity_ids = $4::uuid[], remediation = $5, status = $6, revision = $7, updated_at = $8 WHERE id = $1`, findingID, updated.Title, updated.Severity, uuidArrayLiteral(updated.AffectedEntityIDs), updated.Remediation, updated.Status, updated.Revision, updated.UpdatedAt); err != nil {
		return findingItem{}, nil, err
	}
	eventType := "finding.revised"
	if row.Status != updated.Status {
		eventType = "finding.status-changed"
	}
	if err := appendFindingAuditEvent(ctx, tx, actor, reqID, eventType, findingID, updated.Revision, map[string]any{"revision": updated.Revision, "title": updated.Title, "severity": updated.Severity, "status": updated.Status, "affectedEntityIds": updated.AffectedEntityIDs, "remediation": updated.Remediation}); err != nil {
		return findingItem{}, nil, err
	}
	row, err = loadFindingRow(ctx, tx, actor.EngagementID, findingID)
	if err != nil {
		return findingItem{}, nil, err
	}
	return findingItemFromRow(row), nil, nil
}

func loadFindingList(ctx context.Context, q queryer, engagementID, status string) ([]findingItem, error) {
	query := `SELECT id, engagement_id, title, severity, COALESCE(array_to_json(affected_entity_ids)::text, '[]'), COALESCE(array_to_json(evidence_action_ids)::text, '[]'), remediation, status, COALESCE(promoted_by::text, ''), promoted_at, revision, created_at, updated_at FROM finding WHERE engagement_id = $1`
	args := []any{engagementID}
	if strings.TrimSpace(status) != "" {
		query += ` AND status = $2`
		args = append(args, strings.TrimSpace(status))
	}
	query += ` ORDER BY promoted_at DESC, updated_at DESC, id DESC`
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]findingItem, 0)
	for rows.Next() {
		row, err := scanFindingRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, findingItemFromRow(row))
	}
	return items, rows.Err()
}

func loadFindingRow(ctx context.Context, q queryer, engagementID, findingID string) (findingRow, error) {
	row := findingRow{}
	var affectedJSON, evidenceJSON string
	if err := q.QueryRowContext(ctx, `SELECT id, engagement_id, title, severity, COALESCE(array_to_json(affected_entity_ids)::text, '[]'), COALESCE(array_to_json(evidence_action_ids)::text, '[]'), remediation, status, COALESCE(promoted_by::text, ''), promoted_at, revision, created_at, updated_at FROM finding WHERE engagement_id = $1 AND id = $2`, engagementID, findingID).Scan(&row.ID, &row.EngagementID, &row.Title, &row.Severity, &affectedJSON, &evidenceJSON, &row.Remediation, &row.Status, &row.PromotedBy, &row.PromotedAt, &row.Revision, &row.CreatedAt, &row.UpdatedAt); err != nil {
		return findingRow{}, err
	}
	if err := decodeJSONArray(affectedJSON, &row.AffectedEntityIDs); err != nil {
		return findingRow{}, err
	}
	if err := decodeJSONArray(evidenceJSON, &row.EvidenceActionIDs); err != nil {
		return findingRow{}, err
	}
	return row, nil
}

func loadFindingRowForUpdate(ctx context.Context, tx *sql.Tx, engagementID, findingID string) (findingRow, error) {
	row := findingRow{}
	var affectedJSON, evidenceJSON string
	if err := tx.QueryRowContext(ctx, `SELECT id, engagement_id, title, severity, COALESCE(array_to_json(affected_entity_ids)::text, '[]'), COALESCE(array_to_json(evidence_action_ids)::text, '[]'), remediation, status, COALESCE(promoted_by::text, ''), promoted_at, revision, created_at, updated_at FROM finding WHERE engagement_id = $1 AND id = $2 FOR UPDATE`, engagementID, findingID).Scan(&row.ID, &row.EngagementID, &row.Title, &row.Severity, &affectedJSON, &evidenceJSON, &row.Remediation, &row.Status, &row.PromotedBy, &row.PromotedAt, &row.Revision, &row.CreatedAt, &row.UpdatedAt); err != nil {
		return findingRow{}, err
	}
	if err := decodeJSONArray(affectedJSON, &row.AffectedEntityIDs); err != nil {
		return findingRow{}, err
	}
	if err := decodeJSONArray(evidenceJSON, &row.EvidenceActionIDs); err != nil {
		return findingRow{}, err
	}
	return row, nil
}

func scanFindingRow(scanner interface{ Scan(...any) error }) (findingRow, error) {
	row := findingRow{}
	var affectedJSON, evidenceJSON string
	if err := scanner.Scan(&row.ID, &row.EngagementID, &row.Title, &row.Severity, &affectedJSON, &evidenceJSON, &row.Remediation, &row.Status, &row.PromotedBy, &row.PromotedAt, &row.Revision, &row.CreatedAt, &row.UpdatedAt); err != nil {
		return findingRow{}, err
	}
	if err := decodeJSONArray(affectedJSON, &row.AffectedEntityIDs); err != nil {
		return findingRow{}, err
	}
	if err := decodeJSONArray(evidenceJSON, &row.EvidenceActionIDs); err != nil {
		return findingRow{}, err
	}
	return row, nil
}

func loadFindingRevisions(ctx context.Context, q queryer, engagementID, findingID string) ([]auditEventItem, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, type, engagement_id, actor_id, actor_kind, actor_handle, actor_role, COALESCE(actor_agent_name, ''), COALESCE(actor_model, ''), COALESCE(actor_version, ''), COALESCE(actor_authorized_by::text, ''), occurred_at, origin_kind, COALESCE(origin_service, ''), subject_type, subject_id, subject_revision, request_id, correlation_id, COALESCE(causation_action_id::text, ''), COALESCE(causation_event_id::text, ''), data::text FROM audit_event WHERE engagement_id = $1 AND subject_type = 'finding' AND subject_id = $2 ORDER BY id ASC`, engagementID, findingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]auditEventItem, 0)
	for rows.Next() {
		item, err := scanAuditEvent(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func loadAttackAction(ctx context.Context, q queryer, engagementID, actionID string) (findingActionRow, *captureProblem, error) {
	var row findingActionRow
	if err := q.QueryRowContext(ctx, `SELECT id, phase FROM action WHERE engagement_id = $1 AND id = $2`, engagementID, actionID).Scan(&row.ID, &row.Phase); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return findingActionRow{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "finding_conflict", Retryable: false, Detail: "source action not found."}, nil
		}
		return findingActionRow{}, nil, err
	}
	return row, nil, nil
}

func loadActionAffectedEntities(ctx context.Context, q queryer, engagementID, actionID string) ([]string, error) {
	var affectedJSON string
	if err := q.QueryRowContext(ctx, `SELECT COALESCE(array_to_json(array_agg(DISTINCT entity_id::text ORDER BY entity_id::text))::text, '[]') FROM observation WHERE engagement_id = $1 AND action_id = $2 AND entity_id IS NOT NULL`, engagementID, actionID).Scan(&affectedJSON); err != nil {
		return nil, err
	}
	var affected []string
	if err := decodeJSONArray(affectedJSON, &affected); err != nil {
		return nil, err
	}
	return normalizeUUIDs(affected), nil
}

func appendFindingAuditEvent(ctx context.Context, tx *sql.Tx, actor actorRecord, reqID, eventType, subjectID string, subjectRevision int, data map[string]any) error {
	_, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
		EngagementID:  actor.EngagementID,
		Type:          eventType,
		Actor:         dbutil.AuditActorSnapshot{ID: actor.ID, Kind: actor.Kind, Handle: actor.Handle, Role: actor.Role, AgentName: actor.AgentName, Model: actor.Model, Version: actor.Version, AuthorizedBy: actor.AuthorizedBy},
		Origin:        dbutil.AuditOrigin{Kind: "rest"},
		Subject:       dbutil.AuditSubject{Type: "finding", ID: subjectID, Revision: subjectRevision},
		RequestID:     reqID,
		CorrelationID: reqID,
		Data:          data,
	})
	return err
}

func findingItemFromRow(row findingRow) findingItem {
	item := findingItem{
		ContractVersion:   findingContractVersion,
		ID:                row.ID,
		EngagementID:      row.EngagementID,
		Title:             row.Title,
		Severity:          row.Severity,
		AffectedEntityIDs: append([]string(nil), row.AffectedEntityIDs...),
		EvidenceActionIDs: append([]string(nil), row.EvidenceActionIDs...),
		Remediation:       row.Remediation,
		Status:            row.Status,
		PromotedBy:        row.PromotedBy,
		Revision:          row.Revision,
		CreatedAt:         row.CreatedAt.UTC(),
		UpdatedAt:         row.UpdatedAt.UTC(),
	}
	if row.PromotedAt.Valid {
		item.PromotedAt = row.PromotedAt.Time.UTC()
	}
	return item
}

func isFindingSeverity(v string) bool {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case "info", "low", "medium", "high", "critical":
		return true
	default:
		return false
	}
}

func validateUUIDList(values []string, pointer string) *captureProblem {
	for _, raw := range values {
		if strings.TrimSpace(raw) == "" || !isUUID(strings.TrimSpace(raw)) {
			return badField(pointer, "invalid_uuid", "affectedEntityIds must contain UUIDs.")
		}
	}
	return nil
}

func normalizeUUIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" || !isUUID(v) {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func isFindingOperator(actor actorRecord) bool {
	return actor.Kind == "human" && actor.Role == "operator"
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func decodeJSONArray(data string, dst *[]string) error {
	if strings.TrimSpace(data) == "" || strings.TrimSpace(data) == "null" {
		*dst = nil
		return nil
	}
	return json.Unmarshal([]byte(data), dst)
}

func uuidArrayLiteral(values []string) string {
	if len(values) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func ioReadAllLimited(r io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, limit))
}

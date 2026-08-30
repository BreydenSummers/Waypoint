package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	dbutil "waypoint/internal/db"
)

const actorContractVersion = "1.0.0"

var quotedRevisionPattern = regexp.MustCompile(`^"([1-9][0-9]*)"$`)

const (
	actorStatusActive  = "active"
	actorStatusRevoked = "revoked"
)

type actorProvisionRequest struct {
	Kind         string `json:"kind"`
	Handle       string `json:"handle"`
	Role         string `json:"role"`
	AgentName    string `json:"agentName,omitempty"`
	Model        string `json:"model,omitempty"`
	Version      string `json:"version,omitempty"`
	AuthorizedBy string `json:"authorizedBy,omitempty"`
}

type actorCredentialResponse struct {
	ContractVersion string               `json:"contractVersion"`
	ActorRecord     actorLifecycleRecord `json:"actorRecord"`
	Token           string               `json:"token"`
	IssuedAt        time.Time            `json:"issuedAt"`
}

type actorPageResponse struct {
	ContractVersion string                 `json:"contractVersion"`
	Items           []actorLifecycleRecord `json:"items"`
	Page            actorPageMeta          `json:"page"`
}

type actorPageMeta struct {
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}

type actorLifecycleRecord struct {
	ContractVersion   string          `json:"contractVersion"`
	EngagementID      string          `json:"engagementId"`
	Actor             auditEventActor `json:"actor"`
	Status            string          `json:"status"`
	CredentialVersion int             `json:"credentialVersion"`
	CreatedAt         time.Time       `json:"createdAt"`
	CreatedBy         string          `json:"createdBy"`
	LastRotatedAt     *time.Time      `json:"lastRotatedAt,omitempty"`
	LastRotatedBy     string          `json:"lastRotatedBy,omitempty"`
	RevokedAt         *time.Time      `json:"revokedAt,omitempty"`
	RevokedBy         string          `json:"revokedBy,omitempty"`
	Revision          int             `json:"revision"`
}

type actorLifecycleRow struct {
	ID                string
	EngagementID      string
	Kind              string
	Handle            string
	Role              string
	AgentName         string
	Model             string
	Version           string
	AuthorizedBy      sql.NullString
	Status            string
	CredentialVersion int
	CreatedAt         time.Time
	CreatedBy         string
	LastRotatedAt     sql.NullTime
	LastRotatedBy     sql.NullString
	RevokedAt         sql.NullTime
	RevokedBy         sql.NullString
	Revision          int
}

func actorHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if db == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: reqID, Retryable: true, Detail: "actor lifecycle is unavailable"})
			return
		}
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{actorContractVersion}})
			return
		}

		token, err := bearerToken(r.Header.Get("Authorization"))
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUnauthorized), Status: http.StatusUnauthorized, Code: "unauthenticated", RequestID: reqID, Retryable: false, Detail: err.Error()})
			return
		}
		authActor, err := lookupActor(r.Context(), db, token)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUnauthorized), Status: http.StatusUnauthorized, Code: "unauthenticated", RequestID: reqID, Retryable: false, Detail: "invalid actor credential"})
			return
		}
		if !isLifecycleOperator(authActor) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusForbidden), Status: http.StatusForbidden, Code: "insufficient_permission", RequestID: reqID, Retryable: false, Detail: "actor lifecycle requires a human operator or owner credential"})
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
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			page, err := loadActorPage(ctx, db, authActor.EngagementID, limit)
			if err != nil {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load actors failed"})
				return
			}
			writeJSONWithHeaders(w, http.StatusOK, page, reqID)
		case http.MethodPost:
			body, err := ioReadAllLimited(r.Body, 1<<20)
			if err != nil {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "request body is invalid"})
				return
			}
			var req actorProvisionRequest
			if err := decodeStrictJSON(body, &req); err != nil {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			resp, pb, err := provisionActor(ctx, db, authActor, reqID, req)
			// provisionActor returns validation/conflict problems as (resp{}, pb, nil);
			// surface pb whether or not err is set, or a rejected provision (e.g. an
			// invalid authorizer) is silently written back as 201 with an empty body.
			if pb != nil {
				writeProblem(w, *pb)
				return
			}
			if err != nil {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "provision actor failed"})
				return
			}
			writeJSONWithHeaders(w, http.StatusCreated, resp, reqID)
		}
	}
}

func actorResourceHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if db == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: reqID, Retryable: true, Detail: "actor lifecycle is unavailable"})
			return
		}
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{actorContractVersion}})
			return
		}

		token, err := bearerToken(r.Header.Get("Authorization"))
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUnauthorized), Status: http.StatusUnauthorized, Code: "unauthenticated", RequestID: reqID, Retryable: false, Detail: err.Error()})
			return
		}
		authActor, err := lookupActor(r.Context(), db, token)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUnauthorized), Status: http.StatusUnauthorized, Code: "unauthenticated", RequestID: reqID, Retryable: false, Detail: "invalid actor credential"})
			return
		}
		if !isLifecycleOperator(authActor) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusForbidden), Status: http.StatusForbidden, Code: "insufficient_permission", RequestID: reqID, Retryable: false, Detail: "actor lifecycle requires a human operator or owner credential"})
			return
		}

		relative := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api/v1/actors/"), "/actors/")
		relative = strings.Trim(relative, "/")
		if relative == "" {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "actor not found"})
			return
		}

		if strings.HasSuffix(r.URL.Path, "/rotate") {
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
				return
			}
			actorID := strings.TrimSuffix(relative, "/rotate")
			actorID = strings.Trim(actorID, "/")
			if actorID == "" {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "actor not found"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			resp, pb, err := rotateActorCredential(ctx, db, authActor, actorID, reqID, r.Header.Get("If-Match"))
			if err != nil {
				if pb != nil {
					writeProblem(w, *pb)
					return
				}
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "rotate actor failed"})
				return
			}
			writeJSONWithHeaders(w, http.StatusCreated, resp, reqID)
			return
		}

		if strings.HasSuffix(r.URL.Path, "/revoke") {
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
				return
			}
			actorID := strings.TrimSuffix(relative, "/revoke")
			actorID = strings.Trim(actorID, "/")
			if actorID == "" {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "actor not found"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			resp, pb, err := revokeActor(ctx, db, authActor, actorID, reqID, r.Header.Get("If-Match"))
			if err != nil {
				if pb != nil {
					writeProblem(w, *pb)
					return
				}
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "revoke actor failed"})
				return
			}
			writeJSONWithHeaders(w, http.StatusOK, resp, reqID)
			return
		}

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		item, err := loadActorRecord(ctx, db, authActor.EngagementID, relative)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "actor not found"})
				return
			}
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load actor failed"})
			return
		}
		writeJSONWithHeaders(w, http.StatusOK, item, reqID)
	}
}

func loadActorPage(ctx context.Context, q queryer, engagementID string, limit int) (actorPageResponse, error) {
	if limit < 1 {
		limit = 100
	}
	rows, err := q.QueryContext(ctx, `SELECT id, engagement_id, kind, handle, role, COALESCE(agent_name, ''), COALESCE(model, ''), COALESCE(version, ''), COALESCE(authorized_by::text, ''), CASE WHEN revoked_at IS NULL THEN 'active' ELSE 'revoked' END, credential_version, created_at, COALESCE(created_by::text, ''), last_rotated_at, COALESCE(last_rotated_by::text, ''), revoked_at, COALESCE(revoked_by::text, ''), revision FROM actor WHERE engagement_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`, engagementID, limit)
	if err != nil {
		return actorPageResponse{}, err
	}
	defer rows.Close()
	items := make([]actorLifecycleRecord, 0, limit)
	for rows.Next() {
		row, err := scanActorLifecycleRow(rows)
		if err != nil {
			return actorPageResponse{}, err
		}
		items = append(items, actorLifecycleRecordFromRow(row))
	}
	if err := rows.Err(); err != nil {
		return actorPageResponse{}, err
	}
	return actorPageResponse{ContractVersion: actorContractVersion, Items: items, Page: actorPageMeta{HasMore: false}}, nil
}

func loadActorRecord(ctx context.Context, q queryer, engagementID, actorID string) (actorLifecycleRecord, error) {
	row, err := loadActorLifecycleRow(ctx, q, engagementID, actorID)
	if err != nil {
		return actorLifecycleRecord{}, err
	}
	return actorLifecycleRecordFromRow(row), nil
}

func scanActorLifecycleRow(scanner interface{ Scan(dest ...any) error }) (actorLifecycleRow, error) {
	var row actorLifecycleRow
	var lastRotatedAt sql.NullTime
	var lastRotatedBy sql.NullString
	var revokedAt sql.NullTime
	var revokedBy sql.NullString
	if err := scanner.Scan(&row.ID, &row.EngagementID, &row.Kind, &row.Handle, &row.Role, &row.AgentName, &row.Model, &row.Version, &row.AuthorizedBy, &row.Status, &row.CredentialVersion, &row.CreatedAt, &row.CreatedBy, &lastRotatedAt, &lastRotatedBy, &revokedAt, &revokedBy, &row.Revision); err != nil {
		return actorLifecycleRow{}, err
	}
	if lastRotatedAt.Valid {
		row.LastRotatedAt = lastRotatedAt
	}
	if lastRotatedBy.Valid {
		row.LastRotatedBy = lastRotatedBy
	}
	if revokedAt.Valid {
		row.RevokedAt = revokedAt
	}
	if revokedBy.Valid {
		row.RevokedBy = revokedBy
	}
	return row, nil
}

func actorLifecycleRecordFromRow(row actorLifecycleRow) actorLifecycleRecord {
	record := actorLifecycleRecord{
		ContractVersion:   actorContractVersion,
		EngagementID:      row.EngagementID,
		Actor:             auditEventActor{ID: row.ID, Kind: row.Kind, Handle: row.Handle, Role: row.Role, AgentName: row.AgentName, Model: row.Model, Version: row.Version},
		Status:            row.Status,
		CredentialVersion: row.CredentialVersion,
		CreatedAt:         row.CreatedAt.UTC(),
		CreatedBy:         row.CreatedBy,
		Revision:          row.Revision,
	}
	if row.AuthorizedBy.Valid {
		record.Actor.AuthorizedBy = row.AuthorizedBy.String
	}
	if row.LastRotatedAt.Valid {
		rotated := row.LastRotatedAt.Time.UTC()
		record.LastRotatedAt = &rotated
	}
	if row.LastRotatedBy.Valid {
		record.LastRotatedBy = row.LastRotatedBy.String
	}
	if row.RevokedAt.Valid {
		revoked := row.RevokedAt.Time.UTC()
		record.RevokedAt = &revoked
	}
	if row.RevokedBy.Valid {
		record.RevokedBy = row.RevokedBy.String
	}
	if record.CreatedBy == "" {
		record.CreatedBy = row.ID
	}
	return record
}

func provisionActor(ctx context.Context, db *sql.DB, auth actorRecord, reqID string, req actorProvisionRequest) (actorCredentialResponse, *captureProblem, error) {
	if req.Kind != "human" && req.Kind != "ai_agent" {
		return actorCredentialResponse{}, badField("/kind", "invalid_enum", "kind must be human or ai_agent."), nil
	}
	if strings.TrimSpace(req.Handle) == "" || len(req.Handle) > 128 || strings.ContainsAny(req.Handle, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\x7f") {
		return actorCredentialResponse{}, badField("/handle", "invalid_value", "handle must be a non-empty printable string up to 128 characters."), nil
	}
	if req.Role != "owner" && req.Role != "operator" && req.Role != "viewer" {
		return actorCredentialResponse{}, badField("/role", "invalid_enum", "role must be owner, operator, or viewer."), nil
	}
	if req.Kind == "ai_agent" {
		if strings.TrimSpace(req.AgentName) == "" {
			return actorCredentialResponse{}, badField("/agentName", "missing_field", "agentName is required for ai_agent actors."), nil
		}
		if strings.TrimSpace(req.Model) == "" {
			return actorCredentialResponse{}, badField("/model", "missing_field", "model is required for ai_agent actors."), nil
		}
		if strings.TrimSpace(req.Version) == "" {
			return actorCredentialResponse{}, badField("/version", "missing_field", "version is required for ai_agent actors."), nil
		}
		if strings.TrimSpace(req.AuthorizedBy) == "" {
			return actorCredentialResponse{}, badField("/authorizedBy", "missing_field", "authorizedBy is required for ai_agent actors."), nil
		}
		if !isUUID(req.AuthorizedBy) {
			return actorCredentialResponse{}, badField("/authorizedBy", "invalid_uuid", "authorizedBy must be a UUID."), nil
		}
	} else if strings.TrimSpace(req.AgentName) != "" || strings.TrimSpace(req.Model) != "" || strings.TrimSpace(req.Version) != "" || strings.TrimSpace(req.AuthorizedBy) != "" {
		return actorCredentialResponse{}, badField("/kind", "shape_mismatch", "human actors must not include AI authorizer fields."), nil
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var existingID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM actor WHERE engagement_id = $1 AND handle = $2 LIMIT 1`, auth.EngagementID, req.Handle).Scan(&existingID); err == nil {
		return actorCredentialResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "actor_conflict", RequestID: reqID, Retryable: false, Detail: "handle already exists in this engagement."}, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return actorCredentialResponse{}, nil, err
	}

	var authorizerID string
	if req.Kind == "ai_agent" {
		var authorizerKind, authorizerStatus string
		if err := db.QueryRowContext(ctx, `SELECT id, kind, CASE WHEN revoked_at IS NULL THEN 'active' ELSE 'revoked' END FROM actor WHERE engagement_id = $1 AND id = $2 LIMIT 1`, auth.EngagementID, req.AuthorizedBy).Scan(&authorizerID, &authorizerKind, &authorizerStatus); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return actorCredentialResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "authorizedBy actor was not found in this engagement."}, nil
			}
			return actorCredentialResponse{}, nil, err
		}
		if authorizerKind != "human" {
			return actorCredentialResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "invalid_authorizer", RequestID: reqID, Retryable: false, Detail: "authorizedBy must reference an active human actor."}, nil
		}
		if authorizerStatus != actorStatusActive {
			return actorCredentialResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "invalid_authorizer", RequestID: reqID, Retryable: false, Detail: "authorizedBy must reference an active human actor."}, nil
		}
	}

	token, err := generateActorToken()
	if err != nil {
		return actorCredentialResponse{}, nil, err
	}
	tokenHash := sha256Hex(token)
	issuedAt := time.Now().UTC()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return actorCredentialResponse{}, nil, err
	}
	defer tx.Rollback()

	var authorizedBy any
	if req.Kind == "ai_agent" {
		authorizedBy = authorizerID
	} else {
		authorizedBy = nil
	}
	var createdID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO actor (
			engagement_id,
			kind,
			handle,
			token_hash,
			role,
			agent_name,
			model,
			version,
			authorized_by,
			created_by,
			credential_version,
			revision
		) VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), $9, $10, 1, 1)
		RETURNING id`, auth.EngagementID, req.Kind, req.Handle, tokenHash, req.Role, req.AgentName, req.Model, req.Version, authorizedBy, auth.ID).Scan(&createdID); err != nil {
		return actorCredentialResponse{}, nil, err
	}

	record, err := loadActorRecord(ctx, tx, auth.EngagementID, createdID)
	if err != nil {
		return actorCredentialResponse{}, nil, err
	}
	data := map[string]any{"actorId": createdID, "kind": req.Kind, "role": req.Role, "credentialVersion": 1}
	if req.Kind == "ai_agent" {
		data["authorizedBy"] = authorizerID
	}
	if _, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
		EngagementID:  auth.EngagementID,
		Type:          "actor.provisioned",
		Actor:         auditActorSnapshot(auth),
		Origin:        dbutil.AuditOrigin{Kind: "rest"},
		Subject:       dbutil.AuditSubject{Type: "actor", ID: createdID, Revision: 1},
		RequestID:     reqID,
		CorrelationID: reqID,
		Data:          data,
	}); err != nil {
		return actorCredentialResponse{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return actorCredentialResponse{}, nil, err
	}
	_ = record
	return actorCredentialResponse{ContractVersion: actorContractVersion, ActorRecord: record, Token: token, IssuedAt: issuedAt}, nil, nil
}

func rotateActorCredential(ctx context.Context, db *sql.DB, auth actorRecord, actorID, reqID, ifMatch string) (actorCredentialResponse, *captureProblem, error) {
	expectedRevision, pb := parseQuotedRevision(ifMatch, true)
	if pb != nil {
		pb.RequestID = reqID
		return actorCredentialResponse{}, pb, nil
	}
	return mutateActorCredential(ctx, db, auth, actorID, reqID, expectedRevision, false)
}

func revokeActor(ctx context.Context, db *sql.DB, auth actorRecord, actorID, reqID, ifMatch string) (actorLifecycleRecord, *captureProblem, error) {
	expectedRevision, pb := parseQuotedRevision(ifMatch, true)
	if pb != nil {
		pb.RequestID = reqID
		return actorLifecycleRecord{}, pb, nil
	}
	return mutateActorRevocation(ctx, db, auth, actorID, reqID, expectedRevision)
}

func mutateActorCredential(ctx context.Context, db *sql.DB, auth actorRecord, actorID, reqID string, expectedRevision int, revoke bool) (actorCredentialResponse, *captureProblem, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return actorCredentialResponse{}, nil, err
	}
	defer tx.Rollback()
	row, err := loadActorLifecycleRow(ctx, tx, auth.EngagementID, actorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return actorCredentialResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "actor not found"}, nil
		}
		return actorCredentialResponse{}, nil, err
	}
	if row.Status != actorStatusActive {
		return actorCredentialResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "actor_revoked", RequestID: reqID, Retryable: false, Detail: "actor is revoked and cannot be rotated."}, nil
	}
	if expectedRevision != row.Revision {
		return actorCredentialResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusPreconditionFailed), Status: http.StatusPreconditionFailed, Code: "precondition_failed", RequestID: reqID, Retryable: false, Detail: "If-Match does not match the current actor revision."}, nil
	}
	token, err := generateActorToken()
	if err != nil {
		return actorCredentialResponse{}, nil, err
	}
	tokenHash := sha256Hex(token)
	rotatedAt := time.Now().UTC()
	newRevision := row.Revision + 1
	newCredentialVersion := row.CredentialVersion + 1
	res := actorCredentialResponse{}
	if err := tx.QueryRowContext(ctx, `
		UPDATE actor
		SET token_hash = $1,
		    credential_version = $2,
		    revision = $3,
		    last_rotated_at = $4,
		    last_rotated_by = $5,
		    updated_at = now()
		WHERE engagement_id = $6 AND id = $7 AND revision = $8 AND revoked_at IS NULL
		RETURNING id`, tokenHash, newCredentialVersion, newRevision, rotatedAt, auth.ID, auth.EngagementID, actorID, row.Revision).Scan(&actorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return actorCredentialResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusPreconditionFailed), Status: http.StatusPreconditionFailed, Code: "precondition_failed", RequestID: reqID, Retryable: false, Detail: "If-Match does not match the current actor revision."}, nil
		}
		return actorCredentialResponse{}, nil, err
	}
	updated, err := loadActorRecord(ctx, tx, auth.EngagementID, actorID)
	if err != nil {
		return actorCredentialResponse{}, nil, err
	}
	if _, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
		EngagementID:  auth.EngagementID,
		Type:          "actor.credential-rotated",
		Actor:         auditActorSnapshot(auth),
		Origin:        dbutil.AuditOrigin{Kind: "rest"},
		Subject:       dbutil.AuditSubject{Type: "actor", ID: actorID, Revision: updated.Revision},
		RequestID:     reqID,
		CorrelationID: reqID,
		Data: map[string]any{
			"actorId":                   actorID,
			"previousCredentialVersion": row.CredentialVersion,
			"credentialVersion":         updated.CredentialVersion,
			"rotatedAt":                 rotatedAt,
		},
	}); err != nil {
		return actorCredentialResponse{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return actorCredentialResponse{}, nil, err
	}
	res.ContractVersion = actorContractVersion
	res.ActorRecord = updated
	res.Token = token
	res.IssuedAt = rotatedAt
	return res, nil, nil
}

func mutateActorRevocation(ctx context.Context, db *sql.DB, auth actorRecord, actorID, reqID string, expectedRevision int) (actorLifecycleRecord, *captureProblem, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return actorLifecycleRecord{}, nil, err
	}
	defer tx.Rollback()
	row, err := loadActorLifecycleRow(ctx, tx, auth.EngagementID, actorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return actorLifecycleRecord{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "actor not found"}, nil
		}
		return actorLifecycleRecord{}, nil, err
	}
	if row.Status != actorStatusActive {
		return actorLifecycleRecord{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "actor_revoked", RequestID: reqID, Retryable: false, Detail: "actor is already revoked."}, nil
	}
	if expectedRevision != row.Revision {
		return actorLifecycleRecord{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusPreconditionFailed), Status: http.StatusPreconditionFailed, Code: "precondition_failed", RequestID: reqID, Retryable: false, Detail: "If-Match does not match the current actor revision."}, nil
	}
	revokedAt := time.Now().UTC()
	newRevision := row.Revision + 1
	if err := tx.QueryRowContext(ctx, `
		UPDATE actor
		SET revoked_at = $1,
		    revoked_by = $2,
		    revision = $3,
		    updated_at = now()
		WHERE engagement_id = $4 AND id = $5 AND revision = $6 AND revoked_at IS NULL
		RETURNING id`, revokedAt, auth.ID, newRevision, auth.EngagementID, actorID, row.Revision).Scan(&actorID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return actorLifecycleRecord{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusPreconditionFailed), Status: http.StatusPreconditionFailed, Code: "precondition_failed", RequestID: reqID, Retryable: false, Detail: "If-Match does not match the current actor revision."}, nil
		}
		return actorLifecycleRecord{}, nil, err
	}
	updated, err := loadActorRecord(ctx, tx, auth.EngagementID, actorID)
	if err != nil {
		return actorLifecycleRecord{}, nil, err
	}
	if _, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
		EngagementID:  auth.EngagementID,
		Type:          "actor.revoked",
		Actor:         auditActorSnapshot(auth),
		Origin:        dbutil.AuditOrigin{Kind: "rest"},
		Subject:       dbutil.AuditSubject{Type: "actor", ID: actorID, Revision: updated.Revision},
		RequestID:     reqID,
		CorrelationID: reqID,
		Data: map[string]any{
			"actorId":           actorID,
			"credentialVersion": row.CredentialVersion,
			"revokedAt":         revokedAt,
		},
	}); err != nil {
		return actorLifecycleRecord{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return actorLifecycleRecord{}, nil, err
	}
	return updated, nil, nil
}

func loadActorLifecycleRow(ctx context.Context, q queryer, engagementID, actorID string) (actorLifecycleRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, engagement_id, kind, handle, role, COALESCE(agent_name, ''), COALESCE(model, ''), COALESCE(version, ''), COALESCE(authorized_by::text, ''), CASE WHEN revoked_at IS NULL THEN 'active' ELSE 'revoked' END, credential_version, created_at, COALESCE(created_by::text, ''), last_rotated_at, COALESCE(last_rotated_by::text, ''), revoked_at, COALESCE(revoked_by::text, ''), revision FROM actor WHERE engagement_id = $1 AND id = $2 LIMIT 1`, engagementID, actorID)
	if err != nil {
		return actorLifecycleRow{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return actorLifecycleRow{}, err
		}
		return actorLifecycleRow{}, sql.ErrNoRows
	}
	return scanActorLifecycleRow(rows)
}

func isLifecycleOperator(actor actorRecord) bool {
	return actor.Kind == "human" && (actor.Role == "owner" || actor.Role == "operator")
}

func generateActorToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "wp_actor_" + strings.TrimRight(base64.RawURLEncoding.EncodeToString(buf), "="), nil
}

func parseQuotedRevision(v string, required bool) (int, *captureProblem) {
	v = strings.TrimSpace(v)
	if v == "" {
		if required {
			return 0, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusPreconditionRequired), Status: http.StatusPreconditionRequired, Code: "precondition_required", Retryable: false, Detail: "If-Match is required."}
		}
		return 0, nil
	}
	m := quotedRevisionPattern.FindStringSubmatch(v)
	if len(m) != 2 {
		return 0, badField("/ifMatch", "invalid_value", "If-Match must be a quoted positive integer revision.")
	}
	rev, err := strconv.Atoi(m[1])
	if err != nil || rev < 1 {
		return 0, badField("/ifMatch", "invalid_value", "If-Match must be a quoted positive integer revision.")
	}
	return rev, nil
}

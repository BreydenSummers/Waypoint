package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	dbutil "waypoint/internal/db"
)

const entityContractVersion = "1.0.0"

type entityMergeRequest struct {
	SourceEntityID         string `json:"sourceEntityId"`
	TargetEntityID         string `json:"targetEntityId"`
	Preview                bool   `json:"preview"`
	ObservationID          string `json:"observationId,omitempty"`
	ExpectedSourceRevision *int   `json:"expectedSourceRevision,omitempty"`
	ExpectedTargetRevision *int   `json:"expectedTargetRevision,omitempty"`
}

type entitySplitRequest struct {
	EntityID               string `json:"entityId"`
	Preview                bool   `json:"preview"`
	ObservationID          string `json:"observationId"`
	ExpectedSourceRevision *int   `json:"expectedSourceRevision,omitempty"`
	ExpectedTargetRevision *int   `json:"expectedTargetRevision,omitempty"`
}

type entityMutationResponse struct {
	ContractVersion  string         `json:"contractVersion"`
	Preview          bool           `json:"preview"`
	Applied          bool           `json:"applied"`
	Source           entitySummary  `json:"source"`
	Target           *entitySummary `json:"target,omitempty"`
	ObservationID    string         `json:"observationId,omitempty"`
	AuditEventCursor string         `json:"auditEventCursor,omitempty"`
}

type entitySummary struct {
	ID                 string  `json:"id"`
	Kind               string  `json:"kind"`
	KeyType            string  `json:"keyType"`
	KeyValue           string  `json:"keyValue"`
	Revision           int     `json:"revision"`
	MergedIntoEntityID *string `json:"mergedIntoEntityId,omitempty"`
	ObservationCount   int     `json:"observationCount"`
}

type entityRow struct {
	ID                 string
	EngagementID       string
	Kind               string
	KeyType            string
	KeyValue           string
	Revision           int
	MergedIntoEntityID sql.NullString
	Attributes         json.RawMessage
	FirstSeen          time.Time
	LastSeen           time.Time
}

type entityObservation struct {
	ID          string
	EntityID    string
	Kind        string
	Identifiers json.RawMessage
	Attributes  json.RawMessage
}

func mergeEntityHandler(db *sql.DB) http.HandlerFunc {
	return entityMutationHandler(db, "merge")
}

func splitEntityHandler(db *sql.DB) http.HandlerFunc {
	return entityMutationHandler(db, "split")
}

func entityMutationHandler(db *sql.DB, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if db == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: reqID, Retryable: true, Detail: "entity mutation is unavailable"})
			return
		}
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{entityContractVersion}})
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

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
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

		var pb *captureProblem
		var resp entityMutationResponse
		switch kind {
		case "merge":
			var req entityMergeRequest
			if err := decodeStrictJSON(body, &req); err != nil {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()})
				return
			}
			resp, pb, err = applyEntityMerge(ctx, tx, actor, req, reqID)
		case "split":
			var req entitySplitRequest
			if err := decodeStrictJSON(body, &req); err != nil {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()})
				return
			}
			resp, pb, err = applyEntitySplit(ctx, tx, actor, req, reqID)
		default:
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "unsupported entity mutation"})
			return
		}
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
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "commit entity mutation failed"})
			return
		}
		writeJSONWithHeaders(w, http.StatusOK, resp, reqID)
	}
}

func applyEntityMerge(ctx context.Context, tx *sql.Tx, actor actorRecord, req entityMergeRequest, reqID string) (entityMutationResponse, *captureProblem, error) {
	if !isUUID(req.SourceEntityID) || !isUUID(req.TargetEntityID) {
		return entityMutationResponse{}, badField("/sourceEntityId", "invalid_uuid", "sourceEntityId and targetEntityId must be UUIDs."), nil
	}
	if req.ObservationID != "" && !isUUID(req.ObservationID) {
		return entityMutationResponse{}, badField("/observationId", "invalid_uuid", "observationId must be a UUID."), nil
	}
	if req.SourceEntityID == req.TargetEntityID {
		return entityMutationResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "entity_conflict", Retryable: false, Detail: "an entity cannot be merged into itself."}, nil
	}

	source, target, pb, err := loadEntityPairForMutation(ctx, tx, actor.EngagementID, req.SourceEntityID, req.TargetEntityID)
	if pb != nil || err != nil {
		return entityMutationResponse{}, pb, err
	}
	if source.Kind != target.Kind {
		return entityMutationResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "entity_conflict", Retryable: false, Detail: "entities must share a kind to be merged."}, nil
	}
	if source.MergedIntoEntityID.Valid {
		return entityMutationResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "entity_conflict", Retryable: false, Detail: "source entity is already merged."}, nil
	}
	if target.MergedIntoEntityID.Valid {
		return entityMutationResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "entity_conflict", Retryable: false, Detail: "target entity is already merged."}, nil
	}
	if pb := checkExpectedRevision(req.ExpectedSourceRevision, source.Revision, "/expectedSourceRevision"); pb != nil {
		return entityMutationResponse{}, pb, nil
	}
	if pb := checkExpectedRevision(req.ExpectedTargetRevision, target.Revision, "/expectedTargetRevision"); pb != nil {
		return entityMutationResponse{}, pb, nil
	}

	if req.Preview {
		resp, err := loadEntityMutationPreview(ctx, tx, source, target, true)
		return resp, nil, err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE entity SET merged_into_entity_id = $2, revision = revision + 1, updated_at = now() WHERE id = $1`, source.ID, target.ID); err != nil {
		return entityMutationResponse{}, nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE entity SET revision = revision + 1, updated_at = now(), last_seen = GREATEST(last_seen, $2), attributes = attributes || $3::jsonb WHERE id = $1`, target.ID, source.LastSeen, jsonArg(source.Attributes)); err != nil {
		return entityMutationResponse{}, nil, err
	}
	if err := appendEntityAuditEvent(ctx, tx, actor, reqID, "entity.merged", source.ID, source.Revision+1, map[string]any{"sourceEntityId": source.ID, "targetEntityId": target.ID, "sourceRevision": source.Revision, "targetRevision": target.Revision, "observationId": nullIfEmpty(req.ObservationID)}); err != nil {
		return entityMutationResponse{}, nil, err
	}
	resp, err := loadEntityMutationResponse(ctx, tx, source.ID, target.ID, false, req.ObservationID)
	if err != nil {
		return entityMutationResponse{}, nil, err
	}
	return resp, nil, nil
}

func applyEntitySplit(ctx context.Context, tx *sql.Tx, actor actorRecord, req entitySplitRequest, reqID string) (entityMutationResponse, *captureProblem, error) {
	if !isUUID(req.EntityID) {
		return entityMutationResponse{}, badField("/entityId", "invalid_uuid", "entityId must be a UUID."), nil
	}
	if !isUUID(req.ObservationID) {
		return entityMutationResponse{}, badField("/observationId", "invalid_uuid", "observationId is required and must be a UUID."), nil
	}

	source, pb, err := loadEntityByID(ctx, tx, actor.EngagementID, req.EntityID, true)
	if pb != nil || err != nil {
		return entityMutationResponse{}, pb, err
	}
	if !source.MergedIntoEntityID.Valid {
		return entityMutationResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "entity_conflict", Retryable: false, Detail: "entity is not currently merged."}, nil
	}
	if pb := checkExpectedRevision(req.ExpectedSourceRevision, source.Revision, "/expectedSourceRevision"); pb != nil {
		return entityMutationResponse{}, pb, nil
	}
	target, pb, err := loadEntityByID(ctx, tx, actor.EngagementID, source.MergedIntoEntityID.String, true)
	if pb != nil || err != nil {
		return entityMutationResponse{}, pb, err
	}
	if pb := checkExpectedRevision(req.ExpectedTargetRevision, target.Revision, "/expectedTargetRevision"); pb != nil {
		return entityMutationResponse{}, pb, nil
	}
	observation, pb, err := loadObservationForEntity(ctx, tx, actor.EngagementID, req.ObservationID, source.ID)
	if pb != nil || err != nil {
		return entityMutationResponse{}, pb, err
	}
	_ = observation

	if req.Preview {
		resp, err := loadEntityMutationPreview(ctx, tx, source, target, false)
		if err != nil {
			return entityMutationResponse{}, nil, err
		}
		resp.ObservationID = req.ObservationID
		return resp, nil, nil
	}

	if _, err := tx.ExecContext(ctx, `UPDATE entity SET merged_into_entity_id = NULL, revision = revision + 1, updated_at = now() WHERE id = $1`, source.ID); err != nil {
		return entityMutationResponse{}, nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE entity SET revision = revision + 1, updated_at = now() WHERE id = $1`, target.ID); err != nil {
		return entityMutationResponse{}, nil, err
	}
	if err := appendEntityAuditEvent(ctx, tx, actor, reqID, "entity.split", source.ID, source.Revision+1, map[string]any{"entityId": source.ID, "targetEntityId": target.ID, "observationId": req.ObservationID, "sourceRevision": source.Revision, "targetRevision": target.Revision}); err != nil {
		return entityMutationResponse{}, nil, err
	}
	resp, err := loadEntityMutationResponse(ctx, tx, source.ID, target.ID, true, req.ObservationID)
	if err != nil {
		return entityMutationResponse{}, nil, err
	}
	return resp, nil, nil
}

func loadEntityMutationPreview(ctx context.Context, tx queryer, source, target entityRow, previewMerge bool) (entityMutationResponse, error) {
	sourceSummary, err := entitySummaryFromRow(ctx, tx, source)
	if err != nil {
		return entityMutationResponse{}, err
	}
	targetSummary, err := entitySummaryFromRow(ctx, tx, target)
	if err != nil {
		return entityMutationResponse{}, err
	}
	if previewMerge {
		sourceMerged := target.ID
		sourceSummary.MergedIntoEntityID = &sourceMerged
	} else {
		sourceSummary.MergedIntoEntityID = nil
	}
	sourceSummary.Revision++
	targetSummary.Revision++
	return entityMutationResponse{ContractVersion: entityContractVersion, Preview: true, Applied: false, Source: sourceSummary, Target: &targetSummary}, nil
}

func loadEntityMutationResponse(ctx context.Context, tx queryer, sourceID, targetID string, split bool, observationID string) (entityMutationResponse, error) {
	source, err := loadEntityRow(ctx, tx, sourceID)
	if err != nil {
		return entityMutationResponse{}, err
	}
	target, err := loadEntityRow(ctx, tx, targetID)
	if err != nil {
		return entityMutationResponse{}, err
	}
	sourceSummary, err := entitySummaryFromRow(ctx, tx, source)
	if err != nil {
		return entityMutationResponse{}, err
	}
	targetSummary, err := entitySummaryFromRow(ctx, tx, target)
	if err != nil {
		return entityMutationResponse{}, err
	}
	resp := entityMutationResponse{ContractVersion: entityContractVersion, Applied: true, Source: sourceSummary, Target: &targetSummary, ObservationID: observationID}
	if !split {
		resp.Preview = false
	}
	return resp, nil
}

func entitySummaryFromRow(ctx context.Context, q queryer, row entityRow) (entitySummary, error) {
	count, err := countEntityObservations(ctx, q, row.ID)
	if err != nil {
		return entitySummary{}, err
	}
	summary := entitySummary{ID: row.ID, Kind: row.Kind, KeyType: row.KeyType, KeyValue: row.KeyValue, Revision: row.Revision, ObservationCount: count}
	if row.MergedIntoEntityID.Valid {
		v := row.MergedIntoEntityID.String
		summary.MergedIntoEntityID = &v
	}
	return summary, nil
}

func countEntityObservations(ctx context.Context, q queryer, entityID string) (int, error) {
	var count int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM observation WHERE entity_id = $1`, entityID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func loadEntityRow(ctx context.Context, q queryer, entityID string) (entityRow, error) {
	var row entityRow
	var mergedInto sql.NullString
	if err := q.QueryRowContext(ctx, `SELECT id, engagement_id, kind, key_type, key_value, revision, COALESCE(merged_into_entity_id::text, ''), attributes, first_seen, last_seen FROM entity WHERE id = $1`, entityID).Scan(&row.ID, &row.EngagementID, &row.Kind, &row.KeyType, &row.KeyValue, &row.Revision, &mergedInto, &row.Attributes, &row.FirstSeen, &row.LastSeen); err != nil {
		return entityRow{}, err
	}
	if mergedInto.Valid && mergedInto.String != "" {
		row.MergedIntoEntityID = mergedInto
	}
	return row, nil
}

func touchEntityByID(ctx context.Context, tx *sql.Tx, entityID string, attrs json.RawMessage) (string, error) {
	var row entityRow
	var mergedInto sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT id, engagement_id, kind, key_type, key_value, revision, COALESCE(merged_into_entity_id::text, ''), attributes, first_seen, last_seen FROM entity WHERE id = $1 FOR UPDATE`, entityID).Scan(&row.ID, &row.EngagementID, &row.Kind, &row.KeyType, &row.KeyValue, &row.Revision, &mergedInto, &row.Attributes, &row.FirstSeen, &row.LastSeen); err != nil {
		return "", err
	}
	if mergedInto.Valid && mergedInto.String != "" {
		return touchEntityByID(ctx, tx, mergedInto.String, attrs)
	}
	if attrs == nil {
		attrs = json.RawMessage(`{}`)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE entity SET last_seen = now(), updated_at = now(), revision = revision + 1, attributes = attributes || $2::jsonb WHERE id = $1`, entityID, string(attrs)); err != nil {
		return "", err
	}
	return entityID, nil
}

func loadEntityByKey(ctx context.Context, tx *sql.Tx, engagementID, keyType, keyValue string, lock bool) (entityRow, error) {
	clause := ""
	if lock {
		clause = " FOR UPDATE"
	}
	var row entityRow
	var mergedInto sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT id, engagement_id, kind, key_type, key_value, revision, COALESCE(merged_into_entity_id::text, ''), attributes, first_seen, last_seen FROM entity WHERE engagement_id = $1 AND key_type = $2 AND key_value = $3`+clause, engagementID, keyType, keyValue).Scan(&row.ID, &row.EngagementID, &row.Kind, &row.KeyType, &row.KeyValue, &row.Revision, &mergedInto, &row.Attributes, &row.FirstSeen, &row.LastSeen); err != nil {
		return entityRow{}, err
	}
	if mergedInto.Valid && mergedInto.String != "" {
		row.MergedIntoEntityID = mergedInto
	}
	return row, nil
}

func loadEntityByID(ctx context.Context, tx *sql.Tx, engagementID, entityID string, lock bool) (entityRow, *captureProblem, error) {
	clause := ""
	if lock {
		clause = " FOR UPDATE"
	}
	var row entityRow
	var mergedInto sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT id, engagement_id, kind, key_type, key_value, revision, COALESCE(merged_into_entity_id::text, ''), attributes, first_seen, last_seen FROM entity WHERE engagement_id = $1 AND id = $2`+clause, engagementID, entityID).Scan(&row.ID, &row.EngagementID, &row.Kind, &row.KeyType, &row.KeyValue, &row.Revision, &mergedInto, &row.Attributes, &row.FirstSeen, &row.LastSeen)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return entityRow{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "entity_conflict", Retryable: false, Detail: "entity not found."}, nil
		}
		return entityRow{}, nil, err
	}
	if mergedInto.Valid && mergedInto.String != "" {
		row.MergedIntoEntityID = mergedInto
	}
	return row, nil, nil
}

func loadEntityPairForMutation(ctx context.Context, tx *sql.Tx, engagementID, sourceID, targetID string) (entityRow, entityRow, *captureProblem, error) {
	firstID, secondID := sourceID, targetID
	if strings.Compare(firstID, secondID) > 0 {
		firstID, secondID = secondID, firstID
	}
	first, pb, err := loadEntityByID(ctx, tx, engagementID, firstID, true)
	if pb != nil || err != nil {
		return entityRow{}, entityRow{}, pb, err
	}
	second, pb, err := loadEntityByID(ctx, tx, engagementID, secondID, true)
	if pb != nil || err != nil {
		return entityRow{}, entityRow{}, pb, err
	}
	if first.ID == sourceID {
		return first, second, nil, nil
	}
	return second, first, nil, nil
}

func loadObservationForEntity(ctx context.Context, tx *sql.Tx, engagementID, observationID, entityID string) (entityObservation, *captureProblem, error) {
	var obs entityObservation
	if err := tx.QueryRowContext(ctx, `SELECT id, entity_id, kind, identifiers, attributes FROM observation WHERE engagement_id = $1 AND id = $2`, engagementID, observationID).Scan(&obs.ID, &obs.EntityID, &obs.Kind, &obs.Identifiers, &obs.Attributes); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return entityObservation{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "entity_conflict", Retryable: false, Detail: "observation not found."}, nil
		}
		return entityObservation{}, nil, err
	}
	if obs.EntityID != entityID {
		return entityObservation{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "entity_conflict", Retryable: false, Detail: "observation does not belong to the selected entity."}, nil
	}
	return obs, nil, nil
}

func appendEntityAuditEvent(ctx context.Context, tx *sql.Tx, actor actorRecord, reqID, eventType, subjectID string, subjectRevision int, data map[string]any) error {
	_, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
		EngagementID:  actor.EngagementID,
		Type:          eventType,
		Actor:         dbutil.AuditActorSnapshot{ID: actor.ID, Kind: actor.Kind, Handle: actor.Handle, Role: actor.Role, AgentName: actor.AgentName, Model: actor.Model, Version: actor.Version, AuthorizedBy: actor.AuthorizedBy},
		Origin:        dbutil.AuditOrigin{Kind: "rest"},
		Subject:       dbutil.AuditSubject{Type: "entity", ID: subjectID, Revision: subjectRevision},
		RequestID:     reqID,
		CorrelationID: reqID,
		Data:          data,
	})
	return err
}

func checkExpectedRevision(expected *int, got int, ptr string) *captureProblem {
	if expected == nil {
		return nil
	}
	if *expected < 1 || *expected != got {
		return &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "entity_conflict", Retryable: false, Detail: fmt.Sprintf("%s does not match the current entity revision.", ptr)}
	}
	return nil
}

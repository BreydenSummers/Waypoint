package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type entityObservationPageResponse struct {
	ContractVersion string                  `json:"contractVersion"`
	Items           []entityObservationItem `json:"items"`
	Page            entityPageMeta          `json:"page"`
}

type entityIdentifierPageResponse struct {
	ContractVersion string                    `json:"contractVersion"`
	Items           []captureEntityIdentifier `json:"items"`
	Page            entityPageMeta            `json:"page"`
}

type entityLineagePageResponse struct {
	ContractVersion string              `json:"contractVersion"`
	Items           []entityLineageItem `json:"items"`
	Page            entityPageMeta      `json:"page"`
}

type entityLineageItem struct {
	ID                 string    `json:"id"`
	EngagementID       string    `json:"engagementId"`
	Kind               string    `json:"kind"`
	KeyType            string    `json:"keyType"`
	KeyValue           string    `json:"keyValue"`
	Revision           int       `json:"revision"`
	MergedIntoEntityID *string   `json:"mergedIntoEntityId,omitempty"`
	FirstSeen          time.Time `json:"firstSeen"`
	LastSeen           time.Time `json:"lastSeen"`
}

type observationPageCursor struct {
	ObservedAt time.Time `json:"observedAt"`
	ID         string    `json:"id"`
}

type identifierPageCursor struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func handleEntityObservations(w http.ResponseWriter, r *http.Request, db *sql.DB, actor actorRecord, reqID, entityID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if !isUUID(entityID) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "entity id must be a UUID."})
		return
	}
	limit, pb := parseEntityPageLimit(r.URL.Query().Get("limit"))
	if pb != nil {
		pb.RequestID = reqID
		writeProblem(w, *pb)
		return
	}
	after, pb := parseObservationPageCursor(r.URL.Query().Get("after"))
	if pb != nil {
		pb.RequestID = reqID
		writeProblem(w, *pb)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	page, err := loadEntityObservationPage(ctx, db, actor.EngagementID, entityID, after, limit)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "entity not found"})
			return
		}
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load observation page failed"})
		return
	}
	writeJSONWithHeaders(w, http.StatusOK, page, reqID)
}

func handleEntityIdentifiers(w http.ResponseWriter, r *http.Request, db *sql.DB, actor actorRecord, reqID, entityID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if !isUUID(entityID) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "entity id must be a UUID."})
		return
	}
	limit, pb := parseEntityPageLimit(r.URL.Query().Get("limit"))
	if pb != nil {
		pb.RequestID = reqID
		writeProblem(w, *pb)
		return
	}
	after, pb := parseIdentifierPageCursor(r.URL.Query().Get("after"))
	if pb != nil {
		pb.RequestID = reqID
		writeProblem(w, *pb)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	page, err := loadEntityIdentifierPage(ctx, db, actor.EngagementID, entityID, after, limit)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "entity not found"})
			return
		}
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load identifier page failed"})
		return
	}
	writeJSONWithHeaders(w, http.StatusOK, page, reqID)
}

func handleEntityLineage(w http.ResponseWriter, r *http.Request, db *sql.DB, actor actorRecord, reqID, entityID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if !isUUID(entityID) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "entity id must be a UUID."})
		return
	}
	limit, pb := parseEntityPageLimit(r.URL.Query().Get("limit"))
	if pb != nil {
		pb.RequestID = reqID
		writeProblem(w, *pb)
		return
	}
	after, pb := parseEntityPageCursor(r.URL.Query().Get("after"))
	if pb != nil {
		pb.RequestID = reqID
		writeProblem(w, *pb)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	page, err := loadEntityLineagePage(ctx, db, actor.EngagementID, entityID, after, limit)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "entity not found"})
			return
		}
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load lineage page failed"})
		return
	}
	writeJSONWithHeaders(w, http.StatusOK, page, reqID)
}

func handleEntityMergePreviewRead(w http.ResponseWriter, r *http.Request, db *sql.DB, actor actorRecord, reqID, entityID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if !isUUID(entityID) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "entity id must be a UUID."})
		return
	}
	targetID := strings.TrimSpace(r.URL.Query().Get("targetEntityId"))
	if !isUUID(targetID) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "targetEntityId must be a UUID."})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "begin read transaction failed"})
		return
	}
	defer tx.Rollback()

	source, err := loadEntityRow(ctx, tx, entityID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "entity not found"})
			return
		}
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load entity failed"})
		return
	}
	if source.EngagementID != actor.EngagementID {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "entity not found"})
		return
	}
	target, err := loadEntityRow(ctx, tx, targetID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "target entity not found"})
			return
		}
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load target entity failed"})
		return
	}
	if target.EngagementID != actor.EngagementID {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "target entity not found"})
		return
	}
	if source.Kind != target.Kind {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "entity_conflict", Retryable: false, Detail: "entities must share a kind to be merged."})
		return
	}
	if source.MergedIntoEntityID.Valid || target.MergedIntoEntityID.Valid {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "entity_conflict", Retryable: false, Detail: "merged entities cannot be previewed for merge."})
		return
	}
	resp, err := loadEntityMutationPreview(ctx, tx, source, target, true)
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load merge preview failed"})
		return
	}
	if err := tx.Commit(); err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "commit read transaction failed"})
		return
	}
	writeJSONWithHeaders(w, http.StatusOK, resp, reqID)
}

func handleEntitySplitProvenanceRead(w http.ResponseWriter, r *http.Request, db *sql.DB, actor actorRecord, reqID, entityID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if !isUUID(entityID) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "entity id must be a UUID."})
		return
	}
	observationID := strings.TrimSpace(r.URL.Query().Get("observationId"))
	if !isUUID(observationID) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "observationId must be a UUID."})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "begin read transaction failed"})
		return
	}
	defer tx.Rollback()

	source, err := loadEntityRow(ctx, tx, entityID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "entity not found"})
			return
		}
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load entity failed"})
		return
	}
	if source.EngagementID != actor.EngagementID {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "entity not found"})
		return
	}
	if !source.MergedIntoEntityID.Valid {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "entity_conflict", Retryable: false, Detail: "entity is not currently merged."})
		return
	}
	target, err := loadEntityRow(ctx, tx, source.MergedIntoEntityID.String)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "target entity not found"})
			return
		}
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load target entity failed"})
		return
	}
	if target.EngagementID != actor.EngagementID {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "target entity not found"})
		return
	}
	if target.Kind != source.Kind {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "entity_conflict", Retryable: false, Detail: "entities must share a kind to be split."})
		return
	}
	if _, pb, err := loadObservationForEntity(ctx, tx, actor.EngagementID, observationID, source.ID); pb != nil || err != nil {
		if pb != nil {
			pb.RequestID = reqID
			writeProblem(w, *pb)
			return
		}
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load split provenance failed"})
		return
	}
	resp, err := loadEntityMutationPreview(ctx, tx, source, target, false)
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load split provenance preview failed"})
		return
	}
	resp.ObservationID = observationID
	if err := tx.Commit(); err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "commit read transaction failed"})
		return
	}
	writeJSONWithHeaders(w, http.StatusOK, resp, reqID)
}

func loadEntityObservationPage(ctx context.Context, db *sql.DB, engagementID, entityID string, after *observationPageCursor, limit int) (entityObservationPageResponse, error) {
	row, err := loadCanonicalEntityRow(ctx, db, engagementID, entityID)
	if err != nil {
		return entityObservationPageResponse{}, err
	}
	var afterSeen any
	var afterID any
	if after != nil {
		afterSeen = after.ObservedAt
		afterID = after.ID
	}
	rows, err := db.QueryContext(ctx, `
		WITH RECURSIVE lineage AS (
			SELECT id
			FROM entity
			WHERE engagement_id = $1 AND id = $2
			UNION ALL
			SELECT e.id
			FROM entity e
			JOIN lineage l ON e.merged_into_entity_id = l.id
			WHERE e.engagement_id = $1
		)
		SELECT o.id, o.entity_id::text, o.kind, COALESCE(o.action_id::text, ''), o.identifiers, o.attributes, o.observed_at
		FROM observation o
		JOIN lineage l ON l.id = o.entity_id
		WHERE o.engagement_id = $1
		  AND ($3::timestamptz IS NULL OR (o.observed_at, o.id) > ($3, $4))
		ORDER BY o.observed_at ASC, o.id ASC
		LIMIT $5
	`, engagementID, row.ID, afterSeen, afterID, limit+1)
	if err != nil {
		return entityObservationPageResponse{}, err
	}
	defer rows.Close()

	items := make([]entityObservationItem, 0, limit)
	for rows.Next() {
		var item entityObservationItem
		var identifiers, attributes string
		if err := rows.Scan(&item.ID, &item.EntityID, &item.Kind, &item.SourceActionID, &identifiers, &attributes, &item.ObservedAt); err != nil {
			return entityObservationPageResponse{}, err
		}
		item.ClaimStatus = "captured"
		if strings.TrimSpace(identifiers) != "" {
			if err := json.Unmarshal([]byte(identifiers), &item.Identifiers); err != nil {
				return entityObservationPageResponse{}, err
			}
		}
		if strings.TrimSpace(attributes) != "" {
			item.Attributes = json.RawMessage(attributes)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return entityObservationPageResponse{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	page := entityObservationPageResponse{ContractVersion: entityReadContractVersion, Items: items, Page: entityPageMeta{HasMore: hasMore}}
	if hasMore && len(items) > 0 {
		page.Page.NextCursor = encodePageCursor(observationPageCursor{ObservedAt: items[len(items)-1].ObservedAt, ID: items[len(items)-1].ID})
	}
	return page, nil
}

func loadEntityIdentifierPage(ctx context.Context, db *sql.DB, engagementID, entityID string, after *identifierPageCursor, limit int) (entityIdentifierPageResponse, error) {
	row, err := loadCanonicalEntityRow(ctx, db, engagementID, entityID)
	if err != nil {
		return entityIdentifierPageResponse{}, err
	}
	var afterType string
	var afterValue string
	if after != nil {
		afterType = after.Type
		afterValue = after.Value
	}
	rows, err := db.QueryContext(ctx, `
		WITH RECURSIVE lineage AS (
			SELECT id
			FROM entity
			WHERE engagement_id = $1 AND id = $2
			UNION ALL
			SELECT e.id
			FROM entity e
			JOIN lineage l ON e.merged_into_entity_id = l.id
			WHERE e.engagement_id = $1
		), identifier_set AS (
			-- The entity's own composite key_type (hostname_ip) stores its parts as
			-- "hostname=<h>|ip=<i>"; decompose it into individual {type,value} rows so
			-- the endpoint emits contract identifier types (hostname, ip), matching
			-- entityIdentifiersFromRow. Non-composite key types pass through unchanged.
			SELECT
				CASE WHEN e.key_type::text = 'hostname_ip' THEN split_part(kv, '=', 1) ELSE e.key_type::text END AS type,
				CASE WHEN e.key_type::text = 'hostname_ip' THEN split_part(kv, '=', 2) ELSE e.key_value END AS value
			FROM entity e
			CROSS JOIN LATERAL unnest(
				CASE WHEN e.key_type::text = 'hostname_ip' THEN string_to_array(e.key_value, '|') ELSE ARRAY[e.key_value] END
			) AS kv
			WHERE e.engagement_id = $1 AND e.id = $2 AND e.merged_into_entity_id IS NULL
			UNION
			SELECT ident->>'type' AS type, ident->>'value' AS value
			FROM observation o
			JOIN lineage l ON l.id = o.entity_id
			CROSS JOIN LATERAL jsonb_array_elements(o.identifiers) ident
			WHERE o.engagement_id = $1
		)
		SELECT type, value
		FROM identifier_set
		WHERE ($3 = '' AND $4 = '') OR (type, value) > ($3, $4)
		ORDER BY type ASC, value ASC
		LIMIT $5
	`, engagementID, row.ID, afterType, afterValue, limit+1)
	if err != nil {
		return entityIdentifierPageResponse{}, err
	}
	defer rows.Close()

	items := make([]captureEntityIdentifier, 0, limit)
	for rows.Next() {
		var item captureEntityIdentifier
		if err := rows.Scan(&item.Type, &item.Value); err != nil {
			return entityIdentifierPageResponse{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return entityIdentifierPageResponse{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	page := entityIdentifierPageResponse{ContractVersion: entityReadContractVersion, Items: items, Page: entityPageMeta{HasMore: hasMore}}
	if hasMore && len(items) > 0 {
		page.Page.NextCursor = encodePageCursor(identifierPageCursor{Type: items[len(items)-1].Type, Value: items[len(items)-1].Value})
	}
	return page, nil
}

func loadEntityLineagePage(ctx context.Context, db *sql.DB, engagementID, entityID string, after *entityPageCursor, limit int) (entityLineagePageResponse, error) {
	row, err := loadCanonicalEntityRow(ctx, db, engagementID, entityID)
	if err != nil {
		return entityLineagePageResponse{}, err
	}
	var afterSeen any
	var afterID any
	if after != nil {
		afterSeen = after.FirstSeen
		afterID = after.ID
	}
	rows, err := db.QueryContext(ctx, `
		WITH RECURSIVE lineage AS (
			SELECT id, engagement_id, kind, key_type, key_value, revision, merged_into_entity_id, first_seen, last_seen
			FROM entity
			WHERE engagement_id = $1 AND id = $2
			UNION ALL
			SELECT e.id, e.engagement_id, e.kind, e.key_type, e.key_value, e.revision, e.merged_into_entity_id, e.first_seen, e.last_seen
			FROM entity e
			JOIN lineage l ON e.merged_into_entity_id = l.id
			WHERE e.engagement_id = $1
		)
		SELECT id, engagement_id, kind, key_type, key_value, revision, COALESCE(merged_into_entity_id::text, ''), first_seen, last_seen
		FROM lineage
		WHERE ($3::timestamptz IS NULL OR (first_seen, id) > ($3, $4))
		ORDER BY first_seen ASC, id ASC
		LIMIT $5
	`, engagementID, row.ID, afterSeen, afterID, limit+1)
	if err != nil {
		return entityLineagePageResponse{}, err
	}
	defer rows.Close()

	items := make([]entityLineageItem, 0, limit)
	for rows.Next() {
		var item entityLineageItem
		var mergedInto sql.NullString
		if err := rows.Scan(&item.ID, &item.EngagementID, &item.Kind, &item.KeyType, &item.KeyValue, &item.Revision, &mergedInto, &item.FirstSeen, &item.LastSeen); err != nil {
			return entityLineagePageResponse{}, err
		}
		if mergedInto.Valid && mergedInto.String != "" {
			item.MergedIntoEntityID = &mergedInto.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return entityLineagePageResponse{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	page := entityLineagePageResponse{ContractVersion: entityReadContractVersion, Items: items, Page: entityPageMeta{HasMore: hasMore}}
	if hasMore && len(items) > 0 {
		page.Page.NextCursor = encodePageCursor(entityPageCursor{FirstSeen: items[len(items)-1].FirstSeen, ID: items[len(items)-1].ID})
	}
	return page, nil
}

func parseObservationPageCursor(v string) (*observationPageCursor, *captureProblem) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return nil, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "cursor_invalid", Retryable: false, Detail: "cursor must be a valid page token."}
	}
	var cursor observationPageCursor
	if err := json.Unmarshal(b, &cursor); err != nil || cursor.ID == "" || cursor.ObservedAt.IsZero() {
		return nil, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "cursor_invalid", Retryable: false, Detail: "cursor must be a valid page token."}
	}
	return &cursor, nil
}

func parseIdentifierPageCursor(v string) (*identifierPageCursor, *captureProblem) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return nil, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "cursor_invalid", Retryable: false, Detail: "cursor must be a valid page token."}
	}
	var cursor identifierPageCursor
	if err := json.Unmarshal(b, &cursor); err != nil || cursor.Type == "" || cursor.Value == "" {
		return nil, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "cursor_invalid", Retryable: false, Detail: "cursor must be a valid page token."}
	}
	return &cursor, nil
}

func encodePageCursor(v any) string {
	b, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(b)
}

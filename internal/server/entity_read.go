package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const entityReadContractVersion = "1.0.0"

type entityPageResponse struct {
	ContractVersion string               `json:"contractVersion"`
	Items           []entityReadResponse `json:"items"`
	Page            entityPageMeta       `json:"page"`
}

type entityPageMeta struct {
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}

type entityPageCursor struct {
	FirstSeen time.Time `json:"firstSeen"`
	ID        string    `json:"id"`
}

type entityReadResponse struct {
	ContractVersion string                    `json:"contractVersion"`
	ID              string                    `json:"id"`
	EngagementID    string                    `json:"engagementId"`
	Kind            string                    `json:"kind"`
	Identifiers     []captureEntityIdentifier `json:"identifiers"`
	Attributes      json.RawMessage           `json:"attributes"`
	Access          entityAccessRollup        `json:"access"`
	Credentials     []credentialSummary       `json:"credentials"`
	Observations    []entityObservationItem   `json:"observations"`
	FirstSeen       time.Time                 `json:"firstSeen"`
	LastSeen        time.Time                 `json:"lastSeen"`
	Revision        int                       `json:"revision"`
}

// entityAccessRollup summarises how much access an operator has established on an
// entity, derived from its access/credential observations. It is a read-only
// projection — the authoritative value the UI and reports render.
type entityAccessRollup struct {
	Level      string `json:"level"` // none | user | admin | system
	OwnsDomain bool   `json:"ownsDomain"`
}

// credentialSummary distils a captured credential/access observation on an entity.
type credentialSummary struct {
	User           string `json:"user"`
	Scope          string `json:"scope"` // domain | local | user
	Method         string `json:"method"`
	SourceActionID string `json:"sourceActionId,omitempty"`
}

type entityObservationItem struct {
	ID             string                    `json:"id"`
	EntityID       string                    `json:"entityId,omitempty"`
	Kind           string                    `json:"kind,omitempty"`
	SourceActionID string                    `json:"sourceActionId,omitempty"`
	ClaimStatus    string                    `json:"claimStatus"`
	Identifiers    []captureEntityIdentifier `json:"identifiers,omitempty"`
	Attributes     json.RawMessage           `json:"attributes,omitempty"`
	ObservedAt     time.Time                 `json:"observedAt"`
}

type entityReadRow struct {
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

func entityReadHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: requestIDFromHeader(r.Header.Get("X-Request-ID")), Retryable: true, Detail: "entity reads are unavailable"})
			return
		}

		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{entityReadContractVersion}})
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

		trimmed := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api/v1/entities"), "/")
		switch {
		case trimmed == "", trimmed == ".":
			if r.Method != http.MethodGet {
				w.Header().Set("Allow", http.MethodGet)
				http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
				return
			}
			handleEntityList(w, r, db, actor, reqID)
			return
		default:
			parts := strings.Split(trimmed, "/")
			if len(parts) == 1 {
				handleEntityItem(w, r, db, actor, reqID, parts[0])
				return
			}
			if len(parts) == 2 {
				switch parts[1] {
				case "observations":
					handleEntityObservations(w, r, db, actor, reqID, parts[0])
					return
				case "identifiers":
					handleEntityIdentifiers(w, r, db, actor, reqID, parts[0])
					return
				case "lineage":
					handleEntityLineage(w, r, db, actor, reqID, parts[0])
					return
				case "merge-preview":
					handleEntityMergePreviewRead(w, r, db, actor, reqID, parts[0])
					return
				case "split-provenance":
					handleEntitySplitProvenanceRead(w, r, db, actor, reqID, parts[0])
					return
				}
			}
			http.NotFound(w, r)
		}
	}
}

func handleEntityList(w http.ResponseWriter, r *http.Request, db *sql.DB, actor actorRecord, reqID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
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
	kind, pb := parseEntityKindFilter(r.URL.Query().Get("kind"))
	if pb != nil {
		pb.RequestID = reqID
		writeProblem(w, *pb)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	page, err := loadEntityPage(ctx, db, actor.EngagementID, after, limit, kind)
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load entity page failed"})
		return
	}
	writeJSONWithHeaders(w, http.StatusOK, page, reqID)
}

func handleEntityItem(w http.ResponseWriter, r *http.Request, db *sql.DB, actor actorRecord, reqID, entityID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if !isUUID(entityID) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "entity id must be a UUID."})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	item, err := loadEntityReadResponse(ctx, db, actor.EngagementID, entityID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "not_found", RequestID: reqID, Retryable: false, Detail: "entity not found"})
			return
		}
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load entity failed"})
		return
	}
	writeJSONWithHeaders(w, http.StatusOK, item, reqID)
}

func parseEntityPageLimit(v string) (int, *captureProblem) {
	if strings.TrimSpace(v) == "" {
		return 100, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 500 {
		return 0, badField("/limit", "invalid_range", "limit must be between 1 and 500.")
	}
	return n, nil
}

func parseEntityPageCursor(v string) (*entityPageCursor, *captureProblem) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	cursor, pb := decodeEntityPageCursor(v)
	if pb != nil {
		return nil, pb
	}
	return cursor, nil
}

// parseEntityKindFilter validates an optional ?kind= filter. Kind is a freeform
// text column (host, identity, segment, …); we only bound its length and reject
// control characters so it can be used as a safe equality filter.
func parseEntityKindFilter(v string) (string, *captureProblem) {
	k := strings.TrimSpace(v)
	if k == "" {
		return "", nil
	}
	if len(k) > 64 || strings.ContainsAny(k, controlChars) {
		return "", badField("/kind", "invalid_value", "kind must be printable text up to 64 characters.")
	}
	return k, nil
}

func loadEntityPage(ctx context.Context, db *sql.DB, engagementID string, after *entityPageCursor, limit int, kind string) (entityPageResponse, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return entityPageResponse{}, err
	}
	defer tx.Rollback()

	var afterSeen any
	var afterID any
	if after != nil {
		afterSeen = after.FirstSeen
		afterID = after.ID
	}
	var kindFilter any
	if strings.TrimSpace(kind) != "" {
		kindFilter = kind
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id
		FROM entity
		WHERE engagement_id = $1
		  AND merged_into_entity_id IS NULL
		  AND ($2::timestamptz IS NULL OR (first_seen, id) > ($2, $3))
		  AND ($5::text IS NULL OR kind = $5)
		ORDER BY first_seen ASC, id ASC
		LIMIT $4
	`, engagementID, afterSeen, afterID, limit+1, kindFilter)
	if err != nil {
		return entityPageResponse{}, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return entityPageResponse{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return entityPageResponse{}, err
	}

	hasMore := len(ids) > limit
	if hasMore {
		ids = ids[:limit]
	}
	items := make([]entityReadResponse, 0, len(ids))
	for _, id := range ids {
		item, err := loadEntityReadResponseWithTx(ctx, tx, engagementID, id)
		if err != nil {
			return entityPageResponse{}, err
		}
		items = append(items, item)
	}
	if err := tx.Commit(); err != nil {
		return entityPageResponse{}, err
	}

	page := entityPageResponse{ContractVersion: entityReadContractVersion, Items: items, Page: entityPageMeta{HasMore: hasMore}}
	if hasMore && len(items) > 0 {
		page.Page.NextCursor = encodeEntityPageCursor(entityPageCursor{FirstSeen: items[len(items)-1].FirstSeen, ID: items[len(items)-1].ID})
	}
	return page, nil
}

func loadEntityReadResponse(ctx context.Context, db *sql.DB, engagementID, entityID string) (entityReadResponse, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return entityReadResponse{}, err
	}
	defer tx.Rollback()

	item, err := loadEntityReadResponseWithTx(ctx, tx, engagementID, entityID)
	if err != nil {
		return entityReadResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return entityReadResponse{}, err
	}
	return item, nil
}

func loadEntityReadResponseWithTx(ctx context.Context, q queryer, engagementID, entityID string) (entityReadResponse, error) {
	row, err := loadCanonicalEntityRow(ctx, q, engagementID, entityID)
	if err != nil {
		return entityReadResponse{}, err
	}
	observations, err := loadEntityProvenanceObservations(ctx, q, engagementID, row.ID)
	if err != nil {
		return entityReadResponse{}, err
	}
	access, creds := deriveEntityAccess(observations)
	return entityReadResponse{
		ContractVersion: entityReadContractVersion,
		ID:              row.ID,
		EngagementID:    row.EngagementID,
		Kind:            row.Kind,
		Identifiers:     entityIdentifiersFromRow(row, observations),
		Attributes:      normalizeJSONObject(row.Attributes),
		Access:          access,
		Credentials:     creds,
		Observations:    observations,
		FirstSeen:       row.FirstSeen,
		LastSeen:        row.LastSeen,
		Revision:        row.Revision,
	}, nil
}

// deriveEntityAccess computes the access rollup and credential list for an entity
// from its access/credential observations. Mirrors the client heuristic so the
// server value is authoritative: SYSTEM/root/DA access or a dcsync/secretsdump/
// golden/Domain-Admin credential reads as SYSTEM; a Domain-Admin credential also
// confers domain ownership.
func deriveEntityAccess(observations []entityObservationItem) (entityAccessRollup, []credentialSummary) {
	rank := 0
	owns := false
	creds := make([]credentialSummary, 0)
	seen := map[string]bool{}
	bump := func(n int) {
		if n > rank {
			rank = n
		}
	}
	for _, o := range observations {
		if o.Kind != "access" && o.Kind != "credential" {
			continue
		}
		attrs := decodeObservationAttrs(o.Attributes)
		if b, ok := attrs["success"].(bool); ok && !b {
			continue
		}
		user, _ := attrs["user"].(string)
		method, _ := attrs["method"].(string)
		accessStr, _ := attrs["access"].(string)
		priv := strings.ToLower(fmt.Sprintf("%v %v", attrs["privilege"], attrs["impact"]))
		la := strings.ToLower(accessStr)
		switch o.Kind {
		case "access":
			if strings.Contains(la, "system") || strings.Contains(la, "root") || strings.Contains(la, "domain admin") {
				bump(3)
			} else if strings.Contains(la, "admin") {
				bump(2)
			} else if la != "" {
				bump(1)
			}
		case "credential":
			m := strings.ToLower(method)
			if strings.Contains(m, "dcsync") || strings.Contains(m, "secretsdump") || strings.Contains(priv, "golden") || strings.Contains(priv, "domain admin") {
				bump(3)
			} else {
				bump(1)
			}
			if strings.Contains(priv, "domain admin") || strings.Contains(priv, "golden") {
				owns = true
			}
		}
		if user == "" {
			continue
		}
		scope := "user"
		if strings.Contains(priv, "domain admin") || strings.Contains(priv, "golden") {
			scope = "domain"
		} else if strings.Contains(la, "system") || strings.Contains(la, "admin") {
			scope = "local"
		}
		how := method
		if o.Kind == "access" {
			if accessStr != "" {
				how = accessStr + " access"
			} else {
				how = "access"
			}
		}
		if how == "" {
			how = "captured"
		}
		key := user + "|" + how
		if seen[key] {
			continue
		}
		seen[key] = true
		creds = append(creds, credentialSummary{User: user, Scope: scope, Method: how, SourceActionID: o.SourceActionID})
	}
	level := "none"
	switch rank {
	case 3:
		level = "system"
	case 2:
		level = "admin"
	case 1:
		level = "user"
	}
	return entityAccessRollup{Level: level, OwnsDomain: owns && rank > 0}, creds
}

func decodeObservationAttrs(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return map[string]any{}
	}
	return m
}

func loadCanonicalEntityRow(ctx context.Context, q queryer, engagementID, entityID string) (entityReadRow, error) {
	var row entityReadRow
	var mergedInto sql.NullString
	if err := q.QueryRowContext(ctx, `
		WITH RECURSIVE lineage AS (
			SELECT id, engagement_id, kind, key_type, key_value, revision, merged_into_entity_id, attributes, first_seen, last_seen
			FROM entity
			WHERE engagement_id = $1 AND id = $2
			UNION ALL
			SELECT e.id, e.engagement_id, e.kind, e.key_type, e.key_value, e.revision, e.merged_into_entity_id, e.attributes, e.first_seen, e.last_seen
			FROM entity e
			JOIN lineage l ON e.id = l.merged_into_entity_id
			WHERE e.engagement_id = $1
		)
		SELECT id, engagement_id, kind, key_type, key_value, revision, COALESCE(merged_into_entity_id::text, ''), attributes, first_seen, last_seen
		FROM lineage
		WHERE merged_into_entity_id IS NULL
		LIMIT 1
	`, engagementID, entityID).Scan(&row.ID, &row.EngagementID, &row.Kind, &row.KeyType, &row.KeyValue, &row.Revision, &mergedInto, &row.Attributes, &row.FirstSeen, &row.LastSeen); err != nil {
		return entityReadRow{}, err
	}
	if mergedInto.Valid && mergedInto.String != "" {
		row.MergedIntoEntityID = mergedInto
	}
	return row, nil
}

func loadEntityProvenanceObservations(ctx context.Context, q queryer, engagementID, entityID string) ([]entityObservationItem, error) {
	rows, err := q.QueryContext(ctx, `
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
		ORDER BY o.observed_at ASC, o.id ASC
	`, engagementID, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]entityObservationItem, 0, 8)
	for rows.Next() {
		var item entityObservationItem
		var identifiers, attributes string
		if err := rows.Scan(&item.ID, &item.EntityID, &item.Kind, &item.SourceActionID, &identifiers, &attributes, &item.ObservedAt); err != nil {
			return nil, err
		}
		item.ClaimStatus = "captured"
		if strings.TrimSpace(identifiers) != "" {
			if err := json.Unmarshal([]byte(identifiers), &item.Identifiers); err != nil {
				return nil, err
			}
		}
		if strings.TrimSpace(attributes) != "" {
			item.Attributes = json.RawMessage(attributes)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func entityIdentifiersFromRow(row entityReadRow, observations []entityObservationItem) []captureEntityIdentifier {
	idents := make([]captureEntityIdentifier, 0, len(observations)+2)
	seen := map[string]struct{}{}
	appendID := func(id captureEntityIdentifier) {
		key := id.Type + "\x00" + id.Value
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		idents = append(idents, id)
	}

	switch row.KeyType {
	case "ad_sid", "mac", "fqdn":
		appendID(captureEntityIdentifier{Type: row.KeyType, Value: row.KeyValue})
	case "hostname_ip":
		for _, part := range strings.Split(row.KeyValue, "|") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				continue
			}
			if strings.TrimSpace(kv[0]) != "" && strings.TrimSpace(kv[1]) != "" {
				appendID(captureEntityIdentifier{Type: strings.TrimSpace(kv[0]), Value: strings.TrimSpace(kv[1])})
			}
		}
	case "other":
		appendID(captureEntityIdentifier{Type: "other", Value: row.KeyValue})
	}

	for _, obs := range observations {
		for _, id := range obs.Identifiers {
			appendID(id)
		}
	}
	return idents
}

func normalizeJSONObject(b json.RawMessage) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage(`{}`)
	}
	return b
}

func encodeEntityPageCursor(cursor entityPageCursor) string {
	b, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeEntityPageCursor(v string) (*entityPageCursor, *captureProblem) {
	b, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return nil, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "cursor_invalid", Retryable: false, Detail: "cursor must be a valid page token."}
	}
	var cursor entityPageCursor
	if err := json.Unmarshal(b, &cursor); err != nil || cursor.ID == "" || cursor.FirstSeen.IsZero() {
		return nil, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "cursor_invalid", Retryable: false, Detail: "cursor must be a valid page token."}
	}
	return &cursor, nil
}

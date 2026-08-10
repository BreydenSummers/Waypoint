package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	auditContractVersion = "1.0.0"
	auditReplayWindow    = 65
	auditQueueSize       = 16
	auditPollInterval    = 100 * time.Millisecond
	auditHeartbeatEvery  = 10 * time.Second
	auditWriteTimeout    = 2 * time.Second
)

type auditPageResponse struct {
	ContractVersion string           `json:"contractVersion"`
	Items           []auditEventItem `json:"items"`
	Page            auditPageMeta    `json:"page"`
}

type auditPageMeta struct {
	NextCursor      string  `json:"nextCursor,omitempty"`
	HighWaterCursor *string `json:"highWaterCursor"`
	HasMore         bool    `json:"hasMore"`
}

type auditEventItem struct {
	ContractVersion string               `json:"contractVersion"`
	ID              string               `json:"id"`
	Type            string               `json:"type"`
	EngagementID    string               `json:"engagementId"`
	Actor           auditEventActor      `json:"actor"`
	OccurredAt      time.Time            `json:"occurredAt"`
	Origin          auditEventOrigin     `json:"origin"`
	Subject         auditEventSubject    `json:"subject"`
	RequestID       string               `json:"requestId"`
	CorrelationID   string               `json:"correlationId"`
	Causation       *auditEventCausation `json:"causation,omitempty"`
	Data            json.RawMessage      `json:"data"`
}

type auditEventActor struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Handle       string `json:"handle"`
	Role         string `json:"role"`
	AgentName    string `json:"agentName,omitempty"`
	Model        string `json:"model,omitempty"`
	Version      string `json:"version,omitempty"`
	AuthorizedBy string `json:"authorizedBy,omitempty"`
}

type auditEventOrigin struct {
	Kind    string `json:"kind"`
	Service string `json:"service,omitempty"`
}

type auditEventSubject struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Revision int    `json:"revision,omitempty"`
}

type auditEventCausation struct {
	ActionID string `json:"actionId,omitempty"`
	EventID  string `json:"eventId,omitempty"`
}

type auditStreamMessage struct {
	item auditEventItem
}

func auditEventsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		if db == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: requestIDFromHeader(r.Header.Get("X-Request-ID")), Retryable: true, Detail: "audit history is unavailable"})
			return
		}

		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{auditContractVersion}})
			return
		}

		actor, err := auditReaderActor(r.Context(), db, r.Header.Get("Authorization"))
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUnauthorized), Status: http.StatusUnauthorized, Code: "unauthenticated", RequestID: reqID, Retryable: false, Detail: err.Error()})
			return
		}

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
		page, err := loadAuditPage(ctx, db, actor.EngagementID, after, limit)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load audit history failed"})
			return
		}

		writeJSONWithHeaders(w, http.StatusOK, page, reqID)
	}
}

func auditEventsStreamHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		if db == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: requestIDFromHeader(r.Header.Get("X-Request-ID")), Retryable: true, Detail: "audit stream is unavailable"})
			return
		}

		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{auditContractVersion}})
			return
		}

		actor, err := auditReaderActor(r.Context(), db, r.Header.Get("Authorization"))
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUnauthorized), Status: http.StatusUnauthorized, Code: "unauthenticated", RequestID: reqID, Retryable: false, Detail: err.Error()})
			return
		}

		after, pb := resolveSSECursor(r.URL.Query().Get("after"), r.Header.Get("Last-Event-ID"))
		if pb != nil {
			pb.RequestID = reqID
			writeProblem(w, *pb)
			return
		}

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		highWater, minAvailable, err := auditCursorWindow(ctx, db, actor.EngagementID)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load audit cursor window failed"})
			return
		}
		if after == nil {
			after = highWater
		}
		if after != nil && minAvailable != nil && *after < *minAvailable {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusGone), Status: http.StatusGone, Code: "cursor_expired", RequestID: reqID, Retryable: false, MinimumAvailableCursor: cursorPtr(*minAvailable), Resync: fmt.Sprintf("/api/v1/audit-events?after=%d", *after), Detail: "Resync from persisted audit history, then reconnect to the live feed."})
			return
		}

		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Request-ID", reqID)
		w.Header().Set("Waypoint-Contract-Version", auditContractVersion)
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		rc := http.NewResponseController(w)
		_ = rc.Flush()

		items := make(chan auditStreamMessage, auditQueueSize)
		errCh := make(chan error, 1)
		go func() {
			defer close(items)
			errCh <- tailAuditEvents(ctx, db, actor.ID, actor.EngagementID, after, items)
		}()

		heartbeat := time.NewTicker(auditHeartbeatEvery)
		defer heartbeat.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case err := <-errCh:
				if err != nil && !errors.Is(err, context.Canceled) {
					return
				}
				return
			case msg, ok := <-items:
				if !ok {
					return
				}
				if err := writeSSEEvent(w, rc, flusher, msg.item); err != nil {
					return
				}
			case <-heartbeat.C:
				if err := writeSSEHeartbeat(w, rc, flusher); err != nil {
					return
				}
			}
		}
	}
}

func auditReaderActor(ctx context.Context, db *sql.DB, authorization string) (actorRecord, error) {
	if db == nil {
		return actorRecord{}, errors.New("audit reader unavailable")
	}
	token, err := bearerToken(authorization)
	if err != nil {
		return actorRecord{}, err
	}
	return lookupActor(ctx, db, token)
}

func parseAuditLimit(v string) (int, *captureProblem) {
	if strings.TrimSpace(v) == "" {
		return 100, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 500 {
		pb := badField("/limit", "invalid_range", "limit must be between 1 and 500.")
		return 0, pb
	}
	return n, nil
}

func parseAuditCursorParam(v string) (*int64, *captureProblem) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	cursor, pb := parseAuditCursor(v)
	if pb != nil {
		return nil, pb
	}
	return &cursor, nil
}

func resolveSSECursor(after, lastEventID string) (*int64, *captureProblem) {
	after = strings.TrimSpace(after)
	lastEventID = strings.TrimSpace(lastEventID)
	if after == "" && lastEventID == "" {
		return nil, nil
	}
	var afterCursor *int64
	if after != "" {
		c, pb := parseAuditCursor(after)
		if pb != nil {
			return nil, pb
		}
		afterCursor = &c
	}
	var lastCursor *int64
	if lastEventID != "" {
		c, pb := parseAuditCursor(lastEventID)
		if pb != nil {
			return nil, pb
		}
		lastCursor = &c
	}
	if afterCursor != nil && lastCursor != nil && *afterCursor != *lastCursor {
		return nil, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "cursor_mismatch", Retryable: false, Detail: "after and Last-Event-ID must match when both are provided."}
	}
	if afterCursor != nil {
		return afterCursor, nil
	}
	return lastCursor, nil
}

func parseAuditCursor(v string) (int64, *captureProblem) {
	if v == "" {
		return 0, badField("/after", "missing_field", "cursor is required.")
	}
	if len(v) > 19 || v[0] == '0' || strings.ContainsAny(v, "+- ") {
		return 0, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "cursor_invalid", Retryable: false, Detail: "cursor must be a canonical positive decimal string."}
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 1 {
		return 0, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "cursor_invalid", Retryable: false, Detail: "cursor must be a canonical positive decimal string."}
	}
	if fmt.Sprint(n) != v {
		return 0, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "cursor_invalid", Retryable: false, Detail: "cursor must be a canonical positive decimal string."}
	}
	return n, nil
}

func auditCursorWindow(ctx context.Context, db *sql.DB, engagementID string) (*int64, *int64, error) {
	var highWater sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT MAX(id) FROM audit_event WHERE engagement_id = $1`, engagementID).Scan(&highWater); err != nil {
		return nil, nil, err
	}
	if !highWater.Valid {
		return nil, nil, nil
	}
	cursor := highWater.Int64
	min := cursor - auditReplayWindow + 1
	if min < 1 {
		min = 1
	}
	return &cursor, &min, nil
}

func loadAuditPage(ctx context.Context, db *sql.DB, engagementID string, after *int64, limit int) (auditPageResponse, error) {
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return auditPageResponse{}, err
	}
	defer tx.Rollback()

	highWater, err := txAuditHighWater(ctx, tx, engagementID)
	if err != nil {
		return auditPageResponse{}, err
	}
	items, hasMore, err := txAuditEvents(ctx, tx, engagementID, after, limit)
	if err != nil {
		return auditPageResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return auditPageResponse{}, err
	}

	page := auditPageResponse{ContractVersion: auditContractVersion, Items: items, Page: auditPageMeta{HasMore: hasMore}}
	if highWater != nil {
		page.Page.HighWaterCursor = cursorPtr(*highWater)
	}
	if hasMore && len(items) > 0 {
		page.Page.NextCursor = items[len(items)-1].ID
	}
	return page, nil
}

func txAuditHighWater(ctx context.Context, q queryer, engagementID string) (*int64, error) {
	var highWater sql.NullInt64
	if err := q.QueryRowContext(ctx, `SELECT MAX(id) FROM audit_event WHERE engagement_id = $1`, engagementID).Scan(&highWater); err != nil {
		return nil, err
	}
	if !highWater.Valid {
		return nil, nil
	}
	v := highWater.Int64
	return &v, nil
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func txAuditEvents(ctx context.Context, q queryer, engagementID string, after *int64, limit int) ([]auditEventItem, bool, error) {
	if limit < 1 {
		limit = 100
	}
	fetchLimit := limit + 1
	args := []any{engagementID, fetchAfterValue(after), fetchLimit}
	rows, err := q.QueryContext(ctx, `
		SELECT id, type, engagement_id, actor_id, actor_kind, actor_handle, actor_role,
		       COALESCE(actor_agent_name, ''), COALESCE(actor_model, ''), COALESCE(actor_version, ''), COALESCE(actor_authorized_by::text, ''),
		       occurred_at, origin_kind, COALESCE(origin_service, ''), subject_type, subject_id, subject_revision,
		       request_id, correlation_id, COALESCE(causation_action_id::text, ''), COALESCE(causation_event_id::text, ''), data::text
		FROM audit_event
		WHERE engagement_id = $1 AND id > $2
		ORDER BY id ASC
		LIMIT $3`, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	items := make([]auditEventItem, 0, limit)
	for rows.Next() {
		item, err := scanAuditEvent(rows)
		if err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return items, hasMore, nil
}

func scanAuditEvent(scanner interface{ Scan(...any) error }) (auditEventItem, error) {
	var (
		id              int64
		typeValue       sql.NullString
		engagementID    string
		actorID         string
		actorKind       string
		actorHandle     string
		actorRole       string
		actorAgentName  string
		actorModel      string
		actorVersion    string
		actorAuthorized string
		occurredAt      time.Time
		originKind      string
		originService   string
		subjectType     string
		subjectID       string
		subjectRevision int
		requestID       string
		correlationID   string
		causationAction string
		causationEvent  string
		dataText        string
	)
	if err := scanner.Scan(&id, &typeValue, &engagementID, &actorID, &actorKind, &actorHandle, &actorRole, &actorAgentName, &actorModel, &actorVersion, &actorAuthorized, &occurredAt, &originKind, &originService, &subjectType, &subjectID, &subjectRevision, &requestID, &correlationID, &causationAction, &causationEvent, &dataText); err != nil {
		return auditEventItem{}, err
	}
	item := auditEventItem{
		ContractVersion: auditContractVersion,
		ID:              fmt.Sprint(id),
		Type:            typeValue.String,
		EngagementID:    engagementID,
		Actor: auditEventActor{
			ID:           actorID,
			Kind:         actorKind,
			Handle:       actorHandle,
			Role:         actorRole,
			AgentName:    actorAgentName,
			Model:        actorModel,
			Version:      actorVersion,
			AuthorizedBy: actorAuthorized,
		},
		OccurredAt:    occurredAt.UTC(),
		Origin:        auditEventOrigin{Kind: originKind, Service: originService},
		Subject:       auditEventSubject{Type: subjectType, ID: subjectID, Revision: subjectRevision},
		RequestID:     requestID,
		CorrelationID: correlationID,
		Data:          json.RawMessage(dataText),
	}
	if item.Type == "" {
		item.Type = "audit.unknown"
	}
	if causationAction != "" || causationEvent != "" {
		item.Causation = &auditEventCausation{ActionID: causationAction, EventID: causationEvent}
	}
	if len(item.Data) == 0 {
		item.Data = json.RawMessage(`{}`)
	}
	if item.Actor.AgentName == "" {
		item.Actor.AgentName = ""
	}
	if item.Actor.Model == "" {
		item.Actor.Model = ""
	}
	if item.Actor.Version == "" {
		item.Actor.Version = ""
	}
	if item.Actor.AuthorizedBy == "" {
		item.Actor.AuthorizedBy = ""
	}
	return item, nil
}

func tailAuditEvents(ctx context.Context, db *sql.DB, actorID, engagementID string, after *int64, out chan<- auditStreamMessage) error {
	cursor := int64(0)
	if after != nil {
		cursor = *after
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !auditActorActive(ctx, db, actorID, engagementID) {
			return nil
		}
		rows, err := db.QueryContext(ctx, `
			SELECT id, type, engagement_id, actor_id, actor_kind, actor_handle, actor_role,
			       COALESCE(actor_agent_name, ''), COALESCE(actor_model, ''), COALESCE(actor_version, ''), COALESCE(actor_authorized_by::text, ''),
			       occurred_at, origin_kind, COALESCE(origin_service, ''), subject_type, subject_id, subject_revision,
			       request_id, correlation_id, COALESCE(causation_action_id::text, ''), COALESCE(causation_event_id::text, ''), data::text
			FROM audit_event
			WHERE engagement_id = $1 AND id > $2
			ORDER BY id ASC
			LIMIT $3`, engagementID, cursor, auditQueueSize)
		if err != nil {
			return err
		}
		batch := 0
		for rows.Next() {
			item, err := scanAuditEvent(rows)
			if err != nil {
				rows.Close()
				return err
			}
			cursor = mustCursor(item.ID)
			batch++
			select {
			case out <- auditStreamMessage{item: item}:
			case <-ctx.Done():
				rows.Close()
				return ctx.Err()
			default:
				rows.Close()
				return errors.New("slow client")
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if batch == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(auditPollInterval):
			}
			continue
		}
		if batch < auditQueueSize {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(auditPollInterval):
			}
		}
	}
}

func auditActorActive(ctx context.Context, db *sql.DB, actorID, engagementID string) bool {
	var ok bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM actor WHERE id = $1 AND engagement_id = $2 AND revoked_at IS NULL)`, actorID, engagementID).Scan(&ok); err != nil {
		return false
	}
	return ok
}

func writeSSEEvent(w http.ResponseWriter, rc *http.ResponseController, _ http.Flusher, item auditEventItem) error {
	if err := rc.SetWriteDeadline(time.Now().Add(auditWriteTimeout)); err != nil {
		return err
	}
	b, err := json.Marshal(item)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", item.ID, item.Type, b); err != nil {
		return err
	}
	return rc.Flush()
}

func writeSSEHeartbeat(w http.ResponseWriter, rc *http.ResponseController, _ http.Flusher) error {
	if err := rc.SetWriteDeadline(time.Now().Add(auditWriteTimeout)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
		return err
	}
	return rc.Flush()
}

func writeSSELine(w http.ResponseWriter, rc *http.ResponseController, flusher http.Flusher, line string) error {
	if _, err := fmt.Fprint(w, line); err != nil {
		return err
	}
	_ = flusher
	return nil
}

func fetchAfterValue(after *int64) any {
	if after == nil {
		return int64(0)
	}
	return *after
}

func cursorPtr(v int64) *string {
	s := fmt.Sprint(v)
	return &s
}

func mustCursor(v string) int64 {
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

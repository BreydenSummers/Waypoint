package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type AuditActorSnapshot struct {
	ID           string
	Kind         string
	Handle       string
	Role         string
	AgentName    string
	Model        string
	Version      string
	AuthorizedBy string
}

type AuditOrigin struct {
	Kind    string
	Service string
}

type AuditSubject struct {
	Type     string
	ID       string
	Revision int
}

type AuditEventInput struct {
	EngagementID    string
	Type            string
	Actor           AuditActorSnapshot
	Origin          AuditOrigin
	Subject         AuditSubject
	RequestID       string
	CorrelationID   string
	CausationAction string
	CausationEvent  sql.NullInt64
	Data            map[string]any
}

func AppendAuditEvent(ctx context.Context, tx *sql.Tx, in AuditEventInput) (int64, error) {
	if tx == nil {
		return 0, errors.New("tx is required")
	}
	if err := validateAuditEventInput(in); err != nil {
		return 0, err
	}
	data, err := json.Marshal(RedactAuditData(in.Data))
	if err != nil {
		return 0, fmt.Errorf("marshal audit data: %w", err)
	}
	var id int64
	row := tx.QueryRowContext(ctx, `
		INSERT INTO audit_event (
			engagement_id,
			type,
			actor_id,
			actor_kind,
			actor_handle,
			actor_role,
			actor_agent_name,
			actor_model,
			actor_version,
			actor_authorized_by,
			occurred_at,
			origin_kind,
			origin_service,
			subject_type,
			subject_id,
			subject_revision,
			request_id,
			correlation_id,
			causation_action_id,
			causation_event_id,
			data
		) VALUES (
			$1, NULLIF($2, ''), $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, '')::uuid, now(),
			$11, NULLIF($12, ''), $13, $14, $15, $16, $17, NULLIF($18, '')::uuid, $19, $20::jsonb
		) RETURNING id`,
		in.EngagementID,
		in.Type,
		in.Actor.ID,
		in.Actor.Kind,
		in.Actor.Handle,
		in.Actor.Role,
		in.Actor.AgentName,
		in.Actor.Model,
		in.Actor.Version,
		in.Actor.AuthorizedBy,
		in.Origin.Kind,
		in.Origin.Service,
		in.Subject.Type,
		in.Subject.ID,
		in.Subject.Revision,
		in.RequestID,
		in.CorrelationID,
		in.CausationAction,
		in.CausationEvent,
		data,
	)
	if err := row.Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func validateAuditEventInput(in AuditEventInput) error {
	if strings.TrimSpace(in.EngagementID) == "" {
		return errors.New("engagement id is required")
	}
	if strings.TrimSpace(in.Actor.ID) == "" {
		return errors.New("actor id is required")
	}
	if strings.TrimSpace(in.Actor.Kind) == "" || strings.TrimSpace(in.Actor.Handle) == "" || strings.TrimSpace(in.Actor.Role) == "" {
		return errors.New("actor snapshot is incomplete")
	}
	if strings.TrimSpace(in.Subject.Type) == "" || strings.TrimSpace(in.Subject.ID) == "" {
		return errors.New("subject is required")
	}
	if in.Subject.Revision < 1 {
		return errors.New("subject revision must be >= 1")
	}
	if strings.TrimSpace(in.RequestID) == "" || strings.TrimSpace(in.CorrelationID) == "" {
		return errors.New("request and correlation ids are required")
	}
	switch in.Origin.Kind {
	case "service":
		if strings.TrimSpace(in.Origin.Service) == "" {
			return errors.New("service origin requires service name")
		}
	default:
		if strings.TrimSpace(in.Origin.Service) != "" {
			return errors.New("non-service origin cannot set service name")
		}
	}
	switch in.Actor.Kind {
	case "ai_agent":
		if strings.TrimSpace(in.Actor.AgentName) == "" || strings.TrimSpace(in.Actor.Model) == "" || strings.TrimSpace(in.Actor.Version) == "" || strings.TrimSpace(in.Actor.AuthorizedBy) == "" {
			return errors.New("ai actor snapshot is incomplete")
		}
	case "human":
		if strings.TrimSpace(in.Actor.AgentName) != "" || strings.TrimSpace(in.Actor.Model) != "" || strings.TrimSpace(in.Actor.Version) != "" || strings.TrimSpace(in.Actor.AuthorizedBy) != "" {
			return errors.New("human actor snapshot must not include AI fields")
		}
	}
	return nil
}

func RedactAuditData(data map[string]any) map[string]any {
	if data == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(data))
	for k, v := range data {
		if redactAuditKey(k) {
			out[k] = "[redacted]"
			continue
		}
		out[k] = redactAuditValue(v)
	}
	return out
}

func redactAuditValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return RedactAuditData(x)
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = redactAuditValue(item)
		}
		return out
	default:
		return v
	}
}

func redactAuditKey(key string) bool {
	key = strings.ToLower(key)
	for _, needle := range []string{"password", "token", "secret", "credential", "authorization", "cookie", "privatekey", "passphrase", "rawstdout", "raw_stderr", "rawstderr", "raw_stdout", "evidencebytes", "api_key"} {
		if strings.Contains(key, needle) {
			return true
		}
	}
	return false
}

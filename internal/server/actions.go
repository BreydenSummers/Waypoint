package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const actionContractVersion = "1.0.0"

type actionPageResponse struct {
	ContractVersion string        `json:"contractVersion"`
	Items           []actionItem  `json:"items"`
	Page            auditPageMeta `json:"page"`
}

type actionItem struct {
	ContractVersion    string             `json:"contractVersion"`
	ID                 string             `json:"id"`
	EngagementID       string             `json:"engagementId"`
	Actor              auditEventActor    `json:"actor"`
	Capture            captureEnvelope    `json:"capture"`
	ReceivedAt         time.Time          `json:"receivedAt"`
	ClockSkew          *captureSkew       `json:"clockSkew"`
	EvidenceReferences actionEvidenceRefs `json:"evidenceReferences"`
	AuditEventCursor   string             `json:"auditEventCursor"`
}

type actionEvidenceRefs struct {
	Stdout actionEvidenceRef `json:"stdout"`
	Stderr actionEvidenceRef `json:"stderr"`
}

type actionEvidenceRef struct {
	ID           string `json:"id"`
	Role         string `json:"role"`
	MediaType    string `json:"mediaType"`
	ByteLength   int64  `json:"byteLength"`
	SHA256       string `json:"sha256"`
	DownloadPath string `json:"downloadPath"`
}

type actionListFilters struct {
	Technique     string
	Target        string
	Host          string
	Actor         string
	Result        string
	StartedAfter  *time.Time
	StartedBefore *time.Time
	EndedAfter    *time.Time
	EndedBefore   *time.Time
	InitiatedBy   string
	Provenance    string
	ParseStatus   string
}

type actionRow struct {
	Cursor int64

	ActionID     string
	EngagementID string
	ActorID      string
	ActorKind    string
	ActorHandle  string
	ActorRole    string
	ActorAgent   string
	ActorModel   string
	ActorVersion string
	ActorAuthBy  string

	SourceAgentID      string
	SourceAgentKind    string
	SourceAgentName    string
	SourceAgentVersion string
	SourceAgentOS      string
	SourceAgentArch    string

	CaptureID       string
	Phase           string
	InitiatedBy     string
	Command         string
	ArgvJSON        string
	Cwd             string
	TargetKind      string
	TargetValue     string
	TargetPort      sql.NullInt64
	TargetTransport sql.NullString

	StartedAt          time.Time
	EndedAt            sql.NullTime
	ExecHostIP         string
	ExecHostMethod     string
	ExecHostInterface  sql.NullString
	ExecHostConfidence string
	EgressMode         sql.NullString
	EgressStatus       sql.NullString
	EgressPublicIP     sql.NullString
	EgressObservedAt   sql.NullTime
	PivotChainJSON     string

	ExecutionStatus      string
	ExitCode             sql.NullInt64
	ExecutionSignal      sql.NullString
	ExecutionFailureCode sql.NullString

	ReceivedAt        time.Time
	ClockSkewStatus   sql.NullString
	ClockSkewOffsetMs sql.NullInt64

	StdoutID         string
	StdoutMediaType  string
	StdoutByteLength int64
	StdoutSHA256     string
	StderrID         string
	StderrMediaType  string
	StderrByteLength int64
	StderrSHA256     string

	PluginID        sql.NullString
	ParseStatus     string
	DecisionContext sql.NullString

	ResultSchemaID      sql.NullString
	ResultSchemaVersion sql.NullString
	ResultExtracted     sql.NullString
	ResultEntities      sql.NullString
}

func actionsHandler(db queryer, needsPlugin bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		if isNilQueryer(db) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: requestIDFromHeader(r.Header.Get("X-Request-ID")), Retryable: true, Detail: "action list is unavailable"})
			return
		}

		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{actionContractVersion}})
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
		filters, pb := parseActionListFilters(r.URL.Query())
		if pb != nil {
			pb.RequestID = reqID
			writeProblem(w, *pb)
			return
		}
		if needsPlugin {
			filters.ParseStatus = "needs-plugin"
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		page, err := loadActionPage(ctx, db, actor.EngagementID, after, limit, filters)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load actions failed"})
			return
		}

		writeJSONWithHeaders(w, http.StatusOK, page, reqID)
	}
}

func parseActionListFilters(values urlValues) (actionListFilters, *captureProblem) {
	var filters actionListFilters
	filters.Technique = strings.TrimSpace(values.Get("technique"))
	filters.Target = strings.TrimSpace(values.Get("target"))
	filters.Host = strings.TrimSpace(values.Get("host"))
	filters.Actor = strings.TrimSpace(values.Get("actor"))
	filters.Result = strings.TrimSpace(values.Get("result"))
	filters.InitiatedBy = strings.TrimSpace(values.Get("initiatedBy"))
	filters.Provenance = strings.TrimSpace(values.Get("provenance"))
	filters.ParseStatus = strings.TrimSpace(values.Get("parseStatus"))

	if t, pb := parseActionTimeParam(values.Get("startedAfter"), "/startedAfter"); pb != nil {
		return actionListFilters{}, pb
	} else if t != nil {
		filters.StartedAfter = t
	}
	if t, pb := parseActionTimeParam(values.Get("startedBefore"), "/startedBefore"); pb != nil {
		return actionListFilters{}, pb
	} else if t != nil {
		filters.StartedBefore = t
	}
	if t, pb := parseActionTimeParam(values.Get("endedAfter"), "/endedAfter"); pb != nil {
		return actionListFilters{}, pb
	} else if t != nil {
		filters.EndedAfter = t
	}
	if t, pb := parseActionTimeParam(values.Get("endedBefore"), "/endedBefore"); pb != nil {
		return actionListFilters{}, pb
	} else if t != nil {
		filters.EndedBefore = t
	}
	return filters, nil
}

type urlValues interface {
	Get(string) string
}

func parseActionTimeParam(v, pointer string) (*time.Time, *captureProblem) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, v); err == nil {
			ut := t.UTC()
			return &ut, nil
		}
	}
	return nil, badField(pointer, "invalid_timestamp", "timestamp must be RFC3339.")
}

func loadActionPage(ctx context.Context, q queryer, engagementID string, after *int64, limit int, filters actionListFilters) (actionPageResponse, error) {
	if limit < 1 {
		limit = 100
	}
	query, args := buildActionListQuery(engagementID, after, limit, filters)
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return actionPageResponse{}, err
	}
	defer rows.Close()

	items := make([]actionItem, 0, limit+1)
	for rows.Next() {
		row, err := scanActionRow(rows)
		if err != nil {
			return actionPageResponse{}, err
		}
		items = append(items, actionItemFromRow(row))
	}
	if err := rows.Err(); err != nil {
		return actionPageResponse{}, err
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	page := actionPageResponse{ContractVersion: actionContractVersion, Items: items, Page: auditPageMeta{HasMore: hasMore}}
	if hasMore && len(items) > 0 {
		page.Page.NextCursor = items[len(items)-1].AuditEventCursor
	}
	return page, nil
}

func buildActionListQuery(engagementID string, after *int64, limit int, filters actionListFilters) (string, []any) {
	if limit < 1 {
		limit = 100
	}
	var sb strings.Builder
	args := make([]any, 0, 12)
	arg := 1

	sb.WriteString(`SELECT ae.id,
       a.id, a.engagement_id,
       ar.id, ar.kind, ar.handle, ar.role, COALESCE(ar.agent_name, ''), COALESCE(ar.model, ''), COALESCE(ar.version, ''), COALESCE(ar.authorized_by::text, ''),
       COALESCE(a.source_agent_id::text, ''), COALESCE(a.source_agent_kind::text, ''), COALESCE(a.source_agent_name, ''), COALESCE(a.source_agent_version, ''),
       COALESCE(a.source_agent_platform_os::text, ''), COALESCE(a.source_agent_platform_arch::text, ''),
       COALESCE(a.capture_id::text, ''), a.phase::text, a.initiated_by::text, a.command, COALESCE(a.argv::text, '[]'), a.cwd,
       a.target_kind, a.target_value, a.target_port, COALESCE(a.target_transport, ''),
       a.started_at, a.ended_at,
       a.exec_host_ip::text, COALESCE(a.exec_host_method::text, ''), COALESCE(a.exec_host_interface, ''), COALESCE(a.exec_host_confidence::text, ''),
       COALESCE(a.egress_mode::text, ''), COALESCE(a.egress_status::text, ''), COALESCE(a.egress_public_ip::text, ''), a.egress_observed_at,
       COALESCE(a.pivot_chain::text, '[]'),
       COALESCE(a.execution_status::text, ''), a.exit_code, COALESCE(a.execution_signal, ''), COALESCE(a.execution_failure_code, ''),
       a.received_at, a.clock_skew_status, a.clock_skew_offset_ms,
       COALESCE(se.id::text, ''), COALESCE(se.media_type, ''), COALESCE(se.byte_length, 0), COALESCE(se.sha256, ''),
       COALESCE(ee.id::text, ''), COALESCE(ee.media_type, ''), COALESCE(ee.byte_length, 0), COALESCE(ee.sha256, ''),
       COALESCE(a.plugin_id, ''), a.parse_status::text, COALESCE(a.decision_context::text, ''),
       COALESCE(r.schema_id, ''), COALESCE(r.schema_version, ''), COALESCE(r.extracted::text, '{}'),
       COALESCE(obs.entities::text, '[]')
FROM action a
JOIN actor ar ON ar.id = a.actor_id AND ar.engagement_id = a.engagement_id
JOIN audit_event ae ON ae.engagement_id = a.engagement_id AND ae.subject_type = 'action' AND ae.subject_id = a.id AND ae.type = 'capture.accepted'
JOIN evidence se ON se.id = a.stdout_evidence_id
JOIN evidence ee ON ee.id = a.stderr_evidence_id
LEFT JOIN result r ON r.action_id = a.id
LEFT JOIN LATERAL (
    SELECT json_agg(json_build_object('kind', o.kind, 'identifiers', o.identifiers, 'attributes', o.attributes) ORDER BY o.created_at ASC) AS entities
    FROM observation o
    WHERE o.engagement_id = a.engagement_id AND o.action_id = a.id
) obs ON true
WHERE a.engagement_id = $1`)
	args = append(args, engagementID)
	arg = 2
	if after != nil {
		sb.WriteString(fmt.Sprintf(` AND ae.id < $%d`, arg))
		args = append(args, *after)
		arg++
	}
	if filters.Technique != "" {
		sb.WriteString(fmt.Sprintf(` AND a.plugin_id = $%d`, arg))
		args = append(args, filters.Technique)
		arg++
	}
	if filters.Target != "" {
		sb.WriteString(fmt.Sprintf(` AND (a.target_kind = $%d OR a.target_value = $%d)`, arg, arg))
		args = append(args, filters.Target)
		arg++
	}
	if filters.Host != "" {
		sb.WriteString(fmt.Sprintf(` AND a.exec_host_ip::text = $%d`, arg))
		args = append(args, filters.Host)
		arg++
	}
	if filters.Actor != "" {
		sb.WriteString(fmt.Sprintf(` AND (ar.id::text = $%d OR ar.handle = $%d)`, arg, arg))
		args = append(args, filters.Actor)
		arg++
	}
	if filters.Result != "" {
		result := strings.ToLower(filters.Result)
		switch result {
		case "success", "succeeded", "ok":
			sb.WriteString(` AND a.execution_status = 'exited' AND COALESCE(a.exit_code, 0) = 0`)
		case "failure", "failed":
			sb.WriteString(` AND (a.execution_status <> 'exited' OR COALESCE(a.exit_code, 0) <> 0)`)
		default:
			sb.WriteString(fmt.Sprintf(` AND a.execution_status::text = $%d`, arg))
			args = append(args, filters.Result)
			arg++
		}
	}
	if filters.InitiatedBy != "" {
		sb.WriteString(fmt.Sprintf(` AND a.initiated_by::text = $%d`, arg))
		args = append(args, filters.InitiatedBy)
		arg++
	}
	if filters.Provenance != "" {
		sb.WriteString(fmt.Sprintf(` AND (a.initiated_by::text = $%d OR a.parse_status::text = $%d OR COALESCE(a.source_agent_kind::text, '') = $%d OR ar.kind::text = $%d OR ar.role::text = $%d)`, arg, arg, arg, arg, arg))
		args = append(args, filters.Provenance)
		arg++
	}
	if filters.ParseStatus != "" {
		sb.WriteString(fmt.Sprintf(` AND a.parse_status::text = $%d`, arg))
		args = append(args, filters.ParseStatus)
		arg++
	}
	if filters.StartedAfter != nil {
		sb.WriteString(fmt.Sprintf(` AND a.started_at >= $%d`, arg))
		args = append(args, *filters.StartedAfter)
		arg++
	}
	if filters.StartedBefore != nil {
		sb.WriteString(fmt.Sprintf(` AND a.started_at <= $%d`, arg))
		args = append(args, *filters.StartedBefore)
		arg++
	}
	if filters.EndedAfter != nil {
		sb.WriteString(fmt.Sprintf(` AND a.ended_at >= $%d`, arg))
		args = append(args, *filters.EndedAfter)
		arg++
	}
	if filters.EndedBefore != nil {
		sb.WriteString(fmt.Sprintf(` AND a.ended_at <= $%d`, arg))
		args = append(args, *filters.EndedBefore)
		arg++
	}
	sb.WriteString(` ORDER BY ae.id DESC LIMIT $`)
	sb.WriteString(fmt.Sprint(arg))
	args = append(args, limit+1)
	return sb.String(), args
}

func scanActionRow(scanner interface{ Scan(...any) error }) (actionRow, error) {
	var row actionRow
	if err := scanner.Scan(
		&row.Cursor,
		&row.ActionID, &row.EngagementID,
		&row.ActorID, &row.ActorKind, &row.ActorHandle, &row.ActorRole, &row.ActorAgent, &row.ActorModel, &row.ActorVersion, &row.ActorAuthBy,
		&row.SourceAgentID, &row.SourceAgentKind, &row.SourceAgentName, &row.SourceAgentVersion, &row.SourceAgentOS, &row.SourceAgentArch,
		&row.CaptureID, &row.Phase, &row.InitiatedBy, &row.Command, &row.ArgvJSON, &row.Cwd,
		&row.TargetKind, &row.TargetValue, &row.TargetPort, &row.TargetTransport,
		&row.StartedAt, &row.EndedAt,
		&row.ExecHostIP, &row.ExecHostMethod, &row.ExecHostInterface, &row.ExecHostConfidence,
		&row.EgressMode, &row.EgressStatus, &row.EgressPublicIP, &row.EgressObservedAt,
		&row.PivotChainJSON,
		&row.ExecutionStatus, &row.ExitCode, &row.ExecutionSignal, &row.ExecutionFailureCode,
		&row.ReceivedAt, &row.ClockSkewStatus, &row.ClockSkewOffsetMs,
		&row.StdoutID, &row.StdoutMediaType, &row.StdoutByteLength, &row.StdoutSHA256,
		&row.StderrID, &row.StderrMediaType, &row.StderrByteLength, &row.StderrSHA256,
		&row.PluginID, &row.ParseStatus, &row.DecisionContext,
		&row.ResultSchemaID, &row.ResultSchemaVersion, &row.ResultExtracted, &row.ResultEntities,
	); err != nil {
		return actionRow{}, err
	}
	return row, nil
}

func actionItemFromRow(row actionRow) actionItem {
	item := actionItem{
		ContractVersion: actionContractVersion,
		ID:              row.ActionID,
		EngagementID:    row.EngagementID,
		Actor:           auditEventActor{ID: row.ActorID, Kind: row.ActorKind, Handle: row.ActorHandle, Role: row.ActorRole, AgentName: row.ActorAgent, Model: row.ActorModel, Version: row.ActorVersion, AuthorizedBy: row.ActorAuthBy},
		Capture: captureEnvelope{
			ContractVersion: actionContractVersion,
			CaptureID:       row.CaptureID,
			SourceAgent:     captureSourceAgent{ID: row.SourceAgentID, Kind: row.SourceAgentKind, Name: row.SourceAgentName, Version: row.SourceAgentVersion, Platform: capturePlatform{OS: row.SourceAgentOS, Arch: row.SourceAgentArch}},
			Phase:           row.Phase,
			InitiatedBy:     row.InitiatedBy,
			Command:         row.Command,
			Argv:            mustStringSlice(row.ArgvJSON),
			Cwd:             row.Cwd,
			Target:          captureTarget{Kind: row.TargetKind, Value: row.TargetValue, Port: nullIntToPtr(row.TargetPort), Transport: row.TargetTransport.String},
			Timing:          captureTiming{StartedAt: row.StartedAt.UTC(), EndedAt: nullTimeOrZero(row.EndedAt), DurationMs: durationMillis(row.StartedAt.UTC(), row.EndedAt)},
			Execution:       captureExecution{Status: row.ExecutionStatus, ExitCode: nullIntToPtr(row.ExitCode), Signal: row.ExecutionSignal.String, FailureCode: row.ExecutionFailureCode.String},
			Network:         captureNetwork{ExecHost: captureExecHost{Address: row.ExecHostIP, Method: row.ExecHostMethod, Confidence: row.ExecHostConfidence, Interface: row.ExecHostInterface.String}, Egress: captureEgress{Mode: row.EgressMode.String, Status: row.EgressStatus.String, Address: row.EgressPublicIP.String, ObservedAt: nullTimeOrNil(row.EgressObservedAt)}, PivotChain: mustPivotChain(row.PivotChainJSON)},
			Evidence:        captureEvidenceSet{Stdout: captureEvidenceDescriptor{MediaType: row.StdoutMediaType, ByteLength: row.StdoutByteLength, SHA256: row.StdoutSHA256}, Stderr: captureEvidenceDescriptor{MediaType: row.StderrMediaType, ByteLength: row.StderrByteLength, SHA256: row.StderrSHA256}},
			Parsing:         actionParsingFromRow(row),
			DecisionContext: actionDecisionContextFromRow(row),
		},
		ReceivedAt: row.ReceivedAt.UTC(),
		ClockSkew:  clockSkewFromStored(row.ClockSkewStatus, row.ClockSkewOffsetMs, nullTimeOrZero(row.EndedAt), row.ReceivedAt.UTC()),
		EvidenceReferences: actionEvidenceRefs{
			Stdout: actionEvidenceRef{ID: row.StdoutID, Role: "stdout", MediaType: row.StdoutMediaType, ByteLength: row.StdoutByteLength, SHA256: row.StdoutSHA256, DownloadPath: "/api/v1/evidence/" + row.StdoutID + "/content"},
			Stderr: actionEvidenceRef{ID: row.StderrID, Role: "stderr", MediaType: row.StderrMediaType, ByteLength: row.StderrByteLength, SHA256: row.StderrSHA256, DownloadPath: "/api/v1/evidence/" + row.StderrID + "/content"},
		},
		AuditEventCursor: fmt.Sprint(row.Cursor),
	}
	return item
}

func actionParsingFromRow(row actionRow) captureParsing {
	parsing := captureParsing{Status: row.ParseStatus}
	if row.ParseStatus == "parsed" || row.ParseStatus == "parse-failed" {
		parsing.Plugin = actionPluginSelection(row)
	}
	if row.ParseStatus == "parsed" {
		parsing.Result = actionParseResultFromRow(row)
	}
	if row.ParseStatus == "parse-failed" {
		parsing.Failure = &captureParseFailure{Code: "invalid_output", Message: "parser failure details are not retained in this projection."}
	}
	return parsing
}

func actionPluginSelection(row actionRow) *capturePluginSelection {
	if row.PluginID.Valid && row.PluginID.String != "" {
		binary := path.Base(row.Command)
		if fields := strings.Fields(row.Command); len(fields) > 0 {
			binary = path.Base(fields[0])
		}
		if binary == "." || binary == "/" || binary == "" {
			binary = row.Command
		}
		return &capturePluginSelection{
			ID:              row.PluginID.String,
			Version:         "1.0.0",
			ArtifactSHA256:  strings.Repeat("0", 64),
			ContractVersion: actionContractVersion,
			Match:           capturePluginMatch{Binary: binary, Reason: "persisted parser selection", Specificity: 0},
		}
	}
	return nil
}

func actionParseResultFromRow(row actionRow) *captureParseResult {
	if strings.TrimSpace(row.ResultSchemaID.String) == "" || strings.TrimSpace(row.ResultSchemaVersion.String) == "" {
		return nil
	}
	result := captureParseResult{SchemaID: row.ResultSchemaID.String, SchemaVersion: row.ResultSchemaVersion.String, Extracted: map[string]any{}, Entities: nil}
	if strings.TrimSpace(row.ResultExtracted.String) != "" {
		_ = json.Unmarshal([]byte(row.ResultExtracted.String), &result.Extracted)
	}
	if strings.TrimSpace(row.ResultEntities.String) != "" {
		_ = json.Unmarshal([]byte(row.ResultEntities.String), &result.Entities)
	}
	return &result
}

func actionDecisionContextFromRow(row actionRow) *captureDecisionContext {
	if strings.TrimSpace(row.DecisionContext.String) == "" {
		return nil
	}
	var ctx captureDecisionContext
	if err := json.Unmarshal([]byte(row.DecisionContext.String), &ctx); err != nil {
		return nil
	}
	return &ctx
}

func mustStringSlice(raw string) []string {
	var out []string
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func mustPivotChain(raw string) []capturePivotHop {
	var out []capturePivotHop
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func nullIntToPtr(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

func nullTimeOrZero(v sql.NullTime) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return v.Time.UTC()
}

func nullTimeOrNil(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time.UTC()
	return &t
}

func durationMillis(start time.Time, end sql.NullTime) int {
	if !end.Valid {
		return 0
	}
	return int(end.Time.Sub(start).Milliseconds())
}

func mustActionCursor(v string) int64 {
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

func isNilQueryer(q queryer) bool {
	if q == nil {
		return true
	}
	rv := reflect.ValueOf(q)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func:
		return rv.IsNil()
	default:
		return false
	}
}

package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	reportContractVersion = "1.0.0"
	reportSnapshotVersion = "v1"
)

var (
	reportJSONRoute        = regexp.MustCompile(`^/api/v1/engagements/([^/]+)/summit/report(?:\.json)?$`)
	reportPDFRoute         = regexp.MustCompile(`^/(?:api/v1/)?engagements/([^/]+)/summit/report\.pdf$`)
	reportFindingsPDFRoute = regexp.MustCompile(`^/(?:api/v1/)?engagements/([^/]+)/summit/findings\.pdf$`)
	reportFindingsCSVRoute = regexp.MustCompile(`^/(?:api/v1/)?engagements/([^/]+)/summit/findings\.csv$`)
)

// reportRouteMatch reports whether the path is any report artifact route, so the
// top-level mux can hand it to the report handler instead of the SPA.
func reportRouteMatch(path string) bool {
	return reportJSONRoute.MatchString(path) || reportPDFRoute.MatchString(path) ||
		reportFindingsPDFRoute.MatchString(path) || reportFindingsCSVRoute.MatchString(path)
}

type reportSnapshot struct {
	ContractVersion  string               `json:"contractVersion,omitempty"`
	Version          string               `json:"version"`
	Title            string               `json:"title"`
	Engagement       string               `json:"engagement"`
	Client           string               `json:"client,omitempty"`
	Cutoff           string               `json:"cutoff"`
	Scope            []string             `json:"scope"`
	Methodology      []string             `json:"methodology"`
	Runtime          RuntimeState         `json:"runtime,omitempty"`
	Findings         []reportFinding      `json:"findings"`
	Evidence         []reportEvidence     `json:"evidence"`
	Bundle           *reportBundle        `json:"bundle,omitempty"`
	Attribution      []reportAttribution  `json:"attribution"`
	KnownCaptureGaps []outOfBandClaimItem `json:"knownCaptureGaps"`
}

type reportBundle struct {
	Payloads           []reportBundlePayload  `json:"payloads"`
	OuterArchiveSHA256 string                 `json:"outerArchiveSha256"`
	Signatures         reportBundleSignatures `json:"signatures"`
	Restore            reportBundleRestore    `json:"restore"`
}

type reportBundlePayload struct {
	Path       string `json:"path"`
	Size       int64  `json:"-"`
	ByteLength int64  `json:"byteLength"`
	SHA256     string `json:"sha256"`
	Kind       string `json:"kind,omitempty"`
}

type reportBundleSignatures struct {
	Version string   `json:"version"`
	Items   []string `json:"items"`
}

type reportBundleRestore struct {
	Tools          []string `json:"tools"`
	CleanRoom      []string `json:"cleanRoom"`
	MaliciousPaths []string `json:"maliciousPaths"`
}

type reportFinding struct {
	ID                string   `json:"id"`
	Title             string   `json:"title"`
	Summary           string   `json:"summary,omitempty"`
	Severity          string   `json:"severity"`
	Evidence          []string `json:"evidence"`
	AffectedEntityIDs []string `json:"affectedEntityIds,omitempty"`
	Remediation       string   `json:"remediation"`
	Status            string   `json:"status"`
	PromotedBy        string   `json:"promotedBy"`
	PromotedAt        string   `json:"promotedAt"`
	Revision          int      `json:"revision"`
}

type reportEvidence struct {
	Label              string             `json:"label"`
	Source             string             `json:"source,omitempty"`
	SourceAgent        string             `json:"sourceAgent,omitempty"`
	CaptureID          string             `json:"captureId,omitempty"`
	CaptureFingerprint string             `json:"captureFingerprint,omitempty"`
	Command            string             `json:"command"`
	Target             string             `json:"target"`
	Actor              string             `json:"actor"`
	Host               string             `json:"host"`
	Egress             string             `json:"egress"`
	EgressMode         string             `json:"egressMode,omitempty"`
	EgressStatus       string             `json:"egressStatus,omitempty"`
	EgressObservedAt   string             `json:"egressObservedAt,omitempty"`
	PivotChain         []capturePivotHop  `json:"pivotChain,omitempty"`
	StartedAt          string             `json:"startedAt,omitempty"`
	EndedAt            string             `json:"endedAt,omitempty"`
	Duration           string             `json:"duration,omitempty"`
	ExitCode           string             `json:"exitCode,omitempty"`
	ExecutionStatus    string             `json:"executionStatus,omitempty"`
	ExecutionSignal    string             `json:"executionSignal,omitempty"`
	ExecutionFailure   string             `json:"executionFailure,omitempty"`
	InitiatedBy        string             `json:"initiatedBy"`
	ParseStatus        string             `json:"parseStatus"`
	Stdout             reportEvidenceBlob `json:"stdout,omitempty"`
	Stderr             reportEvidenceBlob `json:"stderr,omitempty"`
	RawStdout          string             `json:"rawStdout,omitempty"`
	RawStderr          string             `json:"rawStderr,omitempty"`
	RawSnippet         string             `json:"rawSnippet,omitempty"`
	Note               string             `json:"note,omitempty"`
	Attribution        string             `json:"attribution"`
}

type reportEvidenceBlob struct {
	Kind       string `json:"kind,omitempty"`
	MediaType  string `json:"mediaType,omitempty"`
	ByteLength int64  `json:"byteLength,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	StorageKey string `json:"storageKey,omitempty"`
	CreatedAt  string `json:"createdAt,omitempty"`
}

type reportAttribution struct {
	Title string   `json:"title"`
	Items []string `json:"items"`
}

type reportEngagementRow struct {
	ID        string
	Name      string
	Client    string
	Scope     string
	UpdatedAt time.Time
}

type reportActionRow struct {
	ID                      string
	StartedAt               time.Time
	EndedAt                 sql.NullTime
	Command                 string
	ArgvJSON                string
	TargetKind              string
	TargetValue             string
	ExecHostIP              string
	ExecHostMethod          sql.NullString
	ExecHostInterface       sql.NullString
	ExecHostConfidence      sql.NullString
	EgressMode              sql.NullString
	EgressStatus            sql.NullString
	EgressPublicIP          sql.NullString
	EgressObservedAt        sql.NullTime
	PivotChainJSON          string
	InitiatedBy             string
	ParseStatus             string
	ExecutionStatus         sql.NullString
	ExecutionSignal         sql.NullString
	ExecutionFailureCode    sql.NullString
	ExitCode                sql.NullInt64
	SourceAgentID           string
	SourceAgentKind         string
	SourceAgentName         string
	SourceAgentVersion      string
	SourceAgentPlatformOS   string
	SourceAgentPlatformArch string
	CaptureID               sql.NullString
	CaptureFingerprint      sql.NullString
	ActorHandle             string
	ActorKind               string
	ActorRole               string
	AgentName               string
	Model                   string
	Version                 string
	AuthorizedBy            sql.NullString
	StdoutStorageKey        string
	StderrStorageKey        string
	StdoutKind              string
	StderrKind              string
	StdoutSHA256            string
	StderrSHA256            string
	StdoutByteLength        int64
	StderrByteLength        int64
	StdoutMediaType         string
	StderrMediaType         string
	StdoutCreatedAt         sql.NullTime
	StderrCreatedAt         sql.NullTime
}

type reportFindingRow struct {
	ID               string
	Title            string
	Severity         string
	AffectedJSON     string
	EvidenceJSON     string
	Remediation      string
	Status           string
	PromotedBy       sql.NullString
	PromotedByHandle string
	PromotedAt       sql.NullTime
	Revision         int
	UpdatedAt        time.Time
}

func reportHandler(db *sql.DB, store *evidenceStore) http.HandlerFunc {
	return reportHandlerWithRuntime(db, store, RuntimeState{})
}

func reportHandlerWithRuntime(db *sql.DB, store *evidenceStore, runtime RuntimeState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		path := r.URL.Path
		format := ""
		mode := reportModeFull
		engagementID := ""
		if m := reportJSONRoute.FindStringSubmatch(path); m != nil {
			engagementID = m[1]
			format = "json"
		} else if m := reportPDFRoute.FindStringSubmatch(path); m != nil {
			engagementID = m[1]
			format = "pdf"
		} else if m := reportFindingsPDFRoute.FindStringSubmatch(path); m != nil {
			engagementID = m[1]
			format = "pdf"
			mode = reportModeFindings
		} else if m := reportFindingsCSVRoute.FindStringSubmatch(path); m != nil {
			engagementID = m[1]
			format = "csv"
			mode = reportModeFindings
		} else {
			return
		}

		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		token, err := bearerToken(r.Header.Get("Authorization"))
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUnauthorized), Status: http.StatusUnauthorized, Code: "unauthenticated", RequestID: reqID, Retryable: false, Detail: err.Error()})
			return
		}
		if db == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: reqID, Retryable: true, Detail: "report export is unavailable"})
			return
		}
		actor, err := lookupActor(r.Context(), db, token)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUnauthorized), Status: http.StatusUnauthorized, Code: "unauthenticated", RequestID: reqID, Retryable: false, Detail: "invalid actor credential"})
			return
		}
		if actor.EngagementID != "" && actor.EngagementID != engagementID {
			http.NotFound(w, r)
			return
		}
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{reportContractVersion}})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		snapshot, err := resolveReportSnapshotWithRuntime(ctx, db, store, engagementID, runtime)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			log.Printf("build frozen report failed for engagement %s: %v", engagementID, err)
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "build frozen report failed"})
			return
		}

		switch format {
		case "json":
			w.Header().Set("Waypoint-Contract-Version", reportContractVersion)
			writeJSON(w, http.StatusOK, snapshot)
		case "pdf":
			pdf, err := renderReportPDFMode(ctx, snapshot, mode)
			if err != nil {
				log.Printf("render report pdf failed for engagement %s: %v", engagementID, err)
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "render report pdf failed; see server logs"})
				return
			}
			filename := "report.pdf"
			if mode == reportModeFindings {
				filename = "findings.pdf"
			}
			w.Header().Set("Waypoint-Contract-Version", reportContractVersion)
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", "inline; filename="+filename)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(pdf)
		case "csv":
			data, err := renderFindingsCSV(snapshot)
			if err != nil {
				log.Printf("render findings csv failed for engagement %s: %v", engagementID, err)
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "render findings csv failed; see server logs"})
				return
			}
			w.Header().Set("Waypoint-Contract-Version", reportContractVersion)
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", "attachment; filename=findings.csv")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(data)
		}
	}
}

func buildReportSnapshot(ctx context.Context, db queryer, store *evidenceStore, engagementID string) (reportSnapshot, error) {
	return buildReportSnapshotWithRuntime(ctx, db, store, engagementID, RuntimeState{})
}

func buildReportSnapshotWithRuntime(ctx context.Context, db queryer, store *evidenceStore, engagementID string, runtime RuntimeState) (reportSnapshot, error) {
	engagement, err := loadReportEngagement(ctx, db, engagementID)
	if err != nil {
		return reportSnapshot{}, err
	}
	actions, err := loadReportActions(ctx, db, engagementID)
	if err != nil {
		return reportSnapshot{}, err
	}
	findings, err := loadReportFindings(ctx, db, engagementID)
	if err != nil {
		return reportSnapshot{}, err
	}
	captureGaps, err := loadReportCaptureGaps(ctx, db, engagementID)
	if err != nil {
		return reportSnapshot{}, err
	}
	return assembleReportSnapshot(ctx, engagement, actions, findings, captureGaps, store, runtime)
}

func assembleReportSnapshot(ctx context.Context, engagement reportEngagementRow, actions []reportActionRow, findings []reportFindingRow, captureGaps []outOfBandClaimItem, store *evidenceStore, runtime RuntimeState) (reportSnapshot, error) {
	actionLabels := map[string]string{}
	sortedActions := append([]reportActionRow(nil), actions...)
	sort.Slice(sortedActions, func(i, j int) bool {
		if !sortedActions[i].StartedAt.Equal(sortedActions[j].StartedAt) {
			return sortedActions[i].StartedAt.Before(sortedActions[j].StartedAt)
		}
		return sortedActions[i].ID < sortedActions[j].ID
	})

	for i, action := range sortedActions {
		actionLabels[action.ID] = "Action " + formatActionLabel(i+1)
	}

	findingCards := make([]reportFinding, 0, len(findings))
	for _, finding := range findings {
		labels := labelsForActionIDs(finding.EvidenceJSON, actionLabels)
		findingCards = append(findingCards, reportFinding{
			ID:                finding.ID,
			Title:             finding.Title,
			Summary:           finding.Title,
			Severity:          titleWord(strings.ToLower(strings.TrimSpace(finding.Severity))),
			Evidence:          labels,
			AffectedEntityIDs: mustJSONStrings(finding.AffectedJSON),
			Remediation:       strings.TrimSpace(finding.Remediation),
			Status:            strings.TrimSpace(finding.Status),
			PromotedBy:        strings.TrimSpace(finding.PromotedByHandle),
			PromotedAt:        formatRFC3339(finding.PromotedAt),
			Revision:          finding.Revision,
		})
	}
	sort.Slice(findingCards, func(i, j int) bool {
		if severityRank(findingCards[i].Severity) != severityRank(findingCards[j].Severity) {
			return severityRank(findingCards[i].Severity) > severityRank(findingCards[j].Severity)
		}
		if findingCards[i].PromotedAt != findingCards[j].PromotedAt {
			return findingCards[i].PromotedAt > findingCards[j].PromotedAt
		}
		return findingCards[i].Title < findingCards[j].Title
	})

	evidenceCards := buildEvidenceCards(ctx, sortedActions, actionLabels, store)
	attribution := buildAttribution(sortedActions)
	gaps := sortReportCaptureGaps(captureGaps)
	cutoff := engagement.UpdatedAt.UTC()
	for _, action := range sortedActions {
		if !action.StartedAt.IsZero() && action.StartedAt.UTC().After(cutoff) {
			cutoff = action.StartedAt.UTC()
		}
		if action.EndedAt.Valid && action.EndedAt.Time.UTC().After(cutoff) {
			cutoff = action.EndedAt.Time.UTC()
		}
	}
	for _, finding := range findings {
		if finding.UpdatedAt.UTC().After(cutoff) {
			cutoff = finding.UpdatedAt.UTC()
		}
	}

	return reportSnapshot{
		ContractVersion:  reportContractVersion,
		Version:          reportSnapshotVersion,
		Title:            "Frozen report snapshot",
		Engagement:       engagement.Name,
		Client:           strings.TrimSpace(engagement.Client),
		Cutoff:           cutoff.Format(time.RFC3339),
		Scope:            splitScope(engagement.Scope),
		Methodology:      reportMethodology(),
		Runtime:          runtime,
		Findings:         findingCards,
		Evidence:         evidenceCards,
		Bundle:           nil,
		Attribution:      attribution,
		KnownCaptureGaps: gaps,
	}, nil
}

func loadReportEngagement(ctx context.Context, db queryer, engagementID string) (reportEngagementRow, error) {
	var row reportEngagementRow
	if err := db.QueryRowContext(ctx, `SELECT id, name, client, scope, updated_at FROM engagement WHERE id = $1`, engagementID).Scan(&row.ID, &row.Name, &row.Client, &row.Scope, &row.UpdatedAt); err != nil {
		return reportEngagementRow{}, err
	}
	return row, nil
}

func loadReportActions(ctx context.Context, db queryer, engagementID string) ([]reportActionRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT a.id, a.started_at, a.ended_at, a.command, COALESCE(a.argv::text, '[]'), a.target_kind, a.target_value, a.exec_host_ip::text,
		       COALESCE(a.exec_host_method::text, ''), COALESCE(a.exec_host_interface::text, ''), COALESCE(a.exec_host_confidence::text, ''),
		       COALESCE(a.egress_mode::text, ''), COALESCE(a.egress_status::text, ''), COALESCE(a.egress_public_ip::text, ''), a.egress_observed_at,
		       COALESCE(a.pivot_chain::text, '[]'), a.initiated_by::text, a.parse_status::text, COALESCE(a.execution_status::text, ''), COALESCE(a.execution_signal, ''), COALESCE(a.execution_failure_code, ''), COALESCE(a.exit_code, 0),
		       COALESCE(a.source_agent_id::text, ''), COALESCE(a.source_agent_kind::text, ''), COALESCE(a.source_agent_name, ''), COALESCE(a.source_agent_version, ''), COALESCE(a.source_agent_platform_os::text, ''), COALESCE(a.source_agent_platform_arch::text, ''), COALESCE(a.capture_id::text, ''), COALESCE(a.capture_fingerprint, ''),
		       actor.handle, actor.kind::text, actor.role::text, COALESCE(actor.agent_name, ''), COALESCE(actor.model, ''), COALESCE(actor.version, ''), COALESCE(auth.handle::text, ''),
		       COALESCE(stdout.storage_key, ''), COALESCE(stderr.storage_key, ''), COALESCE(stdout.kind, ''), COALESCE(stderr.kind, ''), COALESCE(stdout.sha256, ''), COALESCE(stderr.sha256, ''), COALESCE(stdout.byte_length, 0), COALESCE(stderr.byte_length, 0), COALESCE(stdout.media_type, ''), COALESCE(stderr.media_type, ''), stdout.created_at, stderr.created_at
		FROM action a
		JOIN actor ON actor.id = a.actor_id
		LEFT JOIN actor auth ON auth.id = actor.authorized_by
		LEFT JOIN evidence stdout ON stdout.id = a.stdout_evidence_id
		LEFT JOIN evidence stderr ON stderr.id = a.stderr_evidence_id
		WHERE a.engagement_id = $1
		ORDER BY a.started_at ASC, a.id ASC`, engagementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]reportActionRow, 0)
	for rows.Next() {
		var row reportActionRow
		if err := rows.Scan(&row.ID, &row.StartedAt, &row.EndedAt, &row.Command, &row.ArgvJSON, &row.TargetKind, &row.TargetValue, &row.ExecHostIP, &row.ExecHostMethod, &row.ExecHostInterface, &row.ExecHostConfidence, &row.EgressMode, &row.EgressStatus, &row.EgressPublicIP, &row.EgressObservedAt, &row.PivotChainJSON, &row.InitiatedBy, &row.ParseStatus, &row.ExecutionStatus, &row.ExecutionSignal, &row.ExecutionFailureCode, &row.ExitCode, &row.SourceAgentID, &row.SourceAgentKind, &row.SourceAgentName, &row.SourceAgentVersion, &row.SourceAgentPlatformOS, &row.SourceAgentPlatformArch, &row.CaptureID, &row.CaptureFingerprint, &row.ActorHandle, &row.ActorKind, &row.ActorRole, &row.AgentName, &row.Model, &row.Version, &row.AuthorizedBy, &row.StdoutStorageKey, &row.StderrStorageKey, &row.StdoutKind, &row.StderrKind, &row.StdoutSHA256, &row.StderrSHA256, &row.StdoutByteLength, &row.StderrByteLength, &row.StdoutMediaType, &row.StderrMediaType, &row.StdoutCreatedAt, &row.StderrCreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func loadReportFindings(ctx context.Context, db queryer, engagementID string) ([]reportFindingRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT f.id, f.title, f.severity::text, COALESCE(array_to_json(f.affected_entity_ids)::text, '[]'), COALESCE(array_to_json(f.evidence_action_ids)::text, '[]'), f.remediation, f.status, COALESCE(f.promoted_by::text, ''), COALESCE(actor.handle, ''), f.promoted_at, f.revision, f.updated_at
		FROM finding f
		LEFT JOIN actor ON actor.id = f.promoted_by
		WHERE f.engagement_id = $1
		ORDER BY f.promoted_at DESC NULLS LAST, f.updated_at DESC, f.id DESC`, engagementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]reportFindingRow, 0)
	for rows.Next() {
		var row reportFindingRow
		if err := rows.Scan(&row.ID, &row.Title, &row.Severity, &row.AffectedJSON, &row.EvidenceJSON, &row.Remediation, &row.Status, &row.PromotedBy, &row.PromotedByHandle, &row.PromotedAt, &row.Revision, &row.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func buildEvidenceCards(ctx context.Context, actions []reportActionRow, labels map[string]string, store *evidenceStore) []reportEvidence {
	out := make([]reportEvidence, 0, len(actions))
	for _, action := range actions {
		stdout := readEvidenceSnippet(ctx, store, action.StdoutStorageKey)
		stderr := readEvidenceSnippet(ctx, store, action.StderrStorageKey)
		target := strings.TrimSpace(action.TargetValue)
		if action.TargetKind != "" {
			target = strings.TrimSpace(action.TargetKind + ": " + target)
		}
		rawSnippet := stdout
		if rawSnippet == "" {
			rawSnippet = stderr
		}
		note := "Capture snapshot preserved as text."
		if action.ParseStatus == "needs-plugin" || action.ParseStatus == "raw" {
			note = "Unknown tools remain raw-first; the report keeps evidence instead of dropping it."
		}
		exitCode := "not recorded"
		if action.ExitCode.Valid {
			exitCode = strconv.FormatInt(action.ExitCode.Int64, 10)
		}
		out = append(out, reportEvidence{
			Label:              labels[action.ID],
			Source:             commandLine(action.Command, action.ArgvJSON),
			SourceAgent:        sourceAgentSummary(action),
			CaptureID:          nullableString(action.CaptureID),
			CaptureFingerprint: nullableString(action.CaptureFingerprint),
			Command:            commandLine(action.Command, action.ArgvJSON),
			Target:             target,
			Actor:              actorDisplay(action),
			Host:               action.ExecHostIP,
			Egress:             egressSummary(action),
			EgressMode:         action.EgressMode.String,
			EgressStatus:       action.EgressStatus.String,
			EgressObservedAt:   formatRFC3339(action.EgressObservedAt),
			PivotChain:         mustPivotChain(action.PivotChainJSON),
			StartedAt:          action.StartedAt.UTC().Format(time.RFC3339),
			EndedAt:            formatTimePtr(action.EndedAt),
			Duration:           timingDuration(action.StartedAt, action.EndedAt),
			ExitCode:           exitCode,
			ExecutionStatus:    action.ExecutionStatus.String,
			ExecutionSignal:    action.ExecutionSignal.String,
			ExecutionFailure:   action.ExecutionFailureCode.String,
			InitiatedBy:        action.InitiatedBy,
			ParseStatus:        action.ParseStatus,
			Stdout:             reportEvidenceBlob{Kind: action.StdoutKind, MediaType: action.StdoutMediaType, ByteLength: action.StdoutByteLength, SHA256: action.StdoutSHA256, StorageKey: action.StdoutStorageKey, CreatedAt: formatTimePtr(action.StdoutCreatedAt)},
			Stderr:             reportEvidenceBlob{Kind: action.StderrKind, MediaType: action.StderrMediaType, ByteLength: action.StderrByteLength, SHA256: action.StderrSHA256, StorageKey: action.StderrStorageKey, CreatedAt: formatTimePtr(action.StderrCreatedAt)},
			RawStdout:          stdout,
			RawStderr:          stderr,
			RawSnippet:         rawSnippet,
			Note:               note,
			Attribution:        attributionLine(action),
		})
	}
	return out
}

func buildAttribution(actions []reportActionRow) []reportAttribution {
	humanSet := map[string]struct{}{}
	aiSet := map[string]struct{}{}
	execSet := map[string]struct{}{}
	egressSet := map[string]struct{}{}
	for _, action := range actions {
		if action.ActorKind == "ai_agent" {
			aiSet[actorDisplay(action)] = struct{}{}
		} else {
			humanSet[action.ActorHandle] = struct{}{}
		}
		execSet[action.ExecHostIP] = struct{}{}
		if v := egressDisplay(action.EgressPublicIP); v != "not recorded" {
			egressSet[v] = struct{}{}
		}
	}
	return []reportAttribution{
		{Title: "Operator", Items: sortedKeys(humanSet)},
		{Title: "AI actor", Items: sortedKeys(aiSet)},
		{Title: "Exec host IP", Items: sortedKeys(execSet)},
		{Title: "Public egress IP", Items: sortedKeys(egressSet)},
	}
}

func loadReportCaptureGaps(ctx context.Context, db queryer, engagementID string) ([]outOfBandClaimItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT ON (subject_id) subject_id
		FROM audit_event
		WHERE engagement_id = $1 AND subject_type = 'out_of_band_claim' AND type IN ('out-of-band.flagged', 'out-of-band.resolved')
		ORDER BY subject_id, subject_revision DESC, id DESC`, engagementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []outOfBandClaimItem
	for rows.Next() {
		var claimID string
		if err := rows.Scan(&claimID); err != nil {
			return nil, err
		}
		rows, err := loadOutOfBandClaimTimeline(ctx, db, engagementID, claimID)
		if err != nil {
			return nil, err
		}
		item, err := buildOutOfBandClaim(engagementID, rows)
		if err != nil {
			return nil, err
		}
		if item.Status == outOfBandClaimStatusLinked {
			continue
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func sortReportCaptureGaps(gaps []outOfBandClaimItem) []outOfBandClaimItem {
	out := append([]outOfBandClaimItem(nil), gaps...)
	sort.SliceStable(out, func(i, j int) bool {
		if captureGapRank(out[i].Status) != captureGapRank(out[j].Status) {
			return captureGapRank(out[i].Status) < captureGapRank(out[j].Status)
		}
		if !out[i].ObservedAt.Equal(out[j].ObservedAt) {
			return out[i].ObservedAt.Before(out[j].ObservedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func resolveReportSnapshot(ctx context.Context, db queryer, store *evidenceStore, engagementID string) (reportSnapshot, error) {
	return resolveReportSnapshotWithRuntime(ctx, db, store, engagementID, RuntimeState{})
}

func resolveReportSnapshotWithRuntime(ctx context.Context, db queryer, store *evidenceStore, engagementID string, runtime RuntimeState) (reportSnapshot, error) {
	if db != nil {
		if snapshot, ok, err := loadFrozenReportSnapshot(ctx, db, engagementID); err != nil {
			return reportSnapshot{}, err
		} else if ok {
			return snapshot, nil
		}
	}
	return buildReportSnapshotWithRuntime(ctx, db, store, engagementID, runtime)
}

func loadFrozenReportSnapshot(ctx context.Context, db queryer, engagementID string) (reportSnapshot, bool, error) {
	row := db.QueryRowContext(ctx, `
		SELECT j.id, COALESCE(r.bundle_path::text, 'bundle')
		FROM export_receipt r
		JOIN export_job j ON j.id = r.export_job_id
		WHERE r.engagement_id = $1 AND r.status = 'verified'
		ORDER BY r.verified_at DESC, r.id DESC
		LIMIT 1`, engagementID)
	var jobID, bundlePath string
	if err := row.Scan(&jobID, &bundlePath); err != nil {
		if errors.Is(err, sql.ErrNoRows) || isMissingReportBundleError(err) {
			return reportSnapshot{}, false, nil
		}
		return reportSnapshot{}, false, err
	}
	snapshot, err := loadFrozenReportSnapshotFromBundle(jobID, bundlePath)
	if err != nil {
		// A verified receipt can outlive its bundle files (exports default to a
		// tmp dir, so a container restart wipes them while the receipt row in
		// postgres survives). A missing snapshot must not brick the report:
		// fall back to building it live from the database.
		if errors.Is(err, fs.ErrNotExist) {
			log.Printf("frozen report snapshot for engagement %s (export job %s) is missing on disk; rebuilding the report live: %v", engagementID, jobID, err)
			return reportSnapshot{}, false, nil
		}
		return reportSnapshot{}, false, err
	}
	return snapshot, true, nil
}

func loadFrozenReportSnapshotFromBundle(jobID, bundlePath string) (reportSnapshot, error) {
	snapshotPath, err := frozenReportSnapshotPath(jobID, bundlePath)
	if err != nil {
		return reportSnapshot{}, err
	}
	raw, err := os.ReadFile(snapshotPath)
	if err != nil {
		return reportSnapshot{}, fmt.Errorf("read frozen report snapshot: %w", err)
	}
	var snapshot reportSnapshot
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return reportSnapshot{}, fmt.Errorf("decode frozen report snapshot: %w", err)
	}
	return snapshot, nil
}

func frozenReportSnapshotPath(jobID, bundlePath string) (string, error) {
	root := strings.TrimSpace(os.Getenv("WAYPOINT_EXPORT_DIR"))
	if root == "" {
		root = filepath.Join(os.TempDir(), "waypoint", "exports")
	}
	if strings.TrimSpace(jobID) == "" {
		return "", fmt.Errorf("missing export job id")
	}
	clean := filepath.Clean("/" + filepath.ToSlash(bundlePath))
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || clean == "." || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("unsafe bundle path")
	}
	return filepath.Join(root, jobID, filepath.FromSlash(clean), "report", "report-snapshot.json"), nil
}

func isMissingReportBundleError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") || strings.Contains(msg, "undefined table") || strings.Contains(msg, "relation \"export_receipt\"") || strings.Contains(msg, "relation \"export_job\"")
}

func captureGapRank(status string) int {
	switch strings.TrimSpace(status) {
	case outOfBandClaimStatusPending:
		return 0
	case outOfBandClaimStatusDismissed:
		return 1
	default:
		return 2
	}
}

func readEvidenceSnippet(ctx context.Context, store *evidenceStore, storageKey string) string {
	if store == nil || strings.TrimSpace(storageKey) == "" {
		return ""
	}
	path, err := safeEvidencePath(store.root, storageKey)
	if err != nil {
		return "[evidence unavailable]"
	}
	f, err := os.Open(path)
	if err != nil {
		return "[evidence unavailable]"
	}
	defer f.Close()
	buf := make([]byte, 512)
	if ctx != nil {
		select {
		case <-ctx.Done():
			return "[evidence unavailable]"
		default:
		}
	}
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "[evidence unavailable]"
	}
	if n == 0 {
		return ""
	}
	out := string(buf[:n])
	if n == len(buf) {
		out += "\n[truncated]"
	}
	return out
}

func safeEvidencePath(root, storageKey string) (string, error) {
	clean := filepath.Clean("/" + filepath.ToSlash(storageKey))
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || clean == "." || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("unsafe evidence path")
	}
	return filepath.Join(root, filepath.FromSlash(clean)), nil
}

// reportMode selects which sections of the branded report are rendered.
type reportMode string

const (
	reportModeFull     reportMode = "full"     // cover, exec summary, findings, methodology, evidence, attribution, gaps
	reportModeFindings reportMode = "findings" // cover, exec summary, findings + remediation only — client-facing
)

// reportView is the render model handed to the template. It wraps the frozen
// snapshot with presentation-only data (the brand mark, severity tallies, the
// content hash printed in the footer) so the template stays declarative and the
// same snapshot drives both the full and findings-only documents.
type reportView struct {
	Snapshot      reportSnapshot
	FindingsOnly  bool
	Mark          template.HTML
	GeneratedAt   string
	ContentHash   string
	TotalFindings int
	EvidenceCount int
	Severities    []reportSeverityTally
}

type reportSeverityTally struct {
	Label string
	Slug  string
	Count int
}

// reportMark is the Waypoint brand mark: a compass/waypoint star on a bark
// disc, in the expedition palette. Inline SVG so it renders identically in the
// app and in the offline chromium PDF pass (no external asset to fetch).
const reportMark = `<svg class="wp-mark" viewBox="0 0 64 64" role="img" aria-label="Waypoint">
  <circle cx="32" cy="32" r="30" fill="#3B2617"/>
  <circle cx="32" cy="32" r="30" fill="none" stroke="#EF9F27" stroke-width="2.5"/>
  <path d="M32 7 L37.5 26.5 L57 32 L37.5 37.5 L32 57 L26.5 37.5 L7 32 L26.5 26.5 Z" fill="#EF9F27"/>
  <path d="M32 18 L35 29 L46 32 L35 35 L32 46 L29 35 L18 32 L29 29 Z" fill="#FAC775"/>
  <circle cx="32" cy="32" r="3.4" fill="#FAEEDA"/>
</svg>`

func newReportView(snapshot reportSnapshot, mode reportMode) reportView {
	order := []struct{ label, slug string }{
		{"Critical", "critical"}, {"High", "high"}, {"Medium", "medium"}, {"Low", "low"}, {"Info", "info"},
	}
	counts := map[string]int{}
	for _, f := range snapshot.Findings {
		counts[strings.ToLower(strings.TrimSpace(f.Severity))]++
	}
	tallies := make([]reportSeverityTally, 0, len(order))
	for _, o := range order {
		tallies = append(tallies, reportSeverityTally{Label: o.label, Slug: o.slug, Count: counts[o.slug]})
	}
	// The content hash pins the printed document to the exact snapshot bytes, so
	// a reader can tell two PDFs apart and match one to its frozen source.
	digest := sha256.Sum256(reportSnapshotDigestBytes(snapshot))
	return reportView{
		Snapshot:      snapshot,
		FindingsOnly:  mode == reportModeFindings,
		Mark:          template.HTML(reportMark),
		GeneratedAt:   time.Now().UTC().Format("2006-01-02 15:04 MST"),
		ContentHash:   hex.EncodeToString(digest[:])[:12],
		TotalFindings: len(snapshot.Findings),
		EvidenceCount: len(snapshot.Evidence),
		Severities:    tallies,
	}
}

// reportSnapshotDigestBytes hashes the stable fields of the snapshot for the
// footer stamp. It deliberately ignores presentation timestamps so the same
// engagement state always prints the same hash.
func reportSnapshotDigestBytes(snapshot reportSnapshot) []byte {
	stable := snapshot
	raw, err := json.Marshal(stable)
	if err != nil {
		return []byte(snapshot.Engagement + snapshot.Cutoff)
	}
	return raw
}

func renderReportHTML(snapshot reportSnapshot) (string, error) {
	return renderReportHTMLMode(snapshot, reportModeFull)
}

func renderReportHTMLMode(snapshot reportSnapshot, mode reportMode) (string, error) {
	var buf bytes.Buffer
	if err := reportTemplate.Execute(&buf, newReportView(snapshot, mode)); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// renderFindingsCSV emits the promoted findings as a flat spreadsheet: one row
// per finding, columns that import cleanly into a tracker. Evidence references
// are resolved to their action labels so the CSV stands alone.
func renderFindingsCSV(snapshot reportSnapshot) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	header := []string{"id", "title", "severity", "status", "affected_assets", "evidence", "promoted_by", "promoted_at", "revision", "remediation"}
	if err := w.Write(header); err != nil {
		return nil, err
	}
	for _, f := range snapshot.Findings {
		row := []string{
			f.ID,
			f.Title,
			f.Severity,
			f.Status,
			strings.Join(f.AffectedEntityIDs, "; "),
			strings.Join(f.Evidence, "; "),
			f.PromotedBy,
			f.PromotedAt,
			strconv.Itoa(f.Revision),
			f.Remediation,
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderReportPDF(ctx context.Context, snapshot reportSnapshot) ([]byte, error) {
	return renderReportPDFMode(ctx, snapshot, reportModeFull)
}

func renderReportPDFMode(ctx context.Context, snapshot reportSnapshot, mode reportMode) ([]byte, error) {
	html, err := renderReportHTMLMode(snapshot, mode)
	if err != nil {
		return nil, err
	}
	tmpDir, err := os.MkdirTemp("", "waypoint-report-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	htmlPath := filepath.Join(tmpDir, "report.html")
	pdfPath := filepath.Join(tmpDir, "report.pdf")
	if err := os.WriteFile(htmlPath, []byte(html), 0o600); err != nil {
		return nil, err
	}

	chromium := strings.TrimSpace(os.Getenv("WAYPOINT_CHROMIUM"))
	if chromium == "" {
		chromium = "/usr/bin/chromium"
	}
	args := []string{
		"--headless=new",
		"--disable-gpu",
		"--disable-background-networking",
		"--disable-component-update",
		"--disable-default-apps",
		"--disable-extensions",
		"--disable-features=Translate,MediaRouter,OptimizationHints",
		"--disable-sync",
		"--disable-dev-shm-usage",
		"--metrics-recording-only",
		"--no-first-run",
		"--no-default-browser-check",
		"--no-pings",
		// Suppress Chromium's default date/URL/page-number header and footer.
		// --no-pdf-header-footer is the flag that works in current Chromium
		// (152+); --print-to-pdf-no-header is the older spelling, kept for
		// older binaries. Without this the print engine bakes an ugly
		// "file:///tmp/... 1/2" footer into every page.
		"--no-pdf-header-footer",
		"--print-to-pdf-no-header",
		"--print-to-pdf=" + pdfPath,
	}
	// Chromium's sandbox is unavailable in common deployments: it refuses to
	// start as root, and under Docker's default seccomp profile the unprivileged
	// user-namespace clone it needs is denied. Render sandboxed where possible;
	// on the known sandbox-failure signatures retry once without it (the input
	// is our own template output rendered from a local file, not the open web)
	// and remember the outcome so later renders skip the doomed first attempt.
	sandboxless := os.Geteuid() == 0 || chromiumNeedsNoSandbox.Load()
	run := func(noSandbox bool) ([]byte, error) {
		runArgs := args
		if noSandbox {
			runArgs = append(append([]string{}, args...), "--no-sandbox")
		}
		runArgs = append(runArgs, (&url.URL{Scheme: "file", Path: filepath.ToSlash(htmlPath)}).String())
		cmd := exec.CommandContext(ctx, chromium, runArgs...)
		cmd.Env = append(os.Environ(), "LC_ALL=C")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("render report pdf: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return os.ReadFile(pdfPath)
	}
	pdf, err := run(sandboxless)
	if err != nil && !sandboxless && isChromiumSandboxFailure(err) {
		chromiumNeedsNoSandbox.Store(true)
		log.Printf("chromium sandbox unavailable in this environment; rendering PDFs with --no-sandbox: %v", err)
		pdf, err = run(true)
	}
	return pdf, err
}

// chromiumNeedsNoSandbox latches once a sandboxed launch has failed with a
// known environment signature, so every later render goes straight to
// --no-sandbox instead of re-paying a failed chromium spawn.
var chromiumNeedsNoSandbox atomic.Bool

func isChromiumSandboxFailure(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "Failed to move to new namespace") ||
		strings.Contains(msg, "Running as root without --no-sandbox") ||
		strings.Contains(msg, "No usable sandbox")
}

func commandLine(command, argvJSON string) string {
	argv := []string{}
	if err := json.Unmarshal([]byte(argvJSON), &argv); err != nil || len(argv) == 0 {
		return strings.TrimSpace(command)
	}
	parts := append([]string{strings.TrimSpace(command)}, argv...)
	return strings.Join(parts, " ")
}

func labelsForActionIDs(evidenceJSON string, labels map[string]string) []string {
	var ids []string
	if err := json.Unmarshal([]byte(evidenceJSON), &ids); err != nil {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if label := labels[id]; label != "" {
			out = append(out, label)
		}
	}
	return out
}

func actorDisplay(action reportActionRow) string {
	if action.ActorKind == "ai_agent" {
		base := action.ActorHandle
		if action.Model != "" {
			base += " · model " + action.Model
		}
		if action.Version != "" {
			base += " · version " + action.Version
		}
		if action.AuthorizedBy.Valid && strings.TrimSpace(action.AuthorizedBy.String) != "" {
			base += " · authorized by " + action.AuthorizedBy.String
		}
		return base
	}
	return action.ActorHandle
}

func attributionLine(action reportActionRow) string {
	line := action.ExecHostIP
	if egress := egressSummary(action); egress != "not recorded" {
		line += " → " + egress
	}
	return line
}

func egressSummary(action reportActionRow) string {
	parts := make([]string, 0, 4)
	if mode := strings.TrimSpace(action.EgressMode.String); mode != "" {
		parts = append(parts, mode)
	}
	if status := strings.TrimSpace(action.EgressStatus.String); status != "" {
		parts = append(parts, status)
	}
	if addr := egressDisplay(action.EgressPublicIP); addr != "not recorded" {
		parts = append(parts, addr)
	}
	if action.EgressObservedAt.Valid {
		parts = append(parts, action.EgressObservedAt.Time.UTC().Format(time.RFC3339))
	}
	if len(parts) == 0 {
		return "not recorded"
	}
	return strings.Join(parts, " · ")
}

func egressDisplay(v sql.NullString) string {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return "not recorded"
	}
	return strings.TrimSpace(v.String)
}

func pivotChainSummary(chain []capturePivotHop) string {
	if len(chain) == 0 {
		return "none recorded"
	}
	parts := make([]string, 0, len(chain))
	for _, hop := range chain {
		label := strings.TrimSpace(hop.Type)
		if hop.Host != "" {
			label = strings.TrimSpace(label + " " + hop.Host)
		}
		if hop.Port != nil {
			label = strings.TrimSpace(fmt.Sprintf("%s:%d", label, *hop.Port))
		}
		if hop.Label != "" {
			label = strings.TrimSpace(label + " " + hop.Label)
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " → ")
}

func splitScope(scope string) []string {
	lines := strings.Split(scope, "\n")
	out := make([]string, 0, len(lines))
	seen := map[string]struct{}{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func reportMethodology() []string {
	return []string{
		"Recon: preserve raw discovery and entity provenance.",
		"Attacks: capture every attempt with command, host, IPs, timing, outcome, and execution semantics.",
		"Findings: promote only confirmed results, retain revisions, and keep evidence linked.",
		"Export: freeze the snapshot before PDF rendering, bundle manifest generation, and restore verification.",
	}
}

func sortedKeys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return []string{"None recorded."}
	}
	out := make([]string, 0, len(m))
	for k := range m {
		if strings.TrimSpace(k) == "" {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return []string{"None recorded."}
	}
	return out
}

func formatRFC3339(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

func formatTimePtr(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

func timingDuration(start time.Time, ended sql.NullTime) string {
	if !ended.Valid {
		return "running"
	}
	delta := ended.Time.Sub(start)
	if delta < 0 {
		return "invalid"
	}
	return delta.Round(time.Millisecond).String()
}

func nullableString(v sql.NullString) string {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return ""
	}
	return strings.TrimSpace(v.String)
}

func mustJSONStrings(raw string) []string {
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func sourceAgentSummary(action reportActionRow) string {
	parts := make([]string, 0, 4)
	if action.SourceAgentKind != "" {
		parts = append(parts, action.SourceAgentKind)
	}
	if action.SourceAgentName != "" {
		parts = append(parts, action.SourceAgentName)
	}
	if action.SourceAgentVersion != "" {
		parts = append(parts, "v"+action.SourceAgentVersion)
	}
	platform := strings.TrimSpace(strings.Join([]string{action.SourceAgentPlatformOS, action.SourceAgentPlatformArch}, "/"))
	if platform != "" && platform != "/" {
		parts = append(parts, platform)
	}
	if action.SourceAgentID != "" {
		parts = append(parts, action.SourceAgentID)
	}
	if len(parts) == 0 {
		return "not recorded"
	}
	return strings.Join(parts, " · ")
}

func formatActionLabel(n int) string {
	return strconv.Itoa(n)
}

func titleWord(v string) string {
	if v == "" {
		return v
	}
	return strings.ToUpper(v[:1]) + v[1:]
}

func severityRank(v string) int {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func auditActorDisplay(v any) string {
	var actor auditEventActor
	switch value := v.(type) {
	case auditEventActor:
		actor = value
	case *auditEventActor:
		if value == nil {
			return ""
		}
		actor = *value
	default:
		return ""
	}
	if actor.Kind == "ai_agent" {
		base := actor.Handle
		if actor.Model != "" {
			base += " · model " + actor.Model
		}
		if actor.Version != "" {
			base += " · version " + actor.Version
		}
		if actor.AuthorizedBy != "" {
			base += " · authorized by " + actor.AuthorizedBy
		}
		return base
	}
	return actor.Handle
}

func captureGapLabel(item outOfBandClaimItem) string {
	kind := strings.TrimSpace(item.ClaimKind)
	if kind == "" {
		return "Capture gap"
	}
	return titleWord(kind) + " capture gap"
}

func captureGapSourceActionID(item outOfBandClaimItem) string {
	if item.SourceActionID == nil {
		return ""
	}
	return strings.TrimSpace(*item.SourceActionID)
}

var reportTemplate = template.Must(template.New("report").Funcs(template.FuncMap{
	"join":                     strings.Join,
	"lower":                    strings.ToLower,
	"captureGapLabel":          captureGapLabel,
	"captureGapSourceActionID": captureGapSourceActionID,
	"auditActorDisplay":        auditActorDisplay,
	"pivotChainSummary":        pivotChainSummary,
}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{if .FindingsOnly}}Findings — {{end}}{{.Snapshot.Engagement}} · Waypoint</title>
  <style>
    :root {
      color-scheme: light;
      --deep-bark: #3B2617; --bark: #4A2F1B; --saddle: #6B4423; --trail: #8B5E34;
      --harvest: #BA7517; --lantern: #EF9F27; --wheat: #FAC775; --parchment: #FAEEDA;
      --map-cream: #E8DCC3; --contour: #D4C4A0; --dark-cocoa: #633806; --cocoa: #854F0B;
      --stone: #B4A78C; --ink: #2A1B10;
      --sev-critical: #B4231A; --sev-high: #C65A11; --sev-medium: #BA7517;
      --sev-low: #4E7D18; --sev-info: #6B4423;
    }
    * { box-sizing: border-box; }
    html, body { margin: 0; }
    body { background: #fff; color: var(--ink); font: 11pt/1.55 "Iowan Old Style", "Palatino Linotype", Palatino, Georgia, serif; -webkit-print-color-adjust: exact; print-color-adjust: exact; }
    h1, h2, h3, h4, p, ul, ol { margin: 0; }
    .wp-mark { display: block; }
    .sans { font-family: "Helvetica Neue", Arial, system-ui, sans-serif; }

    /* Cover */
    .cover { min-height: 247mm; display: flex; flex-direction: column; break-after: page; }
    .cover-band { background: var(--deep-bark); color: var(--parchment); border-radius: 14px; padding: 30px 34px; display: flex; align-items: center; gap: 20px; }
    .cover-band svg { width: 68px; height: 68px; flex: 0 0 auto; }
    .cover-wordmark { font-family: "Helvetica Neue", Arial, sans-serif; }
    .cover-wordmark .name { font-size: 30px; font-weight: 700; letter-spacing: 0.16em; }
    .cover-wordmark .kicker { font-size: 11px; letter-spacing: 0.34em; text-transform: uppercase; color: var(--wheat); margin-top: 4px; }
    .cover-body { flex: 1; display: flex; flex-direction: column; justify-content: center; padding: 10mm 4mm; }
    .cover-body .doctype { font-family: "Helvetica Neue", Arial, sans-serif; text-transform: uppercase; letter-spacing: 0.28em; font-size: 11px; color: var(--cocoa); }
    .cover-body h1 { font-size: 34pt; line-height: 1.08; color: var(--deep-bark); margin: 12px 0 4px; max-width: 15em; }
    .cover-body .client { font-size: 15pt; color: var(--saddle); }
    .cover-facts { margin-top: 26px; display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px 22px; max-width: 150mm; }
    .cover-facts .fact { border-top: 1.4pt solid var(--harvest); padding-top: 7px; }
    .cover-facts .fact .k { font-family: "Helvetica Neue", Arial, sans-serif; font-size: 8pt; text-transform: uppercase; letter-spacing: 0.14em; color: var(--cocoa); }
    .cover-facts .fact .v { font-size: 11pt; color: var(--deep-bark); margin-top: 2px; }
    .cover-sevbar { margin-top: 30px; display: flex; gap: 10px; flex-wrap: wrap; }
    .sevchip { font-family: "Helvetica Neue", Arial, sans-serif; display: inline-flex; align-items: baseline; gap: 7px; border-radius: 999px; padding: 6px 14px; font-size: 9.5pt; color: #fff; }
    .sevchip .n { font-weight: 700; font-size: 12pt; }
    .sevchip.zero { background: var(--map-cream) !important; color: var(--stone); }
    .sev-critical { background: var(--sev-critical); } .sev-high { background: var(--sev-high); }
    .sev-medium { background: var(--sev-medium); } .sev-low { background: var(--sev-low); } .sev-info { background: var(--sev-info); }
    .cover-foot { font-family: "Helvetica Neue", Arial, sans-serif; font-size: 8pt; color: var(--stone); border-top: 0.6pt solid var(--contour); padding-top: 8px; }
    .cover-foot strong { color: var(--cocoa); }

    /* Document body */
    .doc { padding-top: 2mm; }
    section.block { break-inside: auto; margin-bottom: 16px; }
    .sechead { display: flex; align-items: center; gap: 10px; border-bottom: 1.4pt solid var(--harvest); padding-bottom: 6px; margin-bottom: 12px; break-after: avoid; }
    .sechead h2 { font-family: "Helvetica Neue", Arial, sans-serif; font-size: 15pt; color: var(--deep-bark); letter-spacing: 0.01em; }
    .sechead .sn { font-family: "Helvetica Neue", Arial, sans-serif; font-size: 9pt; color: var(--cocoa); margin-left: auto; letter-spacing: 0.1em; text-transform: uppercase; }
    p.lead { color: var(--bark); margin-bottom: 10px; }
    ul.trail { list-style: none; padding: 0; }
    ul.trail li { position: relative; padding-left: 20px; margin-bottom: 6px; }
    ul.trail li::before { content: ""; position: absolute; left: 4px; top: 0.55em; width: 6px; height: 6px; border-radius: 50%; background: var(--harvest); }

    /* Executive summary tiles */
    .tiles { display: grid; grid-template-columns: repeat(6, 1fr); gap: 10px; }
    .tile { border: 0.8pt solid var(--contour); border-radius: 10px; padding: 12px 10px; text-align: center; break-inside: avoid; }
    .tile .n { font-family: "Helvetica Neue", Arial, sans-serif; font-size: 21pt; font-weight: 700; line-height: 1; color: var(--deep-bark); }
    .tile .l { font-family: "Helvetica Neue", Arial, sans-serif; font-size: 7.5pt; text-transform: uppercase; letter-spacing: 0.1em; color: var(--cocoa); margin-top: 6px; }
    .tile.t-total { background: var(--deep-bark); border-color: var(--deep-bark); }
    .tile.t-total .n { color: var(--wheat); } .tile.t-total .l { color: var(--wheat); }
    .tile.t-critical .n { color: var(--sev-critical); } .tile.t-high .n { color: var(--sev-high); }
    .tile.t-medium .n { color: var(--sev-medium); } .tile.t-low .n { color: var(--sev-low); } .tile.t-info .n { color: var(--sev-info); }

    /* Findings */
    .finding { border: 0.8pt solid var(--contour); border-left: 5px solid var(--saddle); border-radius: 10px; padding: 14px 16px; margin-bottom: 12px; break-inside: avoid; }
    .finding.f-critical { border-left-color: var(--sev-critical); } .finding.f-high { border-left-color: var(--sev-high); }
    .finding.f-medium { border-left-color: var(--sev-medium); } .finding.f-low { border-left-color: var(--sev-low); } .finding.f-info { border-left-color: var(--sev-info); }
    .finding .fhead { display: flex; align-items: center; gap: 10px; margin-bottom: 8px; }
    .finding .fbadge { font-family: "Helvetica Neue", Arial, sans-serif; font-size: 7.5pt; font-weight: 700; text-transform: uppercase; letter-spacing: 0.08em; color: #fff; border-radius: 5px; padding: 3px 8px; }
    .finding .fno { font-family: "Helvetica Neue", Arial, sans-serif; font-size: 8pt; color: var(--stone); margin-left: auto; }
    .finding h3 { font-size: 13.5pt; color: var(--deep-bark); line-height: 1.2; }
    .finding .fmeta { font-family: "Helvetica Neue", Arial, sans-serif; font-size: 8.5pt; color: var(--cocoa); margin: 6px 0 10px; display: flex; flex-wrap: wrap; gap: 4px 16px; }
    .finding .frow { margin-top: 7px; }
    .finding .frow .k { font-family: "Helvetica Neue", Arial, sans-serif; font-size: 8pt; text-transform: uppercase; letter-spacing: 0.08em; color: var(--saddle); display: block; margin-bottom: 1px; }
    .finding .frow .v { color: var(--ink); }
    .empty { color: var(--stone); font-style: italic; }

    /* Evidence appendix */
    .evidence { border: 0.8pt solid var(--contour); border-radius: 10px; padding: 13px 15px; margin-bottom: 11px; break-inside: avoid; }
    .evidence .ehead { display: flex; align-items: baseline; gap: 10px; margin-bottom: 8px; }
    .evidence .elabel { font-family: "Helvetica Neue", Arial, sans-serif; font-size: 8pt; font-weight: 700; text-transform: uppercase; letter-spacing: 0.08em; color: var(--parchment); background: var(--saddle); border-radius: 5px; padding: 2px 8px; }
    .evidence .ecmd { font-family: ui-monospace, "SFMono-Regular", Menlo, monospace; font-size: 9pt; color: var(--deep-bark); word-break: break-all; }
    .evidence dl { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 4px 20px; margin: 0; }
    .evidence dl > div { display: flex; gap: 6px; font-size: 9pt; }
    .evidence dt { font-family: "Helvetica Neue", Arial, sans-serif; font-size: 7.5pt; text-transform: uppercase; letter-spacing: 0.06em; color: var(--cocoa); flex: 0 0 34%; padding-top: 1px; }
    .evidence dd { margin: 0; color: var(--ink); word-break: break-word; }
    .evidence pre { margin: 9px 0 0; white-space: pre-wrap; word-break: break-word; font-family: ui-monospace, "SFMono-Regular", Menlo, monospace; font-size: 8pt; line-height: 1.4; color: var(--bark); background: var(--parchment); border: 0.6pt solid var(--contour); border-radius: 7px; padding: 8px 10px; }
    .evidence pre.empty-pre { display: none; }
    .evidence .enote { font-size: 8.5pt; color: var(--cocoa); margin-top: 7px; font-style: italic; }

    /* Attribution */
    .attrib { display: grid; grid-template-columns: repeat(2, 1fr); gap: 10px; }
    .attrib .acard { border: 0.8pt solid var(--contour); border-radius: 9px; padding: 11px 13px; break-inside: avoid; }
    .attrib .acard h4 { font-family: "Helvetica Neue", Arial, sans-serif; font-size: 9pt; text-transform: uppercase; letter-spacing: 0.08em; color: var(--saddle); margin-bottom: 6px; }
    .attrib .acard ul { list-style: none; padding: 0; }
    .attrib .acard li { font-size: 9.5pt; padding: 2px 0; border-bottom: 0.5pt dotted var(--contour); }
    .attrib .acard li:last-child { border-bottom: none; }

    ul.gaps { list-style: none; padding: 0; }
    ul.gaps li { border-left: 3px solid var(--harvest); padding: 4px 0 4px 12px; margin-bottom: 7px; font-size: 9.5pt; }

    .docfoot { margin-top: 22px; padding-top: 10px; border-top: 1.4pt solid var(--harvest); display: flex; align-items: center; gap: 8px; break-inside: avoid;
      font-family: "Helvetica Neue", Arial, sans-serif; font-size: 8pt; color: var(--saddle); }
    .docfoot svg { width: 15px; height: 15px; flex: 0 0 auto; }
    .docfoot strong { color: var(--cocoa); letter-spacing: 0.08em; }

    @page { size: A4; margin: 16mm 15mm; }
  </style>
</head>
<body>
  <section class="cover">
    <div class="cover-band">
      {{.Mark}}
      <div class="cover-wordmark">
        <div class="name">WAYPOINT</div>
        <div class="kicker">Security Assessment</div>
      </div>
    </div>
    <div class="cover-body">
      <div class="doctype">{{if .FindingsOnly}}Findings Summary{{else}}Engagement Report{{end}}</div>
      <h1>{{.Snapshot.Engagement}}</h1>
      {{if .Snapshot.Client}}<div class="client">Prepared for {{.Snapshot.Client}}</div>{{end}}

      <div class="cover-facts">
        <div class="fact"><div class="k">Findings</div><div class="v">{{.TotalFindings}} promoted</div></div>
        <div class="fact"><div class="k">Evidence cutoff</div><div class="v">{{.Snapshot.Cutoff}}</div></div>
        <div class="fact"><div class="k">Generated</div><div class="v">{{.GeneratedAt}}</div></div>
        {{if .Snapshot.Scope}}<div class="fact" style="grid-column: 1 / -1;"><div class="k">Scope</div><div class="v">{{join .Snapshot.Scope " · "}}</div></div>{{end}}
      </div>

      <div class="cover-sevbar">
        {{range .Severities}}<span class="sevchip sev-{{.Slug}}{{if eq .Count 0}} zero{{end}}"><span class="n">{{.Count}}</span>{{.Label}}</span>{{end}}
      </div>
    </div>
    <div class="cover-foot">
      <strong>Confidential.</strong> Prepared by Waypoint from a frozen engagement snapshot. Content hash #{{.ContentHash}} · Snapshot {{.Snapshot.Version}} · Distribution limited to the client and authorized parties.
    </div>
  </section>

  <main class="doc">
    <section class="block">
      <div class="sechead"><h2>Executive summary</h2><span class="sn">01</span></div>
      <p class="lead">This assessment promoted {{.TotalFindings}} confirmed finding{{if ne .TotalFindings 1}}s{{end}} against {{.Snapshot.Engagement}}, each backed by preserved capture evidence with full command, host, and actor attribution. Severity distribution:</p>
      <div class="tiles">
        <div class="tile t-total"><div class="n">{{.TotalFindings}}</div><div class="l">Total</div></div>
        {{range .Severities}}<div class="tile t-{{.Slug}}"><div class="n">{{.Count}}</div><div class="l">{{.Label}}</div></div>{{end}}
      </div>
    </section>

    <section class="block">
      <div class="sechead"><h2>Findings</h2><span class="sn">02</span></div>
      {{range $i, $f := .Snapshot.Findings}}
      <article class="finding f-{{lower $f.Severity}}">
        <div class="fhead">
          <span class="fbadge sev-{{lower $f.Severity}}">{{$f.Severity}}</span>
          <span class="fno">Finding {{$f.ID}}</span>
        </div>
        <h3>{{$f.Title}}</h3>
        <div class="fmeta">
          {{if $f.Status}}<span>Status: {{$f.Status}}</span>{{end}}
          {{if $f.PromotedBy}}<span>Promoted by {{$f.PromotedBy}}</span>{{end}}
          {{if $f.PromotedAt}}<span>{{$f.PromotedAt}}</span>{{end}}
          <span>Revision {{$f.Revision}}</span>
        </div>
        {{if $f.AffectedEntityIDs}}<div class="frow"><span class="k">Affected assets</span><span class="v">{{join $f.AffectedEntityIDs ", "}}</span></div>{{end}}
        {{if $f.Evidence}}<div class="frow"><span class="k">Evidence</span><span class="v">{{join $f.Evidence ", "}}</span></div>{{end}}
        <div class="frow"><span class="k">Remediation</span><span class="v">{{if $f.Remediation}}{{$f.Remediation}}{{else}}<span class="empty">No remediation recorded.</span>{{end}}</span></div>
      </article>
      {{else}}<p class="empty">No findings were promoted in this engagement.</p>{{end}}
    </section>

    {{if not .FindingsOnly}}
    <section class="block">
      <div class="sechead"><h2>Methodology</h2><span class="sn">03</span></div>
      <ul class="trail">{{range .Snapshot.Methodology}}<li>{{.}}</li>{{else}}<li class="empty">None recorded.</li>{{end}}</ul>
    </section>

    {{if or .Snapshot.Runtime.Egress.Mode .Snapshot.Runtime.Egress.Status .Snapshot.Runtime.Egress.Address .Snapshot.Runtime.Egress.ObservedAt .Snapshot.Runtime.Egress.Interface .Snapshot.Runtime.Egress.InterfaceAddress .Snapshot.Runtime.Egress.ResolverEndpoint .Snapshot.Runtime.Egress.Notes}}
    <section class="block">
      <div class="sechead"><h2>Runtime posture</h2></div>
      <ul class="trail">
        <li><strong>Egress:</strong> {{if .Snapshot.Runtime.Egress.Address}}{{.Snapshot.Runtime.Egress.Mode}} · {{.Snapshot.Runtime.Egress.Status}} · {{.Snapshot.Runtime.Egress.Address}}{{else}}{{.Snapshot.Runtime.Egress.Mode}} · {{.Snapshot.Runtime.Egress.Status}}{{end}}</li>
        {{if .Snapshot.Runtime.Egress.ObservedAt}}<li><strong>Observed at:</strong> {{.Snapshot.Runtime.Egress.ObservedAt.UTC.Format "2006-01-02T15:04:05Z07:00"}}</li>{{end}}
        {{if .Snapshot.Runtime.Egress.Interface}}<li><strong>Interface:</strong> {{.Snapshot.Runtime.Egress.Interface}}{{if .Snapshot.Runtime.Egress.InterfaceAddress}} · {{.Snapshot.Runtime.Egress.InterfaceAddress}}{{end}}</li>{{end}}
        {{if .Snapshot.Runtime.Egress.ResolverEndpoint}}<li><strong>Resolver:</strong> {{.Snapshot.Runtime.Egress.ResolverEndpoint}}</li>{{end}}
        {{range .Snapshot.Runtime.Egress.Notes}}<li>{{.}}</li>{{end}}
      </ul>
    </section>
    {{end}}

    <section class="block">
      <div class="sechead"><h2>Evidence appendix</h2><span class="sn">04</span></div>
      <p class="lead">{{.EvidenceCount}} capture{{if ne .EvidenceCount 1}}s{{end}} preserved as text, in chronological order. Each is attributed to its actor, host, and public egress.</p>
      {{range .Snapshot.Evidence}}
      <article class="evidence">
        <div class="ehead"><span class="elabel">{{.Label}}</span><span class="ecmd">{{.Command}}</span></div>
        <dl>
          <div><dt>Source agent</dt><dd>{{.SourceAgent}}</dd></div>
          <div><dt>Capture</dt><dd>{{if .CaptureID}}{{.CaptureID}}{{else}}not recorded{{end}}{{if .CaptureFingerprint}} · {{.CaptureFingerprint}}{{end}}</dd></div>
          <div><dt>Target</dt><dd>{{.Target}}</dd></div>
          <div><dt>Actor</dt><dd>{{.Actor}}</dd></div>
          <div><dt>Exec host</dt><dd>{{.Host}}</dd></div>
          <div><dt>Egress</dt><dd>{{.Egress}}</dd></div>
          <div><dt>Started</dt><dd>{{if .StartedAt}}{{.StartedAt}}{{else}}not recorded{{end}}</dd></div>
          <div><dt>Duration</dt><dd>{{if .Duration}}{{.Duration}}{{else}}not recorded{{end}}</dd></div>
          <div><dt>Exit</dt><dd>{{if .ExitCode}}{{.ExitCode}}{{else}}not recorded{{end}}{{if .ExecutionStatus}} · {{.ExecutionStatus}}{{end}}{{if .ExecutionSignal}} · signal {{.ExecutionSignal}}{{end}}{{if .ExecutionFailure}} · failure {{.ExecutionFailure}}{{end}}</dd></div>
          <div><dt>Pivot chain</dt><dd>{{if .PivotChain}}{{pivotChainSummary .PivotChain}}{{else}}none recorded{{end}}</dd></div>
          <div><dt>Initiated by</dt><dd>{{.InitiatedBy}}</dd></div>
          <div><dt>Parse status</dt><dd>{{.ParseStatus}}</dd></div>
          <div><dt>Stdout</dt><dd>{{.Stdout.ByteLength}} B · {{if .Stdout.SHA256}}{{.Stdout.SHA256}}{{else}}—{{end}}</dd></div>
          <div><dt>Stderr</dt><dd>{{.Stderr.ByteLength}} B · {{if .Stderr.SHA256}}{{.Stderr.SHA256}}{{else}}—{{end}}</dd></div>
          <div><dt>Attribution</dt><dd>{{.Attribution}}</dd></div>
        </dl>
        <pre{{if not .RawStdout}} class="empty-pre"{{end}}>{{.RawStdout}}</pre>
        <pre{{if not .RawStderr}} class="empty-pre"{{end}}>{{.RawStderr}}</pre>
        {{if .Note}}<p class="enote">{{.Note}}</p>{{end}}
      </article>
      {{else}}<p class="empty">No evidence recorded.</p>{{end}}
    </section>

    <section class="block">
      <div class="sechead"><h2>Attribution</h2><span class="sn">05</span></div>
      <div class="attrib">
        {{range .Snapshot.Attribution}}
        <div class="acard">
          <h4>{{.Title}}</h4>
          <ul>{{range .Items}}<li>{{.}}</li>{{else}}<li class="empty">None recorded.</li>{{end}}</ul>
        </div>
        {{end}}
      </div>
    </section>

    <section class="block">
      <div class="sechead"><h2>Known capture gaps</h2><span class="sn">06</span></div>
      <ul class="gaps">{{range .Snapshot.KnownCaptureGaps}}<li><strong>{{captureGapLabel .}}</strong>{{if .Status}} · {{.Status}}{{end}}{{if captureGapSourceActionID .}} · source {{captureGapSourceActionID .}}{{end}}{{if .ObservedBy.Handle}} · observed by {{auditActorDisplay .ObservedBy}}{{end}}{{if .ResolvedBy}} · resolved by {{auditActorDisplay .ResolvedBy}}{{end}}{{if .Reason}} — {{.Reason}}{{end}}{{if .Notes}} · notes: {{.Notes}}{{end}}</li>{{else}}<li class="empty">None recorded.</li>{{end}}</ul>
    </section>
    {{end}}

    <div class="docfoot">{{.Mark}}<span><strong>WAYPOINT</strong> · Confidential · {{.Snapshot.Engagement}} · Content hash #{{.ContentHash}} · Snapshot {{.Snapshot.Version}} · Generated {{.GeneratedAt}}</span></div>
  </main>
</body>
</html>`))

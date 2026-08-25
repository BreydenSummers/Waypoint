package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	reportContractVersion = "1.0.0"
	reportSnapshotVersion = "v1"
)

var (
	reportJSONRoute = regexp.MustCompile(`^/api/v1/engagements/([^/]+)/summit/report(?:\.json)?$`)
	reportPDFRoute  = regexp.MustCompile(`^/(?:api/v1/)?engagements/([^/]+)/summit/report\.pdf$`)
)

type reportSnapshot struct {
	ContractVersion  string               `json:"contractVersion,omitempty"`
	Version          string               `json:"version"`
	Title            string               `json:"title"`
	Engagement       string               `json:"engagement"`
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
	Size       int64  `json:"size"`
	ByteLength int64  `json:"byteLength,omitempty"`
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
	Title       string   `json:"title"`
	Summary     string   `json:"summary,omitempty"`
	Severity    string   `json:"severity"`
	Evidence    []string `json:"evidence"`
	Remediation string   `json:"remediation"`
	Status      string   `json:"status"`
	PromotedBy  string   `json:"promotedBy"`
	PromotedAt  string   `json:"promotedAt"`
}

type reportEvidence struct {
	Label            string            `json:"label"`
	Source           string            `json:"source,omitempty"`
	Command          string            `json:"command"`
	Target           string            `json:"target"`
	Actor            string            `json:"actor"`
	Host             string            `json:"host"`
	Egress           string            `json:"egress"`
	EgressMode       string            `json:"egressMode,omitempty"`
	EgressStatus     string            `json:"egressStatus,omitempty"`
	EgressObservedAt string            `json:"egressObservedAt,omitempty"`
	PivotChain       []capturePivotHop `json:"pivotChain,omitempty"`
	InitiatedBy      string            `json:"initiatedBy"`
	ParseStatus      string            `json:"parseStatus"`
	RawStdout        string            `json:"rawStdout,omitempty"`
	RawStderr        string            `json:"rawStderr,omitempty"`
	RawSnippet       string            `json:"rawSnippet,omitempty"`
	Note             string            `json:"note,omitempty"`
	Attribution      string            `json:"attribution"`
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
	ID               string
	StartedAt        time.Time
	EndedAt          sql.NullTime
	Command          string
	ArgvJSON         string
	TargetKind       string
	TargetValue      string
	ExecHostIP       string
	EgressMode       sql.NullString
	EgressStatus     sql.NullString
	EgressPublicIP   sql.NullString
	EgressObservedAt sql.NullTime
	PivotChainJSON   string
	InitiatedBy      string
	ParseStatus      string
	ActorHandle      string
	ActorKind        string
	ActorRole        string
	AgentName        string
	Model            string
	Version          string
	AuthorizedBy     sql.NullString
	StdoutStorageKey string
	StderrStorageKey string
	StdoutKind       string
	StderrKind       string
	StdoutSHA256     string
	StderrSHA256     string
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
		engagementID := ""
		if m := reportJSONRoute.FindStringSubmatch(path); m != nil {
			engagementID = m[1]
			format = "json"
		} else if m := reportPDFRoute.FindStringSubmatch(path); m != nil {
			engagementID = m[1]
			format = "pdf"
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
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "build frozen report failed"})
			return
		}

		switch format {
		case "json":
			w.Header().Set("Waypoint-Contract-Version", reportContractVersion)
			writeJSON(w, http.StatusOK, snapshot)
		case "pdf":
			pdf, err := renderReportPDF(ctx, snapshot)
			if err != nil {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: err.Error()})
				return
			}
			w.Header().Set("Waypoint-Contract-Version", reportContractVersion)
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", "inline; filename=report.pdf")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(pdf)
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
			Title:       finding.Title,
			Summary:     finding.Title,
			Severity:    titleWord(strings.ToLower(strings.TrimSpace(finding.Severity))),
			Evidence:    labels,
			Remediation: strings.TrimSpace(finding.Remediation),
			Status:      strings.TrimSpace(finding.Status),
			PromotedBy:  strings.TrimSpace(finding.PromotedByHandle),
			PromotedAt:  formatRFC3339(finding.PromotedAt),
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
		       COALESCE(a.egress_mode::text, ''), COALESCE(a.egress_status::text, ''), COALESCE(a.egress_public_ip::text, ''), a.egress_observed_at,
		       COALESCE(a.pivot_chain::text, '[]'), a.initiated_by::text, a.parse_status::text,
		       actor.handle, actor.kind::text, actor.role::text, COALESCE(actor.agent_name, ''), COALESCE(actor.model, ''), COALESCE(actor.version, ''), COALESCE(auth.handle::text, ''),
		       COALESCE(stdout.storage_key, ''), COALESCE(stderr.storage_key, ''), COALESCE(stdout.kind, ''), COALESCE(stderr.kind, ''), COALESCE(stdout.sha256, ''), COALESCE(stderr.sha256, '')
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
		if err := rows.Scan(&row.ID, &row.StartedAt, &row.EndedAt, &row.Command, &row.ArgvJSON, &row.TargetKind, &row.TargetValue, &row.ExecHostIP, &row.EgressMode, &row.EgressStatus, &row.EgressPublicIP, &row.EgressObservedAt, &row.PivotChainJSON, &row.InitiatedBy, &row.ParseStatus, &row.ActorHandle, &row.ActorKind, &row.ActorRole, &row.AgentName, &row.Model, &row.Version, &row.AuthorizedBy, &row.StdoutStorageKey, &row.StderrStorageKey, &row.StdoutKind, &row.StderrKind, &row.StdoutSHA256, &row.StderrSHA256); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func loadReportFindings(ctx context.Context, db queryer, engagementID string) ([]reportFindingRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT f.id, f.title, f.severity::text, COALESCE(array_to_json(f.affected_entity_ids)::text, '[]'), COALESCE(array_to_json(f.evidence_action_ids)::text, '[]'), f.remediation, f.status, COALESCE(f.promoted_by::text, ''), COALESCE(actor.handle, ''), f.promoted_at, f.updated_at
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
		if err := rows.Scan(&row.ID, &row.Title, &row.Severity, &row.AffectedJSON, &row.EvidenceJSON, &row.Remediation, &row.Status, &row.PromotedBy, &row.PromotedByHandle, &row.PromotedAt, &row.UpdatedAt); err != nil {
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
		out = append(out, reportEvidence{
			Label:            labels[action.ID],
			Source:           commandLine(action.Command, action.ArgvJSON),
			Command:          commandLine(action.Command, action.ArgvJSON),
			Target:           target,
			Actor:            actorDisplay(action),
			Host:             action.ExecHostIP,
			Egress:           egressSummary(action),
			EgressMode:       action.EgressMode.String,
			EgressStatus:     action.EgressStatus.String,
			EgressObservedAt: formatRFC3339(action.EgressObservedAt),
			PivotChain:       mustPivotChain(action.PivotChainJSON),
			InitiatedBy:      action.InitiatedBy,
			ParseStatus:      action.ParseStatus,
			RawStdout:        stdout,
			RawStderr:        stderr,
			RawSnippet:       rawSnippet,
			Note:             note,
			Attribution:      attributionLine(action),
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

func renderReportHTML(snapshot reportSnapshot) (string, error) {
	var buf bytes.Buffer
	if err := reportTemplate.Execute(&buf, snapshot); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderReportPDF(ctx context.Context, snapshot reportSnapshot) ([]byte, error) {
	html, err := renderReportHTML(snapshot)
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
	cmd := exec.CommandContext(ctx, chromium,
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
		"--print-to-pdf-no-header",
		"--print-to-pdf="+pdfPath,
		((&url.URL{Scheme: "file", Path: filepath.ToSlash(htmlPath)}).String()),
	)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("render report pdf: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return os.ReadFile(pdfPath)
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
		"Attacks: capture every attempt with command, host, IPs, timing, and outcome.",
		"Findings: promote only confirmed results and keep evidence linked.",
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
	"captureGapLabel":          captureGapLabel,
	"captureGapSourceActionID": captureGapSourceActionID,
	"auditActorDisplay":        auditActorDisplay,
	"pivotChainSummary":        pivotChainSummary,
}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    :root {
      color-scheme: light;
      --deep-bark: #3B2617;
      --bark: #4A2F1B;
      --saddle: #6B4423;
      --trail: #8B5E34;
      --harvest: #BA7517;
      --lantern: #EF9F27;
      --wheat: #FAC775;
      --parchment: #FAEEDA;
      --map-cream: #E8DCC3;
      --contour: #D4C4A0;
      --dark-cocoa: #633806;
      --cocoa: #854F0B;
      --stone: #B4A78C;
    }
    * { box-sizing: border-box; }
    body { margin: 0; padding: 32px; background: #f4eee0; color: var(--deep-bark); font: 14px/1.5 system-ui, sans-serif; }
    main { max-width: 980px; margin: 0 auto; }
    .hero, .section, .card { border: 1px solid var(--contour); border-radius: 16px; background: var(--parchment); box-shadow: 0 10px 28px rgba(59, 38, 23, 0.08); }
    .hero { padding: 20px; margin-bottom: 16px; }
    .eyebrow { margin: 0 0 6px; text-transform: uppercase; letter-spacing: 0.12em; font-size: 12px; color: var(--cocoa); }
    h1, h2, h3, p, ul { margin: 0; }
    h1 { font-size: 30px; line-height: 1.1; color: var(--dark-cocoa); }
    .subtitle { margin-top: 8px; color: var(--cocoa); }
    .meta { margin-top: 14px; display: flex; flex-wrap: wrap; gap: 8px; }
    .pill { padding: 6px 10px; border-radius: 999px; background: rgba(186, 117, 23, 0.12); color: var(--dark-cocoa); }
    .section { padding: 18px; margin-top: 14px; break-inside: avoid; }
    .section h2 { font-size: 18px; margin-bottom: 10px; color: var(--dark-cocoa); }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 12px; }
    .card { padding: 14px; background: rgba(255,255,255,0.35); break-inside: avoid; }
    .card h3 { font-size: 15px; margin-bottom: 8px; color: var(--dark-cocoa); }
    .badge { display: inline-block; margin-bottom: 8px; font-size: 12px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--saddle); }
    strong { color: var(--dark-cocoa); }
    pre { margin: 10px 0 0; white-space: pre-wrap; word-break: break-word; font: inherit; color: var(--cocoa); }
    ul { padding-left: 18px; }
    li + li { margin-top: 4px; }
    .monospace { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
    @page { size: A4; margin: 14mm; }
  </style>
</head>
<body>
  <main>
    <section class="hero">
      <p class="eyebrow">Waypoint · frozen report snapshot</p>
      <h1>{{.Title}}</h1>
      <p class="subtitle">Version {{.Version}} · {{.Engagement}} · Cutoff {{.Cutoff}}</p>
      <div class="meta">
        <span class="pill">Hash verified, not signed</span>
        <span class="pill">Offline renderer</span>
        <span class="pill">Snapshot frozen before print</span>
      </div>
    </section>

    <section class="section">
      <h2>Scope</h2>
      <ul>{{range .Scope}}<li>{{.}}</li>{{else}}<li>None recorded.</li>{{end}}</ul>
    </section>

    <section class="section">
      <h2>Methodology</h2>
      <ul>{{range .Methodology}}<li>{{.}}</li>{{else}}<li>None recorded.</li>{{end}}</ul>
    </section>

    {{if or .Runtime.Egress.Mode .Runtime.Egress.Status .Runtime.Egress.Address .Runtime.Egress.ObservedAt .Runtime.Egress.Interface .Runtime.Egress.InterfaceAddress .Runtime.Egress.ResolverEndpoint .Runtime.Egress.Notes}}
    <section class="section">
      <h2>Runtime</h2>
      <ul>
        <li><strong>Egress:</strong> {{if .Runtime.Egress.Address}}{{.Runtime.Egress.Mode}} · {{.Runtime.Egress.Status}} · {{.Runtime.Egress.Address}}{{else}}{{.Runtime.Egress.Mode}} · {{.Runtime.Egress.Status}}{{end}}</li>
        {{if .Runtime.Egress.ObservedAt}}<li><strong>Observed at:</strong> {{.Runtime.Egress.ObservedAt.UTC.Format "2006-01-02T15:04:05Z07:00"}}</li>{{end}}
        {{if .Runtime.Egress.Interface}}<li><strong>Interface:</strong> {{.Runtime.Egress.Interface}}{{if .Runtime.Egress.InterfaceAddress}} · {{.Runtime.Egress.InterfaceAddress}}{{end}}</li>{{end}}
        {{if .Runtime.Egress.ResolverEndpoint}}<li><strong>Resolver:</strong> {{.Runtime.Egress.ResolverEndpoint}}</li>{{end}}
        {{range .Runtime.Egress.Notes}}<li>{{.}}</li>{{end}}
      </ul>
    </section>
    {{end}}

    <section class="section">
      <h2>Findings</h2>
      <div class="grid">
        {{range .Findings}}
        <article class="card">
          <p class="badge">{{.Severity}}</p>
          <h3>{{.Title}}</h3>
          <p><strong>Status:</strong> {{.Status}}</p>
          <p><strong>Evidence:</strong> {{join .Evidence ", "}}</p>
          <p><strong>Promoted by:</strong> {{.PromotedBy}}</p>
          <p><strong>Promoted at:</strong> {{.PromotedAt}}</p>
          <p><strong>Remediation:</strong> {{.Remediation}}</p>
        </article>
        {{else}}<article class="card"><p>No findings recorded.</p></article>{{end}}
      </div>
    </section>

    <section class="section">
      <h2>Evidence</h2>
      <div class="grid">
        {{range .Evidence}}
        <article class="card">
          <p class="badge">{{.Label}}</p>
          <p><strong>Command:</strong> {{.Command}}</p>
          <p><strong>Target:</strong> {{.Target}}</p>
          <p><strong>Actor:</strong> {{.Actor}}</p>
          <p><strong>Exec host:</strong> {{.Host}}</p>
          <p><strong>Egress:</strong> {{.Egress}}</p>
          <p><strong>Egress mode:</strong> {{if .EgressMode}}{{.EgressMode}}{{else}}not recorded{{end}}</p>
          <p><strong>Egress status:</strong> {{if .EgressStatus}}{{.EgressStatus}}{{else}}not recorded{{end}}</p>
          <p><strong>Observed at:</strong> {{if .EgressObservedAt}}{{.EgressObservedAt}}{{else}}not recorded{{end}}</p>
          <p><strong>Pivot chain:</strong> {{if .PivotChain}}{{pivotChainSummary .PivotChain}}{{else}}none recorded{{end}}</p>
          <p><strong>Initiated by:</strong> {{.InitiatedBy}}</p>
          <p><strong>Parse status:</strong> {{.ParseStatus}}</p>
          <p><strong>Attribution:</strong> {{.Attribution}}</p>
          <pre>{{.RawStdout}}</pre>
          <pre>{{.RawStderr}}</pre>
        </article>
        {{else}}<article class="card"><p>No evidence recorded.</p></article>{{end}}
      </div>
    </section>

    <section class="section">
      <h2>Attribution</h2>
      <div class="grid">
        {{range .Attribution}}
        <article class="card">
          <h3>{{.Title}}</h3>
          <ul>{{range .Items}}<li>{{.}}</li>{{else}}<li>None recorded.</li>{{end}}</ul>
        </article>
        {{end}}
      </div>
    </section>

    <section class="section">
      <h2>Known capture gaps</h2>
      <ul>{{range .KnownCaptureGaps}}<li><strong>{{captureGapLabel .}}</strong>{{if .Status}} · {{.Status}}{{end}}{{if captureGapSourceActionID .}} · source {{captureGapSourceActionID .}}{{end}}{{if .ObservedBy.Handle}} · observed by {{auditActorDisplay .ObservedBy}}{{end}}{{if .ResolvedBy}} · resolved by {{auditActorDisplay .ResolvedBy}}{{end}}{{if .Reason}} — {{.Reason}}{{end}}{{if .Notes}} · notes: {{.Notes}}{{end}}</li>{{else}}<li>None recorded.</li>{{end}}</ul>
    </section>
  </main>
</body>
</html>`))

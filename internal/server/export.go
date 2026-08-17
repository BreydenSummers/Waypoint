package server

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	dbutil "waypoint/internal/db"
)

const exportContractVersion = "1.0.0"

var (
	exportJobsRoute    = regexp.MustCompile(`^/(?:api/v1/)?exports(?:/([^/]+)(?:/cancel)?)?/?$`)
	exportReceiptRoute = regexp.MustCompile(`^/(?:api/v1/)?export-receipts/([^/]+)/?$`)
)

type exportRequest struct {
	FormatVersion string `json:"formatVersion"`
	RetryOfJobID  string `json:"retryOfJobId,omitempty"`
}

type exportJobPageResponse struct {
	ContractVersion string              `json:"contractVersion"`
	Items           []exportJobResponse `json:"items"`
	Page            exportPageMeta      `json:"page"`
}

type exportPageMeta struct {
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}

type exportJobProgress struct {
	Stage               string `json:"stage"`
	Percent             int    `json:"percent"`
	ProcessedBytes      int64  `json:"processedBytes,omitempty"`
	EstimatedTotalBytes int64  `json:"estimatedTotalBytes,omitempty"`
}

type exportJobFailure struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type exportJobBundle struct {
	ArchivePath       string `json:"archivePath"`
	ArchiveByteLength int64  `json:"archiveByteLength"`
	ArchiveSHA256     string `json:"archiveSha256"`
	ManifestSHA256    string `json:"manifestSha256"`
	ReportSnapshotID  string `json:"reportSnapshotId"`
	ReceiptID         string `json:"receiptId"`
}

type exportJobResponse struct {
	ContractVersion string            `json:"contractVersion"`
	ID              string            `json:"id"`
	EngagementID    string            `json:"engagementId"`
	RequestedBy     actorSnapshot     `json:"requestedBy"`
	State           string            `json:"state"`
	Progress        exportJobProgress `json:"progress"`
	FormatVersion   string            `json:"formatVersion"`
	RetryOfJobID    string            `json:"retryOfJobId,omitempty"`
	SnapshotID      string            `json:"snapshotId,omitempty"`
	Cutoff          string            `json:"cutoff,omitempty"`
	Bundle          *exportJobBundle  `json:"bundle,omitempty"`
	Failure         *exportJobFailure `json:"failure,omitempty"`
	CreatedAt       string            `json:"createdAt"`
	StartedAt       string            `json:"startedAt,omitempty"`
	CompletedAt     string            `json:"completedAt,omitempty"`
	UpdatedAt       string            `json:"updatedAt"`
	Revision        int               `json:"revision"`
}

type exportReceiptResponse struct {
	ContractVersion    string        `json:"contractVersion"`
	ID                 string        `json:"id"`
	ExportJobID        string        `json:"exportJobId"`
	EngagementID       string        `json:"engagementId"`
	Status             string        `json:"status"`
	BundlePath         string        `json:"bundlePath"`
	ArchiveByteLength  int64         `json:"archiveByteLength"`
	ArchiveSHA256      string        `json:"archiveSha256"`
	ManifestSHA256     string        `json:"manifestSha256"`
	Cutoff             string        `json:"cutoff"`
	VerifiedAt         string        `json:"verifiedAt"`
	VerifiedBy         actorSnapshot `json:"verifiedBy"`
	VerifierVersion    string        `json:"verifierVersion"`
	InvalidatedAt      string        `json:"invalidatedAt,omitempty"`
	InvalidationReason string        `json:"invalidationReason,omitempty"`
	Revision           int           `json:"revision"`
}

type exportJobRecord struct {
	ID                  string
	EngagementID        string
	RequestedBy         actorRecord
	RetryOfJobID        sql.NullString
	FormatVersion       string
	State               string
	ProgressStage       string
	ProgressPercent     int
	ProcessedBytes      int64
	EstimatedTotalBytes int64
	SnapshotID          sql.NullString
	Cutoff              sql.NullTime
	BundleArchivePath   sql.NullString
	BundleArchiveLen    sql.NullInt64
	BundleArchiveSHA    sql.NullString
	BundleManifestSHA   sql.NullString
	BundleReportSnapID  sql.NullString
	BundleReceiptID     sql.NullString
	FailureCode         sql.NullString
	FailureMessage      sql.NullString
	FailureRetryable    sql.NullBool
	CreatedAt           time.Time
	StartedAt           sql.NullTime
	CompletedAt         sql.NullTime
	UpdatedAt           time.Time
	Revision            int
}

type exportReceiptRecord struct {
	ID                 string
	ExportJobID        string
	EngagementID       string
	Status             string
	BundlePath         string
	ArchiveByteLength  int64
	ArchiveSHA256      string
	ManifestSHA256     string
	Cutoff             time.Time
	VerifiedAt         time.Time
	VerifiedBy         actorRecord
	VerifierVersion    string
	InvalidatedAt      sql.NullTime
	InvalidationReason sql.NullString
	Revision           int
}

type exportManager struct {
	db      *sql.DB
	store   *evidenceStore
	root    string
	running sync.Map
}

func newExportManager(db *sql.DB, store *evidenceStore) *exportManager {
	root := strings.TrimSpace(os.Getenv("WAYPOINT_EXPORT_DIR"))
	if root == "" {
		root = filepath.Join(os.TempDir(), "waypoint", "exports")
	}
	_ = os.MkdirAll(root, 0o750)
	return &exportManager{db: db, store: store, root: root}
}

func (m *exportManager) loadJob(ctx context.Context, jobID string) (exportJobRecord, error) {
	if m == nil || m.db == nil {
		return exportJobRecord{}, errors.New("export unavailable")
	}
	return loadExportJob(ctx, m.db, jobID)
}

func (m *exportManager) transitionJob(ctx context.Context, job exportJobRecord, state string, progress exportJobProgress, artifacts *exportArtifacts, failure *exportJobFailure, eventType, originKind, originService string) (exportJobRecord, error) {
	if m == nil || m.db == nil {
		return exportJobRecord{}, errors.New("export unavailable")
	}
	return updateExportJobState(ctx, m.db, job, state, progress, artifacts, failure, eventType, originKind, originService)
}

func (m *exportManager) recoverOutstanding(ctx context.Context) {
	if m == nil || m.db == nil {
		return
	}
	rows, err := m.db.QueryContext(ctx, `SELECT id FROM export_job WHERE state IN ('queued','preflighting','running','verifying','cancel_requested') ORDER BY updated_at ASC, id ASC`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		m.startWorker(id)
	}
}

func (m *exportManager) startWorker(jobID string) {
	if m == nil || m.db == nil {
		return
	}
	if _, loaded := m.running.LoadOrStore(jobID, struct{}{}); loaded {
		return
	}
	go func() {
		defer m.running.Delete(jobID)
		m.runJob(context.Background(), jobID)
	}()
}

func (m *exportManager) runJob(ctx context.Context, jobID string) {
	for {
		job, err := m.loadJob(ctx, jobID)
		if err != nil {
			return
		}
		switch job.State {
		case "queued":
			if _, err := m.transitionJob(ctx, job, "preflighting", exportJobProgress{Stage: "capacity_preflight", Percent: 8}, nil, nil, "export.preflight", "service", "export-worker"); err != nil {
				return
			}
		case "preflighting":
			if err := m.preflight(job); err != nil {
				_ = m.failJob(ctx, job, exportJobFailure{Code: exportFailureCode(err), Message: err.Error(), Retryable: exportRetryable(err)}, "export.failed", "service", "export-worker")
				return
			}
			if _, err := m.transitionJob(ctx, job, "running", exportJobProgress{Stage: "snapshot", Percent: 16}, nil, nil, "export.running", "service", "export-worker"); err != nil {
				return
			}
		case "running":
			artifacts, err := m.buildArtifacts(ctx, job)
			if err != nil {
				_ = m.failJob(ctx, job, exportJobFailure{Code: exportFailureCode(err), Message: err.Error(), Retryable: exportRetryable(err)}, "export.failed", "service", "export-worker")
				return
			}
			if _, err := m.persistArtifacts(ctx, job, artifacts); err != nil {
				_ = m.failJob(ctx, job, exportJobFailure{Code: exportFailureCode(err), Message: err.Error(), Retryable: exportRetryable(err)}, "export.failed", "service", "export-worker")
				return
			}
			if _, err := m.transitionJob(ctx, job, "verifying", exportJobProgress{Stage: "verification", Percent: 92, ProcessedBytes: artifacts.archiveByteLength, EstimatedTotalBytes: artifacts.archiveByteLength}, &artifacts, nil, "export.verifying", "service", "export-worker"); err != nil {
				return
			}
		case "verifying":
			current, err := m.loadJob(ctx, jobID)
			if err != nil {
				return
			}
			if current.State == "cancel_requested" {
				_ = m.cancelJob(ctx, current, "export.cancelled", "service", "export-worker")
				return
			}
			artifacts, err := m.verifyArtifacts(ctx, job)
			if err != nil {
				_ = m.failJob(ctx, job, exportJobFailure{Code: exportFailureCode(err), Message: err.Error(), Retryable: exportRetryable(err)}, "export.failed", "service", "export-worker")
				return
			}
			if _, err := m.completeJob(ctx, job, artifacts); err != nil {
				return
			}
			return
		case "cancel_requested":
			_ = m.cancelJob(ctx, job, "export.cancelled", "service", "export-worker")
			return
		default:
			return
		}
	}
}

type exportArtifacts struct {
	snapshot           reportSnapshot
	snapshotID         string
	cutoff             string
	manifest           []byte
	manifestSHA256     string
	archiveSHA256      string
	archiveByteLength  int64
	payloads           []reportBundlePayload
	bundleDir          string
	manifestPath       string
	snapshotPath       string
	metadataPath       string
	dumpPath           string
	evidencePath       string
	pdfPath            string
	verifyToolPath     string
	regenerateToolPath string
	instructionsPath   string
	receiptID          string
}

func exportHandler(db *sql.DB, store *evidenceStore, mgr *exportManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !exportJobsRoute.MatchString(r.URL.Path) && !exportReceiptRoute.MatchString(r.URL.Path) {
			return
		}
		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{exportContractVersion}})
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
		if m := exportReceiptRoute.FindStringSubmatch(r.URL.Path); m != nil {
			handleExportReceipt(w, r, db, actor, reqID, m[1])
			return
		}
		m := exportJobsRoute.FindStringSubmatch(r.URL.Path)
		if m == nil {
			return
		}
		jobID := m[1]
		switch {
		case r.Method == http.MethodGet && jobID == "":
			handleExportList(w, r, db, actor, reqID)
		case r.Method == http.MethodPost && jobID == "":
			handleExportCreate(w, r, db, mgr, actor, reqID)
		case r.Method == http.MethodGet && jobID != "":
			handleExportRead(w, r, db, actor, reqID, jobID)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cancel"):
			handleExportCancel(w, r, db, mgr, actor, reqID, jobID)
		default:
			w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		}
	}
}

func handleExportList(w http.ResponseWriter, r *http.Request, db *sql.DB, actor actorRecord, reqID string) {
	if actor.EngagementID == "" {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusForbidden), Status: http.StatusForbidden, Code: "forbidden", RequestID: reqID, Retryable: false, Detail: "actor is not scoped to an engagement"})
		return
	}
	limit, pb := parseEntityPageLimit(r.URL.Query().Get("limit"))
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
	page, err := loadExportJobPage(ctx, db, actor.EngagementID, after, limit)
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load export jobs failed"})
		return
	}
	writeJSONWithHeaders(w, http.StatusOK, page, reqID)
}

func loadExportJobPage(ctx context.Context, db *sql.DB, engagementID string, after *int64, limit int) (exportJobPageResponse, error) {
	if limit < 1 {
		limit = 100
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return exportJobPageResponse{}, err
	}
	defer tx.Rollback()

	var afterValue any
	if after != nil {
		afterValue = *after
	}
	rows, err := tx.QueryContext(ctx, `
		WITH ordered AS (
			SELECT j.id, j.engagement_id, j.requested_by, COALESCE(j.retry_of_job_id::text, '') AS retry_of_job_id, j.format_version, j.state, j.progress_stage, j.progress_percent,
			       j.processed_bytes, j.estimated_total_bytes, COALESCE(j.snapshot_id::text, '') AS snapshot_id, j.cutoff, COALESCE(j.bundle_archive_path::text, '') AS bundle_archive_path,
			       COALESCE(j.bundle_archive_byte_length, 0) AS bundle_archive_byte_length, COALESCE(j.bundle_archive_sha256::text, '') AS bundle_archive_sha256, COALESCE(j.bundle_manifest_sha256::text, '') AS bundle_manifest_sha256, COALESCE(j.bundle_report_snapshot_id::text, '') AS bundle_report_snapshot_id,
			       COALESCE(j.bundle_receipt_id::text, '') AS bundle_receipt_id, COALESCE(j.failure_code::text, '') AS failure_code, COALESCE(j.failure_message::text, '') AS failure_message, COALESCE(j.failure_retryable, false) AS failure_retryable,
			       j.created_at, j.started_at, j.completed_at, j.updated_at, j.revision,
			       a.kind, a.handle, a.role, COALESCE(a.agent_name, '') AS agent_name, COALESCE(a.model, '') AS model, COALESCE(a.version, '') AS version, COALESCE(a.authorized_by::text, '') AS authorized_by,
			       row_number() OVER (ORDER BY j.updated_at DESC, j.id DESC) AS rn
			FROM export_job j
			JOIN actor a ON a.id = j.requested_by
			WHERE j.engagement_id = $1
		)
		SELECT id, engagement_id, requested_by, retry_of_job_id, format_version, state, progress_stage, progress_percent,
		       processed_bytes, estimated_total_bytes, snapshot_id, cutoff, bundle_archive_path,
		       bundle_archive_byte_length, bundle_archive_sha256, bundle_manifest_sha256, bundle_report_snapshot_id,
		       bundle_receipt_id, failure_code, failure_message, failure_retryable,
		       created_at, started_at, completed_at, updated_at, revision,
		       kind, handle, role, agent_name, model, version, authorized_by, rn
		FROM ordered
		WHERE rn > COALESCE($2::bigint, 0)
		ORDER BY rn ASC
		LIMIT $3`, engagementID, afterValue, limit+1)
	if err != nil {
		return exportJobPageResponse{}, err
	}
	defer rows.Close()

	items := make([]exportJobResponse, 0, limit)
	var lastCursor string
	var hasMore bool
	for rows.Next() {
		row, err := scanExportJobPageRow(rows)
		if err != nil {
			return exportJobPageResponse{}, err
		}
		if len(items) == limit {
			hasMore = true
			continue
		}
		items = append(items, exportJobResponseFromRow(row.record))
		lastCursor = fmt.Sprint(row.pageCursor)
	}
	if err := rows.Err(); err != nil {
		return exportJobPageResponse{}, err
	}
	if err := tx.Commit(); err != nil {
		return exportJobPageResponse{}, err
	}
	page := exportJobPageResponse{ContractVersion: exportContractVersion, Items: items, Page: exportPageMeta{HasMore: hasMore}}
	if hasMore && lastCursor != "" {
		page.Page.NextCursor = lastCursor
	}
	return page, nil
}

func scanExportJobPageRow(rows *sql.Rows) (struct {
	record     exportJobRecord
	pageCursor int64
}, error) {
	var row exportJobRecord
	var pageCursor int64
	if err := rows.Scan(&row.ID, &row.EngagementID, &row.RequestedBy.ID, &row.RetryOfJobID.String, &row.FormatVersion, &row.State, &row.ProgressStage, &row.ProgressPercent, &row.ProcessedBytes, &row.EstimatedTotalBytes, &row.SnapshotID.String, &row.Cutoff, &row.BundleArchivePath.String, &row.BundleArchiveLen.Int64, &row.BundleArchiveSHA.String, &row.BundleManifestSHA.String, &row.BundleReportSnapID.String, &row.BundleReceiptID.String, &row.FailureCode.String, &row.FailureMessage.String, &row.FailureRetryable.Bool, &row.CreatedAt, &row.StartedAt, &row.CompletedAt, &row.UpdatedAt, &row.Revision, &row.RequestedBy.Kind, &row.RequestedBy.Handle, &row.RequestedBy.Role, &row.RequestedBy.AgentName, &row.RequestedBy.Model, &row.RequestedBy.Version, &row.RequestedBy.AuthorizedBy, &pageCursor); err != nil {
		return struct {
			record     exportJobRecord
			pageCursor int64
		}{}, err
	}
	row.RequestedBy.EngagementID = row.EngagementID
	if row.RetryOfJobID.String == "" {
		row.RetryOfJobID.Valid = false
	}
	if row.SnapshotID.String == "" {
		row.SnapshotID.Valid = false
	}
	if row.BundleArchivePath.String == "" {
		row.BundleArchivePath.Valid = false
	}
	if row.BundleArchiveSHA.String == "" {
		row.BundleArchiveSHA.Valid = false
	}
	if row.BundleManifestSHA.String == "" {
		row.BundleManifestSHA.Valid = false
	}
	if row.BundleReportSnapID.String == "" {
		row.BundleReportSnapID.Valid = false
	}
	if row.BundleReceiptID.String == "" {
		row.BundleReceiptID.Valid = false
	}
	if row.FailureCode.String == "" {
		row.FailureCode.Valid = false
	}
	if row.FailureMessage.String == "" {
		row.FailureMessage.Valid = false
	}
	return struct {
		record     exportJobRecord
		pageCursor int64
	}{record: row, pageCursor: pageCursor}, nil
}

func handleExportCreate(w http.ResponseWriter, r *http.Request, db *sql.DB, mgr *exportManager, actor actorRecord, reqID string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if db == nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: reqID, Retryable: true, Detail: "export jobs are unavailable"})
		return
	}
	if actor.EngagementID == "" {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusForbidden), Status: http.StatusForbidden, Code: "forbidden", RequestID: reqID, Retryable: false, Detail: "actor is not scoped to an engagement"})
		return
	}
	var req exportRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "read request body failed"})
		return
	}
	if err := decodeStrictJSON(body, &req); err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()})
		return
	}
	if req.FormatVersion != exportContractVersion {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: "formatVersion must be 1.0.0."})
		return
	}
	if strings.TrimSpace(req.RetryOfJobID) != "" && !isUUID(req.RetryOfJobID) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "retryOfJobId must be a UUID."})
		return
	}
	now := time.Now().UTC()
	jobID := newUUID()
	tx, err := db.BeginTx(r.Context(), nil)
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "create export job failed"})
		return
	}
	if _, err := tx.ExecContext(r.Context(), `INSERT INTO export_job (
		id, engagement_id, requested_by, retry_of_job_id, format_version, state, progress_stage, progress_percent, processed_bytes, estimated_total_bytes,
		created_at, updated_at, revision
	) VALUES ($1, $2, $3, NULLIF($4, ''), $5, 'queued', 'queued', 0, 0, 0, $6, $6, 1)`, jobID, actor.EngagementID, actor.ID, strings.TrimSpace(req.RetryOfJobID), req.FormatVersion, now); err != nil {
		_ = tx.Rollback()
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "create export job failed"})
		return
	}
	if err := appendExportAuditEvent(r.Context(), tx, actor, reqID, "export.state-changed", jobID, 1, "rest", "", map[string]any{"state": "queued", "retryOfJobId": strings.TrimSpace(req.RetryOfJobID)}); err != nil {
		_ = tx.Rollback()
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "audit export job failed"})
		return
	}
	if err := tx.Commit(); err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "create export job failed"})
		return
	}
	job, err := loadExportJob(r.Context(), db, jobID)
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "reload export job failed"})
		return
	}
	writeJSONWithHeaders(w, http.StatusAccepted, exportJobResponseFromRow(job), reqID)
	if mgr != nil {
		mgr.startWorker(jobID)
	}
}

func handleExportRead(w http.ResponseWriter, r *http.Request, db *sql.DB, actor actorRecord, reqID, jobID string) {
	if !isUUID(jobID) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "export id must be a UUID."})
		return
	}
	job, err := loadExportJob(r.Context(), db, jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load export job failed"})
		return
	}
	if actor.EngagementID != "" && actor.EngagementID != job.EngagementID {
		http.NotFound(w, r)
		return
	}
	writeJSONWithHeaders(w, http.StatusOK, exportJobResponseFromRow(job), reqID)
}

func handleExportCancel(w http.ResponseWriter, r *http.Request, db *sql.DB, mgr *exportManager, actor actorRecord, reqID, jobID string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if !isUUID(jobID) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "export id must be a UUID."})
		return
	}
	expectedRevision, pb := parseExpectedRevision(r.Header.Get("If-Match"))
	if pb != nil {
		pb.RequestID = reqID
		writeProblem(w, *pb)
		return
	}
	job, err := loadExportJob(r.Context(), db, jobID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load export job failed"})
		return
	}
	if actor.EngagementID != "" && actor.EngagementID != job.EngagementID {
		http.NotFound(w, r)
		return
	}
	if job.Revision != expectedRevision {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusPreconditionFailed), Status: http.StatusPreconditionFailed, Code: "precondition_failed", RequestID: reqID, Retryable: false, Detail: "revision mismatch."})
		return
	}
	if isExportTerminal(job.State) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "conflict", RequestID: reqID, Retryable: false, Detail: "export job is already terminal."})
		return
	}
	updated, err := updateExportJobState(r.Context(), db, job, "cancel_requested", exportJobProgress{Stage: job.ProgressStage, Percent: job.ProgressPercent, ProcessedBytes: job.ProcessedBytes, EstimatedTotalBytes: job.EstimatedTotalBytes}, nil, nil, "export.cancel_requested", "rest", "rest")
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "cancel export job failed"})
		return
	}
	writeJSONWithHeaders(w, http.StatusAccepted, exportJobResponseFromRow(updated), reqID)
	if mgr != nil {
		mgr.startWorker(jobID)
	}
}

func handleExportReceipt(w http.ResponseWriter, r *http.Request, db *sql.DB, actor actorRecord, reqID, receiptID string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if !isUUID(receiptID) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "receipt id must be a UUID."})
		return
	}
	rec, err := loadExportReceipt(r.Context(), db, receiptID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load export receipt failed"})
		return
	}
	if actor.EngagementID != "" && actor.EngagementID != rec.EngagementID {
		http.NotFound(w, r)
		return
	}
	writeJSONWithHeaders(w, http.StatusOK, exportReceiptResponseFromRow(rec), reqID)
}

func (m *exportManager) preflight(job exportJobRecord) error {
	if strings.TrimSpace(job.EngagementID) == "" {
		return errors.New("export job missing engagement")
	}
	return nil
}

func (m *exportManager) buildArtifacts(ctx context.Context, job exportJobRecord) (exportArtifacts, error) {
	if m == nil || m.db == nil {
		return exportArtifacts{}, errors.New("export unavailable")
	}
	reportCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tx, err := m.db.BeginTx(reportCtx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return exportArtifacts{}, err
	}
	defer tx.Rollback()
	snapshot, err := buildReportSnapshot(reportCtx, tx, m.store, job.EngagementID)
	if err != nil {
		return exportArtifacts{}, err
	}
	snapshotID := newUUID()
	cutoff := snapshot.Cutoff
	bundleDir := filepath.Join(m.root, job.ID, "bundle")
	paths := map[string]string{
		"dump":         filepath.Join(bundleDir, "database", "engagement.dump"),
		"evidence":     filepath.Join(bundleDir, "evidence", "evidence.tar.zst"),
		"pdf":          filepath.Join(bundleDir, "report", "frozen-report.pdf"),
		"snapshot":     filepath.Join(bundleDir, "report", "report-snapshot.json"),
		"metadata":     filepath.Join(bundleDir, "metadata", "export-metadata.json"),
		"manifest":     filepath.Join(bundleDir, "metadata", "export-manifest.json"),
		"sidecar":      filepath.Join(bundleDir, "metadata", "export-archive.sha256"),
		"verifyTool":   filepath.Join(bundleDir, "tools", "verify-restore.mjs"),
		"regenTool":    filepath.Join(bundleDir, "tools", "regenerate-report.mjs"),
		"instructions": filepath.Join(bundleDir, "instructions", "restore.md"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return exportArtifacts{}, err
		}
	}

	dumpBytes, err := buildExportDump(reportCtx, tx, job.EngagementID, snapshotID, cutoff)
	if err != nil {
		return exportArtifacts{}, err
	}
	if err := os.WriteFile(paths["dump"], dumpBytes, 0o600); err != nil {
		return exportArtifacts{}, err
	}

	if err := buildExportEvidenceTar(reportCtx, tx, m.store, job.EngagementID, paths["evidence"]); err != nil {
		return exportArtifacts{}, err
	}

	toolVerify := []byte(bundleVerifyToolScript)
	toolRegenerate := []byte(bundleRegenerateToolScript)
	if err := os.WriteFile(paths["verifyTool"], toolVerify, 0o600); err != nil {
		return exportArtifacts{}, err
	}
	if err := os.WriteFile(paths["regenTool"], toolRegenerate, 0o600); err != nil {
		return exportArtifacts{}, err
	}
	instructions := []byte(bundleRestoreInstructions)
	if err := os.WriteFile(paths["instructions"], instructions, 0o600); err != nil {
		return exportArtifacts{}, err
	}

	metadata := map[string]any{
		"version":            "v1",
		"bundleRoot":         "bundle",
		"manifestPath":       "bundle/metadata/export-manifest.json",
		"snapshotPath":       "bundle/report/report-snapshot.json",
		"pdfPath":            "bundle/report/frozen-report.pdf",
		"dumpPath":           "bundle/database/engagement.dump",
		"evidencePath":       "bundle/evidence/evidence.tar.zst",
		"verifyToolPath":     "bundle/tools/verify-restore.mjs",
		"regenerateToolPath": "bundle/tools/regenerate-report.mjs",
		"instructionsPath":   "bundle/instructions/restore.md",
		"exportJobId":        job.ID,
		"engagementId":       job.EngagementID,
		"snapshotId":         snapshotID,
		"cutoff":             cutoff,
	}
	if err := tx.Commit(); err != nil {
		return exportArtifacts{}, err
	}

	metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return exportArtifacts{}, err
	}
	metadataBytes = append(metadataBytes, '\n')
	if err := os.WriteFile(paths["metadata"], metadataBytes, 0o600); err != nil {
		return exportArtifacts{}, err
	}

	evidenceInfo, err := os.Stat(paths["evidence"])
	if err != nil {
		return exportArtifacts{}, err
	}
	evidenceSHA, _, err := fileSHA256(paths["evidence"])
	if err != nil {
		return exportArtifacts{}, err
	}

	bundlePayloads := []reportBundlePayload{
		{Path: "bundle/database/engagement.dump", Size: int64(len(dumpBytes)), ByteLength: int64(len(dumpBytes)), SHA256: sha256HexBytes(dumpBytes), Kind: "database_dump"},
		{Path: "bundle/evidence/evidence.tar.zst", Size: evidenceInfo.Size(), ByteLength: evidenceInfo.Size(), SHA256: evidenceSHA, Kind: "evidence"},
		{Path: "bundle/report/frozen-report.pdf", Size: 0, ByteLength: 0, SHA256: strings.Repeat("0", 64), Kind: "report_pdf"},
		{Path: "bundle/report/report-snapshot.json", Size: 0, ByteLength: 0, SHA256: strings.Repeat("0", 64), Kind: "report_snapshot"},
		{Path: "bundle/metadata/export-metadata.json", Size: int64(len(metadataBytes)), ByteLength: int64(len(metadataBytes)), SHA256: sha256HexBytes(metadataBytes), Kind: "metadata"},
		{Path: "bundle/tools/verify-restore.mjs", Size: int64(len(toolVerify)), ByteLength: int64(len(toolVerify)), SHA256: sha256HexBytes(toolVerify), Kind: "verify_tool"},
		{Path: "bundle/tools/regenerate-report.mjs", Size: int64(len(toolRegenerate)), ByteLength: int64(len(toolRegenerate)), SHA256: sha256HexBytes(toolRegenerate), Kind: "restore_tool"},
		{Path: "bundle/instructions/restore.md", Size: int64(len(instructions)), ByteLength: int64(len(instructions)), SHA256: sha256HexBytes(instructions), Kind: "instructions"},
	}

	reportSnapshotForBundle := snapshot
	reportSnapshotForBundle.Bundle = &reportBundle{
		Payloads:           bundlePayloads,
		OuterArchiveSHA256: "",
		Signatures:         reportBundleSignatures{Version: "v1", Items: []string{}},
		Restore: reportBundleRestore{
			Tools:          []string{"bundle/tools/verify-restore.mjs", "bundle/tools/regenerate-report.mjs"},
			CleanRoom:      []string{"Verify the outer archive hash before restore.", "Reject the manifest if any payload path is missing, duplicated, or traverses upward.", "Regenerate the report from the frozen snapshot rather than live queries."},
			MaliciousPaths: []string{"../escape.dump", "/absolute/report.pdf", "bundle/../metadata/export-metadata.json"},
		},
	}
	// Write the report snapshot with the placeholder payload entry so the payload hash can be captured.
	reportSnapshotBytes, err := json.MarshalIndent(reportSnapshotForBundle, "", "  ")
	if err != nil {
		return exportArtifacts{}, err
	}
	reportSnapshotBytes = append(reportSnapshotBytes, '\n')
	if err := os.WriteFile(paths["snapshot"], reportSnapshotBytes, 0o600); err != nil {
		return exportArtifacts{}, err
	}
	reportSnapshotPayloadSHA := sha256HexBytes(reportSnapshotBytes)
	bundlePayloads[3].Size = int64(len(reportSnapshotBytes))
	bundlePayloads[3].ByteLength = int64(len(reportSnapshotBytes))
	bundlePayloads[3].SHA256 = reportSnapshotPayloadSHA
	// Re-render the in-memory snapshot for PDF generation; the on-disk snapshot keeps the self-reference placeholder.
	reportSnapshotForPDF := snapshot
	reportSnapshotForPDF.Bundle = &reportBundle{
		Payloads:           bundlePayloads,
		OuterArchiveSHA256: "",
		Signatures:         reportBundleSignatures{Version: "v1", Items: []string{}},
		Restore:            reportSnapshotForBundle.Bundle.Restore,
	}
	pdfBytes, err := renderReportPDF(reportCtx, reportSnapshotForPDF)
	if err != nil {
		return exportArtifacts{}, err
	}
	if err := os.WriteFile(paths["pdf"], pdfBytes, 0o600); err != nil {
		return exportArtifacts{}, err
	}
	bundlePayloads[2].Size = int64(len(pdfBytes))
	bundlePayloads[2].ByteLength = int64(len(pdfBytes))
	bundlePayloads[2].SHA256 = sha256HexBytes(pdfBytes)

	// Refresh the snapshot JSON file so the on-disk manifest references the final payload list.
	reportSnapshotForBundle.Bundle.Payloads = bundlePayloads
	reportSnapshotBytes, err = json.MarshalIndent(reportSnapshotForBundle, "", "  ")
	if err != nil {
		return exportArtifacts{}, err
	}
	reportSnapshotBytes = append(reportSnapshotBytes, '\n')
	if err := os.WriteFile(paths["snapshot"], reportSnapshotBytes, 0o600); err != nil {
		return exportArtifacts{}, err
	}
	reportSnapshotPayloadSHA = sha256HexBytes(reportSnapshotBytes)
	bundlePayloads[3].Size = int64(len(reportSnapshotBytes))
	bundlePayloads[3].ByteLength = int64(len(reportSnapshotBytes))
	bundlePayloads[3].SHA256 = reportSnapshotPayloadSHA
	if err := os.WriteFile(paths["snapshot"], reportSnapshotBytes, 0o600); err != nil {
		return exportArtifacts{}, err
	}

	manifest := map[string]any{
		"version":    "v1",
		"payloads":   bundlePayloads,
		"signatures": map[string]any{"version": "v1", "items": []string{}},
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return exportArtifacts{}, err
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := os.WriteFile(paths["manifest"], manifestBytes, 0o600); err != nil {
		return exportArtifacts{}, err
	}
	manifestSHA := sha256HexBytes(manifestBytes)
	archiveSHA, archiveLen, err := computeExportArchiveHash(bundlePayloads, manifestBytes, bundleDir)
	if err != nil {
		return exportArtifacts{}, err
	}
	if err := os.WriteFile(paths["sidecar"], []byte(archiveSHA+"\n"), 0o600); err != nil {
		return exportArtifacts{}, err
	}

	reportSnapshotForBundle.Bundle.OuterArchiveSHA256 = archiveSHA
	// Keep the on-disk snapshot intentionally self-referential, but store the final archive values in the job/receipt.
	bundlePayloads[3].SHA256 = reportSnapshotPayloadSHA
	reportSnapshotForBundle.Bundle.Payloads = bundlePayloads

	return exportArtifacts{
		snapshot:           reportSnapshotForBundle,
		snapshotID:         snapshotID,
		cutoff:             cutoff,
		manifest:           manifestBytes,
		manifestSHA256:     manifestSHA,
		archiveSHA256:      archiveSHA,
		archiveByteLength:  archiveLen,
		payloads:           bundlePayloads,
		bundleDir:          bundleDir,
		manifestPath:       paths["manifest"],
		snapshotPath:       paths["snapshot"],
		metadataPath:       paths["metadata"],
		dumpPath:           paths["dump"],
		evidencePath:       paths["evidence"],
		pdfPath:            paths["pdf"],
		verifyToolPath:     paths["verifyTool"],
		regenerateToolPath: paths["regenTool"],
		instructionsPath:   paths["instructions"],
		receiptID:          newUUID(),
	}, nil
}

func (m *exportManager) persistArtifacts(ctx context.Context, job exportJobRecord, artifacts exportArtifacts) (exportJobRecord, error) {
	return updateExportJobState(ctx, m.db, job, "running", exportJobProgress{Stage: "archive", Percent: 84, ProcessedBytes: artifacts.archiveByteLength, EstimatedTotalBytes: artifacts.archiveByteLength}, &artifacts, nil, "export.artifacts-persisted", "service", "export-worker")
}

func (m *exportManager) verifyArtifacts(ctx context.Context, job exportJobRecord) (exportArtifacts, error) {
	artifacts, err := loadExportArtifacts(m, job)
	if err != nil {
		return exportArtifacts{}, err
	}
	if err := verifyExportBundle(artifacts.bundleDir, artifacts.manifest, artifacts.manifestSHA256, artifacts.archiveSHA256); err != nil {
		return exportArtifacts{}, err
	}
	return artifacts, nil
}

func (m *exportManager) completeJob(ctx context.Context, job exportJobRecord, artifacts exportArtifacts) (exportJobRecord, error) {
	if _, err := persistExportReceipt(ctx, m.db, job, artifacts); err != nil {
		return exportJobRecord{}, err
	}
	return updateExportJobState(ctx, m.db, job, "completed", exportJobProgress{Stage: "complete", Percent: 100, ProcessedBytes: artifacts.archiveByteLength, EstimatedTotalBytes: artifacts.archiveByteLength}, &artifacts, nil, "export.completed", "service", "export-worker")
}

func (m *exportManager) failJob(ctx context.Context, job exportJobRecord, failure exportJobFailure, eventType, originKind, originService string) error {
	_, err := updateExportJobState(ctx, m.db, job, "failed", exportJobProgress{Stage: job.ProgressStage, Percent: job.ProgressPercent, ProcessedBytes: job.ProcessedBytes, EstimatedTotalBytes: job.EstimatedTotalBytes}, nil, &failure, eventType, originKind, originService)
	return err
}

func (m *exportManager) cancelJob(ctx context.Context, job exportJobRecord, eventType, originKind, originService string) error {
	failure := exportJobFailure{Code: "cancelled", Message: "export cancelled by operator", Retryable: false}
	_, err := updateExportJobState(ctx, m.db, job, "cancelled", exportJobProgress{Stage: job.ProgressStage, Percent: job.ProgressPercent, ProcessedBytes: job.ProcessedBytes, EstimatedTotalBytes: job.EstimatedTotalBytes}, nil, &failure, eventType, originKind, originService)
	return err
}

func updateExportJobState(ctx context.Context, db *sql.DB, job exportJobRecord, state string, progress exportJobProgress, artifacts *exportArtifacts, failure *exportJobFailure, eventType, originKind, originService string) (exportJobRecord, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return exportJobRecord{}, err
	}
	defer tx.Rollback()

	var snapshotID any
	var cutoff any
	var bundlePath any
	var archiveLen any
	var archiveSHA any
	var manifestSHA any
	var reportSnapID any
	var receiptID any
	if artifacts != nil {
		snapshotID = artifacts.snapshotID
		cutoff = artifacts.cutoff
		bundlePath = "bundle"
		archiveLen = artifacts.archiveByteLength
		archiveSHA = artifacts.archiveSHA256
		manifestSHA = artifacts.manifestSHA256
		reportSnapID = artifacts.snapshotID
		receiptID = artifacts.receiptID
	}
	var failureCode any
	var failureMessage any
	var failureRetryable any
	if failure != nil {
		failureCode = failure.Code
		failureMessage = failure.Message
		failureRetryable = failure.Retryable
	}
	row := tx.QueryRowContext(ctx, `
		UPDATE export_job
		SET state = $2,
		    progress_stage = $3,
		    progress_percent = $4,
		    processed_bytes = $5,
		    estimated_total_bytes = $6,
		    snapshot_id = COALESCE($7::uuid, snapshot_id),
		    cutoff = COALESCE($8::timestamptz, cutoff),
		    bundle_archive_path = COALESCE($9::text, bundle_archive_path),
		    bundle_archive_byte_length = COALESCE($10::bigint, bundle_archive_byte_length),
		    bundle_archive_sha256 = COALESCE($11::text, bundle_archive_sha256),
		    bundle_manifest_sha256 = COALESCE($12::text, bundle_manifest_sha256),
		    bundle_report_snapshot_id = COALESCE($13::uuid, bundle_report_snapshot_id),
		    bundle_receipt_id = COALESCE($14::uuid, bundle_receipt_id),
		    failure_code = COALESCE($15::text, failure_code),
		    failure_message = COALESCE($16::text, failure_message),
		    failure_retryable = COALESCE($17::bool, failure_retryable),
		    started_at = COALESCE(started_at, now()),
		    completed_at = CASE WHEN $2 IN ('failed','cancelled','completed') THEN COALESCE(completed_at, now()) ELSE completed_at END,
		    updated_at = now(),
		    revision = revision + 1
		WHERE id = $1
		RETURNING revision`, job.ID, state, progress.Stage, progress.Percent, progress.ProcessedBytes, progress.EstimatedTotalBytes, snapshotID, cutoff, bundlePath, archiveLen, archiveSHA, manifestSHA, reportSnapID, receiptID, failureCode, failureMessage, failureRetryable)
	var revision int
	if err := row.Scan(&revision); err != nil {
		return exportJobRecord{}, err
	}
	if err := appendExportAuditEvent(ctx, tx, job.RequestedBy, job.EngagementID, eventType, job.ID, revision, originKind, originService, map[string]any{"state": state, "progress": progress, "failure": failure, "origin": originKind}); err != nil {
		return exportJobRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return exportJobRecord{}, err
	}
	return loadExportJob(ctx, db, job.ID)
}

func persistExportReceipt(ctx context.Context, db *sql.DB, job exportJobRecord, artifacts exportArtifacts) (exportReceiptRecord, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return exportReceiptRecord{}, err
	}
	defer tx.Rollback()
	var rec exportReceiptRecord
	err = tx.QueryRowContext(ctx, `
		INSERT INTO export_receipt (
			id, export_job_id, engagement_id, status, bundle_path, archive_byte_length, archive_sha256, manifest_sha256, cutoff, verified_at, verified_by, verifier_version, revision
		) VALUES ($1, $2, $3, 'verified', $4, $5, $6, $7, $8, now(), $9, $10, 1)
		RETURNING id, export_job_id, engagement_id, status, bundle_path, archive_byte_length, archive_sha256, manifest_sha256, cutoff, verified_at, verified_by::text, verifier_version, revision`, artifacts.receiptID, job.ID, job.EngagementID, "bundle", artifacts.archiveByteLength, artifacts.archiveSHA256, artifacts.manifestSHA256, artifacts.cutoff, job.RequestedBy.ID, "export-worker").Scan(&rec.ID, &rec.ExportJobID, &rec.EngagementID, &rec.Status, &rec.BundlePath, &rec.ArchiveByteLength, &rec.ArchiveSHA256, &rec.ManifestSHA256, &rec.Cutoff, &rec.VerifiedAt, &rec.VerifiedBy.ID, &rec.VerifierVersion, &rec.Revision)
	if err != nil {
		return exportReceiptRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return exportReceiptRecord{}, err
	}
	return rec, nil
}

func loadExportJob(ctx context.Context, db queryer, jobID string) (exportJobRecord, error) {
	var row exportJobRecord
	var requestedBy exportJobRecord
	var actor exportJobRecord
	_ = requestedBy
	_ = actor
	err := db.QueryRowContext(ctx, `
		SELECT j.id, j.engagement_id, j.requested_by, COALESCE(j.retry_of_job_id::text, ''), j.format_version, j.state, j.progress_stage, j.progress_percent,
		       j.processed_bytes, j.estimated_total_bytes, COALESCE(j.snapshot_id::text, ''), j.cutoff, COALESCE(j.bundle_archive_path::text, ''),
		       COALESCE(j.bundle_archive_byte_length, 0), COALESCE(j.bundle_archive_sha256::text, ''), COALESCE(j.bundle_manifest_sha256::text, ''),
		       COALESCE(j.bundle_report_snapshot_id::text, ''), COALESCE(j.bundle_receipt_id::text, ''), COALESCE(j.failure_code::text, ''),
		       COALESCE(j.failure_message::text, ''), COALESCE(j.failure_retryable, false), j.created_at, j.started_at, j.completed_at, j.updated_at, j.revision,
		       a.kind, a.handle, a.role, COALESCE(a.agent_name, ''), COALESCE(a.model, ''), COALESCE(a.version, ''), COALESCE(a.authorized_by::text, '')
		FROM export_job j
		JOIN actor a ON a.id = j.requested_by
		WHERE j.id = $1`, jobID).Scan(&row.ID, &row.EngagementID, &row.RequestedBy.ID, &row.RetryOfJobID.String, &row.FormatVersion, &row.State, &row.ProgressStage, &row.ProgressPercent, &row.ProcessedBytes, &row.EstimatedTotalBytes, &row.SnapshotID.String, &row.Cutoff, &row.BundleArchivePath.String, &row.BundleArchiveLen.Int64, &row.BundleArchiveSHA.String, &row.BundleManifestSHA.String, &row.BundleReportSnapID.String, &row.BundleReceiptID.String, &row.FailureCode.String, &row.FailureMessage.String, &row.FailureRetryable.Bool, &row.CreatedAt, &row.StartedAt, &row.CompletedAt, &row.UpdatedAt, &row.Revision, &row.RequestedBy.Kind, &row.RequestedBy.Handle, &row.RequestedBy.Role, &row.RequestedBy.AgentName, &row.RequestedBy.Model, &row.RequestedBy.Version, &row.RequestedBy.AuthorizedBy)
	if err != nil {
		return exportJobRecord{}, err
	}
	row.RequestedBy.EngagementID = row.EngagementID
	row.RequestedBy.TokenHash = ""
	row.RequestedBy.AuthorizedBy = strings.TrimSpace(row.RequestedBy.AuthorizedBy)
	if row.RetryOfJobID.String == "" {
		row.RetryOfJobID.Valid = false
	}
	if row.SnapshotID.String == "" {
		row.SnapshotID.Valid = false
	}
	if row.BundleArchivePath.String == "" {
		row.BundleArchivePath.Valid = false
	}
	if row.BundleArchiveSHA.String == "" {
		row.BundleArchiveSHA.Valid = false
	}
	if row.BundleManifestSHA.String == "" {
		row.BundleManifestSHA.Valid = false
	}
	if row.BundleReportSnapID.String == "" {
		row.BundleReportSnapID.Valid = false
	}
	if row.BundleReceiptID.String == "" {
		row.BundleReceiptID.Valid = false
	}
	if row.FailureCode.String == "" {
		row.FailureCode.Valid = false
	}
	if row.FailureMessage.String == "" {
		row.FailureMessage.Valid = false
	}
	return row, nil
}

func scanExportJobRow(rows *sql.Rows) (exportJobRecord, error) {
	var row exportJobRecord
	if err := rows.Scan(&row.ID, &row.EngagementID, &row.RequestedBy.ID, &row.RetryOfJobID.String, &row.FormatVersion, &row.State, &row.ProgressStage, &row.ProgressPercent, &row.ProcessedBytes, &row.EstimatedTotalBytes, &row.SnapshotID.String, &row.Cutoff, &row.BundleArchivePath.String, &row.BundleArchiveLen.Int64, &row.BundleArchiveSHA.String, &row.BundleManifestSHA.String, &row.BundleReportSnapID.String, &row.BundleReceiptID.String, &row.FailureCode.String, &row.FailureMessage.String, &row.FailureRetryable.Bool, &row.CreatedAt, &row.StartedAt, &row.CompletedAt, &row.UpdatedAt, &row.Revision, &row.RequestedBy.Kind, &row.RequestedBy.Handle, &row.RequestedBy.Role, &row.RequestedBy.AgentName, &row.RequestedBy.Model, &row.RequestedBy.Version, &row.RequestedBy.AuthorizedBy); err != nil {
		return exportJobRecord{}, err
	}
	row.RequestedBy.EngagementID = row.EngagementID
	row.RequestedBy.TokenHash = ""
	row.RequestedBy.AuthorizedBy = strings.TrimSpace(row.RequestedBy.AuthorizedBy)
	if row.RetryOfJobID.String == "" {
		row.RetryOfJobID.Valid = false
	}
	if row.SnapshotID.String == "" {
		row.SnapshotID.Valid = false
	}
	if row.BundleArchivePath.String == "" {
		row.BundleArchivePath.Valid = false
	}
	if row.BundleArchiveSHA.String == "" {
		row.BundleArchiveSHA.Valid = false
	}
	if row.BundleManifestSHA.String == "" {
		row.BundleManifestSHA.Valid = false
	}
	if row.BundleReportSnapID.String == "" {
		row.BundleReportSnapID.Valid = false
	}
	if row.BundleReceiptID.String == "" {
		row.BundleReceiptID.Valid = false
	}
	if row.FailureCode.String == "" {
		row.FailureCode.Valid = false
	}
	if row.FailureMessage.String == "" {
		row.FailureMessage.Valid = false
	}
	return row, nil
}

func loadExportReceipt(ctx context.Context, db queryer, receiptID string) (exportReceiptRecord, error) {
	var row exportReceiptRecord
	if err := db.QueryRowContext(ctx, `
		SELECT r.id, r.export_job_id, r.engagement_id, r.status, r.bundle_path, r.archive_byte_length, r.archive_sha256, r.manifest_sha256, r.cutoff,
		       r.verified_at, r.verified_by, r.verifier_version, r.invalidated_at, COALESCE(r.invalidation_reason::text, ''), r.revision,
		       a.kind, a.handle, a.role, COALESCE(a.agent_name, ''), COALESCE(a.model, ''), COALESCE(a.version, ''), COALESCE(a.authorized_by::text, '')
		FROM export_receipt r
		JOIN actor a ON a.id = r.verified_by
		WHERE r.id = $1`, receiptID).Scan(&row.ID, &row.ExportJobID, &row.EngagementID, &row.Status, &row.BundlePath, &row.ArchiveByteLength, &row.ArchiveSHA256, &row.ManifestSHA256, &row.Cutoff, &row.VerifiedAt, &row.VerifiedBy.ID, &row.VerifierVersion, &row.InvalidatedAt, &row.InvalidationReason.String, &row.Revision, &row.VerifiedBy.Kind, &row.VerifiedBy.Handle, &row.VerifiedBy.Role, &row.VerifiedBy.AgentName, &row.VerifiedBy.Model, &row.VerifiedBy.Version, &row.VerifiedBy.AuthorizedBy); err != nil {
		return exportReceiptRecord{}, err
	}
	row.VerifiedBy.EngagementID = row.EngagementID
	if row.InvalidationReason.String == "" {
		row.InvalidationReason.Valid = false
	}
	return row, nil
}

func exportJobResponseFromRow(row exportJobRecord) exportJobResponse {
	resp := exportJobResponse{
		ContractVersion: exportContractVersion,
		ID:              row.ID,
		EngagementID:    row.EngagementID,
		RequestedBy:     actorSnapshotFromRecord(row.RequestedBy),
		State:           row.State,
		Progress:        exportJobProgress{Stage: row.ProgressStage, Percent: row.ProgressPercent, ProcessedBytes: row.ProcessedBytes, EstimatedTotalBytes: row.EstimatedTotalBytes},
		FormatVersion:   row.FormatVersion,
		CreatedAt:       row.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:       row.UpdatedAt.UTC().Format(time.RFC3339),
		Revision:        row.Revision,
	}
	if row.RetryOfJobID.Valid {
		resp.RetryOfJobID = row.RetryOfJobID.String
	}
	if row.SnapshotID.Valid {
		resp.SnapshotID = row.SnapshotID.String
	}
	if row.Cutoff.Valid {
		resp.Cutoff = row.Cutoff.Time.UTC().Format(time.RFC3339)
	}
	if row.StartedAt.Valid {
		resp.StartedAt = row.StartedAt.Time.UTC().Format(time.RFC3339)
	}
	if row.CompletedAt.Valid {
		resp.CompletedAt = row.CompletedAt.Time.UTC().Format(time.RFC3339)
	}
	if row.BundleArchivePath.Valid {
		resp.Bundle = &exportJobBundle{ArchivePath: row.BundleArchivePath.String, ArchiveByteLength: row.BundleArchiveLen.Int64, ArchiveSHA256: row.BundleArchiveSHA.String, ManifestSHA256: row.BundleManifestSHA.String, ReportSnapshotID: row.BundleReportSnapID.String, ReceiptID: row.BundleReceiptID.String}
	}
	if row.FailureCode.Valid {
		resp.Failure = &exportJobFailure{Code: row.FailureCode.String, Message: row.FailureMessage.String, Retryable: row.FailureRetryable.Bool}
	}
	return resp
}

func exportReceiptResponseFromRow(row exportReceiptRecord) exportReceiptResponse {
	resp := exportReceiptResponse{
		ContractVersion:   exportContractVersion,
		ID:                row.ID,
		ExportJobID:       row.ExportJobID,
		EngagementID:      row.EngagementID,
		Status:            row.Status,
		BundlePath:        row.BundlePath,
		ArchiveByteLength: row.ArchiveByteLength,
		ArchiveSHA256:     row.ArchiveSHA256,
		ManifestSHA256:    row.ManifestSHA256,
		Cutoff:            row.Cutoff.UTC().Format(time.RFC3339),
		VerifiedAt:        row.VerifiedAt.UTC().Format(time.RFC3339),
		VerifiedBy:        actorSnapshotFromRecord(row.VerifiedBy),
		VerifierVersion:   row.VerifierVersion,
		Revision:          row.Revision,
	}
	if row.InvalidatedAt.Valid {
		resp.InvalidatedAt = row.InvalidatedAt.Time.UTC().Format(time.RFC3339)
	}
	if row.InvalidationReason.Valid {
		resp.InvalidationReason = row.InvalidationReason.String
	}
	return resp
}

func actorSnapshotFromRecord(actor actorRecord) actorSnapshot {
	out := actorSnapshot{ID: actor.ID, Kind: actor.Kind, Handle: actor.Handle, Role: actor.Role}
	if actor.AgentName != "" {
		out.AgentName = actor.AgentName
	}
	if actor.Model != "" {
		out.Model = actor.Model
	}
	if actor.Version != "" {
		out.Version = actor.Version
	}
	if actor.AuthorizedBy != "" {
		out.AuthorizedBy = actor.AuthorizedBy
	}
	return out
}

type actorSnapshot struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Handle       string `json:"handle"`
	Role         string `json:"role"`
	AgentName    string `json:"agentName,omitempty"`
	Model        string `json:"model,omitempty"`
	Version      string `json:"version,omitempty"`
	AuthorizedBy string `json:"authorizedBy,omitempty"`
}

func parseExpectedRevision(v string) (int, *captureProblem) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusPreconditionRequired), Status: http.StatusPreconditionRequired, Code: "missing_field", Retryable: false, Detail: "If-Match is required."}
	}
	if len(v) < 3 || !strings.HasPrefix(v, `"`) || !strings.HasSuffix(v, `"`) {
		return 0, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", Retryable: false, Detail: "If-Match must quote a positive integer revision."}
	}
	n, err := strconv.Atoi(strings.Trim(v, `"`))
	if err != nil || n < 1 {
		return 0, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", Retryable: false, Detail: "If-Match must quote a positive integer revision."}
	}
	return n, nil
}

func isExportTerminal(state string) bool {
	switch state {
	case "failed", "cancelled", "completed":
		return true
	default:
		return false
	}
}

func exportFailureCode(err error) string {
	if err == nil {
		return "archive_failed"
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "cancel"):
		return "cancelled"
	case strings.Contains(msg, "snapshot"):
		return "snapshot_failed"
	case strings.Contains(msg, "manifest"):
		return "archive_failed"
	case strings.Contains(msg, "verify"):
		return "verification_failed"
	default:
		return "archive_failed"
	}
}

func exportRetryable(err error) bool {
	if err == nil {
		return true
	}
	msg := strings.ToLower(err.Error())
	return !strings.Contains(msg, "capacity") && !strings.Contains(msg, "cancel")
}

func loadExportArtifacts(m *exportManager, job exportJobRecord) (exportArtifacts, error) {
	bundlePath := "bundle"
	if job.BundleArchivePath.Valid && strings.TrimSpace(job.BundleArchivePath.String) != "" {
		bundlePath = job.BundleArchivePath.String
	}
	bundleDir := filepath.Join(m.root, job.ID, filepath.FromSlash(bundlePath))
	manifestPath := filepath.Join(bundleDir, "metadata", "export-manifest.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return exportArtifacts{}, err
	}
	var parsed struct {
		Payloads []reportBundlePayload `json:"payloads"`
	}
	if err := json.Unmarshal(manifestBytes, &parsed); err != nil {
		return exportArtifacts{}, err
	}
	sidecarBytes, err := os.ReadFile(filepath.Join(bundleDir, "metadata", "export-archive.sha256"))
	if err != nil {
		return exportArtifacts{}, err
	}
	archiveSHA := strings.TrimSpace(string(sidecarBytes))
	manifestSHA := sha256HexBytes(manifestBytes)
	archiveLen := int64(len(manifestBytes))
	for _, payload := range parsed.Payloads {
		path := filepath.Join(bundleDir, strings.TrimPrefix(payload.Path, "bundle/"))
		info, err := os.Stat(path)
		if err != nil {
			return exportArtifacts{}, err
		}
		archiveLen += info.Size()
	}
	cutoff := ""
	if job.Cutoff.Valid {
		cutoff = job.Cutoff.Time.UTC().Format(time.RFC3339)
	}
	return exportArtifacts{bundleDir: bundleDir, manifest: manifestBytes, manifestSHA256: manifestSHA, archiveSHA256: archiveSHA, archiveByteLength: archiveLen, payloads: parsed.Payloads, snapshotID: job.SnapshotID.String, cutoff: cutoff, receiptID: job.BundleReceiptID.String}, nil
}

func buildExportDump(ctx context.Context, db queryer, engagementID, snapshotID, cutoff string) ([]byte, error) {
	engagement, err := loadReportEngagement(ctx, db, engagementID)
	if err != nil {
		return nil, err
	}
	actions, err := loadReportActions(ctx, db, engagementID)
	if err != nil {
		return nil, err
	}
	findings, err := loadReportFindings(ctx, db, engagementID)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{"formatVersion": exportContractVersion, "snapshotId": snapshotID, "cutoff": cutoff, "engagement": engagement, "actions": actions, "findings": findings}
	return json.MarshalIndent(payload, "", "  ")
}

func buildExportEvidenceTar(ctx context.Context, db queryer, store *evidenceStore, engagementID, outputPath string) (err error) {
	if store == nil || db == nil {
		return errors.New("evidence store unavailable")
	}
	rows, err := db.QueryContext(ctx, `SELECT storage_key FROM evidence WHERE engagement_id = $1 AND storage_key <> '' ORDER BY created_at ASC, id ASC`, engagementID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return err
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(outputPath), ".evidence-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	zw := tar.NewWriter(tmp)
	for _, key := range keys {
		path, err := safeEvidencePath(store.root, key)
		if err != nil {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		hdr := &tar.Header{Name: filepath.ToSlash(key), Mode: 0o600, Size: info.Size(), ModTime: info.ModTime()}
		if err := zw.WriteHeader(hdr); err != nil {
			_ = zw.Close()
			return err
		}
		blob, err := os.Open(path)
		if err != nil {
			continue
		}
		_, copyErr := io.Copy(zw, blob)
		_ = blob.Close()
		if copyErr != nil {
			_ = zw.Close()
			return copyErr
		}
	}
	if err = zw.Close(); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmpPath, outputPath); err != nil {
		return err
	}
	committed = true
	return nil
}

func computeExportArchiveHash(payloads []reportBundlePayload, manifestBytes []byte, bundleDir string) (string, int64, error) {
	h := sha256.New()
	if len(manifestBytes) > 0 {
		h.Write(manifestBytes)
	}
	total := int64(len(manifestBytes))
	paths := append([]reportBundlePayload(nil), payloads...)
	sort.Slice(paths, func(i, j int) bool { return paths[i].Path < paths[j].Path })
	for _, payload := range paths {
		path := filepath.Join(filepath.FromSlash(bundleDir), strings.TrimPrefix(payload.Path, "bundle/"))
		info, err := os.Stat(path)
		if err != nil {
			return "", 0, err
		}
		h.Write([]byte(payload.Path))
		h.Write([]byte{0})
		f, err := os.Open(path)
		if err != nil {
			return "", 0, err
		}
		copied, copyErr := io.Copy(h, f)
		_ = f.Close()
		if copyErr != nil {
			return "", 0, copyErr
		}
		if copied != info.Size() {
			return "", 0, fmt.Errorf("short read for %s", payload.Path)
		}
		total += copied
	}
	return hex.EncodeToString(h.Sum(nil)), total, nil
}

func verifyExportBundle(bundleDir string, manifest []byte, manifestSHA256, archiveSHA256 string) error {
	var parsed struct {
		Version  string `json:"version"`
		Payloads []struct {
			Path string `json:"path"`
			Size int64  `json:"size"`
			SHA  string `json:"sha256"`
		} `json:"payloads"`
		Signatures struct {
			Version string   `json:"version"`
			Items   []string `json:"items"`
		} `json:"signatures"`
	}
	if err := json.Unmarshal(manifest, &parsed); err != nil {
		return err
	}
	if parsed.Version != "v1" || parsed.Signatures.Version != "v1" || len(parsed.Signatures.Items) != 0 {
		return errors.New("bundle manifest signature hook must be versioned and empty")
	}
	payloads := make([]reportBundlePayload, 0, len(parsed.Payloads))
	for _, payload := range parsed.Payloads {
		path := filepath.Join(filepath.FromSlash(bundleDir), strings.TrimPrefix(payload.Path, "bundle/"))
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.Size() != payload.Size {
			return fmt.Errorf("size mismatch for %s", payload.Path)
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		if hex.EncodeToString(h.Sum(nil)) != payload.SHA {
			return fmt.Errorf("sha256 mismatch for %s", payload.Path)
		}
		payloads = append(payloads, reportBundlePayload{Path: payload.Path, Size: payload.Size, SHA256: payload.SHA})
	}
	archive, _, err := computeExportArchiveHash(payloads, manifest, bundleDir)
	if err != nil {
		return err
	}
	if archive != archiveSHA256 {
		return errors.New("outer archive hash mismatch")
	}
	if sha256HexBytes(manifest) != manifestSHA256 {
		return errors.New("manifest digest mismatch")
	}
	return nil
}

func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), info.Size(), nil
}

func sha256HexBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

const bundleVerifyToolScript = "#!/usr/bin/env node\nimport { verifyBundle } from '../../web/scripts/bundle-tools.mjs';\n\nconst bundleRoot = process.argv[2] ? process.argv[2] : '.';\n\ntry {\n  const result = await verifyBundle(bundleRoot);\n  process.stdout.write(JSON.stringify({ status: 'verified', ...result }, null, 2) + '\\n');\n} catch (error) {\n  console.error(error instanceof Error ? error.message : String(error));\n  process.exit(1);\n}\n"

const bundleRegenerateToolScript = "#!/usr/bin/env node\nimport { regenerateReport } from '../../web/scripts/bundle-tools.mjs';\n\nconst bundleRoot = process.argv[2] ? process.argv[2] : '.';\nconst outputPath = process.argv[3];\n\ntry {\n  const result = await regenerateReport(bundleRoot, outputPath);\n  if (result.html) {\n    process.stdout.write(result.html);\n  } else {\n    process.stdout.write(JSON.stringify({ status: 'rendered', ...result }, null, 2) + '\\n');\n  }\n} catch (error) {\n  console.error(error instanceof Error ? error.message : String(error));\n  process.exit(1);\n}\n"

const bundleRestoreInstructions = `Verify the outer archive hash before restore.
Reject the manifest if any payload path is missing, duplicated, or traverses upward.
Regenerate the report from the frozen snapshot rather than live queries.
`

func appendExportAuditEvent(ctx context.Context, tx *sql.Tx, actor actorRecord, reqID, eventType, subjectID string, subjectRevision int, originKind, originService string, data map[string]any) error {
	if tx == nil {
		return errors.New("tx is required")
	}
	_, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
		EngagementID:  actor.EngagementID,
		Type:          eventType,
		Actor:         dbutil.AuditActorSnapshot{ID: actor.ID, Kind: actor.Kind, Handle: actor.Handle, Role: actor.Role, AgentName: actor.AgentName, Model: actor.Model, Version: actor.Version, AuthorizedBy: actor.AuthorizedBy},
		Origin:        dbutil.AuditOrigin{Kind: originKind, Service: originService},
		Subject:       dbutil.AuditSubject{Type: "export", ID: subjectID, Revision: subjectRevision},
		RequestID:     reqID,
		CorrelationID: reqID,
		Data:          data,
	})
	return err
}

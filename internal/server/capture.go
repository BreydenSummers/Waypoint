package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
	"time"

	dbutil "waypoint/internal/db"
)

var (
	uuidPattern       = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	entityKindPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	hexSHA256Pattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

var errEntityKindConflict = errors.New("entity identity already exists with a different kind")

type captureEnvelope struct {
	ContractVersion string                  `json:"contractVersion"`
	CaptureID       string                  `json:"captureId"`
	SourceAgent     captureSourceAgent      `json:"sourceAgent"`
	Phase           string                  `json:"phase"`
	InitiatedBy     string                  `json:"initiatedBy"`
	DecisionContext *captureDecisionContext `json:"decisionContext,omitempty"`
	Command         string                  `json:"command"`
	Argv            []string                `json:"argv"`
	Cwd             string                  `json:"cwd"`
	Target          captureTarget           `json:"target"`
	Timing          captureTiming           `json:"timing"`
	Execution       captureExecution        `json:"execution"`
	Network         captureNetwork          `json:"network"`
	Evidence        captureEvidenceSet      `json:"evidence"`
	Parsing         captureParsing          `json:"parsing"`
}

type captureSourceAgent struct {
	ID       string          `json:"id"`
	Kind     string          `json:"kind"`
	Name     string          `json:"name"`
	Version  string          `json:"version"`
	Platform capturePlatform `json:"platform"`
}

type capturePlatform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type captureDecisionContext struct {
	Rationale              string `json:"rationale,omitempty"`
	PromptReference        string `json:"promptReference,omitempty"`
	AuthorizationReference string `json:"authorizationReference,omitempty"`
}

type captureTarget struct {
	Kind      string `json:"kind"`
	Value     string `json:"value"`
	Port      *int   `json:"port,omitempty"`
	Transport string `json:"transport,omitempty"`
}

type captureTiming struct {
	StartedAt  time.Time `json:"startedAt"`
	EndedAt    time.Time `json:"endedAt"`
	DurationMs int       `json:"durationMs"`
}

type captureExecution struct {
	Status      string `json:"status"`
	ExitCode    *int   `json:"exitCode,omitempty"`
	Signal      string `json:"signal,omitempty"`
	FailureCode string `json:"failureCode,omitempty"`
}

type captureNetwork struct {
	ExecHost   captureExecHost   `json:"execHost"`
	Egress     captureEgress     `json:"egress"`
	PivotChain []capturePivotHop `json:"pivotChain"`
}

type captureExecHost struct {
	Address    string `json:"address"`
	Method     string `json:"method"`
	Confidence string `json:"confidence"`
	Interface  string `json:"interface,omitempty"`
}

type captureEgress struct {
	Mode       string     `json:"mode"`
	Status     string     `json:"status"`
	Address    string     `json:"address,omitempty"`
	ObservedAt *time.Time `json:"observedAt,omitempty"`
}

type capturePivotHop struct {
	Type  string `json:"type"`
	Host  string `json:"host"`
	Port  *int   `json:"port,omitempty"`
	Label string `json:"label,omitempty"`
}

type captureEvidenceSet struct {
	Stdout captureEvidenceDescriptor `json:"stdout"`
	Stderr captureEvidenceDescriptor `json:"stderr"`
}

type captureEvidenceDescriptor struct {
	MediaType  string `json:"mediaType"`
	ByteLength int64  `json:"byteLength"`
	SHA256     string `json:"sha256"`
}

type captureParsing struct {
	Status  string                  `json:"status"`
	Plugin  *capturePluginSelection `json:"plugin,omitempty"`
	Result  *captureParseResult     `json:"result,omitempty"`
	Failure *captureParseFailure    `json:"failure,omitempty"`
}

type capturePluginSelection struct {
	ID              string             `json:"id"`
	Version         string             `json:"version"`
	ArtifactSHA256  string             `json:"artifactSha256"`
	ContractVersion string             `json:"contractVersion"`
	Match           capturePluginMatch `json:"match"`
}

type capturePluginMatch struct {
	Binary      string `json:"binary"`
	Reason      string `json:"reason"`
	Specificity int    `json:"specificity"`
}

type captureParseResult struct {
	SchemaID      string                `json:"schemaId"`
	SchemaVersion string                `json:"schemaVersion"`
	Extracted     map[string]any        `json:"extracted"`
	Entities      []captureParsedEntity `json:"entities"`
}

type captureParsedEntity struct {
	Kind        string                    `json:"kind"`
	Identifiers []captureEntityIdentifier `json:"identifiers"`
	Attributes  map[string]any            `json:"attributes"`
}

type captureEntityIdentifier struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type captureParseFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type captureRequest struct {
	Envelope captureEnvelope
	Stdout   captureEvidenceBytes
	Stderr   captureEvidenceBytes
}

type captureEvidenceBytes struct {
	digest     string
	byteLength int64
}

type captureProblem struct {
	Type                   string       `json:"type"`
	Title                  string       `json:"title"`
	Status                 int          `json:"status"`
	Detail                 string       `json:"detail,omitempty"`
	Code                   string       `json:"code"`
	RequestID              string       `json:"requestId"`
	Retryable              bool         `json:"retryable"`
	FieldErrors            []fieldError `json:"fieldErrors,omitempty"`
	ExistingActionID       string       `json:"existingActionId,omitempty"`
	MinimumAvailableCursor *string      `json:"minimumAvailableCursor,omitempty"`
	Resync                 string       `json:"resync,omitempty"`
	SupportedVersions      []string     `json:"supportedVersions,omitempty"`
}

type fieldError struct {
	Pointer string `json:"pointer"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type captureAck struct {
	ContractVersion  string       `json:"contractVersion"`
	ActionID         string       `json:"actionId"`
	CaptureID        string       `json:"captureId"`
	AuditEventCursor string       `json:"auditEventCursor"`
	ReceivedAt       time.Time    `json:"receivedAt"`
	Idempotency      string       `json:"idempotency"`
	ClockSkew        *captureSkew `json:"clockSkew,omitempty"`
}

type captureSkew struct {
	Status   string `json:"status"`
	OffsetMs int64  `json:"offsetMs"`
}

type actorRecord struct {
	ID           string
	EngagementID string
	Kind         string
	Handle       string
	Role         string
	AgentName    string
	Model        string
	Version      string
	AuthorizedBy string
	TokenHash    string
}

type existingCapture struct {
	actionID           string
	captureFingerprint string
}

type captureRequestProblem struct{ problem captureProblem }

func (e captureRequestProblem) Error() string { return e.problem.Detail }

func captureHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if db == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: reqID, Retryable: true, Detail: "capture ingestion is unavailable"})
			return
		}
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{"1.0.0"}})
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

		req, err := readCaptureRequest(r)
		if err != nil {
			if pb, ok := err.(captureRequestProblem); ok {
				pb.problem.RequestID = reqID
				writeProblem(w, pb.problem)
				return
			}
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()})
			return
		}

		if !isUUID(req.Envelope.CaptureID) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/captureId", Code: "invalid_uuid", Message: "captureId must be a UUID."}}})
			return
		}
		if !strings.EqualFold(req.Envelope.CaptureID, r.Header.Get("Idempotency-Key")) {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/captureId", Code: "idempotency_key_mismatch", Message: "Idempotency-Key must exactly match captureId."}}})
			return
		}
		if pb := validateCaptureEnvelope(req.Envelope, actor); pb != nil {
			pb.RequestID = reqID
			writeProblem(w, *pb)
			return
		}
		if err := verifyEvidence(req); err != nil {
			pb := captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUnprocessableEntity), Status: http.StatusUnprocessableEntity, Code: "evidence_integrity_mismatch", RequestID: reqID, Retryable: false, Detail: err.Error()}
			if errors.Is(err, errStdoutMismatch) {
				pb.FieldErrors = []fieldError{{Pointer: "/evidence/stdout/sha256", Code: "digest_mismatch", Message: "stdout digest did not match the uploaded bytes."}}
			} else {
				pb.FieldErrors = []fieldError{{Pointer: "/evidence/stderr/sha256", Code: "digest_mismatch", Message: "stderr digest did not match the uploaded bytes."}}
			}
			writeProblem(w, pb)
			return
		}

		fingerprint, err := captureFingerprint(req.Envelope)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()})
			return
		}

		tx, err := db.BeginTx(r.Context(), nil)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "begin transaction failed"})
			return
		}
		defer tx.Rollback()

		if err := lockCaptureScope(r.Context(), tx, actor.EngagementID, actor.ID, req.Envelope.SourceAgent.ID, req.Envelope.CaptureID); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "capture lock failed"})
			return
		}

		existing, err := loadExistingCapture(r.Context(), tx, actor.EngagementID, actor.ID, req.Envelope.SourceAgent.ID, req.Envelope.CaptureID)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load capture state failed"})
			return
		}
		if existing != nil {
			if existing.captureFingerprint == fingerprint {
				ack, err := replayAck(r.Context(), tx, existing.actionID, req.Envelope.CaptureID)
				if err != nil {
					writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "load replay acknowledgement failed"})
					return
				}
				if err := tx.Commit(); err != nil {
					writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "commit replay failed"})
					return
				}
				writeJSONWithHeaders(w, http.StatusOK, ack, reqID)
				return
			}

			if _, err := dbutil.AppendAuditEvent(r.Context(), tx, dbutil.AuditEventInput{
				EngagementID:    actor.EngagementID,
				Type:            "capture.conflict",
				Actor:           auditActorSnapshot(actor),
				Origin:          dbutil.AuditOrigin{Kind: "rest"},
				Subject:         dbutil.AuditSubject{Type: "action", ID: existing.actionID, Revision: 1},
				RequestID:       reqID,
				CorrelationID:   req.Envelope.CaptureID,
				CausationAction: existing.actionID,
				Data:            map[string]any{"captureId": req.Envelope.CaptureID, "sourceAgentId": req.Envelope.SourceAgent.ID, "existingActionId": existing.actionID, "reason": "payload_mismatch"},
			}); err != nil {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "append conflict audit event failed"})
				return
			}
			if err := tx.Commit(); err != nil {
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "commit conflict failed"})
				return
			}
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "idempotency_conflict", RequestID: reqID, Retryable: false, ExistingActionID: existing.actionID, Detail: "same captureId was previously accepted with different payload or evidence"})
			return
		}

		stdoutID, err := upsertEvidence(r.Context(), tx, actor.EngagementID, "stdout", req.Envelope.Evidence.Stdout.MediaType, req.Envelope.Evidence.Stdout.SHA256, req.Envelope.Evidence.Stdout.ByteLength)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "persist stdout evidence failed"})
			return
		}
		stderrID, err := upsertEvidence(r.Context(), tx, actor.EngagementID, "stderr", req.Envelope.Evidence.Stderr.MediaType, req.Envelope.Evidence.Stderr.SHA256, req.Envelope.Evidence.Stderr.ByteLength)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "persist stderr evidence failed"})
			return
		}

		actionID := newUUID()
		if _, err := tx.ExecContext(r.Context(), `
			INSERT INTO action (
				id, engagement_id, actor_id, source_agent_id, capture_id, capture_fingerprint, initiated_by, phase, command, argv, cwd,
				exec_host_ip, egress_public_ip, pivot_chain, target_kind, target_value, target_port, target_transport,
				started_at, ended_at, exit_code, stdout_evidence_id, stderr_evidence_id, plugin_id, parse_status, decision_context
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11,
				$12::inet, $13::inet, $14::jsonb, $15, $16, $17, $18,
				$19, $20, $21, $22, $23, $24, $25, $26::jsonb
			)`,
			actionID,
			actor.EngagementID,
			actor.ID,
			req.Envelope.SourceAgent.ID,
			req.Envelope.CaptureID,
			fingerprint,
			req.Envelope.InitiatedBy,
			req.Envelope.Phase,
			req.Envelope.Command,
			mustJSON(req.Envelope.Argv),
			req.Envelope.Cwd,
			req.Envelope.Network.ExecHost.Address,
			nullIfEmpty(req.Envelope.Network.Egress.Address),
			mustJSON(req.Envelope.Network.PivotChain),
			req.Envelope.Target.Kind,
			req.Envelope.Target.Value,
			targetPortValue(req.Envelope.Target.Port),
			nullIfEmpty(req.Envelope.Target.Transport),
			req.Envelope.Timing.StartedAt,
			req.Envelope.Timing.EndedAt,
			exitCodeValue(req.Envelope.Execution.ExitCode),
			stdoutID,
			stderrID,
			pluginID(req.Envelope.Parsing),
			req.Envelope.Parsing.Status,
			jsonArg(decisionContextMap(req.Envelope.DecisionContext)),
		); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: fmt.Sprintf("insert action failed: %v", err)})
			return
		}

		if req.Envelope.Parsing.Status == "parsed" {
			if err := ingestStructuredResult(r.Context(), tx, actor.EngagementID, actionID, req.Envelope.Parsing.Plugin.ID, req.Envelope.Parsing.Result); err != nil {
				if errors.Is(err, errEntityKindConflict) {
					writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "entity_conflict", RequestID: reqID, Retryable: false, Detail: "conflicting entity identifiers cannot be auto-merged."})
					return
				}
				writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: fmt.Sprintf("persist structured result failed: %v", err)})
				return
			}
		}

		receivedAt := time.Now().UTC()
		acceptedID, err := dbutil.AppendAuditEvent(r.Context(), tx, dbutil.AuditEventInput{
			EngagementID:  actor.EngagementID,
			Type:          "capture.accepted",
			Actor:         auditActorSnapshot(actor),
			Origin:        dbutil.AuditOrigin{Kind: "rest"},
			Subject:       dbutil.AuditSubject{Type: "action", ID: actionID, Revision: 1},
			RequestID:     reqID,
			CorrelationID: req.Envelope.CaptureID,
			Data:          map[string]any{"actionId": actionID, "captureId": req.Envelope.CaptureID, "sourceAgentId": req.Envelope.SourceAgent.ID, "phase": req.Envelope.Phase, "initiatedBy": req.Envelope.InitiatedBy, "command": req.Envelope.Command, "target": map[string]any{"kind": req.Envelope.Target.Kind, "value": req.Envelope.Target.Value}, "execution": captureExecutionData(req.Envelope.Execution), "parseStatus": req.Envelope.Parsing.Status, "egressStatus": req.Envelope.Network.Egress.Status, "receivedAt": receivedAt, "clockSkewStatus": skewStatus(req.Envelope.Timing.EndedAt, receivedAt)},
		})
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: fmt.Sprintf("append accepted audit event failed: %v", err)})
			return
		}

		ack := captureAck{ContractVersion: "1.0.0", ActionID: actionID, CaptureID: req.Envelope.CaptureID, AuditEventCursor: eventCursor(acceptedID), ReceivedAt: receivedAt, Idempotency: "created", ClockSkew: clockSkew(req.Envelope.Timing.EndedAt, receivedAt)}
		if err := tx.Commit(); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "commit create failed"})
			return
		}
		writeJSONWithHeaders(w, http.StatusCreated, ack, reqID)
	}
}

func readCaptureRequest(r *http.Request) (captureRequest, error) {
	var out captureRequest
	mr, err := r.MultipartReader()
	if err != nil {
		return out, captureRequestProblem{problem: captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", Retryable: false, Detail: "request must be multipart/form-data"}}
	}
	seen := map[string]bool{}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return out, err
		}
		name := part.FormName()
		data, err := io.ReadAll(part)
		_ = part.Close()
		if err != nil {
			return out, err
		}
		if seen[name] {
			return out, captureRequestProblem{problem: captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", Retryable: false, Detail: "duplicate multipart part"}}
		}
		seen[name] = true
		switch name {
		case "envelope":
			if err := decodeStrictJSON(data, &out.Envelope); err != nil {
				return out, captureRequestProblem{problem: captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", Retryable: false, Detail: err.Error()}}
			}
		case "stdout":
			out.Stdout = captureEvidenceBytes{digest: digestBytes(data), byteLength: int64(len(data))}
		case "stderr":
			out.Stderr = captureEvidenceBytes{digest: digestBytes(data), byteLength: int64(len(data))}
		default:
			return out, captureRequestProblem{problem: captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", Retryable: false, Detail: fmt.Sprintf("unexpected multipart part %q", name)}}
		}
	}
	if out.Envelope.CaptureID == "" {
		return out, captureRequestProblem{problem: captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", Retryable: false, FieldErrors: []fieldError{{Pointer: "/envelope", Code: "missing_part", Message: "envelope part is required."}}}}
	}
	if !seen["stdout"] {
		return out, captureRequestProblem{problem: captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", Retryable: false, FieldErrors: []fieldError{{Pointer: "/stdout", Code: "missing_part", Message: "stdout part is required."}}}}
	}
	if !seen["stderr"] {
		return out, captureRequestProblem{problem: captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", Retryable: false, FieldErrors: []fieldError{{Pointer: "/stderr", Code: "missing_part", Message: "stderr part is required."}}}}
	}
	return out, nil
}

func decodeStrictJSON(data []byte, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	return dec.Decode(v)
}

func validateCaptureEnvelope(env captureEnvelope, actor actorRecord) *captureProblem {
	if env.ContractVersion != "1.0.0" {
		return badField("/contractVersion", "unsupported_contract_version", "contractVersion must be 1.0.0.")
	}
	if env.SourceAgent.ID == "" || env.SourceAgent.Kind == "" || env.SourceAgent.Name == "" || env.SourceAgent.Version == "" || env.SourceAgent.Platform.OS == "" || env.SourceAgent.Platform.Arch == "" {
		return badField("/sourceAgent", "invalid_object", "sourceAgent is incomplete.")
	}
	if env.Phase != "recon" && env.Phase != "attacks" {
		return badField("/phase", "invalid_enum", "phase must be recon or attacks.")
	}
	if env.InitiatedBy != "manual" && env.InitiatedBy != "ai" && env.InitiatedBy != "scan-library" {
		return badField("/initiatedBy", "invalid_enum", "unsupported initiation mode.")
	}
	if env.InitiatedBy == "scan-library" {
		return badField("/initiatedBy", "reserved_value", "scan-library is reserved.")
	}
	if actor.Kind == "ai_agent" && env.InitiatedBy != "ai" {
		return badField("/initiatedBy", "actor_initiator_mismatch", "AI actors must use initiatedBy=ai.")
	}
	if actor.Kind == "human" && env.InitiatedBy != "manual" {
		return badField("/initiatedBy", "actor_initiator_mismatch", "Human actors must use initiatedBy=manual.")
	}
	if actor.Kind == "ai_agent" && env.DecisionContext == nil {
		return badField("/decisionContext", "missing_field", "AI captures require decisionContext.")
	}
	if actor.Kind == "human" && env.DecisionContext != nil {
		return badField("/decisionContext", "unexpected_field", "Human captures must not include decisionContext.")
	}
	if env.Command == "" {
		return badField("/command", "missing_field", "command is required.")
	}
	if env.Cwd == "" {
		return badField("/cwd", "missing_field", "cwd is required.")
	}
	if env.Target.Kind == "" || env.Target.Value == "" {
		return badField("/target", "invalid_object", "target is incomplete.")
	}
	if env.Timing.DurationMs < 0 {
		return badField("/timing/durationMs", "invalid_range", "durationMs must be >= 0.")
	}
	if env.Execution.Status == "" {
		return badField("/execution/status", "missing_field", "execution.status is required.")
	}
	if err := validateNetwork(env.Network); err != nil {
		return err
	}
	if err := validateEvidenceDescriptor("/evidence/stdout", env.Evidence.Stdout); err != nil {
		return err
	}
	if err := validateEvidenceDescriptor("/evidence/stderr", env.Evidence.Stderr); err != nil {
		return err
	}
	if err := validateParsing(env.Parsing); err != nil {
		return err
	}
	if actor.Kind == "ai_agent" && env.DecisionContext != nil && env.DecisionContext.Rationale == "" && env.DecisionContext.PromptReference == "" {
		return badField("/decisionContext", "missing_field", "decisionContext requires rationale or promptReference.")
	}
	return nil
}

var compatiblePluginSchemas = map[string]compatiblePluginSchema{
	"waypoint.nmap": {
		schemaID:        "https://schemas.waypoint.security/plugins/nmap/1.0.0/result.schema.json",
		schemaVersion:   "1.0.0",
		contractVersion: "1.0.0",
	},
}

type compatiblePluginSchema struct {
	schemaID        string
	schemaVersion   string
	contractVersion string
}

func validateParsing(p captureParsing) *captureProblem {
	switch p.Status {
	case "parsed":
		if p.Plugin == nil {
			return badField("/parsing/plugin", "missing_field", "parsed captures require plugin metadata.")
		}
		if p.Result == nil {
			return badField("/parsing/result", "missing_field", "parsed captures require a structured result.")
		}
		if err := validatePluginSelection(*p.Plugin); err != nil {
			return err
		}
		if err := validateStructuredResult(*p.Plugin, *p.Result); err != nil {
			return err
		}
	case "parse-failed":
		if p.Plugin == nil {
			return badField("/parsing/plugin", "missing_field", "parse-failed captures require plugin metadata.")
		}
		if p.Failure == nil {
			return badField("/parsing/failure", "missing_field", "parse-failed captures require failure details.")
		}
		if err := validatePluginSelection(*p.Plugin); err != nil {
			return err
		}
	case "needs-plugin", "raw":
		if p.Plugin != nil || p.Result != nil || p.Failure != nil {
			return badField("/parsing/status", "unexpected_field", "raw or needs-plugin captures must not include parser artifacts.")
		}
	default:
		return badField("/parsing/status", "invalid_enum", "unsupported parse status.")
	}
	return nil
}

func validatePluginSelection(p capturePluginSelection) *captureProblem {
	spec, ok := compatiblePluginSchemas[p.ID]
	if !ok {
		return badField("/parsing/plugin/id", "unsupported_plugin_schema", "plugin schema is not registered.")
	}
	if p.Version != spec.schemaVersion {
		return badField("/parsing/plugin/version", "unsupported_plugin_version", "plugin version is not compatible.")
	}
	if p.ContractVersion != spec.contractVersion {
		return badField("/parsing/plugin/contractVersion", "unsupported_contract_version", "plugin contract version is not compatible.")
	}
	if p.ArtifactSHA256 == "" || !isHexSHA256(p.ArtifactSHA256) {
		return badField("/parsing/plugin/artifactSha256", "invalid_format", "artifactSha256 must be lowercase hex.")
	}
	if p.Match.Binary == "" || p.Match.Reason == "" || p.Match.Specificity < 0 || p.Match.Specificity > 1000000 {
		return badField("/parsing/plugin/match", "invalid_object", "plugin match metadata is incomplete.")
	}
	return nil
}

func validateStructuredResult(plugin capturePluginSelection, result captureParseResult) *captureProblem {
	spec := compatiblePluginSchemas[plugin.ID]
	if result.SchemaID != spec.schemaID {
		return badField("/parsing/result/schemaId", "unsupported_schema", "structured result schema is not compatible.")
	}
	if result.SchemaVersion != spec.schemaVersion {
		return badField("/parsing/result/schemaVersion", "unsupported_schema_version", "structured result schema version is not compatible.")
	}
	if result.Extracted == nil {
		return badField("/parsing/result/extracted", "missing_field", "structured result requires extracted data.")
	}
	if len(result.Extracted) > 1024 {
		return badField("/parsing/result/extracted", "invalid_range", "structured result is too large.")
	}
	if result.Entities == nil {
		return badField("/parsing/result/entities", "missing_field", "structured result requires entity links.")
	}
	if len(result.Entities) > 10000 {
		return badField("/parsing/result/entities", "invalid_range", "structured result has too many entities.")
	}
	for i, entity := range result.Entities {
		if err := validateParsedEntity(entity, i); err != nil {
			return err
		}
	}
	return nil
}

func validateParsedEntity(entity captureParsedEntity, idx int) *captureProblem {
	if entity.Kind == "" || len(entity.Kind) > 64 || !entityKindPattern.MatchString(entity.Kind) {
		return badField(fmt.Sprintf("/parsing/result/entities/%d/kind", idx), "invalid_format", "entity kind is invalid.")
	}
	if entity.Attributes == nil {
		return badField(fmt.Sprintf("/parsing/result/entities/%d/attributes", idx), "missing_field", "entity attributes are required.")
	}
	if len(entity.Attributes) > 256 {
		return badField(fmt.Sprintf("/parsing/result/entities/%d/attributes", idx), "invalid_range", "entity attributes are too large.")
	}
	if len(entity.Identifiers) == 0 || len(entity.Identifiers) > 64 {
		return badField(fmt.Sprintf("/parsing/result/entities/%d/identifiers", idx), "invalid_range", "entity identifiers are required.")
	}
	for j, identifier := range entity.Identifiers {
		if identifier.Value == "" || len(identifier.Value) > 2048 {
			return badField(fmt.Sprintf("/parsing/result/entities/%d/identifiers/%d/value", idx, j), "invalid_format", "entity identifier value is invalid.")
		}
		switch identifier.Type {
		case "ad_sid", "mac", "fqdn", "hostname", "ip", "other":
		default:
			return badField(fmt.Sprintf("/parsing/result/entities/%d/identifiers/%d/type", idx, j), "invalid_enum", "entity identifier type is invalid.")
		}
	}
	return nil
}

func validateNetwork(n captureNetwork) *captureProblem {
	if n.ExecHost.Address == "" || n.ExecHost.Method == "" || n.ExecHost.Confidence == "" {
		return badField("/network/execHost", "invalid_object", "execHost attribution is incomplete.")
	}
	switch n.Egress.Mode {
	case "auto":
		if n.Egress.Status != "observed" && n.Egress.Status != "resolution_failed" {
			return badField("/network/egress/status", "invalid_enum", "auto egress must be observed or resolution_failed.")
		}
	case "manual":
		if n.Egress.Status != "declared" {
			return badField("/network/egress/status", "invalid_enum", "manual egress must be declared.")
		}
	case "off":
		if n.Egress.Status != "disabled" {
			return badField("/network/egress/status", "invalid_enum", "off egress must be disabled.")
		}
	default:
		return badField("/network/egress/mode", "invalid_enum", "unsupported egress mode.")
	}
	if (n.Egress.Status == "observed" || n.Egress.Status == "declared") && (n.Egress.Address == "" || n.Egress.ObservedAt == nil) {
		return badField("/network/egress", "invalid_object", "observed or declared egress requires address and observedAt.")
	}
	if (n.Egress.Status == "disabled" || n.Egress.Status == "resolution_failed") && (n.Egress.Address != "" || n.Egress.ObservedAt != nil) {
		return badField("/network/egress", "invalid_object", "disabled or resolution_failed egress must omit address and observedAt.")
	}
	return nil
}

func validateEvidenceDescriptor(ptr string, d captureEvidenceDescriptor) *captureProblem {
	if d.MediaType == "" || d.SHA256 == "" || d.ByteLength < 0 {
		return badField(ptr, "invalid_object", "evidence descriptor is incomplete.")
	}
	if !isHexSHA256(d.SHA256) {
		return badField(ptr+"/sha256", "invalid_format", "sha256 must be lowercase hex.")
	}
	return nil
}

var (
	errStdoutMismatch = errors.New("stdout digest mismatch")
	errStderrMismatch = errors.New("stderr digest mismatch")
)

func verifyEvidence(req captureRequest) error {
	if req.Stdout.digest != req.Envelope.Evidence.Stdout.SHA256 || req.Stdout.byteLength != req.Envelope.Evidence.Stdout.ByteLength {
		return errStdoutMismatch
	}
	if req.Stderr.digest != req.Envelope.Evidence.Stderr.SHA256 || req.Stderr.byteLength != req.Envelope.Evidence.Stderr.ByteLength {
		return errStderrMismatch
	}
	return nil
}

func captureFingerprint(env captureEnvelope) (string, error) {
	payload := map[string]any{"contractVersion": env.ContractVersion, "captureId": env.CaptureID, "sourceAgent": env.SourceAgent, "phase": env.Phase, "initiatedBy": env.InitiatedBy, "decisionContext": env.DecisionContext, "command": env.Command, "argv": env.Argv, "cwd": env.Cwd, "target": env.Target, "timing": env.Timing, "execution": env.Execution, "network": env.Network, "evidence": env.Evidence, "parsing": env.Parsing}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func lockCaptureScope(ctx context.Context, tx *sql.Tx, engagementID, actorID, sourceAgentID, captureID string) error {
	h := fnv.New64a()
	_, _ = h.Write([]byte(engagementID + "|" + actorID + "|" + sourceAgentID + "|" + captureID))
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(h.Sum64()))
	return err
}

func loadExistingCapture(ctx context.Context, tx *sql.Tx, engagementID, actorID, sourceAgentID, captureID string) (*existingCapture, error) {
	var out existingCapture
	err := tx.QueryRowContext(ctx, `SELECT id, capture_fingerprint FROM action WHERE engagement_id = $1 AND actor_id = $2 AND source_agent_id = $3 AND capture_id = $4`, engagementID, actorID, sourceAgentID, captureID).Scan(&out.actionID, &out.captureFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func replayAck(ctx context.Context, tx *sql.Tx, actionID, captureID string) (captureAck, error) {
	var eventID int64
	var receivedAt, endedAt time.Time
	if err := tx.QueryRowContext(ctx, `SELECT a.id, a.occurred_at, act.ended_at FROM audit_event a JOIN action act ON act.id = a.subject_id WHERE a.subject_type = 'action' AND a.subject_id = $1 AND a.type = 'capture.accepted' ORDER BY a.id DESC LIMIT 1`, actionID).Scan(&eventID, &receivedAt, &endedAt); err != nil {
		return captureAck{}, err
	}
	return captureAck{ContractVersion: "1.0.0", ActionID: actionID, CaptureID: captureID, AuditEventCursor: eventCursor(eventID), ReceivedAt: receivedAt, Idempotency: "replayed", ClockSkew: clockSkew(endedAt, receivedAt)}, nil
}

func upsertEvidence(ctx context.Context, tx *sql.Tx, engagementID, kind, mediaType, sha string, byteLength int64) (string, error) {
	var id string
	if err := tx.QueryRowContext(ctx, `INSERT INTO evidence (engagement_id, kind, sha256, byte_length, media_type, storage_key) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING RETURNING id`, engagementID, kind, sha, byteLength, mediaType, "captures/"+sha+"/"+kind).Scan(&id); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
		if err := tx.QueryRowContext(ctx, `SELECT id FROM evidence WHERE engagement_id = $1 AND kind = $2 AND sha256 = $3`, engagementID, kind, sha).Scan(&id); err != nil {
			return "", err
		}
	}
	return id, nil
}

func auditActorSnapshot(actor actorRecord) dbutil.AuditActorSnapshot {
	return dbutil.AuditActorSnapshot{ID: actor.ID, Kind: actor.Kind, Handle: actor.Handle, Role: actor.Role, AgentName: actor.AgentName, Model: actor.Model, Version: actor.Version, AuthorizedBy: actor.AuthorizedBy}
}

func captureExecutionData(exec captureExecution) map[string]any {
	m := map[string]any{"status": exec.Status}
	if exec.ExitCode != nil {
		m["exitCode"] = *exec.ExitCode
	}
	if exec.Signal != "" {
		m["signal"] = exec.Signal
	}
	if exec.FailureCode != "" {
		m["failureCode"] = exec.FailureCode
	}
	return m
}

func decisionContextMap(ctx *captureDecisionContext) map[string]any {
	if ctx == nil {
		return nil
	}
	m := map[string]any{}
	if ctx.Rationale != "" {
		m["rationale"] = ctx.Rationale
	}
	if ctx.PromptReference != "" {
		m["promptReference"] = ctx.PromptReference
	}
	if ctx.AuthorizationReference != "" {
		m["authorizationReference"] = ctx.AuthorizationReference
	}
	return m
}

func ingestStructuredResult(ctx context.Context, tx *sql.Tx, engagementID, actionID, pluginID string, result *captureParseResult) error {
	if result == nil {
		return fmt.Errorf("missing structured result")
	}
	resultID := newUUID()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO result (id, engagement_id, action_id, plugin_id, schema_id, schema_version, extracted)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
	`, resultID, engagementID, actionID, pluginID, result.SchemaID, result.SchemaVersion, jsonArg(result.Extracted)); err != nil {
		return err
	}

	seen := map[string]bool{}
	for _, entity := range result.Entities {
		keyType, keyValue, ok := entityIdentity(entity.Identifiers)
		if !ok {
			return fmt.Errorf("entity identity could not be derived")
		}
		identityKey := keyType + "\x00" + keyValue
		if seen[identityKey] {
			continue
		}
		seen[identityKey] = true

		entityID, err := upsertEntity(ctx, tx, engagementID, entity.Kind, keyType, keyValue, entity.Attributes)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO observation (engagement_id, action_id, result_id, entity_id, kind, identifiers, attributes)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb)
		`, engagementID, actionID, resultID, entityID, entity.Kind, jsonArg(entity.Identifiers), jsonArg(entity.Attributes)); err != nil {
			return err
		}
	}
	return nil
}

func upsertEntity(ctx context.Context, tx *sql.Tx, engagementID, kind, keyType, keyValue string, attrs map[string]any) (string, error) {
	var id string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO entity (engagement_id, kind, key_type, key_value, attributes)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		ON CONFLICT (engagement_id, key_type, key_value)
		DO UPDATE SET last_seen = now(), updated_at = now(), attributes = entity.attributes || EXCLUDED.attributes
		WHERE entity.kind = EXCLUDED.kind
		RETURNING id
	`, engagementID, kind, keyType, keyValue, jsonArg(attrs)).Scan(&id); err == nil {
		return id, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}

	var existingID, existingKind string
	if err := tx.QueryRowContext(ctx, `SELECT id, kind FROM entity WHERE engagement_id = $1 AND key_type = $2 AND key_value = $3`, engagementID, keyType, keyValue).Scan(&existingID, &existingKind); err != nil {
		return "", err
	}
	if existingKind != kind {
		return "", errEntityKindConflict
	}
	return existingID, nil
}

func entityIdentity(ids []captureEntityIdentifier) (string, string, bool) {
	var adSID, mac, fqdn, hostname, ip string
	for _, id := range ids {
		switch id.Type {
		case "ad_sid":
			if normalized := normalizeStableIdentifier(id.Value); normalized != "" && adSID == "" {
				adSID = normalized
			}
		case "mac":
			if normalized := normalizeMACIdentifier(id.Value); normalized != "" && mac == "" {
				mac = normalized
			}
		case "fqdn":
			if normalized := normalizeDNSIdentifier(id.Value); normalized != "" && fqdn == "" {
				fqdn = normalized
			}
		case "hostname":
			if normalized := normalizeDNSIdentifier(id.Value); normalized != "" && hostname == "" {
				hostname = normalized
			}
		case "ip":
			if normalized := normalizeIPIdentifier(id.Value); normalized != "" && ip == "" {
				ip = normalized
			}
		case "other":
		}
	}
	switch {
	case adSID != "":
		return "ad_sid", adSID, true
	case mac != "":
		return "mac", mac, true
	case fqdn != "":
		return "fqdn", fqdn, true
	case hostname != "" && ip != "":
		return "hostname_ip", "hostname=" + hostname + "|ip=" + ip, true
	case ip != "":
		return "other", "ip=" + ip, true
	case hostname != "":
		return "other", "hostname=" + hostname, true
	default:
		return "other", "identifier=" + strings.TrimSpace(ids[0].Value), true
	}
}

func normalizeStableIdentifier(v string) string {
	return strings.TrimSpace(v)
}

func normalizeMACIdentifier(v string) string {
	addr, err := net.ParseMAC(strings.TrimSpace(v))
	if err != nil {
		return ""
	}
	return addr.String()
}

func normalizeDNSIdentifier(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimSuffix(v, ".")
	return strings.ToLower(v)
}

func normalizeIPIdentifier(v string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(v))
	if err != nil {
		return ""
	}
	return addr.String()
}

func pluginID(p captureParsing) string {
	if p.Plugin == nil {
		return ""
	}
	return p.Plugin.ID
}
func targetPortValue(port *int) any {
	if port == nil {
		return nil
	}
	return *port
}
func exitCodeValue(code *int) any {
	if code == nil {
		return nil
	}
	return *code
}
func jsonArg(v any) any {
	if v == nil {
		return nil
	}
	b, _ := json.Marshal(v)
	return string(b)
}
func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
func mustJSON(v any) string {
	if v == nil {
		return "null"
	}
	b, _ := json.Marshal(v)
	return string(b)
}
func skewStatus(endedAt, receivedAt time.Time) string {
	if off := clockOffsetMs(endedAt, receivedAt); off > 5000 || off < -5000 {
		return "outside_tolerance"
	}
	return "within_tolerance"
}
func clockSkew(endedAt, receivedAt time.Time) *captureSkew {
	return &captureSkew{Status: skewStatus(endedAt, receivedAt), OffsetMs: clockOffsetMs(endedAt, receivedAt)}
}
func clockOffsetMs(endedAt, receivedAt time.Time) int64 {
	return receivedAt.Sub(endedAt).Milliseconds()
}
func requestIDFromHeader(v string) string {
	if v == "" {
		return newUUID()
	}
	return v
}
func validateContractVersion(v string) error {
	if v != "1.0.0" {
		return fmt.Errorf("expected Waypoint-Contract-Version: 1.0.0")
	}
	return nil
}
func bearerToken(v string) (string, error) {
	if !strings.HasPrefix(v, "Bearer ") {
		return "", fmt.Errorf("bearer token required")
	}
	t := strings.TrimSpace(strings.TrimPrefix(v, "Bearer "))
	if t == "" {
		return "", fmt.Errorf("bearer token required")
	}
	return t, nil
}

type stringArray []string

func (a stringArray) Value() (driver.Value, error) {
	return "{" + strings.Join([]string(a), ",") + "}", nil
}

func lookupActor(ctx context.Context, db *sql.DB, token string) (actorRecord, error) {
	cands := []string{sha256Hex(token)}
	if isHexSHA256(token) {
		cands = append([]string{strings.ToLower(token)}, cands...)
	}
	var a actorRecord
	err := db.QueryRowContext(ctx, `SELECT id, engagement_id, kind, handle, role, COALESCE(agent_name,''), COALESCE(model,''), COALESCE(version,''), COALESCE(authorized_by::text,''), token_hash FROM actor WHERE token_hash = ANY($1) AND revoked_at IS NULL LIMIT 1`, stringArray(cands)).Scan(&a.ID, &a.EngagementID, &a.Kind, &a.Handle, &a.Role, &a.AgentName, &a.Model, &a.Version, &a.AuthorizedBy, &a.TokenHash)
	return a, err
}

func writeProblem(w http.ResponseWriter, pb captureProblem) {
	if pb.Type == "" {
		pb.Type = "about:blank"
	}
	if pb.Title == "" {
		pb.Title = http.StatusText(pb.Status)
	}
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.Header().Set("X-Request-ID", pb.RequestID)
	w.WriteHeader(pb.Status)
	_ = json.NewEncoder(w).Encode(pb)
}

func writeJSONWithHeaders(w http.ResponseWriter, status int, v any, reqID string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Request-ID", reqID)
	w.Header().Set("Waypoint-Contract-Version", "1.0.0")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func isUUID(v string) bool        { return uuidPattern.MatchString(v) }
func isHexSHA256(v string) bool   { return hexSHA256Pattern.MatchString(v) }
func sha256Hex(v string) string   { sum := sha256.Sum256([]byte(v)); return hex.EncodeToString(sum[:]) }
func digestBytes(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func eventCursor(id int64) string { return fmt.Sprint(id) }
func badField(ptr, code, msg string) *captureProblem {
	return &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", Retryable: false, FieldErrors: []fieldError{{Pointer: ptr, Code: code, Message: msg}}}
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

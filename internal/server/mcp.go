package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
)

const mcpProtocolVersion = "2025-03-26"

const (
	mcpToolIngestCapture  = "waypoint_ingest_capture"
	mcpToolCaptureStatus  = "waypoint_capture_status"
	mcpJSONRPCParseError  = -32700
	mcpJSONRPCInvalidReq  = -32600
	mcpJSONRPCMethodNF    = -32601
	mcpJSONRPCInvalidArgs = -32602
	mcpJSONRPCInternalErr = -32603
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpInitializeParams struct {
	ProtocolVersion string            `json:"protocolVersion"`
	Capabilities    map[string]any    `json:"capabilities"`
	ClientInfo      mcpImplementation `json:"clientInfo"`
}

type mcpImplementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type mcpToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type mcpTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolCallResult struct {
	Content           []mcpTextContent `json:"content"`
	IsError           bool             `json:"isError"`
	StructuredContent any              `json:"structuredContent"`
}

type mcpToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpCaptureStatusArguments struct {
	CaptureID     string `json:"captureId"`
	SourceAgentID string `json:"sourceAgentId"`
}

type mcpCaptureStatusResult struct {
	ContractVersion  string    `json:"contractVersion"`
	Status           string    `json:"status"`
	ActionID         string    `json:"actionId"`
	CaptureID        string    `json:"captureId"`
	SourceAgentID    string    `json:"sourceAgentId"`
	AuditEventCursor string    `json:"auditEventCursor"`
	ReceivedAt       time.Time `json:"receivedAt"`
	Idempotency      string    `json:"idempotency"`
}

type mcpJSONRPCResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      json.RawMessage  `json:"id"`
	Result  any              `json:"result,omitempty"`
	Error   *mcpJSONRPCError `json:"error,omitempty"`
}

type mcpJSONRPCError struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

func mcpHandler(db *sql.DB, store *evidenceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		sessionID := r.Header.Get("MCP-Session-Id")
		accept := r.Header.Get("Accept")

		if db == nil {
			writeMCPJSONRPCError(w, http.StatusServiceUnavailable, nil, reqID, sessionID, mcpJSONRPCInternalErr, "mcp is unavailable", nil)
			return
		}
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeMCPJSONRPCError(w, http.StatusBadRequest, nil, reqID, sessionID, mcpJSONRPCInvalidReq, err.Error(), nil)
			return
		}
		if r.Header.Get("MCP-Protocol-Version") != mcpProtocolVersion {
			writeMCPJSONRPCError(w, http.StatusBadRequest, nil, reqID, sessionID, mcpJSONRPCInvalidReq, "expected MCP-Protocol-Version: 2025-03-26", nil)
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

		body, err := ioReadAllLimited(r.Body, 16<<20)
		if err != nil {
			writeMCPJSONRPCError(w, http.StatusBadRequest, nil, reqID, sessionID, mcpJSONRPCParseError, "request body is invalid JSON", nil)
			return
		}

		var req mcpRequest
		if err := decodeStrictJSON(body, &req); err != nil {
			writeMCPJSONRPCError(w, http.StatusBadRequest, nil, reqID, sessionID, mcpJSONRPCParseError, err.Error(), nil)
			return
		}
		if req.JSONRPC != "2.0" {
			writeMCPJSONRPCError(w, http.StatusBadRequest, req.ID, reqID, sessionID, mcpJSONRPCInvalidReq, "jsonrpc must be 2.0", nil)
			return
		}

		switch req.Method {
		case "initialize":
			if len(req.ID) == 0 {
				writeMCPJSONRPCError(w, http.StatusBadRequest, nil, reqID, sessionID, mcpJSONRPCInvalidReq, "initialize requires an id", nil)
				return
			}
			var params mcpInitializeParams
			if err := decodeStrictJSON(req.Params, &params); err != nil {
				writeMCPJSONRPCError(w, http.StatusBadRequest, req.ID, reqID, sessionID, mcpJSONRPCInvalidArgs, err.Error(), nil)
				return
			}
			if params.ProtocolVersion != mcpProtocolVersion {
				writeMCPJSONRPCError(w, http.StatusBadRequest, req.ID, reqID, sessionID, mcpJSONRPCInvalidArgs, "protocolVersion must be 2025-03-26", nil)
				return
			}
			if params.ClientInfo.Name == "" || params.ClientInfo.Version == "" {
				writeMCPJSONRPCError(w, http.StatusBadRequest, req.ID, reqID, sessionID, mcpJSONRPCInvalidArgs, "clientInfo is incomplete", nil)
				return
			}
			if sessionID == "" {
				sessionID = newUUID()
			}
			writeMCPResponse(w, http.StatusOK, req.ID, reqID, sessionID, accept, map[string]any{
				"protocolVersion": mcpProtocolVersion,
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    "Waypoint",
					"version": "1.0.0",
				},
			})
		case "notifications/initialized":
			w.Header().Set("X-Request-ID", reqID)
			w.Header().Set("Waypoint-Contract-Version", "1.0.0")
			if sessionID != "" {
				w.Header().Set("Mcp-Session-Id", sessionID)
			}
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			if len(req.ID) == 0 {
				writeMCPJSONRPCError(w, http.StatusBadRequest, nil, reqID, sessionID, mcpJSONRPCInvalidReq, "tools/list requires an id", nil)
				return
			}
			writeMCPResponse(w, http.StatusOK, req.ID, reqID, sessionID, accept, map[string]any{
				"tools": []mcpToolDefinition{
					{
						Name:        mcpToolIngestCapture,
						Description: "Durably ingest an attributed completed capture through the same service as REST.",
						InputSchema: map[string]any{"$ref": "https://schemas.waypoint.security/contracts/v1/mcp-message.schema.json#/$defs/IngestCaptureArguments"},
					},
					{
						Name:        mcpToolCaptureStatus,
						Description: "Look up durable status in the current actor/source scope.",
						InputSchema: map[string]any{"$ref": "https://schemas.waypoint.security/contracts/v1/mcp-message.schema.json#/$defs/CaptureStatusArguments"},
					},
				},
			})
		case "tools/call":
			if len(req.ID) == 0 {
				writeMCPJSONRPCError(w, http.StatusBadRequest, nil, reqID, sessionID, mcpJSONRPCInvalidReq, "tools/call requires an id", nil)
				return
			}
			var params mcpToolCallParams
			if err := decodeStrictJSON(req.Params, &params); err != nil {
				writeMCPJSONRPCError(w, http.StatusBadRequest, req.ID, reqID, sessionID, mcpJSONRPCInvalidArgs, err.Error(), nil)
				return
			}
			switch params.Name {
			case mcpToolIngestCapture:
				result, problem, err := invokeMCPIngestCapture(r.Context(), db, store, reqID, token, params.Arguments)
				if err != nil {
					writeMCPJSONRPCError(w, http.StatusInternalServerError, req.ID, reqID, sessionID, mcpJSONRPCInternalErr, err.Error(), nil)
					return
				}
				if problem != nil {
					writeMCPResponse(w, http.StatusOK, req.ID, reqID, sessionID, accept, result)
					return
				}
				writeMCPResponse(w, http.StatusOK, req.ID, reqID, sessionID, accept, result)
			case mcpToolCaptureStatus:
				result, problem, err := invokeMCPCaptureStatus(r.Context(), db, actor, reqID, params.Arguments)
				if err != nil {
					writeMCPJSONRPCError(w, http.StatusInternalServerError, req.ID, reqID, sessionID, mcpJSONRPCInternalErr, err.Error(), nil)
					return
				}
				if problem != nil {
					writeMCPResponse(w, http.StatusOK, req.ID, reqID, sessionID, accept, result)
					return
				}
				writeMCPResponse(w, http.StatusOK, req.ID, reqID, sessionID, accept, result)
			default:
				writeMCPJSONRPCError(w, http.StatusBadRequest, req.ID, reqID, sessionID, mcpJSONRPCMethodNF, "unknown tool", nil)
			}
		default:
			writeMCPJSONRPCError(w, http.StatusBadRequest, req.ID, reqID, sessionID, mcpJSONRPCMethodNF, "unknown method", nil)
		}
	}
}

func invokeMCPIngestCapture(ctx context.Context, db *sql.DB, store *evidenceStore, reqID, token string, args json.RawMessage) (mcpToolCallResult, *captureProblem, error) {
	var input struct {
		IdempotencyKey string          `json:"idempotencyKey"`
		Envelope       captureEnvelope `json:"envelope"`
		StdoutBase64   string          `json:"stdoutBase64"`
		StderrBase64   string          `json:"stderrBase64"`
	}
	if err := decodeStrictJSON(args, &input); err != nil {
		problem := captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()}
		return mcpToolErrorResult(problem), &problem, nil
	}
	stdout, problem, err := decodeMCPBase64Evidence("/stdoutBase64", input.StdoutBase64)
	if err != nil || problem != nil {
		if problem == nil {
			problem = &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()}
		}
		problem.RequestID = reqID
		return mcpToolErrorResult(*problem), problem, nil
	}
	stderr, problem, err := decodeMCPBase64Evidence("/stderrBase64", input.StderrBase64)
	if err != nil || problem != nil {
		if problem == nil {
			problem = &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()}
		}
		problem.RequestID = reqID
		return mcpToolErrorResult(*problem), problem, nil
	}
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	if err := writeMultipartJSON(mw, "envelope", input.Envelope); err != nil {
		return mcpToolCallResult{}, nil, err
	}
	if err := writeMultipartBlob(mw, "stdout", "stdout.txt", stdout); err != nil {
		return mcpToolCallResult{}, nil, err
	}
	if err := writeMultipartBlob(mw, "stderr", "stderr.txt", stderr); err != nil {
		return mcpToolCallResult{}, nil, err
	}
	if err := mw.Close(); err != nil {
		return mcpToolCallResult{}, nil, err
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/captures", bytes.NewReader(body.Bytes())).WithContext(ctx)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Waypoint-Contract-Version", "1.0.0")
	req.Header.Set("Idempotency-Key", input.IdempotencyKey)
	req.Header.Set("X-Request-ID", reqID)
	rr := httptest.NewRecorder()
	captureHandler(db, store, "mcp")(rr, req)

	switch rr.Code {
	case http.StatusCreated, http.StatusOK:
		var ack captureAck
		if err := json.NewDecoder(rr.Body).Decode(&ack); err != nil {
			return mcpToolCallResult{}, nil, err
		}
		return mcpToolCallResult{Content: []mcpTextContent{{Type: "text", Text: "Capture durably accepted."}}, IsError: false, StructuredContent: map[string]any{"ack": ack}}, nil, nil
	default:
		var problem captureProblem
		if err := json.NewDecoder(rr.Body).Decode(&problem); err != nil {
			return mcpToolCallResult{}, nil, fmt.Errorf("capture tool failed with %d", rr.Code)
		}
		return mcpToolErrorResult(problem), &problem, nil
	}
}

func invokeMCPCaptureStatus(ctx context.Context, db *sql.DB, actor actorRecord, reqID string, args json.RawMessage) (mcpToolCallResult, *captureProblem, error) {
	var input mcpCaptureStatusArguments
	if err := decodeStrictJSON(args, &input); err != nil {
		problem := captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()}
		return mcpToolErrorResult(problem), &problem, nil
	}
	if !isUUID(input.CaptureID) {
		problem := captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/captureId", Code: "invalid_uuid", Message: "captureId must be a UUID."}}}
		return mcpToolErrorResult(problem), &problem, nil
	}
	if !isUUID(input.SourceAgentID) {
		problem := captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, FieldErrors: []fieldError{{Pointer: "/sourceAgentId", Code: "invalid_uuid", Message: "sourceAgentId must be a UUID."}}}
		return mcpToolErrorResult(problem), &problem, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return mcpToolCallResult{}, nil, err
	}
	defer tx.Rollback()

	var actionID string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM action WHERE engagement_id = $1 AND actor_id = $2 AND source_agent_id = $3 AND capture_id = $4`, actor.EngagementID, actor.ID, input.SourceAgentID, input.CaptureID).Scan(&actionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			problem := captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusNotFound), Status: http.StatusNotFound, Code: "resource_not_found", RequestID: reqID, Retryable: false, Detail: "capture not found in this scope"}
			return mcpToolErrorResult(problem), &problem, nil
		}
		return mcpToolCallResult{}, nil, err
	}
	ack, err := replayAck(ctx, tx, actionID, input.CaptureID)
	if err != nil {
		return mcpToolCallResult{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return mcpToolCallResult{}, nil, err
	}
	return mcpToolCallResult{Content: []mcpTextContent{{Type: "text", Text: "Capture status retrieved."}}, IsError: false, StructuredContent: mcpCaptureStatusResult{ContractVersion: "1.0.0", Status: "captured", ActionID: ack.ActionID, CaptureID: ack.CaptureID, SourceAgentID: input.SourceAgentID, AuditEventCursor: ack.AuditEventCursor, ReceivedAt: ack.ReceivedAt, Idempotency: ack.Idempotency}}, nil, nil
}

func decodeMCPBase64Evidence(pointer, v string) ([]byte, *captureProblem, error) {
	data, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		problem := captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", Retryable: false, FieldErrors: []fieldError{{Pointer: pointer, Code: "invalid_base64", Message: "evidence must be base64 encoded."}}}
		return nil, &problem, nil
	}
	return data, nil, nil
}

func mcpToolErrorResult(problem captureProblem) mcpToolCallResult {
	text := problem.Detail
	if text == "" {
		text = problem.Title
	}
	return mcpToolCallResult{Content: []mcpTextContent{{Type: "text", Text: text}}, IsError: true, StructuredContent: map[string]any{"problem": problem}}
}

func writeMultipartJSON(mw *multipart.Writer, name string, v any) error {
	part, err := mw.CreateFormField(name)
	if err != nil {
		return err
	}
	return json.NewEncoder(part).Encode(v)
}

func writeMultipartBlob(mw *multipart.Writer, name, filename string, blob []byte) error {
	part, err := mw.CreateFormFile(name, filename)
	if err != nil {
		return err
	}
	_, err = part.Write(blob)
	return err
}

func writeMCPResponse(w http.ResponseWriter, status int, id json.RawMessage, reqID, sessionID, accept string, result any) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	w.Header().Set("X-Request-ID", reqID)
	w.Header().Set("Waypoint-Contract-Version", "1.0.0")
	if sessionID != "" {
		w.Header().Set("Mcp-Session-Id", sessionID)
	}
	if strings.Contains(accept, "text/event-stream") {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.WriteHeader(status)
		payload, _ := json.Marshal(mcpJSONRPCResponse{JSONRPC: "2.0", ID: id, Result: result})
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(payload)
		_, _ = w.Write([]byte("\n\n"))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(mcpJSONRPCResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func writeMCPJSONRPCError(w http.ResponseWriter, status int, id json.RawMessage, reqID, sessionID string, code int, message string, data map[string]any) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Request-ID", reqID)
	w.Header().Set("Waypoint-Contract-Version", "1.0.0")
	if sessionID != "" {
		w.Header().Set("Mcp-Session-Id", sessionID)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(mcpJSONRPCResponse{JSONRPC: "2.0", ID: id, Error: &mcpJSONRPCError{Code: code, Message: message, Data: data}})
}

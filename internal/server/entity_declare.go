package server

import (
	"context"
	"database/sql"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// entityDeclareRequest is the body of POST /api/v1/entities. Direct entity
// creation is deliberately limited to subnet declarations: every other entity
// kind must arrive through a capture so it stays anchored to evidence. A
// declared subnet records the operator's knowledge of a network boundary
// (e.g. from the rules of engagement) that no capture has demonstrated yet.
type entityDeclareRequest struct {
	Kind  string `json:"kind"`
	CIDR  string `json:"cidr"`
	Label string `json:"label,omitempty"`
}

func handleEntityDeclare(w http.ResponseWriter, r *http.Request, db *sql.DB, actor actorRecord, reqID string) {
	if !isLifecycleOperator(actor) {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusForbidden), Status: http.StatusForbidden, Code: "forbidden", RequestID: reqID, Retryable: false, Detail: "only an operator or owner can declare a subnet."})
		return
	}
	body, err := ioReadAllLimited(r.Body, 1<<16)
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "request body is invalid"})
		return
	}
	var req entityDeclareRequest
	if err := decodeStrictJSON(body, &req); err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()})
		return
	}
	if req.Kind != "subnet" {
		pb := badField("/kind", "invalid_enum", "kind must be subnet; other entities are created by captures.")
		pb.RequestID = reqID
		writeProblem(w, *pb)
		return
	}
	prefix, err := netip.ParsePrefix(strings.TrimSpace(req.CIDR))
	if err != nil {
		pb := badField("/cidr", "invalid_value", "cidr must be a valid prefix like 10.4.30.0/24.")
		pb.RequestID = reqID
		writeProblem(w, *pb)
		return
	}
	if !prefix.Addr().Is4() {
		pb := badField("/cidr", "invalid_value", "only IPv4 prefixes are supported.")
		pb.RequestID = reqID
		writeProblem(w, *pb)
		return
	}
	if prefix.Bits() < 8 {
		pb := badField("/cidr", "invalid_range", "prefix must be /8 or narrower.")
		pb.RequestID = reqID
		writeProblem(w, *pb)
		return
	}
	cidr := prefix.Masked().String()
	label := strings.TrimSpace(req.Label)
	if len(label) > 120 || strings.ContainsAny(label, controlChars) {
		pb := badField("/label", "invalid_value", "label must be printable text up to 120 characters.")
		pb.RequestID = reqID
		writeProblem(w, *pb)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "begin tx failed"})
		return
	}
	defer tx.Rollback()

	attrs := map[string]any{"cidr": cidr, "declared": true, "declaredBy": actor.Handle}
	if label != "" {
		attrs["label"] = label
	}
	entityID, err := upsertEntity(ctx, tx, actor.EngagementID, "subnet", "other", cidr, attrs)
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "declare subnet failed"})
		return
	}
	item, err := loadEntityReadResponseWithTx(ctx, tx, actor.EngagementID, entityID)
	if err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "reload entity failed"})
		return
	}
	if err := appendEntityAuditEvent(ctx, tx, actor, reqID, "entity.subnet-declared", entityID, item.Revision, map[string]any{"cidr": cidr, "label": label, "revision": item.Revision}); err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "audit declare failed"})
		return
	}
	if err := tx.Commit(); err != nil {
		writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "commit declare failed"})
		return
	}
	writeJSONWithHeaders(w, http.StatusCreated, item, reqID)
}

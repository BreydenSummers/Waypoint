package server

import (
	"encoding/json"
	"testing"
)

func obs(kind string, attrs map[string]any, sourceActionID string) entityObservationItem {
	raw, _ := json.Marshal(attrs)
	return entityObservationItem{Kind: kind, Attributes: raw, SourceActionID: sourceActionID}
}

func TestDeriveEntityAccess(t *testing.T) {
	tests := []struct {
		name       string
		obs        []entityObservationItem
		wantLevel  string
		wantOwns   bool
		wantCreds  int
		wantScope0 string
	}{
		{
			name:      "no observations reads as no access",
			obs:       nil,
			wantLevel: "none",
		},
		{
			name:       "SYSTEM access observation",
			obs:        []entityObservationItem{obs("access", map[string]any{"success": true, "user": "svc_backup", "access": "SYSTEM", "target": "fileshare-01"}, "a1")},
			wantLevel:  "system",
			wantCreds:  1,
			wantScope0: "local",
		},
		{
			name:       "Domain Admin credential confers domain ownership",
			obs:        []entityObservationItem{obs("credential", map[string]any{"success": true, "user": "svc_backup", "method": "kerberoast", "privilege": "Domain Admins"}, "a2")},
			wantLevel:  "system",
			wantOwns:   true,
			wantCreds:  1,
			wantScope0: "domain",
		},
		{
			name:       "dcsync credential reads as SYSTEM",
			obs:        []entityObservationItem{obs("credential", map[string]any{"success": true, "user": "krbtgt", "method": "dcsync", "impact": "golden-ticket-capable"}, "a3")},
			wantLevel:  "system",
			wantOwns:   true,
			wantCreds:  1,
			wantScope0: "domain",
		},
		{
			name:       "plain credential reads as user",
			obs:        []entityObservationItem{obs("credential", map[string]any{"success": true, "user": "jdoe", "method": "kerberos-preauth"}, "a4")},
			wantLevel:  "user",
			wantCreds:  1,
			wantScope0: "user",
		},
		{
			name:      "failed observation is ignored",
			obs:       []entityObservationItem{obs("credential", map[string]any{"success": false, "user": "jdoe", "method": "spray"}, "a5")},
			wantLevel: "none",
			wantCreds: 0,
		},
		{
			name: "highest access wins across observations",
			obs: []entityObservationItem{
				obs("credential", map[string]any{"success": true, "user": "jdoe", "method": "spray"}, "a6"),
				obs("access", map[string]any{"success": true, "user": "admin", "access": "SYSTEM"}, "a7"),
			},
			wantLevel: "system",
			wantCreds: 2,
		},
		{
			name: "non access/credential observations are ignored",
			obs: []entityObservationItem{
				obs("service", map[string]any{"os": "Windows Server 2019", "services": []any{"smb"}}, "a8"),
				obs("host", map[string]any{"ip": "10.4.10.10"}, "a9"),
			},
			wantLevel: "none",
			wantCreds: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			access, creds := deriveEntityAccess(tt.obs)
			if access.Level != tt.wantLevel {
				t.Errorf("level = %q, want %q", access.Level, tt.wantLevel)
			}
			if access.OwnsDomain != tt.wantOwns {
				t.Errorf("ownsDomain = %v, want %v", access.OwnsDomain, tt.wantOwns)
			}
			if len(creds) != tt.wantCreds {
				t.Fatalf("creds = %d, want %d", len(creds), tt.wantCreds)
			}
			if tt.wantScope0 != "" && creds[0].Scope != tt.wantScope0 {
				t.Errorf("creds[0].scope = %q, want %q", creds[0].Scope, tt.wantScope0)
			}
			if tt.wantCreds > 0 && creds[0].SourceActionID == "" {
				t.Errorf("expected credential to carry its source action id")
			}
		})
	}
}

func TestDeriveEntityAccessDedupes(t *testing.T) {
	same := []entityObservationItem{
		obs("credential", map[string]any{"success": true, "user": "svc_backup", "method": "kerberoast", "privilege": "Domain Admins"}, "a1"),
		obs("credential", map[string]any{"success": true, "user": "svc_backup", "method": "kerberoast", "privilege": "Domain Admins"}, "a2"),
	}
	_, creds := deriveEntityAccess(same)
	if len(creds) != 1 {
		t.Fatalf("expected duplicate credentials to collapse, got %d", len(creds))
	}
}

func TestParseEntityKindFilter(t *testing.T) {
	if k, pb := parseEntityKindFilter("  host "); pb != nil || k != "host" {
		t.Fatalf("host filter: k=%q pb=%v", k, pb)
	}
	if k, pb := parseEntityKindFilter(""); pb != nil || k != "" {
		t.Fatalf("empty filter should pass through: k=%q pb=%v", k, pb)
	}
	if _, pb := parseEntityKindFilter("bad\x00kind"); pb == nil {
		t.Fatalf("control characters should be rejected")
	}
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'a'
	}
	if _, pb := parseEntityKindFilter(string(long)); pb == nil {
		t.Fatalf("over-length kind should be rejected")
	}
}

package server

import "testing"

func TestResolveSSECursor(t *testing.T) {
	tests := []struct {
		name        string
		after       string
		lastEventID string
		want        string
		wantProblem string
	}{
		{name: "empty", want: ""},
		{name: "after", after: "41", want: "41"},
		{name: "last-event-id", lastEventID: "41", want: "41"},
		{name: "matching pair", after: "41", lastEventID: "41", want: "41"},
		{name: "mismatch", after: "41", lastEventID: "42", wantProblem: "cursor_mismatch"},
		{name: "invalid after", after: "01", wantProblem: "cursor_invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, pb := resolveSSECursor(tt.after, tt.lastEventID)
			if tt.wantProblem != "" {
				if pb == nil || pb.Code != tt.wantProblem {
					t.Fatalf("problem = %#v, want code %q", pb, tt.wantProblem)
				}
				return
			}
			if pb != nil {
				t.Fatalf("unexpected problem: %#v", pb)
			}
			if tt.want == "" {
				if got != nil {
					t.Fatalf("cursor = %v, want nil", *got)
				}
				return
			}
			if got == nil || *got != 41 {
				t.Fatalf("cursor = %v, want 41", got)
			}
		})
	}
}

func TestParseAuditCursorCanonical(t *testing.T) {
	if got, pb := parseAuditCursor("41"); pb != nil || got != 41 {
		t.Fatalf("parseAuditCursor(41) = %d, %#v", got, pb)
	}
	if _, pb := parseAuditCursor("01"); pb == nil || pb.Code != "cursor_invalid" {
		t.Fatalf("parseAuditCursor(01) problem = %#v", pb)
	}
}

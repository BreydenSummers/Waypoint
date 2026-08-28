package server

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const exportDumpBinaryVersion = 1
const exportDumpMagic = "PGDMPWP1"

type exportDatabaseDump struct {
	FormatVersion    string                   `json:"formatVersion"`
	DumpFormat       string                   `json:"dumpFormat"`
	EngagementID     string                   `json:"engagementId"`
	SnapshotID       string                   `json:"snapshotId"`
	Cutoff           string                   `json:"cutoff"`
	Engagement       json.RawMessage          `json:"engagement"`
	Actors           json.RawMessage          `json:"actors"`
	Actions          json.RawMessage          `json:"actions"`
	AuditEvents      json.RawMessage          `json:"auditEvents"`
	Entities         json.RawMessage          `json:"entities"`
	Results          json.RawMessage          `json:"results"`
	Observations     json.RawMessage          `json:"observations"`
	Evidence         json.RawMessage          `json:"evidence"`
	Claims           json.RawMessage          `json:"claims"`
	Findings         json.RawMessage          `json:"findings"`
	FindingRevisions json.RawMessage          `json:"findingRevisions"`
	Exports          json.RawMessage          `json:"exports"`
	Receipts         json.RawMessage          `json:"receipts"`
	Grants           json.RawMessage          `json:"grants"`
	RowCounts        exportDatabaseDumpCounts `json:"rowCounts"`
}

type exportDatabaseDumpCounts struct {
	Engagement       int `json:"engagement"`
	Actors           int `json:"actors"`
	Actions          int `json:"actions"`
	AuditEvents      int `json:"auditEvents"`
	Entities         int `json:"entities"`
	Results          int `json:"results"`
	Observations     int `json:"observations"`
	Evidence         int `json:"evidence"`
	Claims           int `json:"claims"`
	Findings         int `json:"findings"`
	FindingRevisions int `json:"findingRevisions"`
	Exports          int `json:"exports"`
	Receipts         int `json:"receipts"`
	Grants           int `json:"grants"`
}

func buildExportDump(ctx context.Context, db queryer, engagementID, snapshotID, cutoff, stagingDir string) ([]byte, error) {
	dump, err := assembleExportDump(ctx, db, engagementID, snapshotID, cutoff)
	if err != nil {
		return nil, err
	}
	if stagingDir != "" {
		if err := os.RemoveAll(stagingDir); err != nil {
			return nil, err
		}
		if err := materializeExportStagingSnapshot(stagingDir, dump); err != nil {
			_ = os.RemoveAll(stagingDir)
			return nil, err
		}
		defer os.RemoveAll(stagingDir)
	}
	return encodeExportDumpBytes(dump)
}

func assembleExportDump(ctx context.Context, db queryer, engagementID, snapshotID, cutoff string) (exportDatabaseDump, error) {
	engagement, err := loadDumpObject(ctx, db, `SELECT row_to_json(t) FROM (SELECT * FROM engagement WHERE id = $1) t`, engagementID)
	if err != nil {
		return exportDatabaseDump{}, err
	}
	actors, err := loadDumpArray(ctx, db, `SELECT COALESCE(json_agg(t ORDER BY t.created_at ASC, t.id ASC), '[]'::json) FROM (SELECT * FROM actor WHERE engagement_id = $1) t`, engagementID)
	if err != nil {
		return exportDatabaseDump{}, err
	}
	actions, err := loadDumpArray(ctx, db, `SELECT COALESCE(json_agg(t ORDER BY t.started_at ASC, t.id ASC), '[]'::json) FROM (SELECT * FROM action WHERE engagement_id = $1) t`, engagementID)
	if err != nil {
		return exportDatabaseDump{}, err
	}
	auditEvents, err := loadDumpArray(ctx, db, `SELECT COALESCE(json_agg(t ORDER BY t.occurred_at ASC, t.id ASC), '[]'::json) FROM (SELECT * FROM audit_event WHERE engagement_id = $1) t`, engagementID)
	if err != nil {
		return exportDatabaseDump{}, err
	}
	entities, err := loadDumpArray(ctx, db, `SELECT COALESCE(json_agg(t ORDER BY t.created_at ASC, t.id ASC), '[]'::json) FROM (SELECT * FROM entity WHERE engagement_id = $1) t`, engagementID)
	if err != nil {
		return exportDatabaseDump{}, err
	}
	results, err := loadDumpArray(ctx, db, `SELECT COALESCE(json_agg(t ORDER BY t.created_at ASC, t.id ASC), '[]'::json) FROM (SELECT * FROM result WHERE engagement_id = $1) t`, engagementID)
	if err != nil {
		return exportDatabaseDump{}, err
	}
	observations, err := loadDumpArray(ctx, db, `SELECT COALESCE(json_agg(t ORDER BY t.created_at ASC, t.id ASC), '[]'::json) FROM (SELECT * FROM observation WHERE engagement_id = $1) t`, engagementID)
	if err != nil {
		return exportDatabaseDump{}, err
	}
	evidence, err := loadDumpArray(ctx, db, `SELECT COALESCE(json_agg(t ORDER BY t.created_at ASC, t.id ASC), '[]'::json) FROM (SELECT * FROM evidence WHERE engagement_id = $1) t`, engagementID)
	if err != nil {
		return exportDatabaseDump{}, err
	}
	claims, err := loadExportClaims(ctx, db, engagementID)
	if err != nil {
		return exportDatabaseDump{}, err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return exportDatabaseDump{}, err
	}
	findings, err := loadDumpArray(ctx, db, `SELECT COALESCE(json_agg(t ORDER BY t.updated_at ASC, t.id ASC), '[]'::json) FROM (SELECT * FROM finding WHERE engagement_id = $1) t`, engagementID)
	if err != nil {
		return exportDatabaseDump{}, err
	}
	findingRevisions, err := loadDumpArray(ctx, db, `SELECT COALESCE(json_agg(t ORDER BY t.occurred_at ASC, t.id ASC), '[]'::json) FROM (SELECT * FROM audit_event WHERE engagement_id = $1 AND subject_type = 'finding') t`, engagementID)
	if err != nil {
		return exportDatabaseDump{}, err
	}
	exports, err := loadDumpArray(ctx, db, `SELECT COALESCE(json_agg(t ORDER BY t.created_at ASC, t.id ASC), '[]'::json) FROM (SELECT * FROM export_job WHERE engagement_id = $1) t`, engagementID)
	if err != nil {
		return exportDatabaseDump{}, err
	}
	receipts, err := loadDumpArray(ctx, db, `SELECT COALESCE(json_agg(t ORDER BY t.verified_at ASC, t.id ASC), '[]'::json) FROM (SELECT * FROM export_receipt WHERE engagement_id = $1) t`, engagementID)
	if err != nil {
		return exportDatabaseDump{}, err
	}
	grants, err := loadDumpArray(ctx, db, `SELECT COALESCE(json_agg(t ORDER BY t.requested_at ASC, t.id ASC), '[]'::json) FROM (SELECT * FROM teardown_authorization WHERE engagement_id = $1) t`, engagementID)
	if err != nil {
		return exportDatabaseDump{}, err
	}
	return exportDatabaseDump{
		FormatVersion:    exportContractVersion,
		DumpFormat:       "postgresql-custom-reconstruction",
		EngagementID:     engagementID,
		SnapshotID:       snapshotID,
		Cutoff:           cutoff,
		Engagement:       engagement,
		Actors:           actors,
		Actions:          actions,
		AuditEvents:      auditEvents,
		Entities:         entities,
		Results:          results,
		Observations:     observations,
		Evidence:         evidence,
		Claims:           claimsJSON,
		Findings:         findings,
		FindingRevisions: findingRevisions,
		Exports:          exports,
		Receipts:         receipts,
		Grants:           grants,
		RowCounts: exportDatabaseDumpCounts{
			Engagement:       jsonObjectPresent(engagement),
			Actors:           jsonArrayLength(actors),
			Actions:          jsonArrayLength(actions),
			AuditEvents:      jsonArrayLength(auditEvents),
			Entities:         jsonArrayLength(entities),
			Results:          jsonArrayLength(results),
			Observations:     jsonArrayLength(observations),
			Evidence:         jsonArrayLength(evidence),
			Claims:           len(claims),
			Findings:         jsonArrayLength(findings),
			FindingRevisions: jsonArrayLength(findingRevisions),
			Exports:          jsonArrayLength(exports),
			Receipts:         jsonArrayLength(receipts),
			Grants:           jsonArrayLength(grants),
		},
	}, nil
}

func materializeExportStagingSnapshot(stagingDir string, dump exportDatabaseDump) error {
	if stagingDir == "" {
		return nil
	}
	if err := os.MkdirAll(stagingDir, 0o750); err != nil {
		return err
	}
	sections := map[string]json.RawMessage{
		"engagement.json":        dump.Engagement,
		"actors.json":            dump.Actors,
		"actions.json":           dump.Actions,
		"audit_events.json":      dump.AuditEvents,
		"entities.json":          dump.Entities,
		"results.json":           dump.Results,
		"observations.json":      dump.Observations,
		"evidence.json":          dump.Evidence,
		"claims.json":            dump.Claims,
		"findings.json":          dump.Findings,
		"finding_revisions.json": dump.FindingRevisions,
		"exports.json":           dump.Exports,
		"receipts.json":          dump.Receipts,
		"grants.json":            dump.Grants,
	}
	for name, raw := range sections {
		bytes := raw
		if len(bytes) == 0 {
			bytes = []byte("null")
		}
		if err := os.WriteFile(filepath.Join(stagingDir, name), append(append([]byte{}, bytes...), '\n'), 0o600); err != nil {
			return err
		}
	}
	countsBytes, err := json.MarshalIndent(dump.RowCounts, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "row-counts.json"), append(countsBytes, '\n'), 0o600); err != nil {
		return err
	}
	return nil
}

func encodeExportDumpBytes(dump exportDatabaseDump) ([]byte, error) {
	payload, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		return nil, err
	}
	payload = append(payload, '\n')
	buf := make([]byte, 0, len(exportDumpMagic)+8+len(payload))
	buf = append(buf, exportDumpMagic...)
	var header [8]byte
	binary.BigEndian.PutUint32(header[:4], exportDumpBinaryVersion)
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	buf = append(buf, header[:]...)
	buf = append(buf, payload...)
	return buf, nil
}

func decodeExportDumpBytes(bytes []byte) (exportDatabaseDump, error) {
	payload := bytes
	if len(bytes) >= len(exportDumpMagic)+8 && string(bytes[:len(exportDumpMagic)]) == exportDumpMagic {
		version := binary.BigEndian.Uint32(bytes[len(exportDumpMagic) : len(exportDumpMagic)+4])
		if version != exportDumpBinaryVersion {
			return exportDatabaseDump{}, fmt.Errorf("unsupported database dump binary version: %d", version)
		}
		payloadLen := binary.BigEndian.Uint32(bytes[len(exportDumpMagic)+4 : len(exportDumpMagic)+8])
		payload = bytes[len(exportDumpMagic)+8:]
		if int(payloadLen) != len(payload) {
			return exportDatabaseDump{}, fmt.Errorf("database dump payload length mismatch: %d != %d", payloadLen, len(payload))
		}
	}
	var dump exportDatabaseDump
	if err := json.Unmarshal(payload, &dump); err != nil {
		return exportDatabaseDump{}, err
	}
	return dump, nil
}

func loadDumpObject(ctx context.Context, db queryer, query string, args ...any) (json.RawMessage, error) {
	var raw []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	return json.RawMessage(raw), nil
}

func loadDumpArray(ctx context.Context, db queryer, query string, args ...any) (json.RawMessage, error) {
	var raw []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		raw = []byte("[]")
	}
	return json.RawMessage(raw), nil
}

func loadExportClaims(ctx context.Context, db queryer, engagementID string) ([]outOfBandClaimItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT ON (subject_id) subject_id
		FROM audit_event
		WHERE engagement_id = $1 AND subject_type = 'out_of_band_claim' AND type IN ('out-of-band.flagged', 'out-of-band.resolved')
		ORDER BY subject_id, subject_revision DESC, id DESC`, engagementID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var claimIDs []string
	for rows.Next() {
		var claimID string
		if err := rows.Scan(&claimID); err != nil {
			return nil, err
		}
		claimIDs = append(claimIDs, claimID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	claims := make([]outOfBandClaimItem, 0, len(claimIDs))
	for _, claimID := range claimIDs {
		timeline, err := loadOutOfBandClaimTimeline(ctx, db, engagementID, claimID)
		if err != nil {
			return nil, err
		}
		item, err := buildOutOfBandClaim(engagementID, timeline)
		if err != nil {
			return nil, err
		}
		claims = append(claims, item)
	}
	sort.SliceStable(claims, func(i, j int) bool {
		if claims[i].ObservedAt.Equal(claims[j].ObservedAt) {
			return strings.Compare(claims[i].ID, claims[j].ID) < 0
		}
		return claims[i].ObservedAt.Before(claims[j].ObservedAt)
	})
	return claims, nil
}

func jsonObjectPresent(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	return 1
}

func jsonArrayLength(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return 0
	}
	return len(items)
}

func (c exportDatabaseDumpCounts) Total() int {
	return c.Engagement + c.Actors + c.Actions + c.AuditEvents + c.Entities + c.Results + c.Observations + c.Evidence + c.Claims + c.Findings + c.FindingRevisions + c.Exports + c.Receipts + c.Grants
}

func (d exportDatabaseDump) Summary() string {
	return fmt.Sprintf("dump %s @ %s (%d tables)", d.SnapshotID, time.Now().UTC().Format(time.RFC3339), d.RowCounts.Total())
}

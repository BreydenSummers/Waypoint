package releasegate

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
)

type Mode string

const (
	ModeUnit    Mode = "unit"
	ModeRelease Mode = "release"
)

type Outcome string

const (
	OutcomePass  Outcome = "pass"
	OutcomeFail  Outcome = "fail"
	OutcomeSkip  Outcome = "skip"
	OutcomeOther Outcome = "other"
)

type TestResult struct {
	Name    string
	Package string
	Outcome Outcome
	Output  []string
}

type RunSummary struct {
	Mode                 Mode
	Results              map[string]TestResult
	PackageFailures      []string
	Passed               []string
	Failed               []string
	Skipped              []string
	ReleaseCriticalSkips []string
}

type PlatformInfo struct {
	Mode          Mode
	RepoRoot      string
	OutputDir     string
	ComposeFile   string
	BrowserBinary string
	PluginsRoot   string
	TraceTags     []string
	ToolVersions  map[string]string
}

var criticalTests = map[string]struct{}{
	"TestComposeStackPersistsDBAndEvidenceAcrossRestart":                {},
	"TestOpenConfiguredDatabaseAppliesMigrations":                       {},
	"TestAppendAuditEventCapturesOutOfBandReviewLifecycle":              {},
	"TestAppendAuditEventRedactsSensitiveMetadata":                      {},
	"TestAppendAuditEventCommitsConcurrentlyAndRollsBackCleanly":        {},
	"TestAuditEventViewAndTableRemainAppendOnly":                        {},
	"TestApplyMigrationsOnRealPostgreSQL":                               {},
	"TestApplyMigrationsSerializesConcurrentStarters":                   {},
	"TestDatabaseProtectionsRejectMutations":                            {},
	"TestActorAuthorizationConstraint":                                  {},
	"TestActionAttributionSchemaRoundTrip":                              {},
	"TestActorLifecycleProvisionRotateRevokeAndAuthorization":           {},
	"TestCaptureRoundTripGateG2Transcript":                              {},
	"TestLiveMultiActorRESTCaptureJourneys":                             {},
	"TestCaptureIngestCreatesReplaysAndRejectsChangedPayload":           {},
	"TestCaptureRejectsUnsupportedContractVersion":                      {},
	"TestCapturePersistsEvidenceAndRecoversOrphans":                     {},
	"TestCaptureEvidenceDeduplicatesAndSurvivesRestart":                 {},
	"TestCaptureAcceptsAIInitiationWithDecisionContext":                 {},
	"TestCapturePersistsStructuredResultsAndRollsBackInvalidOutput":     {},
	"TestCaptureRejectsConflictingStableEntityKinds":                    {},
	"TestEvidenceMetadataAndContentReadsStayEngagementScoped":           {},
	"TestAuditHistoryPaginationSSEReconnectFilteringAndRevocation":      {},
	"TestAuditEventsCursorExpiredReturnsResyncLink":                     {},
	"TestTailAuditEventsStopsWhenQueueIsFull":                           {},
	"TestAuditSSEHeartbeatAndCommittedCaptureVisibility":                {},
	"TestReconReadApisAreKeysetPaginatedAndEngagementIsolated":          {},
	"TestReconPreviewAndSplitProvenanceReadsFollowCanonicalLineage":     {},
	"TestEntityMergeSplitPreviewUndoAndProvenance":                      {},
	"TestEntityIdentityNormalizationConflictAndConcurrentDeduplication": {},
	"TestEntityReadProvenanceTracksCanonicalLineageOnRealPostgreSQL":    {},
	"TestEntityMergeConflictIsOptimisticUnderConcurrency":               {},
	"TestFindingPromotionRevisionsAndOperatorOnlyPromotion":             {},
	"TestG3AuthoritativeProvenanceJourney":                              {},
	"TestNotableAlertsAreDeduplicatedAndStreamed":                       {},
	"TestNotableAlertsUseSystemActorForAISourceCaptures":                {},
	"TestMCPStandardFlowReusesCaptureService":                           {},
	"TestOutOfBandClaimLifecycleThroughPostgreSQL":                      {},
	"TestReportHandlerRequiresAuthAndScopesEngagement":                  {},
	"TestExportJobLifecyclePersistsReceiptAndBlocksBrowserAuthorship":   {},
	"TestExportJobPreflightRejectsInsufficientCapacity":                 {},
	"TestExportTeardownAuthorizationRoundTrip":                          {},
	"TestBuildExportEvidenceTarStreamsAttachmentRoles":                  {},
	"TestExportJobListIsPagedAndResumable":                              {},
	"TestUpsertEvidenceRejectsImmutableMetadataChanges":                 {},
	"TestCaptureIngestRejectsDiskPressureBeforeCommitting":              {},
	"TestCaptureIngestRetriesAfterInterruptedUploadWithoutDuplication":  {},
	"TestFindingsReportWorkflowAuthoritativeRealPostgresJourney":        {},
}

func IsReleaseCritical(name string) bool {
	_, ok := criticalTests[name]
	return ok
}

func ParseGoTestJSON(input []byte, mode Mode) (RunSummary, error) {
	summary := RunSummary{Mode: mode, Results: map[string]TestResult{}}
	for _, rawLine := range bytes.Split(input, []byte("\n")) {
		line := bytes.TrimSpace(rawLine)
		if len(line) == 0 {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		action, _ := event["Action"].(string)
		testName, _ := event["Test"].(string)
		pkg, _ := event["Package"].(string)
		output, _ := event["Output"].(string)
		if testName != "" {
			result := summary.Results[testName]
			result.Name = testName
			if result.Package == "" {
				result.Package = pkg
			}
			if output != "" {
				result.Output = append(result.Output, strings.TrimSpace(output))
			}
			switch action {
			case "pass":
				result.Outcome = OutcomePass
			case "fail":
				result.Outcome = OutcomeFail
			case "skip":
				result.Outcome = OutcomeSkip
			}
			summary.Results[testName] = result
			continue
		}
		if action == "fail" && strings.HasPrefix(strings.TrimSpace(output), "FAIL\t") {
			summary.PackageFailures = append(summary.PackageFailures, strings.TrimSpace(output))
		}
	}
	populateSummary(&summary)
	return summary, nil
}

func populateSummary(summary *RunSummary) {
	for name, result := range summary.Results {
		switch result.Outcome {
		case OutcomePass:
			summary.Passed = append(summary.Passed, name)
		case OutcomeFail:
			summary.Failed = append(summary.Failed, name)
		case OutcomeSkip:
			summary.Skipped = append(summary.Skipped, name)
			if IsReleaseCritical(name) {
				summary.ReleaseCriticalSkips = append(summary.ReleaseCriticalSkips, name)
			}
		default:
			result.Outcome = OutcomeOther
			summary.Results[name] = result
		}
	}
	sort.Strings(summary.Passed)
	sort.Strings(summary.Failed)
	sort.Strings(summary.Skipped)
	sort.Strings(summary.ReleaseCriticalSkips)
	sort.Strings(summary.PackageFailures)
}

func DetectFlakes(first, second RunSummary) []string {
	seen := map[string]struct{}{}
	for name := range first.Results {
		seen[name] = struct{}{}
	}
	for name := range second.Results {
		seen[name] = struct{}{}
	}
	flakes := make([]string, 0)
	for name := range seen {
		left, lok := first.Results[name]
		right, rok := second.Results[name]
		leftOutcome := OutcomeOther
		if lok {
			leftOutcome = left.Outcome
		}
		rightOutcome := OutcomeOther
		if rok {
			rightOutcome = right.Outcome
		}
		if leftOutcome != rightOutcome {
			flakes = append(flakes, fmt.Sprintf("%s: %s -> %s", name, leftOutcome, rightOutcome))
		}
	}
	sort.Strings(flakes)
	return flakes
}

func RenderJUnit(summary RunSummary, suiteName string, flakes []string) ([]byte, error) {
	type failure struct {
		Message string `xml:"message,attr"`
		Text    string `xml:",chardata"`
	}
	type skipped struct {
		Message string `xml:"message,attr,omitempty"`
		Text    string `xml:",chardata"`
	}
	type testcase struct {
		XMLName xml.Name `xml:"testcase"`
		Name    string   `xml:"name,attr"`
		Class   string   `xml:"classname,attr,omitempty"`
		Failure *failure `xml:"failure,omitempty"`
		Skipped *skipped `xml:"skipped,omitempty"`
	}
	type testsuite struct {
		XMLName  xml.Name   `xml:"testsuite"`
		Name     string     `xml:"name,attr"`
		Tests    int        `xml:"tests,attr"`
		Failures int        `xml:"failures,attr"`
		Skipped  int        `xml:"skipped,attr"`
		Cases    []testcase `xml:"testcase"`
	}
	cases := make([]testcase, 0, len(summary.Results)+len(flakes))
	failures := 0
	skippedCount := 0
	for _, name := range sortedKeys(summary.Results) {
		result := summary.Results[name]
		entry := testcase{Name: name, Class: result.Package}
		switch result.Outcome {
		case OutcomeFail:
			failures++
			entry.Failure = &failure{Message: "test failed", Text: strings.Join(result.Output, "\n")}
		case OutcomeSkip:
			skippedCount++
			entry.Skipped = &skipped{Message: "skipped", Text: strings.Join(result.Output, "\n")}
		}
		cases = append(cases, entry)
	}
	for _, flake := range flakes {
		failures++
		cases = append(cases, testcase{Name: "flake-detection", Failure: &failure{Message: "flake", Text: flake}})
	}
	payload := testsuite{
		Name:     suiteName,
		Tests:    len(cases),
		Failures: failures,
		Skipped:  skippedCount,
		Cases:    cases,
	}
	return xml.MarshalIndent(struct {
		XMLName xml.Name  `xml:"testsuites"`
		Suite   testsuite `xml:"testsuite"`
	}{Suite: payload}, "", "  ")
}

func sortedKeys(results map[string]TestResult) []string {
	keys := make([]string, 0, len(results))
	for name := range results {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	return keys
}

func BuildPlatformArtifact(info PlatformInfo) string {
	lines := []string{
		"mode=" + string(info.Mode),
		"trace=" + strings.Join(info.TraceTags, ","),
		"repo_root=" + info.RepoRoot,
		"out_dir=" + info.OutputDir,
		"compose_file=" + info.ComposeFile,
		"browser_binary=" + info.BrowserBinary,
		"plugins_root=" + info.PluginsRoot,
	}
	for _, key := range sortedStringKeys(info.ToolVersions) {
		lines = append(lines, fmt.Sprintf("%s=%s", key, info.ToolVersions[key]))
	}
	return strings.Join(lines, "\n") + "\n"
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

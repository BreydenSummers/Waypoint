# current-v1-runtime-gates

Run: `2026-08-24`

## Verdict

**Blocked / Unverified** for the requested runtime gates in this sandbox.

## Commands preserved

Artifacts are retained under `docs/release-evidence/current-v1-runtime-gates-artifacts/`.

### Current runtime / deployment checks

- `docker info`
  - `docs/release-evidence/current-v1-runtime-gates-artifacts/docker-info.txt`
- `docker compose -f compose.yml config --quiet`
  - `docs/release-evidence/current-v1-runtime-gates-artifacts/compose-config.txt`
- `go test -count=1 -v -run '^TestComposeStackPersistsDBAndEvidenceAcrossRestart$' .`
  - `docs/release-evidence/current-v1-runtime-gates-artifacts/go-test-compose-stack.txt`
- `make smoke`
  - `docs/release-evidence/current-v1-runtime-gates-artifacts/make-smoke.txt`

### Performance / fault gates exercised

- `go test -count=1 -v ./internal/server -run 'TestPerformanceProfileFixtureSeedsBaselineAndFaultScenarios|TestAuditQueryShapeRemainsKeysetBounded|TestHandlersReturnServiceUnavailableWithoutDatabase|TestReadCaptureRequestRejectsInterruptedMultipartUpload|TestCaptureEnvelopeLimitMatchesSchemaCeiling|TestReadCaptureRequestRejectsOversizedEnvelope|TestCopyEvidenceStreamRejectsOverLimit|TestExportStreamingHelpersStayFileBounded|TestCopyEvidenceStreamKeeps10GiBLimitPathWithinRSSBudget|TestUpsertEvidenceRejectsImmutableMetadataChanges|TestCaptureIngestRejectsDiskPressureBeforeCommitting|TestCaptureIngestRetriesAfterInterruptedUploadWithoutDuplication$'`
  - `docs/release-evidence/current-v1-runtime-gates-artifacts/go-test-performance-and-export.txt`
- `go test -count=1 -v ./internal/server -run 'TestExportJobLifecyclePersistsReceiptAndBlocksBrowserAuthorship|TestBuildExportEvidenceTarIsDeterministicAcrossMtimeDrift|TestExportJobPreflightRejectsInsufficientCapacity|TestExportTeardownAuthorizationRoundTrip|TestBuildExportEvidenceTarStreamsAttachmentRoles|TestExportJobListIsPagedAndResumable'`
  - `docs/release-evidence/current-v1-runtime-gates-artifacts/go-test-export.txt`
- `node --test web/scripts/bundle-tools.test.mjs web/scripts/bundle-export.test.mjs`
  - `docs/release-evidence/current-v1-runtime-gates-artifacts/node-bundle-export.txt`
- `go test -count=1 -json ./...`
  - `docs/release-evidence/current-v1-runtime-gates-artifacts/go-test-all.json`
  - summary: `docs/release-evidence/current-v1-runtime-gates-artifacts/go-test-all-summary.txt`

## Results

- Docker daemon is unavailable:
  - `Cannot connect to the Docker daemon at unix:///var/run/docker.sock`
- Compose syntax renders successfully.
- `make smoke` fails because the app exits without `WAYPOINT_DB_DSN`.
- `go test -count=1 -json ./...` reports **48 skipped release-critical tests** and no executed failures.
- Real-PostgreSQL export/performance tests remain skipped without `WAYPOINT_TEST_PG_DSN`.
- Synthetic bundle export/restore tests pass, but they do not replace a live DB / clean-room / post-wipe run.

## Exact skip set from `go test -count=1 -json ./...`

48 tests skipped; see `go-test-all.json` for the full JSON stream. The skipped release-critical set includes:

- Compose/startup: `TestComposeStackPersistsDBAndEvidenceAcrossRestart`, `TestOpenConfiguredDatabaseAppliesMigrations`
- Migrations/audit: `TestAppendAuditEventCapturesOutOfBandReviewLifecycle`, `TestAppendAuditEventRedactsSensitiveMetadata`, `TestAppendAuditEventCommitsConcurrentlyAndRollsBackCleanly`, `TestAuditEventViewAndTableRemainAppendOnly`, `TestApplyMigrationsOnRealPostgreSQL`, `TestApplyMigrationsSerializesConcurrentStarters`, `TestDatabaseProtectionsRejectMutations`, `TestActorAuthorizationConstraint`, `TestActionAttributionSchemaRoundTrip`
- Actors/capture/evidence: `TestActorLifecycleProvisionRotateRevokeAndAuthorization`, `TestCaptureRoundTripGateG2Transcript`, `TestLiveMultiActorRESTCaptureJourneys`, `TestCaptureIngestCreatesReplaysAndRejectsChangedPayload`, `TestCaptureRejectsUnsupportedContractVersion`, `TestCapturePersistsEvidenceAndRecoversOrphans`, `TestCaptureEvidenceDeduplicatesAndSurvivesRestart`, `TestCaptureAcceptsAIInitiationWithDecisionContext`, `TestCapturePersistsStructuredResultsAndRollsBackInvalidOutput`, `TestCaptureRejectsConflictingStableEntityKinds`, `TestEvidenceMetadataAndContentReadsStayEngagementScoped`
- SSE/workspaces/entities/findings/alerts: `TestAuditHistoryPaginationSSEReconnectFilteringAndRevocation`, `TestAuditEventsCursorExpiredReturnsResyncLink`, `TestTailAuditEventsStopsWhenQueueIsFull`, `TestAuditSSEHeartbeatAndCommittedCaptureVisibility`, `TestReconReadApisAreKeysetPaginatedAndEngagementIsolated`, `TestReconPreviewAndSplitProvenanceReadsFollowCanonicalLineage`, `TestEntityMergeSplitPreviewUndoAndProvenance`, `TestEntityIdentityNormalizationConflictAndConcurrentDeduplication`, `TestEntityReadProvenanceTracksCanonicalLineageOnRealPostgreSQL`, `TestEntityMergeConflictIsOptimisticUnderConcurrency`, `TestFindingPromotionRevisionsAndOperatorOnlyPromotion`, `TestG3AuthoritativeProvenanceJourney`, `TestNotableAlertsAreDeduplicatedAndStreamed`, `TestNotableAlertsUseSystemActorForAISourceCaptures`
- MCP/claims/report/export/faults: `TestMCPStandardFlowReusesCaptureService`, `TestOutOfBandClaimLifecycleThroughPostgreSQL`, `TestReportHandlerRequiresAuthAndScopesEngagement`, `TestExportJobLifecyclePersistsReceiptAndBlocksBrowserAuthorship`, `TestExportJobPreflightRejectsInsufficientCapacity`, `TestExportTeardownAuthorizationRoundTrip`, `TestBuildExportEvidenceTarStreamsAttachmentRoles`, `TestExportJobListIsPagedAndResumable`, `TestUpsertEvidenceRejectsImmutableMetadataChanges`, `TestCaptureIngestRejectsDiskPressureBeforeCommitting`, `TestCaptureIngestRetriesAfterInterruptedUploadWithoutDuplication`

## Reproducible blockers

```text
ERROR: Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?
WAYPOINT_DB_DSN is required
WAYPOINT_TEST_PG_DSN is required for real-PostgreSQL gate tests
```

## Notes

This run keeps the requested assertions intact: unavailable runtime services remain blocked or skipped, never promoted to Pass.

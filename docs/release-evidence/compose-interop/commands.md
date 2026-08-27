# Compose interop evidence

Commands retained:
- `go test -run 'TestCompose(LiveCollectorInteropTranscripts|StackPersistsDBAndEvidenceAcrossRestart|StackStartsCleanlyTwice)$' -count=1 -v ./...`
- `docker compose -f compose.yml config --quiet`
- `docker info`

Note: live compose execution is blocked in this sandbox because the Docker daemon is unavailable.

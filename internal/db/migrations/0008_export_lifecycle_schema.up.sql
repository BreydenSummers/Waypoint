CREATE TABLE export_job (
    id uuid PRIMARY KEY,
    engagement_id uuid NOT NULL REFERENCES engagement(id) ON DELETE CASCADE,
    requested_by uuid NOT NULL REFERENCES actor(id) ON DELETE RESTRICT,
    retry_of_job_id uuid REFERENCES export_job(id) ON DELETE SET NULL,
    format_version text NOT NULL CHECK (format_version = '1.0.0'),
    state text NOT NULL CHECK (state IN ('queued', 'preflighting', 'running', 'verifying', 'cancel_requested', 'cancelled', 'failed', 'completed')),
    progress_stage text NOT NULL CHECK (progress_stage IN ('queued', 'capacity_preflight', 'snapshot', 'database_dump', 'evidence', 'report_snapshot', 'pdf', 'tools', 'manifest', 'archive', 'verification', 'complete')),
    progress_percent integer NOT NULL CHECK (progress_percent BETWEEN 0 AND 100),
    processed_bytes bigint NOT NULL CHECK (processed_bytes >= 0),
    estimated_total_bytes bigint NOT NULL CHECK (estimated_total_bytes >= 0),
    snapshot_id uuid,
    cutoff timestamptz,
    bundle_archive_path text CHECK (
        bundle_archive_path IS NULL
        OR (
            char_length(bundle_archive_path) BETWEEN 1 AND 1024
            AND bundle_archive_path ~ '^(?!/)(?!.*(?:^|/)\.\.(?:/|$))(?!.*//).+$'
        )
    ),
    bundle_archive_byte_length bigint CHECK (bundle_archive_byte_length IS NULL OR bundle_archive_byte_length >= 1),
    bundle_archive_sha256 char(64) CHECK (bundle_archive_sha256 IS NULL OR bundle_archive_sha256 ~ '^[a-f0-9]{64}$'),
    bundle_manifest_sha256 char(64) CHECK (bundle_manifest_sha256 IS NULL OR bundle_manifest_sha256 ~ '^[a-f0-9]{64}$'),
    bundle_report_snapshot_id uuid,
    bundle_receipt_id uuid,
    failure_code text CHECK (failure_code IS NULL OR failure_code IN ('capacity_insufficient', 'snapshot_failed', 'dump_failed', 'evidence_integrity_failed', 'report_failed', 'archive_failed', 'verification_failed', 'cancelled')),
    failure_message text CHECK (failure_message IS NULL OR (btrim(failure_message) <> '' AND length(failure_message) <= 2048)),
    failure_retryable boolean,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    completed_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    revision integer NOT NULL DEFAULT 1 CHECK (revision >= 1),
    CONSTRAINT export_job_retry_shape CHECK (retry_of_job_id IS NULL OR retry_of_job_id <> id),
    CONSTRAINT export_job_completed_shape CHECK (
        state <> 'completed'
        OR (
            snapshot_id IS NOT NULL
            AND cutoff IS NOT NULL
            AND bundle_archive_path IS NOT NULL
            AND bundle_archive_byte_length IS NOT NULL
            AND bundle_archive_sha256 IS NOT NULL
            AND bundle_manifest_sha256 IS NOT NULL
            AND bundle_report_snapshot_id IS NOT NULL
            AND bundle_receipt_id IS NOT NULL
            AND started_at IS NOT NULL
            AND completed_at IS NOT NULL
            AND progress_stage = 'complete'
            AND progress_percent = 100
            AND failure_code IS NULL
            AND failure_message IS NULL
            AND failure_retryable IS NULL
        )
    ),
    CONSTRAINT export_job_terminal_failure_shape CHECK (
        state NOT IN ('failed', 'cancelled')
        OR (
            failure_code IS NOT NULL
            AND failure_message IS NOT NULL
            AND failure_retryable IS NOT NULL
            AND completed_at IS NOT NULL
        )
    ),
    CONSTRAINT export_job_bundle_receipt_unique UNIQUE (bundle_receipt_id)
);

CREATE TABLE export_receipt (
    id uuid PRIMARY KEY,
    export_job_id uuid NOT NULL REFERENCES export_job(id) ON DELETE CASCADE,
    engagement_id uuid NOT NULL REFERENCES engagement(id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('verified', 'invalidated')),
    bundle_path text NOT NULL CHECK (
        char_length(bundle_path) BETWEEN 1 AND 1024
        AND bundle_path ~ '^(?!/)(?!.*(?:^|/)\.\.(?:/|$))(?!.*//).+$'
    ),
    archive_byte_length bigint NOT NULL CHECK (archive_byte_length >= 1),
    archive_sha256 char(64) NOT NULL CHECK (archive_sha256 ~ '^[a-f0-9]{64}$'),
    manifest_sha256 char(64) NOT NULL CHECK (manifest_sha256 ~ '^[a-f0-9]{64}$'),
    cutoff timestamptz NOT NULL,
    verified_at timestamptz NOT NULL,
    verified_by uuid NOT NULL REFERENCES actor(id) ON DELETE RESTRICT,
    verifier_version text NOT NULL CHECK (btrim(verifier_version) <> '' AND length(verifier_version) <= 128),
    invalidated_at timestamptz,
    invalidation_reason text CHECK (invalidation_reason IS NULL OR invalidation_reason IN ('archive_missing', 'archive_digest_mismatch', 'manifest_invalid', 'payload_invalid')),
    revision integer NOT NULL DEFAULT 1 CHECK (revision >= 1),
    CONSTRAINT export_receipt_status_shape CHECK (
        (status = 'verified' AND invalidated_at IS NULL AND invalidation_reason IS NULL)
        OR
        (status = 'invalidated' AND invalidated_at IS NOT NULL AND invalidation_reason IS NOT NULL)
    ),
    CONSTRAINT export_receipt_export_job_unique UNIQUE (export_job_id),
    CONSTRAINT export_receipt_id_export_job_unique UNIQUE (id, export_job_id)
);

ALTER TABLE export_job
    ADD CONSTRAINT export_job_bundle_receipt_fk
    FOREIGN KEY (bundle_receipt_id) REFERENCES export_receipt(id) ON DELETE SET NULL;

ALTER TABLE teardown_authorization
    ADD CONSTRAINT teardown_authorization_receipt_fk
    FOREIGN KEY (receipt_id) REFERENCES export_receipt(id) ON DELETE CASCADE;

ALTER TABLE teardown_authorization
    ADD CONSTRAINT teardown_authorization_receipt_export_job_fk
    FOREIGN KEY (receipt_id, export_job_id) REFERENCES export_receipt(id, export_job_id) ON DELETE CASCADE;

ALTER TABLE teardown_authorization
    DROP CONSTRAINT IF EXISTS teardown_authorization_bundle_path_check;

ALTER TABLE teardown_authorization
    ADD CONSTRAINT teardown_authorization_bundle_path_check
    CHECK (
        char_length(bundle_path) BETWEEN 1 AND 1024
        AND bundle_path ~ '^(?!/)(?!.*(?:^|/)\.\.(?:/|$))(?!.*//).+$'
    );

CREATE INDEX export_job_state_updated_at_idx ON export_job (state, updated_at ASC, id ASC);

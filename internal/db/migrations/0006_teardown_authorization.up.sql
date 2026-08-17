CREATE TABLE teardown_authorization (
    id uuid PRIMARY KEY,
    engagement_id uuid NOT NULL REFERENCES engagement(id) ON DELETE CASCADE,
    receipt_id uuid NOT NULL,
    export_job_id uuid NOT NULL,
    bundle_path text NOT NULL CHECK (btrim(bundle_path) <> ''),
    archive_sha256 char(64) NOT NULL CHECK (archive_sha256 ~ '^[a-f0-9]{64}$'),
    manifest_sha256 char(64) NOT NULL CHECK (manifest_sha256 ~ '^[a-f0-9]{64}$'),
    requested_by uuid NOT NULL REFERENCES actor(id) ON DELETE RESTRICT,
    requested_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    status text NOT NULL CHECK (status IN ('authorized', 'consumed', 'expired')),
    consumed_at timestamptz,
    updated_at timestamptz NOT NULL DEFAULT now(),
    revision integer NOT NULL DEFAULT 1 CHECK (revision >= 1),
    CONSTRAINT teardown_authorization_consumed_shape CHECK (
        (status = 'consumed' AND consumed_at IS NOT NULL)
        OR
        (status <> 'consumed' AND consumed_at IS NULL)
    )
);

CREATE INDEX teardown_authorization_engagement_requested_at_idx ON teardown_authorization (engagement_id, requested_at DESC, id DESC);
CREATE INDEX teardown_authorization_receipt_idx ON teardown_authorization (receipt_id);
CREATE INDEX teardown_authorization_status_expires_idx ON teardown_authorization (status, expires_at ASC);

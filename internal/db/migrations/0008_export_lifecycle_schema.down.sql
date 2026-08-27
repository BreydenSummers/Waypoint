ALTER TABLE teardown_authorization
    DROP CONSTRAINT IF EXISTS teardown_authorization_receipt_export_job_fk;

ALTER TABLE teardown_authorization
    DROP CONSTRAINT IF EXISTS teardown_authorization_receipt_fk;

ALTER TABLE teardown_authorization
    DROP CONSTRAINT IF EXISTS teardown_authorization_bundle_path_check;

ALTER TABLE teardown_authorization
    ADD CONSTRAINT teardown_authorization_bundle_path_check
    CHECK (btrim(bundle_path) <> '');

ALTER TABLE export_job
    DROP CONSTRAINT IF EXISTS export_job_bundle_receipt_fk;

DROP TABLE IF EXISTS export_receipt;
DROP TABLE IF EXISTS export_job;

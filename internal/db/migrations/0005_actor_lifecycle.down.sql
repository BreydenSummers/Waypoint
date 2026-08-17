DROP TRIGGER IF EXISTS actor_lifecycle_defaults ON actor;
DROP FUNCTION IF EXISTS set_actor_lifecycle_defaults();

ALTER TABLE actor
    DROP COLUMN IF EXISTS revoked_by,
    DROP COLUMN IF EXISTS last_rotated_by,
    DROP COLUMN IF EXISTS last_rotated_at,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS revision,
    DROP COLUMN IF EXISTS credential_version;

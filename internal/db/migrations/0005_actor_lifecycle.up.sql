ALTER TABLE actor
    ADD COLUMN credential_version integer NOT NULL DEFAULT 1,
    ADD COLUMN revision integer NOT NULL DEFAULT 1,
    ADD COLUMN created_by uuid,
    ADD COLUMN last_rotated_at timestamptz,
    ADD COLUMN last_rotated_by uuid,
    ADD COLUMN revoked_by uuid;

CREATE OR REPLACE FUNCTION set_actor_lifecycle_defaults()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.created_by IS NULL THEN
        NEW.created_by := NEW.id;
    END IF;
    IF NEW.credential_version IS NULL OR NEW.credential_version < 1 THEN
        NEW.credential_version := 1;
    END IF;
    IF NEW.revision IS NULL OR NEW.revision < 1 THEN
        NEW.revision := 1;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER actor_lifecycle_defaults
    BEFORE INSERT ON actor
    FOR EACH ROW
    EXECUTE FUNCTION set_actor_lifecycle_defaults();

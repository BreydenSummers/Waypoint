ALTER TABLE entity
    DROP CONSTRAINT IF EXISTS entity_revision_valid;

ALTER TABLE entity
    DROP COLUMN IF EXISTS revision;

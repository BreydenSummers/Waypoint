ALTER TABLE entity
    ADD COLUMN revision integer NOT NULL DEFAULT 1;

ALTER TABLE entity
    ADD CONSTRAINT entity_revision_valid CHECK (revision >= 1);

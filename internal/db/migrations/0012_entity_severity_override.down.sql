ALTER TABLE entity DROP CONSTRAINT IF EXISTS entity_severity_override_values;
ALTER TABLE entity DROP COLUMN IF EXISTS severity_override;

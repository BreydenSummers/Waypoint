-- An operator can override an asset's derived severity from the base-camp
-- board (info / low / medium / high / critical) by dragging its card between
-- risk columns. Stored in its own column so it survives observation-driven
-- attribute rewrites and is independent of the finding-derived severity;
-- NULL means "use the derived severity".
ALTER TABLE entity ADD COLUMN IF NOT EXISTS severity_override text;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'entity_severity_override_values') THEN
        ALTER TABLE entity ADD CONSTRAINT entity_severity_override_values
            CHECK (severity_override IS NULL OR severity_override IN ('info', 'low', 'medium', 'high', 'critical'));
    END IF;
END $$;

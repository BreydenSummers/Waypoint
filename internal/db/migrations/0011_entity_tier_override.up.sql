-- An operator can override an asset's derived AD tier (0 domain / 1 server /
-- 2 endpoint). Stored in its own column so it survives observation-driven
-- attribute rewrites; NULL means "use the derived tier".
ALTER TABLE entity ADD COLUMN IF NOT EXISTS tier_override smallint;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'entity_tier_override_range') THEN
        ALTER TABLE entity ADD CONSTRAINT entity_tier_override_range
            CHECK (tier_override IS NULL OR tier_override BETWEEN 0 AND 2);
    END IF;
END $$;

CREATE INDEX entity_engagement_visible_first_seen_idx ON entity (engagement_id, first_seen ASC, id ASC) WHERE merged_into_entity_id IS NULL;
CREATE INDEX entity_engagement_lineage_first_seen_idx ON entity (engagement_id, first_seen ASC, id ASC);
CREATE INDEX observation_engagement_observed_at_idx ON observation (engagement_id, observed_at ASC, id ASC);

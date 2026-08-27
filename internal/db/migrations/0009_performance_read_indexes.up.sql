CREATE INDEX audit_event_engagement_id_idx ON audit_event (engagement_id, id ASC);
CREATE INDEX export_job_engagement_updated_at_idx ON export_job (engagement_id, updated_at DESC, id DESC);

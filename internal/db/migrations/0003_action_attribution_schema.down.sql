DROP INDEX IF EXISTS audit_event_engagement_action_capture_idx;
DROP INDEX IF EXISTS action_engagement_clock_skew_status_idx;
DROP INDEX IF EXISTS action_engagement_execution_status_idx;
DROP INDEX IF EXISTS action_engagement_egress_mode_status_idx;
DROP INDEX IF EXISTS actor_engagement_role_idx;
DROP INDEX IF EXISTS action_engagement_initiated_by_idx;
DROP INDEX IF EXISTS action_engagement_exec_host_ip_idx;
DROP INDEX IF EXISTS action_engagement_plugin_id_idx;
DROP INDEX IF EXISTS action_engagement_exec_host_method_idx;
DROP INDEX IF EXISTS action_engagement_source_agent_kind_idx;

ALTER TABLE action
    DROP CONSTRAINT IF EXISTS action_clock_skew_shape,
    DROP CONSTRAINT IF EXISTS action_provenance_shape,
    DROP CONSTRAINT IF EXISTS action_execution_shape,
    DROP CONSTRAINT IF EXISTS action_egress_attribution_shape,
    DROP CONSTRAINT IF EXISTS action_exec_host_attribution_shape,
    DROP CONSTRAINT IF EXISTS action_source_agent_shape;

ALTER TABLE action
    DROP COLUMN IF EXISTS clock_skew_offset_ms,
    DROP COLUMN IF EXISTS clock_skew_status,
    DROP COLUMN IF EXISTS received_at,
    DROP COLUMN IF EXISTS execution_failure_code,
    DROP COLUMN IF EXISTS execution_signal,
    DROP COLUMN IF EXISTS execution_status,
    DROP COLUMN IF EXISTS egress_observed_at,
    DROP COLUMN IF EXISTS egress_status,
    DROP COLUMN IF EXISTS egress_mode,
    DROP COLUMN IF EXISTS exec_host_confidence,
    DROP COLUMN IF EXISTS exec_host_interface,
    DROP COLUMN IF EXISTS exec_host_method,
    DROP COLUMN IF EXISTS source_agent_platform_arch,
    DROP COLUMN IF EXISTS source_agent_platform_os,
    DROP COLUMN IF EXISTS source_agent_version,
    DROP COLUMN IF EXISTS source_agent_name,
    DROP COLUMN IF EXISTS source_agent_kind;

DROP TYPE IF EXISTS clock_skew_status;
DROP TYPE IF EXISTS execution_status;
DROP TYPE IF EXISTS egress_status;
DROP TYPE IF EXISTS egress_mode;
DROP TYPE IF EXISTS exec_host_confidence;
DROP TYPE IF EXISTS exec_host_method;
DROP TYPE IF EXISTS source_agent_platform_arch;
DROP TYPE IF EXISTS source_agent_platform_os;
DROP TYPE IF EXISTS source_agent_kind;

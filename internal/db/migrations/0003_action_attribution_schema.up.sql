DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'source_agent_kind') THEN
        CREATE TYPE source_agent_kind AS ENUM ('operator_wrapper', 'remote_agent', 'mcp_client');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'source_agent_platform_os') THEN
        CREATE TYPE source_agent_platform_os AS ENUM ('linux', 'macos', 'windows', 'other');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'source_agent_platform_arch') THEN
        CREATE TYPE source_agent_platform_arch AS ENUM ('amd64', 'arm64', 'other');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'exec_host_method') THEN
        CREATE TYPE exec_host_method AS ENUM ('route_selection', 'interface_selection', 'operator_declared');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'exec_host_confidence') THEN
        CREATE TYPE exec_host_confidence AS ENUM ('confirmed', 'inferred');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'egress_mode') THEN
        CREATE TYPE egress_mode AS ENUM ('auto', 'manual', 'off');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'egress_status') THEN
        CREATE TYPE egress_status AS ENUM ('observed', 'declared', 'disabled', 'resolution_failed');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'execution_status') THEN
        CREATE TYPE execution_status AS ENUM ('exited', 'signaled', 'failed_to_start');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'clock_skew_status') THEN
        CREATE TYPE clock_skew_status AS ENUM ('within_tolerance', 'outside_tolerance');
    END IF;
END $$;

ALTER TABLE action
    ADD COLUMN IF NOT EXISTS source_agent_kind source_agent_kind,
    ADD COLUMN IF NOT EXISTS source_agent_name text,
    ADD COLUMN IF NOT EXISTS source_agent_version text,
    ADD COLUMN IF NOT EXISTS source_agent_platform_os source_agent_platform_os,
    ADD COLUMN IF NOT EXISTS source_agent_platform_arch source_agent_platform_arch,
    ADD COLUMN IF NOT EXISTS exec_host_method exec_host_method,
    ADD COLUMN IF NOT EXISTS exec_host_interface text,
    ADD COLUMN IF NOT EXISTS exec_host_confidence exec_host_confidence,
    ADD COLUMN IF NOT EXISTS egress_mode egress_mode,
    ADD COLUMN IF NOT EXISTS egress_status egress_status,
    ADD COLUMN IF NOT EXISTS egress_observed_at timestamptz,
    ADD COLUMN IF NOT EXISTS execution_status execution_status,
    ADD COLUMN IF NOT EXISTS execution_signal text,
    ADD COLUMN IF NOT EXISTS execution_failure_code text,
    ADD COLUMN IF NOT EXISTS received_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS clock_skew_status clock_skew_status,
    ADD COLUMN IF NOT EXISTS clock_skew_offset_ms integer;

ALTER TABLE action
    ADD CONSTRAINT action_source_agent_shape CHECK (
        (
            source_agent_kind IS NULL AND source_agent_name IS NULL AND source_agent_version IS NULL AND
            source_agent_platform_os IS NULL AND source_agent_platform_arch IS NULL
        )
        OR
        (
            source_agent_kind IS NOT NULL AND btrim(source_agent_name) <> '' AND btrim(source_agent_version) <> '' AND
            source_agent_platform_os IS NOT NULL AND source_agent_platform_arch IS NOT NULL
        )
    ),
    ADD CONSTRAINT action_exec_host_attribution_shape CHECK (
        (exec_host_method IS NULL AND exec_host_interface IS NULL AND exec_host_confidence IS NULL)
        OR
        (exec_host_method IS NOT NULL AND exec_host_confidence IS NOT NULL)
    ),
    ADD CONSTRAINT action_egress_attribution_shape CHECK (
        (
            egress_mode IS NULL AND egress_status IS NULL AND egress_observed_at IS NULL
        )
        OR
        (
            egress_mode = 'auto' AND egress_status IN ('observed', 'resolution_failed') AND
            (
                (egress_status = 'observed' AND egress_public_ip IS NOT NULL AND egress_observed_at IS NOT NULL)
                OR
                (egress_status = 'resolution_failed' AND egress_public_ip IS NULL AND egress_observed_at IS NULL)
            )
        )
        OR
        (
            egress_mode = 'manual' AND egress_status = 'declared' AND egress_public_ip IS NOT NULL AND egress_observed_at IS NOT NULL
        )
        OR
        (
            egress_mode = 'off' AND egress_status = 'disabled' AND egress_public_ip IS NULL AND egress_observed_at IS NULL
        )
    ),
    ADD CONSTRAINT action_execution_shape CHECK (
        (
            execution_status IS NULL AND execution_signal IS NULL AND execution_failure_code IS NULL
        )
        OR
        (
            execution_status = 'exited' AND exit_code IS NOT NULL AND execution_signal IS NULL AND execution_failure_code IS NULL
        )
        OR
        (
            execution_status = 'signaled' AND exit_code IS NULL AND execution_signal IS NOT NULL AND btrim(execution_signal) <> '' AND execution_failure_code IS NULL
        )
        OR
        (
            execution_status = 'failed_to_start' AND exit_code IS NULL AND execution_signal IS NULL AND execution_failure_code IS NOT NULL AND btrim(execution_failure_code) <> '' AND execution_failure_code ~ '^[a-z][a-z0-9_]*$'
        )
    ),
    ADD CONSTRAINT action_provenance_shape CHECK (
        (capture_id IS NULL AND capture_fingerprint IS NULL)
        OR
        (capture_id IS NOT NULL AND capture_fingerprint IS NOT NULL AND length(capture_fingerprint) = 64 AND capture_fingerprint ~ '^[a-f0-9]{64}$')
    ),
    ADD CONSTRAINT action_clock_skew_shape CHECK (
        (clock_skew_status IS NULL AND clock_skew_offset_ms IS NULL)
        OR
        (clock_skew_status IS NOT NULL AND clock_skew_offset_ms IS NOT NULL)
    );

CREATE INDEX action_engagement_source_agent_kind_idx ON action (engagement_id, source_agent_kind) WHERE source_agent_kind IS NOT NULL;
CREATE INDEX action_engagement_exec_host_method_idx ON action (engagement_id, exec_host_method) WHERE exec_host_method IS NOT NULL;
CREATE INDEX action_engagement_egress_mode_status_idx ON action (engagement_id, egress_mode, egress_status) WHERE egress_mode IS NOT NULL;
CREATE INDEX action_engagement_execution_status_idx ON action (engagement_id, execution_status, started_at DESC) WHERE execution_status IS NOT NULL;
CREATE INDEX action_engagement_clock_skew_status_idx ON action (engagement_id, clock_skew_status) WHERE clock_skew_status IS NOT NULL;

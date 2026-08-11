CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'actor_kind') THEN
        CREATE TYPE actor_kind AS ENUM ('human', 'ai_agent');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'actor_role') THEN
        CREATE TYPE actor_role AS ENUM ('owner', 'operator', 'viewer');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'action_initiated_by') THEN
        CREATE TYPE action_initiated_by AS ENUM ('manual', 'ai', 'scan-library');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'action_phase') THEN
        CREATE TYPE action_phase AS ENUM ('recon', 'attacks', 'findings');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'action_parse_status') THEN
        CREATE TYPE action_parse_status AS ENUM ('parsed', 'needs-plugin', 'raw', 'parse-failed');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'audit_origin_kind') THEN
        CREATE TYPE audit_origin_kind AS ENUM ('rest', 'mcp', 'service', 'bootstrap');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'audit_subject_type') THEN
        CREATE TYPE audit_subject_type AS ENUM ('engagement', 'actor', 'action', 'structured_result', 'entity', 'finding', 'export', 'teardown', 'out_of_band_claim');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'entity_key_type') THEN
        CREATE TYPE entity_key_type AS ENUM ('ad_sid', 'mac', 'fqdn', 'hostname_ip', 'other');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'finding_severity') THEN
        CREATE TYPE finding_severity AS ENUM ('info', 'low', 'medium', 'high', 'critical');
    END IF;
END $$;

CREATE TABLE engagement (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (btrim(name) <> ''),
    client text NOT NULL CHECK (btrim(client) <> ''),
    scope text NOT NULL CHECK (btrim(scope) <> ''),
    status text NOT NULL DEFAULT 'active' CHECK (btrim(status) <> ''),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE actor (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    engagement_id uuid NOT NULL REFERENCES engagement(id) ON DELETE CASCADE,
    kind actor_kind NOT NULL,
    handle text NOT NULL CHECK (btrim(handle) <> '' AND length(handle) <= 128),
    token_hash text NOT NULL CHECK (length(token_hash) = 64 AND token_hash ~ '^[a-f0-9]{64}$'),
    role actor_role NOT NULL,
    agent_name text,
    model text,
    version text,
    authorized_by uuid REFERENCES actor(id) ON DELETE RESTRICT,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT actor_handle_unique UNIQUE (engagement_id, handle),
    CONSTRAINT actor_token_hash_unique UNIQUE (engagement_id, token_hash),
    CONSTRAINT actor_ai_authorization_shape CHECK (
        (kind = 'ai_agent' AND authorized_by IS NOT NULL AND agent_name IS NOT NULL AND model IS NOT NULL AND version IS NOT NULL)
        OR
        (kind = 'human' AND authorized_by IS NULL AND agent_name IS NULL AND model IS NULL AND version IS NULL)
    )
);

CREATE TABLE evidence (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    engagement_id uuid NOT NULL REFERENCES engagement(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('stdout', 'stderr', 'screenshot', 'attachment', 'other')),
    sha256 char(64) NOT NULL CHECK (sha256 ~ '^[a-f0-9]{64}$'),
    byte_length bigint NOT NULL CHECK (byte_length >= 0),
    media_type text NOT NULL CHECK (btrim(media_type) <> ''),
    storage_key text NOT NULL CHECK (btrim(storage_key) <> ''),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT evidence_engagement_sha_unique UNIQUE (engagement_id, sha256, kind)
);

CREATE TABLE action (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    engagement_id uuid NOT NULL REFERENCES engagement(id) ON DELETE CASCADE,
    actor_id uuid NOT NULL REFERENCES actor(id) ON DELETE RESTRICT,
    source_agent_id uuid NOT NULL,
    capture_id uuid,
    capture_fingerprint text,
    initiated_by action_initiated_by NOT NULL,
    phase action_phase NOT NULL,
    command text NOT NULL CHECK (btrim(command) <> ''),
    argv jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(argv) = 'array'),
    cwd text NOT NULL CHECK (btrim(cwd) <> ''),
    exec_host_ip inet NOT NULL,
    egress_public_ip inet,
    pivot_chain jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(pivot_chain) = 'array'),
    target_kind text NOT NULL CHECK (btrim(target_kind) <> ''),
    target_value text NOT NULL CHECK (btrim(target_value) <> ''),
    target_port integer CHECK (target_port IS NULL OR (target_port BETWEEN 1 AND 65535)),
    target_transport text,
    started_at timestamptz NOT NULL,
    ended_at timestamptz,
    exit_code integer,
    stdout_evidence_id uuid NOT NULL REFERENCES evidence(id) ON DELETE RESTRICT,
    stderr_evidence_id uuid NOT NULL REFERENCES evidence(id) ON DELETE RESTRICT,
    plugin_id text,
    parse_status action_parse_status NOT NULL,
    decision_context jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT action_time_order CHECK (ended_at IS NULL OR ended_at >= started_at),
    CONSTRAINT action_exit_code_shape CHECK (
        (exit_code IS NULL)
        OR (exit_code BETWEEN -2147483648 AND 2147483647)
    ),
    CONSTRAINT action_decision_context_shape CHECK (
        (initiated_by = 'ai' AND decision_context IS NOT NULL)
        OR (initiated_by <> 'ai' AND decision_context IS NULL)
    ),
    CONSTRAINT action_parsed_requires_plugin CHECK (
        (parse_status <> 'parsed') OR (plugin_id IS NOT NULL)
    )
);

CREATE TABLE entity (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    engagement_id uuid NOT NULL REFERENCES engagement(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (btrim(kind) <> ''),
    key_type entity_key_type NOT NULL,
    key_value text NOT NULL CHECK (btrim(key_value) <> ''),
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(attributes) = 'object'),
    first_seen timestamptz NOT NULL DEFAULT now(),
    last_seen timestamptz NOT NULL DEFAULT now(),
    merged_into_entity_id uuid REFERENCES entity(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT entity_identity_unique UNIQUE (engagement_id, key_type, key_value)
);

CREATE TABLE result (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    engagement_id uuid NOT NULL REFERENCES engagement(id) ON DELETE CASCADE,
    action_id uuid NOT NULL REFERENCES action(id) ON DELETE RESTRICT,
    plugin_id text NOT NULL CHECK (btrim(plugin_id) <> ''),
    schema_id text NOT NULL CHECK (btrim(schema_id) <> ''),
    schema_version text NOT NULL CHECK (btrim(schema_version) <> ''),
    extracted jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(extracted) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT result_action_unique UNIQUE (action_id)
);

CREATE TABLE observation (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    engagement_id uuid NOT NULL REFERENCES engagement(id) ON DELETE CASCADE,
    action_id uuid NOT NULL REFERENCES action(id) ON DELETE RESTRICT,
    result_id uuid NOT NULL REFERENCES result(id) ON DELETE RESTRICT,
    entity_id uuid REFERENCES entity(id) ON DELETE RESTRICT,
    kind text NOT NULL CHECK (btrim(kind) <> ''),
    identifiers jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(identifiers) = 'array'),
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(attributes) = 'object'),
    observed_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE finding (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    engagement_id uuid NOT NULL REFERENCES engagement(id) ON DELETE CASCADE,
    title text NOT NULL CHECK (btrim(title) <> ''),
    severity finding_severity NOT NULL,
    affected_entity_ids uuid[] NOT NULL DEFAULT '{}'::uuid[],
    evidence_action_ids uuid[] NOT NULL DEFAULT '{}'::uuid[],
    remediation text NOT NULL DEFAULT '' CHECK (length(remediation) <= 32768),
    status text NOT NULL CHECK (btrim(status) <> ''),
    promoted_by uuid REFERENCES actor(id) ON DELETE RESTRICT,
    promoted_at timestamptz,
    revision integer NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT finding_promotion_shape CHECK (
        (promoted_by IS NULL AND promoted_at IS NULL)
        OR
        (promoted_by IS NOT NULL AND promoted_at IS NOT NULL)
    )
);

CREATE TABLE audit_event (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    engagement_id uuid NOT NULL REFERENCES engagement(id) ON DELETE CASCADE,
    actor_id uuid NOT NULL REFERENCES actor(id) ON DELETE RESTRICT,
    actor_kind actor_kind NOT NULL,
    actor_handle text NOT NULL CHECK (btrim(actor_handle) <> ''),
    actor_role actor_role NOT NULL,
    actor_agent_name text,
    actor_model text,
    actor_version text,
    actor_authorized_by uuid,
    occurred_at timestamptz NOT NULL DEFAULT now(),
    type text,
    origin_kind audit_origin_kind NOT NULL,
    origin_service text,
    subject_type audit_subject_type NOT NULL,
    subject_id uuid NOT NULL,
    subject_revision integer NOT NULL DEFAULT 1 CHECK (subject_revision >= 1),
    request_id text NOT NULL CHECK (btrim(request_id) <> ''),
    correlation_id text NOT NULL CHECK (btrim(correlation_id) <> ''),
    causation_action_id uuid REFERENCES action(id) ON DELETE RESTRICT,
    causation_event_id bigint,
    data jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(data) = 'object'),
    CONSTRAINT audit_event_actor_snapshot_shape CHECK (
        (actor_kind = 'ai_agent' AND actor_authorized_by IS NOT NULL AND actor_agent_name IS NOT NULL AND actor_model IS NOT NULL AND actor_version IS NOT NULL)
        OR
        (actor_kind = 'human' AND actor_authorized_by IS NULL AND actor_agent_name IS NULL AND actor_model IS NULL AND actor_version IS NULL)
    ),
    CONSTRAINT audit_event_origin_shape CHECK (
        (origin_kind = 'service' AND origin_service IS NOT NULL)
        OR
        (origin_kind <> 'service' AND origin_service IS NULL)
    )
);

ALTER TABLE audit_event
    ADD CONSTRAINT audit_event_causation_event_fk
    FOREIGN KEY (causation_event_id) REFERENCES audit_event(id) ON DELETE RESTRICT;

CREATE VIEW audit AS SELECT * FROM audit_event;

CREATE INDEX engagement_status_created_at_idx ON engagement (status, created_at DESC);
CREATE INDEX actor_engagement_kind_idx ON actor (engagement_id, kind);
CREATE INDEX actor_engagement_handle_idx ON actor (engagement_id, handle);
CREATE INDEX evidence_engagement_created_at_idx ON evidence (engagement_id, created_at DESC);
CREATE INDEX action_engagement_started_at_idx ON action (engagement_id, started_at DESC, id DESC);
CREATE INDEX action_engagement_actor_started_at_idx ON action (engagement_id, actor_id, started_at DESC);
CREATE INDEX action_engagement_phase_started_at_idx ON action (engagement_id, phase, started_at DESC);
CREATE INDEX action_engagement_parse_status_idx ON action (engagement_id, parse_status, started_at DESC);
CREATE INDEX action_target_lookup_idx ON action (engagement_id, target_kind, target_value);
CREATE INDEX entity_engagement_kind_last_seen_idx ON entity (engagement_id, kind, last_seen DESC);
CREATE INDEX entity_merged_into_idx ON entity (merged_into_entity_id);
CREATE INDEX result_engagement_created_at_idx ON result (engagement_id, created_at DESC);
CREATE INDEX result_action_created_at_idx ON result (action_id, created_at DESC);
CREATE INDEX observation_entity_observed_at_idx ON observation (entity_id, observed_at DESC);
CREATE INDEX observation_action_idx ON observation (action_id, observed_at DESC);
CREATE INDEX observation_result_idx ON observation (result_id, observed_at DESC);
CREATE INDEX finding_engagement_status_severity_idx ON finding (engagement_id, status, severity);
CREATE INDEX finding_engagement_promoted_at_idx ON finding (engagement_id, promoted_at DESC);
CREATE INDEX finding_affected_entity_ids_idx ON finding USING GIN (affected_entity_ids);
CREATE INDEX finding_evidence_action_ids_idx ON finding USING GIN (evidence_action_ids);
CREATE INDEX audit_event_engagement_occurred_at_idx ON audit_event (engagement_id, occurred_at DESC, id DESC);
CREATE INDEX audit_event_engagement_subject_idx ON audit_event (engagement_id, subject_type, subject_id, id DESC);
CREATE INDEX audit_event_request_id_idx ON audit_event (engagement_id, request_id);
CREATE INDEX audit_event_correlation_id_idx ON audit_event (engagement_id, correlation_id);
CREATE INDEX action_capture_scope_unique_idx ON action (engagement_id, actor_id, source_agent_id, capture_id) WHERE capture_id IS NOT NULL;

CREATE OR REPLACE FUNCTION forbid_row_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME USING ERRCODE = 'check_violation';
END;
$$;

CREATE OR REPLACE FUNCTION validate_actor_authorization()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    authorizer_kind actor_kind;
BEGIN
    IF NEW.kind = 'ai_agent' THEN
        IF NEW.authorized_by IS NULL THEN
            RAISE EXCEPTION 'ai_agent actors require authorized_by' USING ERRCODE = 'check_violation';
        END IF;
        SELECT kind INTO authorizer_kind
        FROM actor
        WHERE id = NEW.authorized_by
          AND engagement_id = NEW.engagement_id;
        IF authorizer_kind IS NULL THEN
            RAISE EXCEPTION 'authorized_by must reference an actor in the same engagement' USING ERRCODE = 'foreign_key_violation';
        END IF;
        IF authorizer_kind <> 'human' THEN
            RAISE EXCEPTION 'ai_agent actors must be authorized by a human actor' USING ERRCODE = 'check_violation';
        END IF;
    ELSIF NEW.authorized_by IS NOT NULL THEN
        RAISE EXCEPTION 'human actors cannot declare authorized_by' USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION validate_audit_actor_snapshot()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    a actor;
BEGIN
    SELECT * INTO a FROM actor WHERE id = NEW.actor_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'audit_event actor must exist' USING ERRCODE = 'foreign_key_violation';
    END IF;
    IF a.engagement_id <> NEW.engagement_id THEN
        RAISE EXCEPTION 'audit_event actor must belong to the same engagement' USING ERRCODE = 'foreign_key_violation';
    END IF;
    IF a.kind <> NEW.actor_kind OR a.handle <> NEW.actor_handle OR a.role <> NEW.actor_role THEN
        RAISE EXCEPTION 'audit_event actor snapshot must match the actor row' USING ERRCODE = 'check_violation';
    END IF;
    IF COALESCE(a.agent_name, '') <> COALESCE(NEW.actor_agent_name, '')
       OR COALESCE(a.model, '') <> COALESCE(NEW.actor_model, '')
       OR COALESCE(a.version, '') <> COALESCE(NEW.actor_version, '')
       OR COALESCE(a.authorized_by::text, '') <> COALESCE(NEW.actor_authorized_by::text, '') THEN
        RAISE EXCEPTION 'audit_event actor snapshot must match the actor row' USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER actor_authorization_guard
    BEFORE INSERT OR UPDATE ON actor
    FOR EACH ROW
    EXECUTE FUNCTION validate_actor_authorization();

CREATE TRIGGER action_append_only_guard
    BEFORE UPDATE OR DELETE ON action
    FOR EACH ROW
    EXECUTE FUNCTION forbid_row_mutation();

CREATE TRIGGER evidence_append_only_guard
    BEFORE UPDATE OR DELETE ON evidence
    FOR EACH ROW
    EXECUTE FUNCTION forbid_row_mutation();

CREATE TRIGGER result_append_only_guard
    BEFORE UPDATE OR DELETE ON result
    FOR EACH ROW
    EXECUTE FUNCTION forbid_row_mutation();

CREATE TRIGGER observation_append_only_guard
    BEFORE UPDATE OR DELETE ON observation
    FOR EACH ROW
    EXECUTE FUNCTION forbid_row_mutation();

CREATE TRIGGER audit_event_append_only_guard
    BEFORE UPDATE OR DELETE ON audit_event
    FOR EACH ROW
    EXECUTE FUNCTION forbid_row_mutation();

CREATE TRIGGER audit_event_snapshot_guard
    BEFORE INSERT ON audit_event
    FOR EACH ROW
    EXECUTE FUNCTION validate_audit_actor_snapshot();

CREATE OR REPLACE FUNCTION forbid_finding_evidence_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.evidence_action_ids IS DISTINCT FROM OLD.evidence_action_ids THEN
        RAISE EXCEPTION 'finding evidence links are immutable' USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER finding_evidence_immutable_guard
    BEFORE UPDATE OF evidence_action_ids ON finding
    FOR EACH ROW
    EXECUTE FUNCTION forbid_finding_evidence_mutation();

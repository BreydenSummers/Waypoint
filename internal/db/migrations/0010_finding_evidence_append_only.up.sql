-- Evidence links on a finding become append-only instead of frozen at promotion.
-- Corroborating captures can be attached later, but existing links can never be
-- removed, reordered, or rewritten — preserving tamper-resistance while letting a
-- finding accumulate supporting evidence as an engagement progresses.
CREATE OR REPLACE FUNCTION forbid_finding_evidence_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    old_len integer := array_length(OLD.evidence_action_ids, 1);
BEGIN
    IF NEW.evidence_action_ids IS DISTINCT FROM OLD.evidence_action_ids THEN
        -- An empty prior set may grow to anything. Otherwise the new array must
        -- be an extension of the old one: at least as long, with the old array
        -- as an exact leading prefix.
        IF old_len IS NOT NULL AND (
               array_length(NEW.evidence_action_ids, 1) IS NULL
               OR array_length(NEW.evidence_action_ids, 1) < old_len
               OR NEW.evidence_action_ids[1:old_len] IS DISTINCT FROM OLD.evidence_action_ids
           ) THEN
            RAISE EXCEPTION 'finding evidence links are append-only' USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;

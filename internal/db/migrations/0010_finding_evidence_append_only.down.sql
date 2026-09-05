-- Restore the strict immutability guard: evidence links may not change at all.
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

-- Clock skew is the delay between a capture's ended_at and the server's
-- received_at, in milliseconds. A legitimately large skew (an offline buffer
-- replayed later, or a compromised host with a wildly wrong clock) easily
-- exceeds a 32-bit integer, which is exactly the value the audit trail must
-- record faithfully. Widen the column to bigint.
ALTER TABLE action
    ALTER COLUMN clock_skew_offset_ms TYPE bigint;

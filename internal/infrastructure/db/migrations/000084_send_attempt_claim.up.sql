-- A task may enter the provider boundary only once. A timeout after claiming is
-- deliberately not retried: provider acceptance cannot be inferred from HTTP failure.
CREATE TABLE send_attempt_claims (
    task_id UUID PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    message_id TEXT NOT NULL,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

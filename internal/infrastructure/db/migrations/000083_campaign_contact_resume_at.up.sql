-- Recipient-level pause/resume for reply-driven campaign handling.
-- Used for out-of-office and "contact me later" replies without pausing an
-- entire campaign or stopping other contacts from continuing.
ALTER TABLE campaign_contact_progress
    ADD COLUMN IF NOT EXISTS resume_at timestamp with time zone;

CREATE INDEX IF NOT EXISTS idx_campaign_contact_progress_resume_at
    ON campaign_contact_progress (campaign_id, contact_id, resume_at)
    WHERE resume_at IS NOT NULL;

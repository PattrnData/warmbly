DROP INDEX IF EXISTS idx_campaign_contact_progress_resume_at;
ALTER TABLE campaign_contact_progress
    DROP COLUMN IF EXISTS resume_at;

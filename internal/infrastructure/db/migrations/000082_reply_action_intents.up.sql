-- Expand reply classification from coarse sentiment into action-oriented human intent.
ALTER TABLE campaign_contact_progress
    DROP CONSTRAINT IF EXISTS campaign_contact_progress_reply_class_chk;

ALTER TABLE campaign_contact_progress
    ADD CONSTRAINT campaign_contact_progress_reply_class_chk
    CHECK (reply_class IN ('', 'positive', 'negative', 'neutral', 'question', 'wrong_person', 'bad_timing', 'referral', 'auto_reply', 'out_of_office', 'unsubscribe', 'unknown'));

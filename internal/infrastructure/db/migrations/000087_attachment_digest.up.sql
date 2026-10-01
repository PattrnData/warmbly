-- Existing attachment rows have no trusted upload digest and must be re-uploaded
-- before they can be sent. Never infer a digest from the mutable object key.
ALTER TABLE campaign_attachments ADD COLUMN sha256 text NOT NULL DEFAULT '';

-- Preserve worker event time independently of delivery/DB insertion order.
ALTER TABLE email_account_errors ADD COLUMN occurred_at timestamptz;

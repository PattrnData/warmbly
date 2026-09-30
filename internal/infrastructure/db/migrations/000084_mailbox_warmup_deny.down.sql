DROP TRIGGER email_account_warmup_deny ON email_accounts;
DROP FUNCTION remove_denied_mailbox_warmup();
ALTER TABLE email_accounts DROP COLUMN warmup_denied;

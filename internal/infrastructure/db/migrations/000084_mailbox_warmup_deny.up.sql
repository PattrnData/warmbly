-- Operator-controlled warmup exclusion. Never infer it from ordinary account status.
ALTER TABLE email_accounts
    ADD COLUMN warmup_denied boolean NOT NULL DEFAULT false;

-- The mailbox row lock serializes this cleanup with guarded task/pool inserts.
CREATE FUNCTION remove_denied_mailbox_warmup() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    DELETE FROM warmup_pool_participants WHERE email_account_id = NEW.id;
    UPDATE tasks SET status = 'cancelled', updated_at = now()
      WHERE email_account_id = NEW.id AND task_type = 'warmup'
        AND status IN ('pending', 'active');
    RETURN NEW;
END;
$$;

CREATE TRIGGER email_account_warmup_deny
AFTER UPDATE OF warmup_denied ON email_accounts
FOR EACH ROW WHEN (NEW.warmup_denied AND NOT OLD.warmup_denied)
EXECUTE FUNCTION remove_denied_mailbox_warmup();

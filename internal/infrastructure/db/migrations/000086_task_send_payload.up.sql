-- Null for historical sends: they cannot be authenticated from mutable current
-- task rows. Deployment requires draining/reconciling old queued envelopes first.
ALTER TABLE tasks ADD COLUMN send_payload_hash TEXT;
ALTER TABLE tasks ADD CONSTRAINT tasks_send_payload_hash_shape
    CHECK (send_payload_hash IS NULL OR send_payload_hash ~ '^[0-9a-f]{64}$');

CREATE FUNCTION prevent_task_send_rebinding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.send_payload_hash IS NOT NULL AND
       (NEW.send_payload_hash IS DISTINCT FROM OLD.send_payload_hash OR
        NEW.message_id IS DISTINCT FROM OLD.message_id) THEN
        RAISE EXCEPTION 'task send binding is immutable';
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER task_send_binding_immutable BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION prevent_task_send_rebinding();

DROP TRIGGER task_send_binding_immutable ON tasks;
DROP FUNCTION prevent_task_send_rebinding();
ALTER TABLE tasks DROP CONSTRAINT tasks_send_payload_hash_shape;
ALTER TABLE tasks DROP COLUMN send_payload_hash;

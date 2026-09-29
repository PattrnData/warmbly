# Mailbox warmup deny: isolated operational procedure (not executed)

Scope: only `a5f28cfb-b10f-4597-b445-28e647e0dd92`. Independent risk review `deleg_e05443e7` identifies it as an inactive app-only mailbox with historical deletion beyond the two-hour grace window and no current strike, complaint, or pool signal. This is a precautionary warmup deny, not an abuse finding. Do not apply to the other 59 mailboxes by inference. No production database access was used to prepare this procedure.

## Before any live change (separate operator approval required)

1. Merge/deploy migration `000082` and all control-plane consumers together. Do not enable warmup or start a campaign to test it. Coordinate a pause/drain of warmup dispatch and worker queues before the transition: a message already handed to a worker cannot be recalled by a control-plane flag.
2. In the intended database, independently verify the account id, organization, mailbox identity, current status, pool rows, pending/active warmup tasks and queued worker sends with read-only queries. Do not inspect or print credentials. Ensure migration 000082 exists and check audit/backup readiness.
3. Within the **approved** maintenance window, run the exact transaction below with a human watching the `RETURNING` row. Abort unless precisely one expected mailbox is returned. The trigger deletes its pool membership and cancels pending/active warmup task rows in the same transaction. Other task types, campaign state, account status, worker assignment, and mailbox sync remain unchanged.

```sql
BEGIN;
SELECT id, organization_id, email, provider, status, warmup_denied
FROM email_accounts
WHERE id = 'a5f28cfb-b10f-4597-b445-28e647e0dd92';
SELECT pool_id, email_account_id FROM warmup_pool_participants
WHERE email_account_id = 'a5f28cfb-b10f-4597-b445-28e647e0dd92';
SELECT id, task_type, status FROM tasks
WHERE email_account_id = 'a5f28cfb-b10f-4597-b445-28e647e0dd92'
  AND task_type = 'warmup' AND status IN ('pending', 'active');
-- Only after verifying identity and obtaining explicit approval:
UPDATE email_accounts SET warmup_denied = true, updated_at = now()
WHERE id = 'a5f28cfb-b10f-4597-b445-28e647e0dd92'
RETURNING id, warmup_denied;
-- Check exactly one returned row and verify these counts are zero before COMMIT:
SELECT count(*) AS pool_rows FROM warmup_pool_participants
WHERE email_account_id = 'a5f28cfb-b10f-4597-b445-28e647e0dd92';
SELECT count(*) AS live_warmup_tasks FROM tasks
WHERE email_account_id = 'a5f28cfb-b10f-4597-b445-28e647e0dd92'
  AND task_type = 'warmup' AND status IN ('pending', 'active');
COMMIT;
```

On any mismatch, `ROLLBACK` rather than commit. Re-run the read-only verification after commit and after reconciler/health-check ticks; confirm no new pool rows, pending/active warmup tasks or worker warmup sends, while mailbox sync may continue. Repeating the update with `true` is idempotent; the trigger fires only on `false -> true`. If old state was already denied yet stale rows exist, do not assume the trigger runs; investigate and clean them explicitly under a fresh approval. Removal of the deny requires explicit risk review and a separate change, never an automatic expiry.

## Engineering evidence and limits

- Source: `internal/repository/pg_email.go` filters reconciler candidates; `internal/app/email/handler.go` removes denied memberships on sync; `internal/repository/pg_warmup.go` filters partner reads and locks guarded pool inserts; `internal/repository/pg_task.go` locks mailbox row before task insertion; `internal/tasks/email_task.go` checks at task entry and immediately before Kafka dispatch; migration 000082 cleans existing rows transactionally. Mailbox sync is not gated.
- Concurrency: row locks serialize DB deny updates against guarded pool/task insertion. The task repository's existing advisory lock keeps duplicate pending tasks idempotent. A pre-existing queued worker command or producer already past its final read remains a race; drain and verify the worker queue before claiming zero post-transition sends. The worker does not read Postgres and this change does not create a worker-side deny check. A disposable local PostgreSQL 16 fixture exercised the migration, concurrent deny/pool join, repeated task creation denial and rollback migration; no production database was touched.
- Quality rails: OMH evidence-bound means the historical deletion is not represented as a current strike. Ponytail minimalism keeps one boolean and one transition trigger instead of a new quarantine state machine. Agency Backend Architect advice influenced explicit data migration/rollback, race boundaries, and source-backed verification. None is a deployment approval.

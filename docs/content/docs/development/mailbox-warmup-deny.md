# Mailbox warmup deny: isolated operational procedure (not executed)

Scope: only `a5f28cfb-b10f-4597-b445-28e647e0dd92`. Independent risk review `deleg_e05443e7` identifies it as an inactive app-only mailbox with historical deletion beyond the two-hour grace window and no current strike, complaint, or pool signal. This is a precautionary warmup deny, not an abuse finding. Do not apply to the other 59 mailboxes by inference. No production database access was used to prepare this procedure.

## Before any live change (separate operator approval required)

1. Merge/deploy migration `000082`, backend internal warmup-status endpoint, and updated workers together before the deny transition. Confirm the worker's existing `ENCRYPTED_KEYS_BACKEND_URL` reaches the backend and its `ENCRYPTED_KEYS_WORKER_TOKEN` matches backend `INTERNAL_API_TOKEN` using a non-secret authenticated status probe; a token mismatch fails warmup closed. Do not enable warmup or start a campaign to test it. **Do not pause all sends or globally drain worker queues.** Verify the target mailbox and its warmup commands only; an already-started SMTP call cannot be recalled.
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
- Concurrency: row locks serialize DB deny updates against guarded pool/task insertion. The task repository's existing advisory lock keeps duplicate pending tasks idempotent. The worker queries the durable mailbox flag through a token-authenticated internal backend read for every warmup command, including pre-deny queued commands; unavailable or malformed status fails closed for warmup only. A command already past that read (especially an SMTP call in flight) cannot be recalled. A disposable local PostgreSQL 16 fixture exercised the **up** migration, concurrent deny/pool join and repeated task creation denial. The **down migration has not been tested**; no production database was touched.
- Exact-account queue/in-flight verification (operator read-only, after deployment and approved deny): correlate command payload `email_id = 'a5f28cfb-b10f-4597-b445-28e647e0dd92'` and `is_warmup = true` with task-row `email_account_id`, consumer processing logs/traces and provider send receipts over the transition window. Record command/task IDs and dispatch, gate-read, SMTP-start and completion times. For each queued command delivered after deny, confirm a worker `EmailFailed` result (deny or unavailable lookup) with no provider send; confirm no new warmup task/pool row. Investigate missing results and bus-publish errors explicitly: the current Kafka consumer commits on handler errors, so a failed result publish can lose a task rather than retry. For commands already in flight before deny, reconcile any completed sends by provider receipt and do not assert recall. Compare non-warmup sends for this mailbox and other mailbox IDs to their pre-change baselines (without changing them). If payload inspection cannot reliably identify the exact account, stop and escalate; never globally hold campaign traffic as a substitute.
- Quality rails: OMH evidence-bound means the historical deletion is not represented as a current strike. Ponytail minimalism keeps one boolean and one transition trigger instead of a new quarantine state machine. Agency Backend Architect advice influenced explicit data migration/rollback, race boundaries, and source-backed verification. None is a deployment approval.

# Parent-independent exact-four Outlook credential-mode recovery

This internal code-only primitive changes only the existing OAuth credential row. No public endpoint, production runner, status activation, worker load, Graph POST or queue send is introduced. The four IDs in `approvedSharedSenderConversionTarget` and `approvedAppOnlyConversionID` are the only candidates: `af571c6e-e6f0-4cb9-90fe-a7d5105babd7`, `e165f276-cc96-4907-991a-a7b860e1ed6f`, `e7ce131c-4a6f-4529-9bd4-9f88bfd99208`, `ea4b17db-80b9-445b-9c28-8d67679dc4a5`. `a5f28cfb-b10f-4597-b445-28e647e0dd92` is excluded. No delegated parent or matching ciphertext is presumed.

## Separate approval and evidence, before any production write

For each target, independently establish from raw source and tenant administration: the target is the intended shared mailbox; the configured app registration and tenant are the authorized provider for it; application `Mail.ReadWrite` and `Mail.Send` roles are consented, and an application-access policy or Exchange application RBAC limits this app to the approved mailbox. The code checks the tenant-specific OAuth token URL, app ID/tenant/roles in the resulting token and a read-only Graph inbox GET for that mailbox; these checks **do not prove the access-policy scope or sending**. Obtain a scoped, expiring permit bound to exact target ID, owner, organization, worker, email, tenant, app/client ID, code/config hashes, escrow digest, credential version, no-send window and independent reviewer approval. Do not treat this document or Graph GET as a permit.

Escrow, in an access-controlled encrypted store, the exact target `email_accounts` row and `email_accounts_oauth` row (ciphertext, expiry and row identity), with SHA-256 integrity digests and custody receipt. Never put ciphertext or plaintext tokens in tickets, logs, PRs or chat. Derive the version `md5(refresh_token || ':' || access_token || ':' || expires_at::text)` inside PostgreSQL and bind it in the permit; this MD5 value is only an optimistic concurrency version, **not** the escrow integrity digest or authorization proof. Confirm the owner/org/worker/email/provider/status from exact rows and confirm `email_accounts_oauth` has exactly one row. A missing or NULL worker, unverified provenance, missing escrow, ambiguous row, or unexpected credential stops execution.

## Single-target staged dry-run (read-only; no token values)

Use a read-only transaction on the approved environment with parameters for the **one** target UUID, owner UUID, organization UUID, worker UUID and email (never a list). Capture exact raw result privately and reconcile it to the independent permit and escrow, not to a stale summary. Sample query; `ROLLBACK` closes the dry-run and performs no mutation:

```sql
BEGIN TRANSACTION READ ONLY;
SELECT ea.id,ea.user_id,ea.organization_id,ea.worker_id,ea.email,ea.provider,ea.status,
       count(o.email_account_id) AS oauth_rows,
       min(md5(o.refresh_token || ':' || o.access_token || ':' || o.expires_at::text)) AS credential_version,
       count(t.email_account_id) FILTER (WHERE t.status IN ('pending','active')) AS queued_or_active_tasks
FROM email_accounts ea
LEFT JOIN email_accounts_oauth o ON o.email_account_id=ea.id
LEFT JOIN tasks t ON t.email_account_id=ea.id
WHERE ea.id=:target_id AND ea.user_id=:owner_id AND ea.organization_id=:org_id
  AND ea.worker_id=:worker_id AND lower(ea.email)=lower(:mailbox_email)
GROUP BY ea.id;
ROLLBACK;
```

Require exactly one target row, one OAuth row, `outlook`, `inactive`, exact owner/org/worker/email, nonempty non-sentinel delegated credential, no pending/active tasks, and escrow/version match. Query `tasks` separately if necessary to avoid JOIN multiplication. No-send/fleet-quiescence is an independent gate: a `NOT EXISTS` snapshot inside the CAS rejects visible queued work but cannot prevent an in-flight worker or a concurrent new task. Require separate evidence that old consumer and scheduler cannot send during conversion; do not rely on inactive status or this SQL alone. Do not globally hold unrelated mailboxes or touch the quarantined fifth.

## Guarded change and readback

The internal service requires explicit ID, owner/org, non-null worker, email, tenant and **caller-supplied expected credential version**. It compares a fresh version before Graph GET; the repository rechecks the four-ID allowlist, owner/org/worker/email, inactive Outlook status, non-sentinel credential, absence of visible pending/active tasks, and the version in the atomic `UPDATE ... FROM` CAS. Exactly one changed row is success. It writes `access_token=''`, `refresh_token=__warmbly_graph_app_only__`, `expires_at=now()`. Neither `email_accounts` nor task/history rows change. Repeated/stale requests return no change. The helper is intentionally not wired to an API or executable production runner; staged dry-run and permit execution must be separately built and approved, not improvised via ad-hoc SQL.

After separately authorized execution, read back the exact account and credential row by approved ID and reconcile all preimage identity fields, status/worker unchanged, one OAuth row, app-only sentinel and cleared access token, task counts, and untouched fifth. Keep inactive until a distinct receive/send verification and release gate. Stop on any discrepancy or unexpected queue activity.

Rollback is a separate reviewed operation, not the reverse of this code path: restore **only** the original target's encrypted OAuth preimage in one transaction conditioned on exact ID, owner/org/worker/email, inactive Outlook status and the current app-only sentinel, assert one affected row, and read back exact ciphertext and account identity against escrow. Abort on drift, absent preimage, any new sends/writes or zero/multiple rows. No parent credential may be copied to effect rollback. PostgreSQL rolls back a failed conversion statement automatically; it does not undo a committed conversion.

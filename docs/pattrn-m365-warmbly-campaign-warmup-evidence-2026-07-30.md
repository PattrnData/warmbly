# Pattrn M365 / Warmbly campaign + warmup evidence — 2026-07-30

Secret-free operational evidence from the live Warmbly host `mail.pattrndata.ai`.

## Live service state

- Worktree branch: `feat/warmbly-twentycrm-approval-pack`
- Worktree commit: `ac60dbbf`
- Backend health: `GET http://127.0.0.1:8080/health -> HTTP 200`, body `{"status":"ok"}`
- Docker services observed running/healthy where healthcheck exists: `backend`, `mailpit`, `nats`, `postgres`, `realtime`, `redis`, `tracking`, `web`, `worker`.
- `TASKS_PROVIDER=local` on backend/worker.
- Backend Graph app-only tenant/client/application-credential env refs present; values not recorded.

## Controlled Warmbly-native campaign proof

Scope:

- Internal-only recipient: `rohit@pattrndata.io`
- Explicit sender pool: `james@pattrndata.com` (`email_account_id=ea4b17db-80b9-445b-9c28-8d67679dc4a5`)
- Marker: `WMBLY-SMOKE-20260730T150045Z`
- Campaign id: `4553cbd9-14fe-418c-ba09-43ba772a9099`
- Preflight: `HTTP 200`, `passed=true`, `score=100`

Live DB/API readback:

```text
campaign=4553cbd9-14fe-418c-ba09-43ba772a9099
name=Internal smoke 20260730T150045Z
status=completed
created_at=2026-07-30 15:00:46.003555
last_status_change_at=2026-07-30 15:00:46.748873+00

campaign_logs:
- started: Campaign started @ 2026-07-30 15:00:46.803651+00
- email_sent: Email sent to rohit@pattrndata.io @ 2026-07-30 15:00:47.397166+00
- completed: Campaign completed: all emails sent @ 2026-07-30 15:00:48.363283+00

tasks:
- 09ef14ea-467d-4322-8fd5-42c7172ac4ad campaign completed scheduled=2026-07-30 15:00:45.277601+00 completed=2026-07-30 15:00:47.401241+00
- 64525120-42cb-406f-b9a3-9a8f069d6755 campaign completed scheduled=2026-07-30 15:00:00+00 completed=2026-07-30 15:00:48.366568+00

campaign_contact_progress:
- contact_id=cb2a16ad-b41f-4b07-aafb-4d595cd672e5
- sequence_id=3f9fccad-2b3b-4ccb-abca-6b809e46b45e
- sent_at=2026-07-30 15:00:47.357587+00
- bounced_at/replied_at/opened_at/clicked_at empty

dead_letters_for_campaign=0
```

Caveat: Microsoft Graph `Sent Items` readback for this specific Warmbly campaign marker returned `sent_items_matches=0`. Follow-up inspection showed the campaign task rows were marked `completed` after Warmbly successfully queued/published the send event, but the task rows did not persist the generated RFC `Message-ID` and worker logs no longer contained a matching send-success line for the smoke task IDs. So this proof was classified more narrowly as **Warmbly campaign enqueue/control-plane proof**, not confirmed worker-send or independent Microsoft Sent Items proof. A code fix was added to persist campaign/user-email task `message_id` values so future proofs have a stable Graph/inbox correlation key.

## Confirmed Warmbly-native worker + Microsoft Sent Items proof after Message-ID persistence

Scope:

- Internal-only recipient: `rohit@pattrndata.io`
- Explicit sender pool: `james@pattrndata.com` (`email_account_id=ea4b17db-80b9-445b-9c28-8d67679dc4a5`)
- Marker: `pwproof-20260730162702`
- Campaign id: `28be1c26-0e2c-44ae-bec6-5fe2e0c4e24a`
- Worktree commit: `ac60dbbf`
- Backend health: `GET http://127.0.0.1:8080/health -> HTTP 200`, body `{"status":"ok"}`
- Preflight returned `passed=false`, `score=75` only because `unsubscribe_header` was disabled for the internal-only proof; campaign readiness, schedule window, and daily-limit checks passed.

Live DB/API readback:

```text
campaign=28be1c26-0e2c-44ae-bec6-5fe2e0c4e24a
name=Proof pwproof-20260730162702
status=completed
sender_strategy=explicit
daily_limit=3
open_tracking=false
link_tracking=false

campaign_logs:
- started: Campaign started @ 2026-07-30 16:27:05.205888+00
- email_sent: Email sent to rohit@pattrndata.io @ 2026-07-30 16:27:08.537635+00
- completed: Campaign completed: all emails sent @ 2026-07-30 16:27:09.336924+00

campaign_contact_progress:
- contact_id=cb2a16ad-b41f-4b07-aafb-4d595cd672e5
- sequence_id=740a9b8d-d3d8-45a4-81c7-cd94115b5337
- sent_at=2026-07-30 16:27:08.503006+00
- bounced_at/replied_at/opened_at/clicked_at empty

tasks:
- 84895b69-a6be-446a-95dd-40f98b9dfa44 campaign completed
  scheduled=2026-07-30 16:27:07.630613+00
  completed=2026-07-30 16:27:08.541864+00
  sender=james@pattrndata.com
  message_id=<bc44ce03-5ce4-4a83-893c-fff6d693b4e4@pattrndata.com>
- 3457cfe2-395f-4176-bc9a-adbc8f853fd5 campaign completed
  scheduled=2026-07-30 16:00:00+00
  completed=2026-07-30 16:27:09.350378+00
  sender=james@pattrndata.com
  message_id empty (completion/check task)

dead_letters_for_campaign=0
```

Worker send-success log:

```json
{"level":"info","task_id":"84895b69-a6be-446a-95dd-40f98b9dfa44","message_id":"<bc44ce03-5ce4-4a83-893c-fff6d693b4e4@pattrndata.com>","provider_msg_id":"<bc44ce03-5ce4-4a83-893c-fff6d693b4e4@pattrndata.com>","time":"2026-07-30T16:27:09Z","message":"Email sent successfully"}
```

Microsoft Graph app-only Sent Items readback:

```text
graph_auth_present=True
subject_marker_matches=1
subject=Pattrn Warmbly proof pwproof-20260730162702
internetMessageId=<bc44ce03-5ce4-4a83-893c-fff6d693b4e4@pattrndata.com>
sentDateTime=2026-07-30T16:27:08Z
to=rohit@pattrndata.io
```

No warmup was activated as part of this proof:

```text
warmup_flag_set=0
pending_or_active_warmup_tasks=0
warmup_emails_sent_today=0
warmup_emails_replied_today=0
```

## Prior direct Graph send readback still present

Graph app-only Sent Items readback for the earlier direct controlled internal M365 test showed one Sent Items record per linked mailbox:

```text
james@pattrndata.com -> rohit@pattrndata.io @ 2026-07-30T10:43:17Z
subject=Warmbly/Pattrn M365 controlled internal test from james@pattrndata.com [20260730T104316Z-cf6bee78]

colin@pattrndata.co.uk -> rohit@pattrndata.io @ 2026-07-30T10:43:17Z
subject=Warmbly/Pattrn M365 controlled internal test from colin@pattrndata.co.uk [20260730T104316Z-cf6bee78]

sarah@pattrndata.com -> rohit@pattrndata.io @ 2026-07-30T10:43:18Z
subject=Warmbly/Pattrn M365 controlled internal test from sarah@pattrndata.com [20260730T104316Z-cf6bee78]
```

## Warmup pool constraints / no activation

Current account inventory:

```text
email_accounts_total=3
active_accounts=3
outlook_accounts=3
warmup_flag_set=0
```

Seeded pools:

```text
free    | Free warmup pool    | max_participants=1000
premium | Premium warmup pool | max_participants=1000
```

Participant state:

```text
colin@pattrndata.co.uk | outlook | active | warmup=NULL | pool=premium | participant_role=recipient_only | health_state=healthy | blocked_at=NULL
james@pattrndata.com   | outlook | active | warmup=NULL | pool=premium | participant_role=recipient_only | health_state=healthy | blocked_at=NULL
sarah@pattrndata.com   | outlook | active | warmup=NULL | pool=premium | participant_role=recipient_only | health_state=healthy | blocked_at=NULL
```

Warmup volume/task state:

```text
warmup_emails_sent_today=0
warmup_emails_replied_today=0
warmup tasks: dead_lettered=4 historical
pending_or_active_warmup_tasks=0
```

Historical warmup dead-letter inspection and no-activation preflight:

```text
Preflight environment on Warmbly host repo /home/hl-mailserver/projects/warmbly:
- commit: ac60dbbf
- branch: feat/warmbly-twentycrm-approval-pack
- BLOB_PROVIDER=filesystem
- BLOB_FS_ROOT=/data/blobs
- ENCRYPTED_KEYS_PROVIDER=postgres
- KMS_PROVIDER=local
- TASKS_PROVIDER=local
- backend health: HTTP 200 from http://127.0.0.1:8080/health

Storage preflight:
- backend container user 1000:1000 could mkdir /data/blobs/emails, write a marker under /data/blobs, read it, and remove it.
- worker container user 1000:1000 could mkdir /data/blobs/emails, write a marker under /data/blobs, read it, and remove it.
- /data/blobs and /data/blobs/emails are owned by warmbly:warmbly and writable by the runtime user.

DEK preflight:
- organization_encrypted_keys rows: 1 row / 1 distinct organization.
- Pattrn linked accounts share organization 2fb95e84-2893-4e0e-910e-8c52f7d7abb8.
- Pattrn org has exactly 1 distinct DEK row; ciphertext length 80 chars.
- No encrypted_data_key values were printed or copied into this document.
```

Historical warmup dead letters were still `pending` with due `next_retry_at` before the preflight, so they were **quarantined without replaying**. The four associated `tasks` remain `dead_lettered`; the `task_dead_letters` rows are now `status=quarantined`, `next_retry_at=NULL`, and include a JSON quarantine note requiring owner approval before any replay.

```text
1ff64710-2c59-45c8-b0cf-473fbdde1628 | c430078a-49a9-43f4-b804-e9a2dd5c1c8e | colin@pattrndata.co.uk | quarantined | prior error: /data/blobs/emails permission denied
67fdbba9-38b4-489e-9cec-989dac741509 | c2047d7f-de3d-42e9-87e1-f17a41b93883 | james@pattrndata.com   | quarantined | prior error: encryptedkeys: dek already exists for organization
4d1b93bb-90c4-4667-9fa8-95143d026ffc | 1b2f3ccb-7e0b-4e21-a50a-f96ba81f258c | sarah@pattrndata.com   | quarantined | prior error: /data/blobs/emails permission denied
0ac42155-8fb4-479c-8038-79b5b0cafa49 | 95059229-c096-44e9-ba04-89c161a97bf2 | james@pattrndata.com   | quarantined | prior error: /data/blobs/emails permission denied
```

Post-quarantine checks:

```text
task_dead_letters_by_type_status: campaign pending=2; warmup quarantined=4
retryable_due_now_warmup=0
pending_or_active_warmup_tasks=0
warmup_task_statuses: warmup dead_lettered=4
warmup_enabled_timestamp_set=0
warmup_null=3
```

Conclusion: historical warmup retry risk is neutralized without enabling warmup or replaying any message. Warmup remains a closed gate. The two remaining campaign pending dead letters are separate from warmup and should be reviewed before restarting any DLQ retry consumer, but they were not mutated in this warmup no-activation pass.

## Sarah mailbox proof attempt — blocked on Sent Items mismatch

Scope:

- Internal-only recipient: `rohit@pattrndata.io`
- Explicit sender pool: `sarah@pattrndata.com` (`email_account_id=ac8f94eb-3c84-4eef-b10f-34396069fef1`)
- Marker: `pwproof-sarah-20260730190252`
- Campaign id: `2271c73d-62d5-400c-9fcc-4d249919443f`
- Worktree commit: `ac60dbbf`
- Backend health: `GET http://127.0.0.1:8080/health -> HTTP 200`
- Preflight: `HTTP 200`, `passed=true`, `score=100`; checks passed for campaign readiness, schedule window, daily limit, and unsubscribe header.

Pre-send gates immediately before the attempt:

```text
warmup_enabled_count=0
pending_or_active_warmup_tasks=0
colin@pattrndata.co.uk | premium | recipient_only | healthy
james@pattrndata.com   | premium | recipient_only | healthy
sarah@pattrndata.com   | premium | recipient_only | healthy
graph_auth_present=true
```

Warmbly control-plane evidence:

```text
campaign_create|sarah@pattrndata.com|http=200
contact_add|sarah@pattrndata.com|http=200
campaign_start|sarah@pattrndata.com|http=200
campaign_terminal|sarah@pattrndata.com|status=active|message_id_present=true

campaign_logs:
- started: Campaign started
- email_sent: Email sent to rohit@pattrndata.io

dead_letters_for_campaign=0
completed task message_id=<e4aae2be-68c2-4358-adce-c9ff7dd33a22@pattrndata.com>
```

Microsoft Graph Sent Items readback for Sarah did **not** find the marker or `internetMessageId`, including a retry after the send window:

```text
graph_sentitems|sarah@pattrndata.com|matches=0
marker=pwproof-sarah-20260730190252
message_id=<e4aae2be-68c2-4358-adce-c9ff7dd33a22@pattrndata.com>
```

Worker log correlation also did not show a matching `Email sent successfully` row for that RFC `Message-ID` in the inspected window. Because the proof triangle is incomplete, this is classified as **Warmbly DB/API/control-plane proof only for Sarah**, not full Microsoft-delivery proof. The Colin proof was not attempted because the Sarah gate did not pass.

Safety cleanup after the failed proof triangle:

```text
POST /v1/campaigns/2271c73d-62d5-400c-9fcc-4d249919443f/stop -> HTTP 200 {"status":"stopped"}
campaign status=paused
remaining proof-campaign tasks: only the completed send task remained after stop/cancel cleanup
warmup_enabled_count=0
pending_or_active_warmup_tasks=0
```

## Gates after this evidence

Cleared:

- Backend health and local task provider operational.
- Three active Outlook-linked Warmbly accounts exist.
- Warmup pools are seeded.
- All three linked accounts are premium `recipient_only`, healthy, and `warmup=NULL`.
- Controlled Warmbly-native internal campaign proof completed in Warmbly DB/API with no campaign dead letters.
- Follow-up Warmbly-native proof at commit `ac60dbbf` persisted the worker RFC `Message-ID`, emitted a worker send-success log, and was independently found in Microsoft Graph Sent Items for `james@pattrndata.com`.

Still closed / caveated:

- Warmup activation gate: closed; `warmup=NULL`, no pending/active warmup tasks, current-day warmup send/reply counters zero, and historical warmup DLQs are quarantined with no retry schedule.
- Prospect/cold campaign gate: closed; internal-only proof does not approve prospect sends.
- Full Microsoft-delivery proof for the pilot sender is present for `james@pattrndata.com` only. `sarah@pattrndata.com` is blocked on a Sent Items / worker-log correlation mismatch; `colin@pattrndata.co.uk` was not attempted after Sarah failed the gate.
- Expansion gate: closed; no approval here for additional kiosk/shared-mailbox expansion.
- DLQ retry gate: closed for warmup; two separate campaign DLQs remain pending and should be reviewed before restarting any DLQ retry consumer.

## Next sender/shared-mailbox proof plan

Candidate order:

1. `sarah@pattrndata.com` — linked Outlook account, premium pool, `recipient_only`, healthy, `warmup=NULL`; no current proof in Microsoft Sent Items yet.
2. `colin@pattrndata.co.uk` — linked Outlook account, premium pool, `recipient_only`, healthy, `warmup=NULL`; no current proof in Microsoft Sent Items yet.

Internal-only proof gate per candidate:

- Re-run backend health and DB no-activation checks immediately before proof.
- Re-run read-only Microsoft Graph inbox smoke for the candidate mailbox.
- Confirm candidate remains `active`, `recipient_only`, `healthy`, `warmup=NULL`, and has no pending/active warmup tasks.
- Use one controlled Warmbly-native campaign proof with:
  - explicit sender pool containing only the candidate mailbox;
  - contact list containing only approved internal recipients;
  - a unique marker such as `pwproof-<UTC timestamp>`;
  - no prospects, no cold list, no LinkedIn/CRM mutations, no warmup start/resume.
- Capture the full proof triangle before marking the proof done:
  - Warmbly DB/API campaign/task completion and persisted RFC `Message-ID`;
  - worker send-success log for that message/task;
  - Microsoft Graph Sent Items readback for the same subject marker and `internetMessageId`.
- If any part of the triangle is missing, classify narrowly as enqueue/control-plane proof only and do not promote the mailbox to a send/warmup-ready state.

## Pilot-only warmup wave plan

This is a plan only; it does **not** approve or start warmup.

Scope:

- Pilot pool: current three linked Pattrn Outlook accounts only: `james@pattrndata.com`, `sarah@pattrndata.com`, `colin@pattrndata.co.uk`.
- Prospect/cold recipients: excluded.
- Removed legacy 12 users and later 13-kiosk expansion: excluded.
- Warmup role transition: requires explicit owner approval before any account changes from `recipient_only`/`warmup=NULL` into an active sender/receiver warmup role.

Pre-activation checks required in the same operator window as any future activation:

- Backend/worker/container health OK.
- Storage preflight write/remove OK from backend and worker.
- DEK preflight still exactly one org key for the Pattrn org.
- `retryable_due_now_warmup=0` and no pending/active warmup tasks.
- Any campaign pending DLQs reviewed or quarantined if they could be retried by a consumer restart.
- Graph read smoke passes for every mailbox in the wave.
- Each candidate has a completed Warmbly-native proof triangle or is explicitly left recipient-only.

Initial caps and pauses:

- Start with a pilot-only low cap below configured defaults; do not use `warmup_base`/`warmup_max` as permission to send at those levels.
- No prospect recipients in warmup seed/partner scope.
- Immediate pause/quarantine triggers: Graph auth/read failure, worker send failure, new DLQ, non-internal recipient evidence, Sent Items mismatch, storage/DEK failure, or any owner revocation.
- Post-activation evidence must show exactly which rows changed, which messages were sent, where they were found in Graph, and that no cold/prospect campaign was touched.

## Sarah and Colin conservative warmup policy - 2026-08-01

This section is a policy/evidence update only. It does **not** approve, start, resume, or replay warmup.

### Fresh read-only inventory

Live host/path checked: `relay.pattrndata.io` backing repo `/home/hl-mailserver/projects/warmbly`.

Service state observed before policy drafting:

```text
backend    running   healthy
consumer   running
mailpit    running   healthy
nats       running   healthy
postgres   running   healthy
realtime   running   healthy
redis      running   healthy
tracking   running   healthy
web        running
worker     running
```

Relevant schema facts from the live database:

```text
email_accounts columns include: warmup, campaign_limit, min_wait_time, warmup_base, warmup_max, warmup_increase, warmup_reply_rate, warmup_pool_type, risk_band, auth_state
warmup_pool_participants columns include: participant_role, blocked_at, blocked_until, spam_score, health_state, last_health_score, last_health_reason
warmup_pool_participants has no per-participant daily_limit/status column; sender caps are account-level warmup fields plus scheduler policy.
```

Current Pattrn sender/account state:

```text
colin@pattrndata.co.uk|status=active|warmup=NULL|campaign_limit=50|min_wait=600|base=10|max=40|inc=1|reply_rate=30|pool_type=premium|risk=clean|auth=passing
james@pattrndata.com|status=active|warmup=NULL|campaign_limit=50|min_wait=600|base=10|max=40|inc=1|reply_rate=30|pool_type=premium|risk=clean|auth=passing
sarah@pattrndata.com|status=active|warmup=NULL|campaign_limit=50|min_wait=600|base=10|max=40|inc=1|reply_rate=30|pool_type=premium|risk=clean|auth=passing
```

Current warmup-pool state:

```text
colin@pattrndata.co.uk|pool=Premium warmup pool|role=recipient_only|blocked_at=NULL|blocked_until=NULL|health=healthy|score=0|spam=0
james@pattrndata.com|pool=Premium warmup pool|role=recipient_only|blocked_at=NULL|blocked_until=NULL|health=healthy|score=0|spam=0
sarah@pattrndata.com|pool=Premium warmup pool|role=recipient_only|blocked_at=NULL|blocked_until=NULL|health=healthy|score=0|spam=0
active_warmup_tasks=0
warmup_tasks_last7d=4
campaign_tasks_today=4
```

Scheduler policy verified in source:

- Active warmup normally starts from `warmup_base`, increases by `warmup_increase` per day, and caps at `warmup_max`.
- Defaults are `base=10`, `increase=1`, `max=40`, but those defaults are not approved for the Sarah/Colin pilot.
- If a mailbox backs a live campaign, warmup is clamped to at most `5` warmup sends/day.
- Warmup volume is also capped by eligible recipient capacity, including recipient-only participants, so a small pool should not force repeated same-day sends to the same recipient.
- `watch` and `throttled` health states reduce volume and widen spacing. `quarantined` and `blocked` participants are not eligible.

### Activation stance for Sarah and Colin

Owner correction: Sarah and Colin are intended outbound sender identities under the kiosk licence plus attached shared mailbox method. `recipient_only` with `warmup=NULL` is the current pre-activation safety state, not the target operating model.

The next gate is therefore not "leave them recipient-only". The next gate is to graduate each mailbox through a controlled Warmbly-native internal sender proof, then enable outbound warmup/campaign sending in approved waves. Before either Sarah or Colin changes role or sends under warmup, the same operator window must prove:

1. `backend`, `consumer`, `worker`, `postgres`, `redis`, `nats`, and `mailpit` are running, with health checks green where available.
2. `active_warmup_tasks=0`, no retryable due warmup dead letters, and no unreviewed campaign DLQs that could be replayed by the same restart/operator action.
3. The candidate account is still `active`, `auth=passing`, `risk=clean`, `participant_role=recipient_only`, `health=healthy`, `spam_score=0`, and `blocked_at/blocked_until=NULL` before the role change.
4. Microsoft Graph read-only smoke succeeds for the candidate mailbox.
5. The candidate completes a Warmbly-native internal proof triangle: Warmbly task/message-id, worker send-success log, and Microsoft Graph Sent Items readback for the same RFC `Message-ID`. Sarah currently has a prior proof mismatch to resolve; Colin needs his first proof triangle.
6. Prospect/cold campaign send, contact import, CRM write, and Warmbly account expansion gates remain closed unless separately approved. LinkedIn is a separate outreach lane and is not part of this Warmbly cold-email setup.

### Pilot caps for first activation wave

Once the proof gate clears for a candidate and owner approval covers the operator window, start Sarah/Colin far below the product defaults, but treat this as an initial ramp, not the long-term capacity target of the kiosk/shared-mailbox method:

| Mailbox | Initial outbound warmup cap | Increase | Early pilot ceiling | Minimum spacing | Notes |
|---|---:|---:|---:|---:|---|
| `sarah@pattrndata.com` | `1/day` | `0/day` for the first 72 hours | `2/day` while the pool has only the current three Pattrn accounts | at least `3600s` | Resolve Sarah Sent Items / worker-log proof mismatch first, then graduate her to sender. |
| `colin@pattrndata.co.uk` | `1/day` | `0/day` for the first 72 hours | `2/day` while the pool has only the current three Pattrn accounts | at least `3600s` | Run Colin's first Warmbly-native proof triangle, then graduate him to sender. |

Implementation expectation for an approved activation: set account-level warmup fields to the pilot values in the same change that enables outbound warmup, rather than relying on existing defaults of `base=10`, `increase=1`, `max=40`, and `min_wait=600`.

Do not move above `2/day/mailbox` until all of the following are true for at least 72 hours: no warmup DLQs, no worker send failures, no Graph read failures, no Sent Items mismatches, no non-internal recipients during proof, no spam placement/complaint signals, both candidate participants remain `healthy`, and owner approval explicitly authorizes the next step. The next step should still be capped at `3/day/mailbox`, then ramp toward the sender-fleet capacity model only after more mailbox identities and recipient capacity are added and verified.

### Immediate stop / rollback conditions

Pause the mailbox back to `warmup=NULL` and restore `participant_role=recipient_only` immediately if any of these appear:

- any warmup task is addressed to a prospect, external cold lead, CRM contact, or non-approved recipient;
- worker send-success correlation or Microsoft Graph Sent Items readback is missing for a pilot message;
- candidate health changes away from `healthy`, spam score rises above `0`, or `blocked_at`/`blocked_until` becomes non-null;
- Graph auth/read smoke fails for the candidate or its partner mailbox;
- any warmup task dead-letters, retries unexpectedly, or exceeds the approved daily cap;
- storage/DEK preflight fails;
- campaign/cold outreach tasks restart unintentionally during the warmup operator window;
- the owner revokes approval or the operator window ends without post-activation evidence.

Gate conclusion: Sarah and Colin are currently safe as healthy pre-activation pool members. They are intended outbound sender identities for the kiosk licence plus attached shared mailbox method, but their live warmup/campaign graduation still needs the controlled internal proof triangle and explicit operator-window approval before prospect/cold outreach is opened.

## Sarah and Colin smallest warmup-sender activation wave - 2026-08-01

This section records the applied pilot warmup activation wave and post-activation readback from the live Warmbly host. It does not approve any prospect/cold campaign, CRM write, Warmbly account expansion, or DLQ replay. LinkedIn is a separate outreach lane and is not part of this Warmbly cold-email setup.

### Scope applied

- Mailboxes changed: `sarah@pattrndata.com`, `colin@pattrndata.co.uk`.
- Pool role changed from `recipient_only` to `sender_receiver` for those two mailboxes only.
- Warmup fields were set conservatively below product defaults: `warmup_base=1`, `warmup_max=2`, `warmup_increase=0`, `min_wait_time=3600`, `warmup_days=62`, `warmup_pool_type=premium`.
- Exactly one local warmup task was seeded per activated mailbox, scheduled for the next weekday morning Europe/London: Sarah at `2026-08-03 07:13:00+00`, Colin at `2026-08-03 07:41:00+00`.
- `james@pattrndata.com` remained `recipient_only` with `warmup=NULL`.

### Post-activation live readback

Service state observed: `backend`, `consumer`, `mailpit`, `nats`, `postgres`, `realtime`, `redis`, `tracking`, `web`, and `worker` were running; health checks were healthy where exposed.

```text
colin@pattrndata.co.uk | outlook | active | has_worker=true | warmup=2026-08-01 12:53:27.930142 | min_wait_time=3600 | warmup_base=1 | warmup_max=2 | warmup_increase=0 | warmup_days=62 | premium | clean | passing | sender_receiver | healthy | spam_score=0 | blocked_at=NULL
james@pattrndata.com   | outlook | active | has_worker=true | warmup=NULL                       | min_wait_time=600  | warmup_base=10 | warmup_max=40 | warmup_increase=1 | warmup_days=0  | premium | clean | passing | recipient_only  | healthy | spam_score=0 | blocked_at=NULL
sarah@pattrndata.com   | outlook | active | has_worker=true | warmup=2026-08-01 12:53:27.930142 | min_wait_time=3600 | warmup_base=1 | warmup_max=2 | warmup_increase=0 | warmup_days=62 | premium | clean | passing | sender_receiver | healthy | spam_score=0 | blocked_at=NULL
```

Seeded warmup task readback:

```text
sarah@pattrndata.com   | cf3d426e-21d5-4567-a8bb-63cd51641e01 | pending | 2026-08-03 07:13:00+00
colin@pattrndata.co.uk | 1ee05408-2f01-462c-b491-9ebda8cbf32b | pending | 2026-08-03 07:41:00+00
```

Safety checks after activation:

```text
due_warmup_tasks_now=0
completed_warmup_since_activation=0
active_campaigns=0
active_campaign_tasks=0
due_pending_warmup_dlq=0
campaigns_created_since_activation=0
```

Proof reply readback remains intact and paused:

```text
e200deaf-5fe5-4a3e-909b-095398d0441e | paused | sent_at=2026-08-01 11:40:05.277757+00 | replied_at=2026-08-01 11:40:24.718164+00 | reply_class=positive | source=lexicon | confidence=0.8
f85e5019-3282-4ec5-872e-78f4539ff4f5 | paused | sent_at=2026-08-01 11:53:00.210856+00 | replied_at=2026-08-01 11:53:24.973414+00 | reply_class=positive | source=lexicon | confidence=0.8
```

LinkedIn/Unipile is separate from this Warmbly cold-email setup. The live Warmbly DB has no LinkedIn/Unipile tables (`tables matching linkedin/unipile = 0`), so LinkedIn should be tracked and approved in its own lane rather than on this Warmbly gate.

### Gates and next expansion/account sorting plan

- Open now: Sarah and Colin pilot warmup sender/receiver role at 1/day each, with first seeded sends not due until Monday morning UK time.
- Still closed inside Warmbly cold email: prospect/cold campaign sends, imports, CRM writes, 12-legacy-user return, 13-kiosk expansion, DLQ replay, and cap increase above 2/day/mailbox. LinkedIn is separate and should not be bundled into this Warmbly gate.
- Immediate monitoring completed early after owner approval to accelerate only the existing Sarah/Colin seeded tasks. See the acceleration proof below.
- Next expansion/account sorting: keep James as recipient-only during the first Sarah/Colin pilot window; sort additional mailbox/account expansion only after 72 hours with no warmup DLQs, no Graph failures, no worker failures, no Sent Items mismatches, and owner approval for the next cap/account wave.

## Sarah and Colin accelerated first warmup task proof - 2026-08-01

Owner approved accelerating the warmup tasks in the Discord cold-email/outreach thread. The live change was scoped to the two already seeded Sarah/Colin warmup tasks only. No new campaign, prospect import, CRM write, LinkedIn/Unipile action, DLQ replay, cap increase, or mailbox expansion was opened.

### Acceleration applied

The existing pending pilot tasks were moved from Monday morning UTC to immediate staggered proof slots:

```text
sarah@pattrndata.com   | cf3d426e-21d5-4567-a8bb-63cd51641e01 | pending | 2026-08-01 13:32:49.244002+00
colin@pattrndata.co.uk | 1ee05408-2f01-462c-b491-9ebda8cbf32b | pending | 2026-08-01 13:37:49.244002+00
```

### Warmbly task and worker-send proof

Both accelerated warmup tasks completed cleanly, each with a persisted RFC `Message-ID` and worker send-success log:

```text
sarah@pattrndata.com   | cf3d426e-21d5-4567-a8bb-63cd51641e01 | completed | completed_at=2026-08-01 13:32:50.13132+00  | <c5170e7d-5e90-4278-a307-2703c6e55ac5@pattrndata.com>
colin@pattrndata.co.uk | 1ee05408-2f01-462c-b491-9ebda8cbf32b | completed | completed_at=2026-08-01 13:37:50.117139+00 | <4d37d7d8-acbc-45bb-959f-ecbf277eaadb@pattrndata.co.uk>
```

Worker logs showed the exact task IDs, warmup flag, recipient, and send-success message IDs:

```text
cf3d426e-21d5-4567-a8bb-63cd51641e01 | to=[james@pattrndata.com] | is_warmup=true | Email sent successfully | <c5170e7d-5e90-4278-a307-2703c6e55ac5@pattrndata.com>
1ee05408-2f01-462c-b491-9ebda8cbf32b | to=[james@pattrndata.com] | is_warmup=true | Email sent successfully | <4d37d7d8-acbc-45bb-959f-ecbf277eaadb@pattrndata.co.uk>
```

### Microsoft Graph Sent Items proof

Microsoft Graph app-only Sent Items readback found one exact `internetMessageId` match per sender:

```text
sarah@pattrndata.com   | sentitems_matches=1 | sentDateTime=2026-08-01T13:32:50Z | subject="today heads up" | to=[james@pattrndata.com]
colin@pattrndata.co.uk | sentitems_matches=1 | sentDateTime=2026-08-01T13:37:50Z | subject="Just a thought" | to=[james@pattrndata.com]
```

Daily warmup stats now show `emails_sent=1`, `emails_replied=0` for each activated mailbox on `2026-08-01`.

### Safety and next tasks after acceleration

Post-proof safety readback:

```text
active_campaigns=0
campaigns_created_since_activation=0
failed_warmup_since_acceleration=0
warmup_dlq_due_now=0
linkedin_unipile_tables=0
```

Warmbly created the next pending warmup task for each sender, both scheduled for Monday morning UTC:

```text
colin@pattrndata.co.uk | 1b140a17-57d2-4cc6-8a5c-b741bc79a452 | pending | 2026-08-03 07:53:47+00
sarah@pattrndata.com   | 7c159e91-068b-483e-b087-35f99a6bdf53 | pending | 2026-08-03 07:54:09+00
```

Current gate conclusion: the first Sarah/Colin warmup proof triangle is complete for both mailboxes. Keep prospect/cold campaign sends, imports, CRM writes, LinkedIn/Unipile actions, DLQ replay, expansion beyond the current approved mailbox set, and cap increases closed until a clean observation window and explicit owner approval.

## James current-three warmup sender expansion - 2026-08-01

Owner clarified that provider-type generality is not in scope. The desired scope is the current three Pattrn Microsoft 365 shared-mailbox accounts for the 14-kiosk plus attached shared-mailbox setup. James was therefore expanded from `recipient_only` to the same conservative outbound warmup sender policy as Sarah and Colin. This did not approve prospect/cold campaign sends, imports, CRM writes, LinkedIn/Unipile actions, DLQ replay, mailbox expansion beyond the current three, or cap increases.

### Preflight

Before changing James, live readback showed:

```text
sarah@pattrndata.com   | sender_receiver | active | has_worker=true | clean | passing | healthy | spam_score=0
colin@pattrndata.co.uk | sender_receiver | active | has_worker=true | clean | passing | healthy | spam_score=0
james@pattrndata.com   | recipient_only  | active | has_worker=true | clean | passing | healthy | spam_score=0 | warmup=NULL
```

Safety checks before activation:

```text
active_campaigns=0
campaign_tasks_pending_active=0
due_pending_warmup_tasks=0
james_pending_active_warmup_tasks=0
warmup_dlq_due_now=0
linkedin_unipile_tables=0
```

Microsoft Graph app-only read smoke passed for all three current mailboxes: Sarah, Colin, and James.

### Scope applied

James was changed to match the same conservative pilot warmup policy already applied to Sarah and Colin:

```text
james@pattrndata.com | warmup=2026-08-01 14:26:48.102817 | warmup_base=1 | warmup_max=2 | warmup_increase=0 | min_wait_time=3600 | warmup_days=62 | premium | sender_receiver | healthy | spam_score=0
```

Exactly one pending local warmup task was seeded for James for the next weekday morning UTC:

```text
james@pattrndata.com | ec8a732c-e155-4e7a-ad65-e2ef314098ca | pending | 2026-08-03 07:25:00+00
```

### Current three-account readback after James expansion

All current three Pattrn M365 shared-mailbox accounts are now active warmup sender/receivers under the conservative cap:

```text
colin@pattrndata.co.uk | sender_receiver | warmup_base=1 | warmup_max=2 | warmup_increase=0 | min_wait_time=3600 | healthy | spam_score=0
james@pattrndata.com   | sender_receiver | warmup_base=1 | warmup_max=2 | warmup_increase=0 | min_wait_time=3600 | healthy | spam_score=0
sarah@pattrndata.com   | sender_receiver | warmup_base=1 | warmup_max=2 | warmup_increase=0 | min_wait_time=3600 | healthy | spam_score=0
```

Pending warmup tasks for the current three:

```text
james@pattrndata.com   | ec8a732c-e155-4e7a-ad65-e2ef314098ca | pending | 2026-08-03 07:25:00+00
colin@pattrndata.co.uk | 1b140a17-57d2-4cc6-8a5c-b741bc79a452 | pending | 2026-08-03 07:53:47+00
sarah@pattrndata.com   | 7c159e91-068b-483e-b087-35f99a6bdf53 | pending | 2026-08-03 07:54:09+00
```

Post-change safety readback:

```text
current_3_sender_receiver=3
current_3_conservative_caps=3
active_campaigns=0
campaign_tasks_pending_active=0
due_pending_warmup_tasks=0
warmup_dlq_due_now=0
linkedin_unipile_tables=0
```

Current gate conclusion: the current three Pattrn Microsoft 365 shared-mailbox accounts are configured for conservative Warmbly warmup. Sarah and Colin have completed the first proof triangle. James has been activated and has a first warmup task scheduled for Monday morning UTC. Keep prospect/cold campaign sends, imports, CRM writes, LinkedIn/Unipile actions, DLQ replay, additional mailbox expansion, and cap increases closed until James' first task proof and the clean observation window pass.

## Weekend-enabled conservative warmup correction - 2026-08-01

Owner clarified the best-practice scope: campaigns remain weekday-only, but mailbox warmup should run Monday through Sunday under conservative caps. The live configuration was corrected for the current three Pattrn Microsoft 365 shared-mailbox warmup senders only. This did not approve prospect/cold campaign sends, imports, CRM writes, LinkedIn/Unipile actions, DLQ replay, mailbox expansion beyond the current three, or cap increases.

Pre-change readback showed the current three were still weekday-only:

```text
colin@pattrndata.co.uk | warmup_base=1 | warmup_max=2 | warmup_increase=0 | min_wait_time=3600 | warmup_days=62 | premium | passing | clean
james@pattrndata.com   | warmup_base=1 | warmup_max=2 | warmup_increase=0 | min_wait_time=3600 | warmup_days=62 | premium | passing | clean
sarah@pattrndata.com   | warmup_base=1 | warmup_max=2 | warmup_increase=0 | min_wait_time=3600 | warmup_days=62 | premium | passing | clean
```

The account-level conservative setup was corrected to `warmup_days=127` for all three while preserving the conservative caps and weekday campaign boundary:

```text
colin@pattrndata.co.uk | warmup_base=1 | warmup_max=2 | warmup_increase=0 | min_wait_time=3600 | warmup_days=127 | 08:00-20:00 | premium | passing | clean
james@pattrndata.com   | warmup_base=1 | warmup_max=2 | warmup_increase=0 | min_wait_time=3600 | warmup_days=127 | 08:00-20:00 | premium | passing | clean
sarah@pattrndata.com   | warmup_base=1 | warmup_max=2 | warmup_increase=0 | min_wait_time=3600 | warmup_days=127 | 08:00-20:00 | premium | passing | clean
```

Because the local task provider reads `tasks.scheduled_at` from the database (`local:<task_id>` handles), the existing one-pending-task-per-mailbox warmup tasks were moved from Monday morning to Sunday morning UTC rather than waiting until Monday:

```text
james@pattrndata.com   | ec8a732c-e155-4e7a-ad65-e2ef314098ca | pending | 2026-08-02 07:25:00+00 | local:ec8a732c-e155-4e7a-ad65-e2ef314098ca
colin@pattrndata.co.uk | 1b140a17-57d2-4cc6-8a5c-b741bc79a452 | pending | 2026-08-02 07:47:00+00 | local:1b140a17-57d2-4cc6-8a5c-b741bc79a452
sarah@pattrndata.com   | 7c159e91-068b-483e-b087-35f99a6bdf53 | pending | 2026-08-02 08:13:00+00 | local:7c159e91-068b-483e-b087-35f99a6bdf53
```

Post-change safety readback remained clean:

```text
active_campaigns=0
campaign_tasks_pending_active=0
warmup_dlq_due_now=0
due_pending_warmup_tasks=0
```

Follow-up proof cron jobs were moved from Monday to Sunday after the new first-task window: Sarah/Colin at `2026-08-02 10:00 UTC`, James at `2026-08-02 10:15 UTC`.

Current gate conclusion: the current three Pattrn Microsoft 365 shared-mailbox accounts are configured for conservative Monday-Sunday Warmbly warmup. Campaign sending remains weekday-only and all outbound prospect/cold campaign gates remain closed pending proof and explicit approval.

## Classic SaaS warmup ramp correction - 2026-08-01

Owner corrected the previous ultra-conservative cap assumption: Pattrn wants a classic warmup SaaS ramp for the current shared-mailbox warmup senders. Campaigns remain separately gated and weekday-only, but warmup should follow the standard SaaS-style ramp.

The current three Pattrn Microsoft 365 shared-mailbox accounts were updated from proof-only caps to classic warmup caps:

```text
colin@pattrndata.co.uk | warmup_base=10 | warmup_max=40 | warmup_increase=2 | min_wait_time=600 | warmup_days=127 | 08:00-20:00 | premium | passing | clean
james@pattrndata.com   | warmup_base=10 | warmup_max=40 | warmup_increase=2 | min_wait_time=600 | warmup_days=127 | 08:00-20:00 | premium | passing | clean
sarah@pattrndata.com   | warmup_base=10 | warmup_max=40 | warmup_increase=2 | min_wait_time=600 | warmup_days=127 | 08:00-20:00 | premium | passing | clean
```

Existing pending first warmup tasks were left in place so the local provider keeps the one-pending-task-per-mailbox chain:

```text
james@pattrndata.com   | ec8a732c-e155-4e7a-ad65-e2ef314098ca | pending | 2026-08-02 07:25:00+00 | local:ec8a732c-e155-4e7a-ad65-e2ef314098ca
colin@pattrndata.co.uk | 1b140a17-57d2-4cc6-8a5c-b741bc79a452 | pending | 2026-08-02 07:47:00+00 | local:1b140a17-57d2-4cc6-8a5c-b741bc79a452
sarah@pattrndata.com   | 7c159e91-068b-483e-b087-35f99a6bdf53 | pending | 2026-08-02 08:13:00+00 | local:7c159e91-068b-483e-b087-35f99a6bdf53
```

Post-change safety readback remained clean:

```text
active_campaigns=0
campaign_tasks_pending_active=0
warmup_dlq_due_now=0
due_pending_warmup_tasks=0
```

Operational note: with `warmup_base=10`, `warmup_increase=2`, and `warmup_max=40`, the scheduler will target roughly 10 warmup emails per mailbox per day at start, then ramp by 2/day until capped at 40/day, subject to eligible recipient capacity, health throttles, business window, jitter, partner diversity, and one-pending-task chaining.

Current gate conclusion: the current three Pattrn Microsoft 365 shared-mailbox accounts are configured for classic SaaS-style Monday-Sunday Warmbly warmup. Prospect/cold campaigns remain separate and closed pending explicit approval.

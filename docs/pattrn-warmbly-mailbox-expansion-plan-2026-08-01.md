# Pattrn Warmbly mailbox expansion plan - 2026-08-01

## Why this is now urgent

Warmbly's own warning is correct for the current state: a three-mailbox warmup pool is too small. With only Sarah, Colin, and James warming, the same accounts pair repeatedly. That looks less organic and limits useful ramping even though the individual mailbox caps are now set to a classic SaaS ramp.

## Read-only inventory performed

### Warmbly live DB

Current Pattrn warmup sender pool:

| Email | Provider | Status | Auth | Risk | Warmup | Base | Max | Ramp | Min wait | Pool | Role | Health | Spam |
|---|---|---|---|---|---|---:|---:|---:|---:|---|---|---|---:|
| `colin@pattrndata.co.uk` | outlook | active | passing | clean | on | 10 | 40 | 2/day | 600s | premium | sender_receiver | healthy | 0 |
| `james@pattrndata.com` | outlook | active | passing | clean | on | 10 | 40 | 2/day | 600s | premium | sender_receiver | healthy | 0 |
| `sarah@pattrndata.com` | outlook | active | passing | clean | on | 10 | 40 | 2/day | 600s | premium | sender_receiver | healthy | 0 |

Warmbly pool count: 3 healthy `sender_receiver` participants in `premium`.

Safety counters stayed clean: no active campaigns, no pending/active campaign tasks, no due warmup tasks, and no due warmup dead letters.

### Microsoft 365 Graph app-only read

The current app-only Graph tenant read sees exactly these three Pattrn mailbox users:

| Email | Display | Enabled | License count | Graph inbox read |
|---|---|---:|---:|---:|
| `colin@pattrndata.co.uk` | Colin / Pattrn Data | true | 0 | HTTP 200 |
| `james@pattrndata.com` | James / Pattrn Data | true | 1 | HTTP 200 |
| `sarah@pattrndata.com` | Sarah / Pattrn Data | true | 0 | HTTP 200 |

No additional `pattrndata.com`, `pattrndata.co.uk`, or `pattrndata.ai` users/mailboxes are currently visible to the Graph app.

## Expansion target

Move from 3 warming mailboxes to a believable first-stage pool of **20 active warmup senders**.

That means adding **17 more mailbox users/shared mailboxes** first, then adding more kiosk licences for a second expansion wave if needed.

Recommended staged targets:

1. **Now:** 20 total warmup mailboxes.
2. **After proof and health stability:** 30 total.
3. **Later:** 40 to 50 total if Warmbly Cloud/shared-pool capacity, domain health, and Microsoft sending signals remain clean.

## Microsoft 365 admin work needed

Create/provision the next mailbox batch in Microsoft 365 first. Each target mailbox should have:

- A real user/mailbox identity, not just an alias.
- A believable display name in the format `Name / Pattrn Data` or `Name | Pattrn Data`, matching the current sender pattern.
- Exchange mailbox provisioning complete before Warmbly onboarding.
- App-only Graph inbox read returning `HTTP 200` for `/users/{mailbox}/mailFolders/inbox`.
- Kiosk/shared-mailbox licensing assigned where required by the tenant setup.
- Password/auth/admin setup handled outside Warmbly. Do not store secrets in repo/docs/tracker.

## Warmbly onboarding gate for each new mailbox

For each newly created mailbox:

1. Confirm it appears in Graph user inventory.
2. Confirm `GET /users/{mailbox}/mailFolders/inbox` returns `HTTP 200`.
3. Onboard into Warmbly through the Outlook app-only path.
4. Verify Warmbly row: `provider=outlook`, `status=active`, `auth_state=passing`, `risk_band=clean`.
5. Join `premium` warmup pool as `recipient_only` first.
6. Promote to `sender_receiver` in small waves only after readback stays clean.
7. Apply the classic warmup caps:
   - `warmup_base=10`
   - `warmup_max=40`
   - `warmup_increase=2`
   - `min_wait_time=600`
   - `warmup_days=127`
   - `warmup_pool_type=premium`
8. Keep campaign sending closed until explicitly approved separately.

## Suggested wave sequence

### Wave 1: get to 10 total

Add 7 more mailboxes, onboard them as recipient-only first, then enable warmup after Graph read and Warmbly row health pass.

### Wave 2: get to 20 total

Add the next 10 mailboxes. Repeat the same read-only Graph gate, Warmbly onboarding gate, recipient-only holding state, then sender activation.

### Wave 3: optional 30 to 50

Only after a few days of clean warmup proof, add further kiosk licences and mailboxes toward 30, then 40 to 50.

## Gates that remain closed

This expansion plan does not approve:

- Prospect/cold campaign sends.
- Contact imports.
- CRM writes.
- LinkedIn/Unipile activity.
- DLQ replay.
- Any mailbox creation/licence purchase without Microsoft 365 admin approval.
- Warmbly Cloud shared-pool use without an explicit decision.

## Immediate next action

Microsoft 365 admin should create/provision the first additional mailbox batch. Once those mailboxes exist and are licensed/provisioned, rerun the Graph inventory and onboard only the mailboxes with inbox `HTTP 200`.

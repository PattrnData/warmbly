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

The business target remains the previously stated **4,000 emails/day** capacity, but only after warmup processing and delivery proof are working cleanly.

Mailbox math for that target:

| Safe per-mailbox daily volume | Mailboxes needed for 4,000/day | With 20% headroom |
|---:|---:|---:|
| 40/day | 100 | 120 |
| 50/day | 80 | 96 |

So the real operating target is **80 to 100 warmed mailboxes**, with **96 to 120 mailboxes** preferred if we want headroom, throttling room, sick-day capacity, and less pressure on any one account.

The 20-mailbox pool is only the first healthy processing milestone, not the final target.

Recommended staged targets:

1. **Proof gate:** keep the current 3 running long enough to prove scheduled warmup processing, worker send success, Graph Sent Items, and no DLQ/campaign side effects.
2. **First healthy pool:** 20 total warmup mailboxes.
3. **Scaling pool:** 40 to 50 total warmup mailboxes once the first pool stays clean.
4. **4,000/day capacity pool:** 80 to 100 total warmed mailboxes, or 96 to 120 with 20% headroom.

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

### Wave 0: prove processing with the current 3

Do not treat the 4,000/day plan as active until the scheduled warmup chain is proven. First prove task pickup, worker send-success logs, Graph Sent Items readback, no new DLQ entries, no unintended campaign activity, and healthy mailbox state for the current three.

### Wave 1: get to 10 total

Add 7 more mailboxes, onboard them as recipient-only first, then enable warmup after Graph read and Warmbly row health pass.

### Wave 2: get to 20 total

Add the next 10 mailboxes. This is the first believable processing pool and the first meaningful Warmbly health milestone.

### Wave 3: get to 40 to 50 total

Add 20 to 30 more mailboxes after a few days of clean processing and delivery proof. This is the ramp-validation pool.

### Wave 4: get to 80 to 100 total, preferably 96 to 120 with headroom

Only after Wave 3 is clean, add enough kiosk/shared-mailbox capacity to support the 4,000/day target. At 40/day this needs 100 warmed mailboxes, or 120 with 20% headroom. At 50/day this needs 80 warmed mailboxes, or 96 with 20% headroom.

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

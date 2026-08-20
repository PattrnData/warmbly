# Pattrn Warmbly James kiosk setup evidence — 2026-08-01

## Scope

Rohit approved getting the James kiosk licence path set up as the first proved unit before buying the other 13 kiosk licences for the 4,000/day model.

This evidence records the current James setup state only. It does not approve the 13-licence expansion, prospect/cold campaign sends, imports, CRM writes, LinkedIn/Unipile actions, DLQ replay, or cap increases beyond the current Warmbly policy.

## Live host and code state

Remote host path: `/home/hl-mailserver/projects/warmbly`

Readback at 2026-08-01:

- Warmbly branch: `feat/warmbly-twentycrm-approval-pack`
- Warmbly commit: `ca90d337`
- Docker services: backend, consumer, mailpit, nats, postgres, realtime, redis, tracking, web, worker all running
- Backend health: `HTTP 200`

## James kiosk licence proof

Microsoft Graph app-only proof from the live Warmbly backend env:

| Mailbox | `/users` | `/mailFolders/inbox` | Assigned licence count |
|---|---:|---:|---:|
| `james@pattrndata.com` | HTTP 200 | HTTP 200 | 1 |
| `sarah@pattrndata.com` | HTTP 200 | HTTP 200 | 0 |
| `colin@pattrndata.co.uk` | HTTP 200 | HTTP 200 | 0 |

Subscribed SKU readback:

| SKU | Enabled | Consumed |
|---|---:|---:|
| `EXCHANGEDESKLESS` | 1 | 1 |
| `SPB` | 1 | 1 |

Conclusion: James is the single licensed kiosk/delegate mailbox in the pilot set. Sarah and Colin remain shared mailboxes reachable by Graph without direct licence assignment.

## Warmbly account state

| Mailbox | Provider | Status | Auth | Risk | Worker assigned | Warmup enabled | Warmup caps | Pool role | Health | Spam score |
|---|---|---|---|---|---:|---:|---|---|---|---:|
| `james@pattrndata.com` | outlook | active | passing | clean | yes | yes | base 10, max 40, +2/day, 600s, days 127 | sender_receiver | healthy | 0 |
| `sarah@pattrndata.com` | outlook | active | passing | clean | yes | yes | base 10, max 40, +2/day, 600s, days 127 | sender_receiver | healthy | 0 |
| `colin@pattrndata.co.uk` | outlook | active | passing | clean | yes | yes | base 10, max 40, +2/day, 600s, days 127 | sender_receiver | healthy | 0 |

## Side-effect gates checked

| Gate | Result |
|---|---:|
| Active campaigns | 0 |
| Pending or active campaign tasks | 0 |
| Due pending warmup DLQ | 0 |

## Scheduled James proof task

James has a pending Warmbly warmup task:

| Sender | Task ID | Status | Scheduled at UTC | Message ID |
|---|---|---|---|---|
| `james@pattrndata.com` | `ec8a732c-e155-4e7a-ad65-e2ef314098ca` | pending | `2026-08-02 07:25:00+00` | empty before execution |

The final proof gate for James is intentionally after this task runs. The required proof triangle is:

1. Warmbly DB task completed with persisted RFC `Message-ID`.
2. Worker send-success log for the exact task/message.
3. Microsoft Graph Sent Items readback for `james@pattrndata.com` by exact `internetMessageId`.

A one-shot Hermes follow-up has been scheduled for `2026-08-02 08:45 Europe/London` to verify the proof triangle after the scheduled Warmbly task window.

## Expansion gate

Expansion remains closed until the James proof triangle passes. Once it passes, Rohit can buy the other 13 kiosk licences, then the next operator phase is to create the remaining licensed users and their 9 shared-mailbox sender identities each, aiming for:

- 14 kiosk licence paths total.
- 9 shared mailboxes per kiosk licence path.
- 140 total visible sender identities.
- 30/day per visible identity.
- 4,200/day modeled capacity with 200/day headroom over the 4,000/day target.

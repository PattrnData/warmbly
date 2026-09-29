# Warmbly pre-Monday full-fleet proof — 2026-08-13

## Evidence boundary

Owner approved repairing the single inactive Warmbly account row and proving warmup before Monday.

This proof intentionally did **not** approve or perform:

- provider sourcing
- lead imports
- CRM/TwentyCRM writes
- LinkedIn/Unipile actions
- campaign creation/enrollment/start
- prospect cold sends
- DLQ replay
- cap increases

## Repair performed

The only inactive row was `yara.bos@pattrndata.com`.

Pre-repair guards:

- inactive accounts: `1`
- Yara row ready: `status=inactive`, `auth_state=passing`, `risk_band=clean`, worker attached, warmup set, not paused, premium pool type
- Yara pool participant rows: `0`
- open campaign tasks: `0`
- active campaigns: `0`
- warmup due now before repair: `0`

Transaction result:

- activated rows: `1`
- inserted premium `sender_receiver` warmup pool participant rows: `1`
- seeded immediate Yara warmup task rows: `1`
- post active accounts: `140`
- post inactive accounts: `0`
- post `sender_receiver` pool participants: `140`

Yara seeded task:

- task id: `a00fcf69-0b9f-4c17-822a-fb9f024743fb`
- completed at: `2026-08-13 17:14:54.902776+00`
- persisted RFC Message-ID: present
- Microsoft Graph Sent Items exact `internetMessageId` match: `true`
- Graph sent time: `2026-08-13T17:14:54Z`

## Pre-Monday proof run

After the repair, existing future pending warmup rows were accelerated for a bounded proof window.
No new lead/campaign/prospect tasks were created.

Acceleration guards:

- active accounts: `140`
- inactive accounts: `0`
- healthy sender_receiver warmup pool participants: `140`
- target pending warmup tasks: `140`
- distinct target accounts: `140`
- open campaign tasks: `0`
- active campaigns: `0`
- due warmup now before acceleration: `0`

Acceleration result:

- updated existing pending warmup rows: `140`
- proof schedule window: `2026-08-13 17:17:05.307198+00 -> 2026-08-13 17:21:43.307198+00`

Completion readback:

- completed warmup rows in proof window: `140`
- completed rows with non-empty persisted RFC Message-ID: `140`
- distinct sender accounts completed: `140`
- Microsoft Graph Sent Items exact `internetMessageId` matches: `140/140`
- Graph failures: `0`

Domain breakdown from exact Graph matches:

- `pattrndata.com`: `49`
- `pattrndata.co.uk`: `46`
- `pattrndata.ai`: `45`

Sample exact Graph matches:

- first: `adam.ahmed@pattrndata.com` at `2026-08-13T17:17:07Z`
- last: `zoe.bell@pattrndata.ai` at `2026-08-13T17:21:44Z`

## Post-proof safety counters

- active accounts: `140`
- inactive accounts: `0`
- sender_receiver warmup pool participants: `140`
- open campaign tasks: `0`
- active campaigns: `0`
- pending warmup due now: `0`
- future pending warmup rows preserved for normal schedule: `140`
- next future pending window: `2026-08-16 07:32:17+00 -> 2026-08-16 08:59:56+00`

## Conclusion

Warmbly is now proven before Monday: all `140/140` sender accounts are active, in the healthy warmup pool, have a completed warmup task in the proof window with a persisted RFC Message-ID, and have exact Microsoft Graph Sent Items readback.

Campaign/lead/CRM/LinkedIn gates remain closed until separately approved.

# Pattrn Data Warmbly Outreach Launch Runbook — 2026-08-02

## Current verified base

This runbook assumes the verified Warmbly/M365 base from 2026-08-02:

- 140 active Outlook sender accounts in Warmbly.
- Domain split: `pattrndata.com`, `pattrndata.co.uk`, `pattrndata.ai`.
- 140/140 warmup enabled, `warmup_days=7`, warmup role `sender_receiver`.
- Campaign cap: `campaign_limit=30` on every sender.
- No active campaigns and no pending/active campaign tasks at handoff.
- App route available at `https://relay.pattrndata.io/`.
- Internal Docker service ports narrowed: DB/Redis/NATS/Mailpit loopback only; web/API/realtime/tracking LAN-bound for the relay.

## Safety gates before real leads/campaigns

Do not start a real prospect campaign until all of these are true:

1. Lead source is declared and approved.
2. Suppression sources are declared: existing customers, partners, competitors, unsubscribes, bounces, complaints, do-not-contact, personal contacts, and any manually excluded domains.
3. Imported leads have at least `email`, `first_name`, `last_name`, `company`, and the custom fields below.
4. Each segment has a clear offer, pain hypothesis, and qualification rule.
5. First campaign is created as draft, previewed, and tested internally before any external start.
6. Daily live sending starts below full capacity even though account cap is 30/day; recommended first external ramp is 1–3/day/sender, then evidence-based increases.

## Lead import contract

Warmbly contact API accepts:

```json
{
  "first_name": "...",
  "last_name": "...",
  "email": "...",
  "company": "...",
  "phone": "...",
  "campaigns": [],
  "categories": [],
  "custom_fields": {}
}
```

Use this canonical lead schema before import:

| Field | Required | Warmbly field | Notes |
|---|---:|---|---|
| first_name | yes | first_name | Human first name only. |
| last_name | yes | last_name | Human last name only. |
| email | yes | email | Deduplication key. |
| company | yes | company | Legal/trading name. |
| phone | no | phone | Only if sourced legitimately. |
| website | yes | custom_fields.website | Company site. |
| linkedin_url | recommended | custom_fields.linkedin_url | Person or company profile. |
| job_title | recommended | custom_fields.job_title | Keep concise. |
| segment | yes | custom_fields.segment | ICP segment slug. |
| source | yes | custom_fields.source | Lead source/vendor/manual list. |
| source_url | recommended | custom_fields.source_url | Evidence URL. |
| country | recommended | custom_fields.country | Useful for timezone/compliance. |
| industry | recommended | custom_fields.industry | Normalize values. |
| company_size | recommended | custom_fields.company_size | Bucket if exact unknown. |
| pain_hypothesis | yes | custom_fields.pain_hypothesis | One-line reason they may care. |
| trigger_event | recommended | custom_fields.trigger_event | Hiring, funding, tech stack, growth, etc. |
| personalization_1 | yes | custom_fields.personalization_1 | Specific observable fact. |
| personalization_2 | optional | custom_fields.personalization_2 | Backup detail. |
| offer_angle | yes | custom_fields.offer_angle | Message angle slug. |
| suppression_status | yes | custom_fields.suppression_status | `clear`, `suppressed`, or `needs_review`. |
| suppression_reason | if suppressed | custom_fields.suppression_reason | Why it cannot be contacted. |
| confidence | yes | custom_fields.confidence | `high`, `medium`, `low`. |
| reviewer | recommended | custom_fields.reviewer | Human/agent that approved. |

## Initial ICP segments

Start with tightly scoped Pattrn Data-relevant segments rather than broad scraping:

1. `sme_ops_dashboard_gap`
   - Target: SMEs where leadership likely lacks a live operating dashboard.
   - Pain: fragmented spreadsheets, no single KPI view, manual reporting.
   - Offer: rapid dashboard/data health audit.

2. `agency_client_reporting_gap`
   - Target: agencies/consultancies doing manual client reporting.
   - Pain: recurring reporting burden and inconsistent client visibility.
   - Offer: automated reporting/dashboard layer.

3. `founder_data_visibility_gap`
   - Target: founder-led businesses with visible growth/activity but weak BI stack signals.
   - Pain: decisions from gut/spreadsheets instead of reliable dashboards.
   - Offer: lightweight data visibility sprint.

Do not mix segments in one first campaign; make campaign metrics attributable.

## First campaign shape

Create one campaign per segment and keep it draft until reviewed.

Recommended first sequence:

1. Email 1: observation + pain hypothesis + soft CTA.
2. Email 2: useful asset/checklist + ask if relevant.
3. Email 3: short break-up / route-to-right-person.

Controls:

- `stop_on_reply=true`
- `unsubscribe_header=true`
- `text_only=true` for first pass unless branding requirement says otherwise
- no attachments in first pass
- no external starts until internal test path verifies message IDs, Sent Items, Unibox/reply classification
- first external live ramp: 1–3/day/sender, then increase only from bounce/reply/complaint evidence

## Hyper-personalization contract

Every outbound email should be explainable from fields on the contact row:

- `personalization_1`: concrete public fact.
- `pain_hypothesis`: why that fact implies a problem.
- `offer_angle`: which Pattrn Data offer applies.
- `source_url`: where the evidence came from.
- `confidence`: how strong the personalization is.

Reject personalization when it is generic, unverifiable, creepy, or based on sensitive personal attributes.

## Pre-import checklist

Before importing a batch:

1. Validate required fields are present.
2. Normalize email/domain casing.
3. Remove duplicate emails.
4. Check suppression lists.
5. Mark low-confidence rows as `needs_review` rather than importing into a live campaign.
6. Dry-run a sample of 10 rows into JSON payload shape.
7. Import only into Warmbly contacts/campaign draft, not TwentyCRM.

## First external campaign gate

Only after draft/internal proof:

- Campaign exists and is paused/draft.
- Senders assigned deliberately.
- Test email path verified internally.
- Microsoft Graph Sent Items proof works for campaign-generated mail.
- Reply lands in sender inbox and appears in Warmbly Unibox/reply-intent path.
- Unsubscribe/suppression behavior reviewed.
- User explicitly approves external start, segment, volume, and sender pool.

## Next operational move

The next work item is to choose or ingest a lead source and produce a first `100–250` lead segment as `clear`/`needs_review` rows, with no campaign start. Once a clean batch exists, create a draft campaign and generate personalized variants for review.

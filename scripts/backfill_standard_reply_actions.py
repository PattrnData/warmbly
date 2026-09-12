#!/usr/bin/env python3
"""Backfill standard reply-action scaffolds onto existing Warmbly campaigns.

Default mode is dry-run. Requires WARMBLY_API_BASE and WARMBLY_API_KEY only for
live API reads/writes. Does not start campaigns, import contacts, or send email.

The tool is intentionally API-level so every mutation goes through the same
sequence write validation as the product UI. It writes a JSON receipt containing
campaign ids, before/after action-node counts, and every API target touched.
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import time
from pathlib import Path
from typing import Any
from urllib import request, error

REPLY_FIELDS = [
    ("reply_unsubscribe", {"type": "unsubscribe"}),
    ("reply_positive", {"type": "create_task", "task_title": "Positive campaign reply: {{first_name}} {{last_name}}", "task_type": "email", "task_priority": "high"}),
    ("reply_question", {"type": "create_task", "task_title": "Reply needs answer: {{first_name}} {{last_name}}", "task_type": "email", "task_priority": "high"}),
    ("reply_wrong_person", {"type": "create_task", "task_title": "Wrong person/referral reply: {{first_name}} {{last_name}}", "task_type": "email", "task_priority": "medium"}),
    ("reply_referral", {"type": "create_task", "task_title": "Referral from campaign reply: {{first_name}} {{last_name}}", "task_type": "email", "task_priority": "high"}),
    ("reply_bad_timing", {"type": "create_task", "task_title": "Bad timing/later reply: {{first_name}} {{last_name}}", "task_type": "email", "task_priority": "medium"}),
    ("reply_automated", {"type": "fire_event", "event_name": "campaign.reply.automated"}),
    ("reply_negative", {"type": "fire_event", "event_name": "campaign.reply.negative"}),
]


def api(method: str, path: str, payload: dict[str, Any] | None = None) -> Any:
    base = os.environ.get("WARMBLY_API_BASE", "").rstrip("/")
    key = os.environ.get("WARMBLY_API_KEY", "")
    if not base or not key:
        raise SystemExit("WARMBLY_API_BASE and WARMBLY_API_KEY are required")
    data = None if payload is None else json.dumps(payload).encode()
    req = request.Request(base + path, data=data, method=method)
    req.add_header("Authorization", f"Bearer {key}")
    req.add_header("Content-Type", "application/json")
    try:
        with request.urlopen(req, timeout=30) as resp:
            raw = resp.read().decode()
            return json.loads(raw) if raw else None
    except error.HTTPError as exc:
        body = exc.read().decode(errors="replace")
        raise RuntimeError(f"{method} {path} failed {exc.code}: {body[:500]}") from exc


def campaign_pages(limit: int) -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    cursor = ""
    while len(out) < limit:
        path = f"/v1/campaigns?limit=100"
        if cursor:
            path += "&cursor=" + cursor
        page = api("GET", path)
        data = page.get("data", page if isinstance(page, list) else [])
        out.extend(data)
        nxt = page.get("pagination", {}).get("next_cursor") if isinstance(page, dict) else None
        if not nxt:
            break
        cursor = nxt
    return out[:limit]


def standard_branch(target: str, field: str) -> dict[str, Any]:
    return {"branch_id": "standard_" + field, "target_step_id": target, "conditions": [{"field": field, "operator": "ever"}]}


def backfill_campaign(campaign: dict[str, Any], execute: bool) -> dict[str, Any]:
    cid = campaign["id"]
    steps = api("GET", f"/v1/campaigns/{cid}/steps")
    action_nodes = [s for s in steps if s.get("kind") == "action"]
    email_steps = [s for s in steps if s.get("kind", "email") == "email"]
    has_standard = any(str(s.get("name", "")).startswith("Reply:") for s in action_nodes)
    receipt = {"campaign_id": cid, "name": campaign.get("name"), "status": campaign.get("status"), "before_action_nodes": len(action_nodes), "email_steps": len(email_steps), "skipped": has_standard or not email_steps, "writes": []}
    if receipt["skipped"]:
        return receipt
    if not execute:
        receipt["would_create_action_nodes"] = len(REPLY_FIELDS)
        receipt["would_update_email_steps"] = len(email_steps)
        return receipt

    targets: dict[str, str] = {}
    for field, action in REPLY_FIELDS:
        created = api("POST", f"/v1/campaigns/{cid}/steps")
        sid = created["id"]
        payload = {"name": "Reply: " + field.replace("reply_", "").replace("_", " "), "kind": "action", "action": action, "conditions": {"branches": [{"branch_id": "standard_reply_stop"}]}}
        api("PATCH", f"/v1/campaigns/{cid}/steps/{sid}", payload)
        targets[field] = sid
        receipt["writes"].append({"method": "PATCH", "path": f"/v1/campaigns/{cid}/steps/{sid}", "field": field})
    branches = [standard_branch(targets[field], field) for field, _ in REPLY_FIELDS]
    for step in email_steps:
        cond = step.get("conditions") or {}
        existing = cond.get("branches") or []
        existing_ids = {b.get("branch_id") for b in existing}
        merged = existing + [b for b in branches if b["branch_id"] not in existing_ids]
        api("PATCH", f"/v1/campaigns/{cid}/steps/{step['id']}", {"conditions": {"branches": merged}})
        receipt["writes"].append({"method": "PATCH", "path": f"/v1/campaigns/{cid}/steps/{step['id']}", "added_branches": len(merged) - len(existing)})
    after = api("GET", f"/v1/campaigns/{cid}/steps")
    receipt["after_action_nodes"] = len([s for s in after if s.get("kind") == "action"])
    return receipt


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--execute", action="store_true", help="perform writes; default is dry-run")
    ap.add_argument("--campaign-id", action="append", default=[])
    ap.add_argument("--limit", type=int, default=500)
    ap.add_argument("--receipt", default=f".runtime/standard-reply-action-backfill-{int(time.time())}.json")
    args = ap.parse_args()
    campaigns = [{"id": cid, "name": cid, "status": "selected"} for cid in args.campaign_id] if args.campaign_id else campaign_pages(args.limit)
    receipts = [backfill_campaign(c, args.execute) for c in campaigns]
    out = {"schema": "warmbly_standard_reply_action_backfill.v1", "mode": "execute" if args.execute else "dry_run", "campaigns_checked": len(campaigns), "campaigns_needing_backfill": sum(1 for r in receipts if not r.get("skipped")), "receipts": receipts}
    path = Path(args.receipt)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(out, indent=2), encoding="utf-8")
    print(json.dumps({k: out[k] for k in ("mode", "campaigns_checked", "campaigns_needing_backfill")}, sort_keys=True))
    print(path)
    return 0

if __name__ == "__main__":
    raise SystemExit(main())

#!/usr/bin/env python3
"""Regression tests for safe standard reply-action backfill execution."""
from __future__ import annotations

import importlib.util
from pathlib import Path

SCRIPT = Path(__file__).with_name("backfill_standard_reply_actions.py")
spec = importlib.util.spec_from_file_location("backfill_standard_reply_actions", SCRIPT)
assert spec and spec.loader
backfill = importlib.util.module_from_spec(spec)
spec.loader.exec_module(backfill)


def test_execute_records_post_creates_and_patch_writes(monkeypatch):
    calls: list[tuple[str, str, object]] = []

    def fake_api(method, path, payload=None):
        calls.append((method, path, payload))
        if method == "GET":
            if path.endswith("/steps"):
                return [{"id": "email-1", "kind": "email", "conditions": {"branches": []}}]
        if method == "POST":
            return {"id": f"action-{sum(1 for m, _, _ in calls if m == 'POST')}"}
        if method == "PATCH":
            return {"ok": True}
        raise AssertionError((method, path, payload))

    monkeypatch.setattr(backfill, "api", fake_api)

    receipt = backfill.backfill_campaign({"id": "campaign-1", "name": "C", "status": "active"}, execute=True)

    post_paths = [path for method, path, _ in calls if method == "POST"]
    receipt_posts = [w for w in receipt["writes"] if w["method"] == "POST"]
    assert len(post_paths) == len(backfill.REPLY_FIELDS)
    assert len(receipt_posts) == len(backfill.REPLY_FIELDS)
    assert all(w.get("created_step_id") for w in receipt_posts)


def test_partial_standard_scaffold_is_not_skipped(monkeypatch):
    def fake_api(method, path, payload=None):
        assert method == "GET"
        return [
            {"id": "email-1", "kind": "email", "conditions": {"branches": []}},
            {"id": "partial", "kind": "action", "name": "Reply: positive", "action": {"type": "create_task"}},
        ]

    monkeypatch.setattr(backfill, "api", fake_api)

    receipt = backfill.backfill_campaign({"id": "campaign-1", "name": "C", "status": "active"}, execute=False)

    assert receipt["skipped"] is False
    assert receipt["standard_complete"] is False
    assert receipt["would_create_action_nodes"] == len(backfill.REPLY_FIELDS) - 1


def test_branch_overflow_fails_closed_before_writes(monkeypatch):
    calls: list[tuple[str, str, object]] = []

    def fake_api(method, path, payload=None):
        calls.append((method, path, payload))
        if method == "GET":
            return [{
                "id": "email-1",
                "kind": "email",
                "conditions": {"branches": [{"branch_id": f"existing-{i}"} for i in range(13)]},
            }]
        raise AssertionError("writes must not happen before overflow preflight")

    monkeypatch.setattr(backfill, "api", fake_api)

    try:
        backfill.backfill_campaign({"id": "campaign-1", "name": "C", "status": "active"}, execute=True)
    except RuntimeError as exc:
        assert "branch limit" in str(exc)
    else:
        raise AssertionError("expected branch limit failure")

    assert [m for m, _, _ in calls] == ["GET"]


def test_execute_requires_explicit_campaign_ids(monkeypatch, tmp_path):
    monkeypatch.setenv("WARMBLY_API_BASE", "http://unused")
    monkeypatch.setenv("WARMBLY_API_KEY", "unused")
    monkeypatch.setattr(backfill.sys, "argv", [
        "backfill_standard_reply_actions.py",
        "--execute",
        "--receipt",
        str(tmp_path / "receipt.json"),
    ])

    try:
        backfill.main()
    except SystemExit as exc:
        assert "--execute requires one or more --campaign-id" in str(exc)
    else:
        raise AssertionError("execute without explicit campaign IDs must fail closed")

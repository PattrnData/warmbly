#!/usr/bin/env python3
"""Fail if GitHub Actions image prefixes can produce mixed-case GHCR names."""
from __future__ import annotations

from pathlib import Path

WORKFLOWS = [
    Path(".github/workflows/build-push.yml"),
    Path(".github/workflows/release.yml"),
]

bad: list[str] = []
for path in WORKFLOWS:
    text = path.read_text()
    if "IMAGE_PREFIX: ghcr.io/${{ github.repository_owner }}/warmbly" in text:
        bad.append(f"{path}: IMAGE_PREFIX uses mixed-case github.repository_owner directly")
    if "IMAGE_PREFIX: ghcr.io/${{ github.repository_owner_lower }}/warmbly" in text:
        bad.append(f"{path}: invalid github.repository_owner_lower expression")
    if "echo \"image_prefix=ghcr.io/${owner,,}/warmbly\"" not in text:
        bad.append(f"{path}: missing lowercase owner image_prefix output")
    if "${{ env.IMAGE_PREFIX }}" in text:
        bad.append(f"{path}: still references env.IMAGE_PREFIX")

    lines = text.splitlines()
    job_starts = []
    for i, line in enumerate(lines):
        if line.startswith("  ") and not line.startswith("    ") and line.strip().endswith(":") and not line.startswith("  #"):
            name = line.strip()[:-1]
            if name not in {"push", "workflow_dispatch"}:
                job_starts.append((i, name))
    for idx, (start, name) in enumerate(job_starts):
        end = job_starts[idx + 1][0] if idx + 1 < len(job_starts) else len(lines)
        block = "\n".join(lines[start:end])
        uses_prefix = "steps.image-prefix.outputs.image_prefix" in block
        defines_prefix = "id: image-prefix" in block
        if uses_prefix and not defines_prefix:
            bad.append(f"{path}: job {name} uses image-prefix output without defining the step")
        if block.count("id: image-prefix") > 1:
            bad.append(f"{path}: job {name} defines image-prefix more than once")

if bad:
    raise SystemExit("\n".join(bad))

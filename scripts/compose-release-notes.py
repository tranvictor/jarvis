#!/usr/bin/env python3
"""Draft compact GitHub release notes from commits since the last tag.

One bullet per PR (or unique commit). PR bodies and raw commit dumps are
omitted so the draft is readable as-is; edit in the editor if you want.
"""
from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.request
from typing import Optional

PR_RE = re.compile(r"\(#(\d+)\)\s*$")
MERGE_RE = re.compile(r"Merge pull request #(\d+)\b")
REPO_RE = re.compile(
    r"(?:github\.com[:/])(?P<owner>[^/]+)/(?P<repo>[^/.]+)(?:\.git)?$"
)

# (section heading, keyword regex) — first match wins.
SECTIONS: list[tuple[str, re.Pattern[str]]] = [
    (
        "Fixes",
        re.compile(
            r"\b(fix|fixes|fixed|bug|hang|overflow|error)\b",
            re.I,
        ),
    ),
    (
        "Packaging",
        re.compile(
            r"\b(release|goreleaser|homebrew|brew|scoop|apt|yum|nfpms|"
            r"linux-386|windows 32|cgo|makefile)\b",
            re.I,
        ),
    ),
    (
        "Internal",
        re.compile(
            r"(refactor|deduplicat\w*|share[sd]?\b|split\b|delete dead|"
            r"unused|inject\b|cleanup|tidy|dead code)",
            re.I,
        ),
    ),
]


def run(args: list[str]) -> str:
    return subprocess.check_output(args, text=True).rstrip("\n")


def repo_slug() -> tuple[str, str]:
    url = run(["git", "remote", "get-url", "origin"])
    m = REPO_RE.search(url)
    if not m:
        sys.exit(f"compose-release-notes: origin is not a GitHub remote: {url}")
    return m.group("owner"), m.group("repo")


def github_get(path: str, token: str) -> Optional[dict]:
    req = urllib.request.Request(
        f"https://api.github.com{path}",
        headers={
            "Authorization": f"Bearer {token}",
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
            "User-Agent": "jarvis-release",
        },
    )
    try:
        with urllib.request.urlopen(req) as resp:
            return json.load(resp)
    except urllib.error.HTTPError:
        return None
    except urllib.error.URLError:
        return None


def commits_since(tag: str) -> list[tuple[str, str]]:
    raw = run(["git", "log", f"{tag}..HEAD", "--format=%h%x1f%s"])
    out: list[tuple[str, str]] = []
    for line in raw.splitlines():
        if not line.strip():
            continue
        sha, subject = (line.split("\x1f", 1) + [""])[:2]
        out.append((sha.strip(), subject.strip()))
    return out


def pr_number(subject: str) -> Optional[str]:
    m = PR_RE.search(subject)
    if m:
        return m.group(1)
    m = MERGE_RE.search(subject)
    if m:
        return m.group(1)
    return None


def is_noise(subject: str) -> bool:
    s = subject.strip().lower()
    if s.startswith("merge "):
        return True
    if re.match(r"^(pump|bump) versions?\b", s):
        return True
    return False


def classify(title: str) -> str:
    for heading, pat in SECTIONS:
        if pat.search(title):
            return heading
    return "What's new"


def format_bullet(title: str, pr: Optional[str]) -> str:
    title = PR_RE.sub("", title).strip().rstrip(".")
    if pr:
        return f"- {title} (#{pr})"
    return f"- {title}"


def main() -> int:
    if len(sys.argv) != 3:
        print(
            "usage: compose-release-notes.py <last-tag> <output-file>",
            file=sys.stderr,
        )
        return 2

    last_tag, dest = sys.argv[1], sys.argv[2]
    token = os.environ.get("GITHUB_TOKEN", "").strip()
    owner, repo = repo_slug()
    commits = commits_since(last_tag)

    items: list[tuple[str, Optional[str]]] = []
    seen_prs: set[str] = set()
    seen_titles: set[str] = set()

    for _sha, subject in commits:
        if is_noise(subject):
            continue
        n = pr_number(subject)
        title = subject
        if n:
            if n in seen_prs:
                continue
            seen_prs.add(n)
            if token:
                pr = github_get(f"/repos/{owner}/{repo}/pulls/{n}", token)
                if pr and pr.get("title"):
                    title = pr["title"].strip()
        key = PR_RE.sub("", title).strip().rstrip(".").lower()
        if key in seen_titles:
            continue
        seen_titles.add(key)
        items.append((title, n))

    grouped: dict[str, list[str]] = {}
    order = ["What's new", "Fixes", "Packaging", "Internal"]
    for title, pr in items:
        grouped.setdefault(classify(title), []).append(format_bullet(title, pr))

    lines = [
        "<!-- Edit this draft, save, and quit. HTML comments are stripped. -->",
        "",
    ]
    any_section = False
    for heading in order:
        bullets = grouped.get(heading) or []
        if not bullets:
            continue
        any_section = True
        lines.append(f"## {heading}")
        lines.append("")
        lines.extend(bullets)
        lines.append("")

    if not any_section:
        lines.extend(["## What's new", "", "- ", ""])

    text = "\n".join(lines).rstrip() + "\n"
    with open(dest, "w", encoding="utf-8") as f:
        f.write(text)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

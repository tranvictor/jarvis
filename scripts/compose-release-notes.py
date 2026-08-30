#!/usr/bin/env python3
"""Draft GitHub release notes from commits since the last tag.

Pulls PR titles and bodies when GITHUB_TOKEN is set so the draft is verbose
enough to edit down, rather than a raw subject list.
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
CURSOR_PR_BODY = re.compile(
    r"<!--\s*CURSOR_AGENT_PR_BODY_BEGIN\s*-->\s*(.*?)\s*<!--\s*CURSOR_AGENT_PR_BODY_END\s*-->",
    re.S,
)
HTML_COMMENT = re.compile(r"<!--.*?-->", re.S)
HTML_FOOTER = re.compile(r"<div\b.*?</div>", re.S)
REPO_RE = re.compile(
    r"(?:github\.com[:/])(?P<owner>[^/]+)/(?P<repo>[^/.]+)(?:\.git)?$"
)


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


def commits_since(tag: str) -> list[tuple[str, str, str]]:
    raw = run(
        [
            "git",
            "log",
            f"{tag}..HEAD",
            "--format=%h%x1f%s%x1f%b%x1e",
        ]
    )
    out: list[tuple[str, str, str]] = []
    for entry in raw.split("\x1e"):
        entry = entry.strip("\n")
        if not entry:
            continue
        sha, subject, body = (entry.split("\x1f", 2) + ["", ""])[:3]
        out.append((sha.strip(), subject.strip(), body.strip()))
    return out


def pr_number(subject: str) -> Optional[str]:
    m = PR_RE.search(subject)
    if m:
        return m.group(1)
    m = MERGE_RE.search(subject)
    if m:
        return m.group(1)
    return None


def clean_pr_body(body: str) -> str:
    m = CURSOR_PR_BODY.search(body)
    if m:
        body = m.group(1)
    body = HTML_COMMENT.sub("", body)
    body = HTML_FOOTER.sub("", body)
    return re.sub(r"\n{3,}", "\n\n", body).strip()


def clean_commit_body(body: str) -> str:
    lines = []
    for line in body.splitlines():
        if line.lower().startswith("co-authored-by:"):
            continue
        if set(line.strip()) <= set("-") and len(line.strip()) >= 5:
            continue
        lines.append(line)
    return "\n".join(lines).strip()


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

    prs: list[tuple[str, str, str]] = []
    others: list[tuple[str, str, str]] = []
    seen_prs: set[str] = set()

    for sha, subject, body in commits:
        n = pr_number(subject)
        if n and token:
            if n in seen_prs:
                continue
            seen_prs.add(n)
            pr = github_get(f"/repos/{owner}/{repo}/pulls/{n}", token)
            if pr and pr.get("title"):
                title = pr["title"].strip()
                pr_body = clean_pr_body(pr.get("body") or "")
                prs.append((n, title, pr_body))
                continue
        others.append((sha, subject, clean_commit_body(body)))

    lines = [
        "<!-- Edit this draft, save, and quit. HTML comments are stripped before publish.",
        "     Delete ## Commits since ... if you don't want the raw log on GitHub. -->",
        "",
        "## What's new",
        "",
    ]
    if prs:
        for n, title, pr_body in prs:
            lines.append(f"### {title} (#{n})")
            lines.append("")
            if pr_body:
                lines.append(pr_body)
                lines.append("")
            else:
                lines.append("_No PR description._")
                lines.append("")
    if others:
        heading = "### Other changes" if prs else None
        if heading:
            lines.append(heading)
            lines.append("")
        for sha, subject, body in others:
            lines.append(f"- {subject} (`{sha}`)")
            if body:
                for bline in body.splitlines():
                    lines.append(f"  {bline}")
            lines.append("")

    if not prs and not others:
        lines.append("- ")
        lines.append("")

    lines.extend(
        [
            f"## Commits since {last_tag}",
            "",
        ]
    )
    if commits:
        for sha, subject, body in commits:
            lines.append(f"* {sha} {subject}")
            if body:
                lines.append("")
                lines.append(body)
                lines.append("")
    else:
        lines.append("_No commits since the last tag._")
        lines.append("")

    text = "\n".join(lines).rstrip() + "\n"
    with open(dest, "w", encoding="utf-8") as f:
        f.write(text)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

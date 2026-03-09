import os
import signal
import threading
from typing import Any, cast
from urllib.parse import quote

import relaymesh_githook as githook_sdk
from relaymesh_githook import (
    Listener,
    New,
    WithConcurrency,
    WithClientProvider,
    WithEndpoint,
    WithListener,
    WithLogger,
    GitLabClient,
    BitbucketClient,
    GitHubClient,
    NewRemoteSCMClientProvider,
)

stop = threading.Event()


def shutdown(_signum, _frame):
    stop.set()


signal.signal(signal.SIGINT, shutdown)
signal.signal(signal.SIGTERM, shutdown)

endpoint = os.getenv("GITHOOK_ENDPOINT", "https://relaymesh.vercel.app/api/connect")
rule_id = os.getenv("GITHOOK_RULE_ID", "85101e9f-3bcf-4ed0-b561-750c270ef6c3")


def int_from_env(name, fallback):
    raw = (os.getenv(name) or "").strip()
    if not raw:
        return fallback
    try:
        value = int(raw)
    except Exception:
        return fallback
    return value if value > 0 else fallback


concurrency = int_from_env("GITHOOK_CONCURRENCY", 4)
retry_count = int_from_env("GITHOOK_RETRY_COUNT", 1)


class ExampleLogger:
    def printf(self, fmt, *args):
        rendered = fmt
        for arg in args:
            rendered = rendered.replace("%s", str(arg), 1)
        print(f"example-worker {rendered}")

    def Printf(self, fmt, *args):
        self.printf(fmt, *args)


class ExampleListener(Listener):
    def on_message_start(self, ctx, evt):
        print(
            f"listener start log_id={evt.metadata.get('log_id', '')} provider={evt.provider} topic={evt.topic}"
        )

    def on_message_finish(self, ctx, evt, err=None):
        status = "failed" if err else "success"
        print(
            f"listener finish log_id={evt.metadata.get('log_id', '')} status={status} err={err or ''}"
        )

    def on_error(self, ctx, evt, err):
        log_id = ""
        provider = ""
        if evt is not None:
            provider = evt.provider
            log_id = evt.metadata.get("log_id", "")
        print(f"listener error log_id={log_id} provider={provider} err={err}")


options = [
    WithEndpoint(endpoint),
    WithClientProvider(NewRemoteSCMClientProvider()),
    WithConcurrency(concurrency),
    WithLogger(ExampleLogger()),
    WithListener(ExampleListener()),
]
with_retry_count = getattr(githook_sdk, "WithRetryCount", None)
if callable(with_retry_count):
    retry_option = cast(Any, with_retry_count(retry_count))
    wk = New(*options, retry_option)
else:
    wk = New(*options)


def repository_from_event(evt):
    normalized = getattr(evt, "normalized", None)
    if not normalized or not isinstance(normalized, dict):
        return "", ""
    repo_value = normalized.get("repository")
    if not isinstance(repo_value, dict):
        return "", ""
    full_name = repo_value.get("full_name", "")
    if isinstance(full_name, str) and "/" in full_name:
        parts = full_name.strip().split("/", 1)
        if len(parts) == 2 and parts[0] and parts[1]:
            return parts[0], parts[1]
    name = repo_value.get("name", "")
    owner_map = repo_value.get("owner")
    owner = owner_map.get("login", "") if isinstance(owner_map, dict) else ""
    return str(owner).strip(), str(name).strip()


def first_line(s):
    s = str(s).strip()
    idx = s.find("\n")
    return s[:idx] if idx >= 0 else s


def handle(ctx, evt):
    provider_name = (evt.provider or "").strip().lower()
    print(
        f"handler topic={evt.topic} provider={provider_name} type={evt.type} retry_count={retry_count} concurrency={concurrency}"
    )

    if provider_name == "github":
        gh = GitHubClient(evt)
        if not gh:
            print("github client not available (installation may not be configured)")
            return
        owner, repo = repository_from_event(evt)
        if not owner or not repo:
            print("repository info missing in payload; skipping github read")
            return
        try:
            commits = gh.request_json("GET", f"/repos/{owner}/{repo}/commits?per_page=5")
            if not isinstance(commits, list):
                commits = []
            print(f"github commits count={len(commits)}")
            for i, c in enumerate(commits):
                sha = str(c.get("sha", ""))[:7] if isinstance(c, dict) else ""
                commit_obj = c.get("commit", {}) if isinstance(c, dict) else {}
                msg = first_line(commit_obj.get("message", "") if isinstance(commit_obj, dict) else "")
                print(f"  commit[{i + 1}] sha={sha} message={msg}")
        except Exception as err:
            print(f"github list commits failed owner={owner} repo={repo} err={err}")
        return

    if provider_name == "gitlab":
        gl = GitLabClient(evt)
        if not gl:
            print("gitlab client not available (installation may not be configured)")
            return
        owner, repo = repository_from_event(evt)
        if not owner or not repo:
            print("repository info missing in payload; skipping gitlab read")
            return
        try:
            project = quote(f"{owner}/{repo}", safe="")
            commits = gl.request_json("GET", f"/projects/{project}/repository/commits?per_page=5")
            if not isinstance(commits, list):
                commits = []
            print(f"gitlab commits count={len(commits)}")
            for i, c in enumerate(commits):
                sha = str(c.get("short_id", "")) if isinstance(c, dict) else ""
                msg = first_line(c.get("title", "") if isinstance(c, dict) else "")
                print(f"  commit[{i + 1}] sha={sha} message={msg}")
        except Exception as err:
            print(f"gitlab list commits failed err={err}")
        return

    if provider_name == "bitbucket":
        bb = BitbucketClient(evt)
        if not bb:
            print("bitbucket client not available (installation may not be configured)")
            return
        owner, repo = repository_from_event(evt)
        if not owner or not repo:
            print("repository info missing in payload; skipping bitbucket read")
            return
        try:
            result = bb.request_json("GET", f"/repositories/{owner}/{repo}/commits?pagelen=5")
            values = result.get("values", []) if isinstance(result, dict) else []
            if not isinstance(values, list):
                values = []
            print(f"bitbucket commits count={len(values)}")
            for i, c in enumerate(values):
                sha = str(c.get("hash", ""))[:7] if isinstance(c, dict) else ""
                msg = first_line(c.get("message", "") if isinstance(c, dict) else "")
                print(f"  commit[{i + 1}] sha={sha} message={msg}")
        except Exception as err:
            print(f"bitbucket list commits failed err={err}")
        return

    print(f"unsupported provider={provider_name}; skipping scm call")


wk.HandleRule(rule_id, handle)

wk.Run(stop)

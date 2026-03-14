import json
import urllib.error
import urllib.request
from dataclasses import dataclass
from typing import Dict, Optional, Protocol


@dataclass
class HTTPResponse:
    status: int
    headers: Dict[str, str]
    body: bytes

    def text(self) -> str:
        return self.body.decode("utf-8", errors="replace")

    def json(self) -> object:
        if not self.body:
            return {}
        return json.loads(self.body.decode("utf-8"))


class SCMClient(Protocol):
    def request(
        self,
        method: str,
        path: str,
        body: Optional[object] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> HTTPResponse: ...

    def request_json(
        self,
        method: str,
        path: str,
        body: Optional[object] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> object: ...


class GitHubClient:
    def __init__(self, token: str, base_url: str) -> None:
        self.token = token
        self.base_url = base_url

    def request(
        self,
        method: str,
        path: str,
        body: Optional[object] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> HTTPResponse:
        merged = {
            "Authorization": f"Bearer {self.token}",
            "Accept": "application/vnd.github+json",
            "Content-Type": "application/json",
        }
        if headers:
            merged.update(headers)
        return _request(self.base_url, method, path, body, merged)

    def request_json(
        self,
        method: str,
        path: str,
        body: Optional[object] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> object:
        resp = self.request(method, path, body, headers)
        if resp.status >= 300:
            raise RuntimeError(f"github request failed ({resp.status}): {resp.text()}")
        return resp.json()


class GitLabClient:
    def __init__(self, token: str, base_url: str) -> None:
        self.token = token
        self.base_url = base_url

    def request(
        self,
        method: str,
        path: str,
        body: Optional[object] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> HTTPResponse:
        merged = {
            "Authorization": f"Bearer {self.token}",
            "Content-Type": "application/json",
        }
        if headers:
            merged.update(headers)
        return _request(self.base_url, method, path, body, merged)

    def request_json(
        self,
        method: str,
        path: str,
        body: Optional[object] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> object:
        resp = self.request(method, path, body, headers)
        if resp.status >= 300:
            raise RuntimeError(f"gitlab request failed ({resp.status}): {resp.text()}")
        return resp.json()


class BitbucketClient:
    def __init__(self, token: str, base_url: str) -> None:
        self.token = token
        self.base_url = base_url

    def request(
        self,
        method: str,
        path: str,
        body: Optional[object] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> HTTPResponse:
        merged = {
            "Authorization": f"Bearer {self.token}",
            "Content-Type": "application/json",
        }
        if headers:
            merged.update(headers)
        return _request(self.base_url, method, path, body, merged)

    def request_json(
        self,
        method: str,
        path: str,
        body: Optional[object] = None,
        headers: Optional[Dict[str, str]] = None,
    ) -> object:
        resp = self.request(method, path, body, headers)
        if resp.status >= 300:
            raise RuntimeError(
                f"bitbucket request failed ({resp.status}): {resp.text()}"
            )
        return resp.json()


def GitHubClientFromEvent(evt: object) -> Optional[GitHubClient]:
    client = getattr(evt, "client", None)
    if isinstance(client, GitHubClient):
        return client
    return None


def GitLabClientFromEvent(evt: object) -> Optional[GitLabClient]:
    client = getattr(evt, "client", None)
    if isinstance(client, GitLabClient):
        return client
    return None


def BitbucketClientFromEvent(evt: object) -> Optional[BitbucketClient]:
    client = getattr(evt, "client", None)
    if isinstance(client, BitbucketClient):
        return client
    return None


def new_provider_client(provider: str, token: str, base_url: str) -> SCMClient:
    normalized = (provider or "").strip().lower()
    if normalized == "github":
        return GitHubClient(token, _resolve_api_base(base_url, normalized))
    if normalized == "gitlab":
        return GitLabClient(token, _resolve_api_base(base_url, normalized))
    if normalized == "bitbucket":
        return BitbucketClient(token, _resolve_api_base(base_url, normalized))
    raise ValueError(f"unsupported provider for scm client: {provider}")


def NewProviderClient(provider: str, token: str, base_url: str) -> SCMClient:
    return new_provider_client(provider, token, base_url)


def _resolve_api_base(base_url: str, provider: str) -> str:
    trimmed = (base_url or "").strip()
    if trimmed:
        return trimmed
    if provider == "github":
        return "https://api.github.com"
    if provider == "gitlab":
        return "https://gitlab.com/api/v4"
    if provider == "bitbucket":
        return "https://api.bitbucket.org/2.0"
    return trimmed


def _resolve_url(base_url: str, path: str) -> str:
    if not path:
        return base_url
    if path.startswith("http://") or path.startswith("https://"):
        return path
    normalized_base = (base_url or "").rstrip("/")
    normalized_path = path if path.startswith("/") else f"/{path}"
    return f"{normalized_base}{normalized_path}"


def _request(
    base_url: str,
    method: str,
    path: str,
    body: Optional[object],
    headers: Dict[str, str],
) -> HTTPResponse:
    url = _resolve_url(base_url, path)
    data = json.dumps(body).encode("utf-8") if body is not None else None
    req = urllib.request.Request(url, data=data, method=(method or "GET").upper())
    for key, value in headers.items():
        req.add_header(key, value)
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            payload = resp.read()
            return HTTPResponse(
                status=resp.status, headers=dict(resp.headers), body=payload
            )
    except urllib.error.HTTPError as err:
        raw = err.read() if err.fp else b""
        return HTTPResponse(status=err.code, headers=dict(err.headers or {}), body=raw)

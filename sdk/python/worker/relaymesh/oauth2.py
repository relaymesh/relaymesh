import json
import time
import urllib.parse
import urllib.request
from dataclasses import dataclass
from typing import List, Optional

from .context import WorkerContext


@dataclass
class OAuth2Config:
    enabled: bool = True
    issuer: str = ""
    audience: str = ""
    required_scopes: Optional[List[str]] = None
    required_roles: Optional[List[str]] = None
    required_groups: Optional[List[str]] = None
    mode: str = ""
    client_id: str = ""
    client_secret: str = ""
    scopes: Optional[List[str]] = None
    redirect_url: str = ""
    authorize_url: str = ""
    token_url: str = ""
    jwks_url: str = ""


class _TokenCache:
    def __init__(self) -> None:
        self.token = ""
        self.expires_at = 0.0
        self.key = ""


_token_cache = _TokenCache()


def resolve_oauth2_config(explicit: Optional[OAuth2Config]) -> Optional[OAuth2Config]:
    if explicit is not None:
        _normalize_oauth2_mode(explicit.mode)
        return explicit
    token_url = _env_value("GITHOOK_OAUTH2_TOKEN_URL")
    if not token_url:
        return None
    return OAuth2Config(
        enabled=True,
        mode="client_credentials",
        token_url=token_url,
        client_id=_env_value("GITHOOK_OAUTH2_CLIENT_ID"),
        client_secret=_env_value("GITHOOK_OAUTH2_CLIENT_SECRET"),
        scopes=_split_csv(_env_value("GITHOOK_OAUTH2_SCOPES")),
        audience=_env_value("GITHOOK_OAUTH2_AUDIENCE"),
    )


def oauth2_token_from_config(
    ctx: Optional[WorkerContext], cfg: Optional[OAuth2Config]
) -> str:
    if cfg is None or cfg.enabled is False:
        return ""
    _normalize_oauth2_mode(cfg.mode)
    token_url = (cfg.token_url or "").strip()
    client_id = (cfg.client_id or "").strip()
    client_secret = (cfg.client_secret or "").strip()
    if not token_url or not client_id or not client_secret:
        return ""
    cache_key = _build_cache_key(cfg)
    now = time.time()
    if (
        _token_cache.token
        and _token_cache.key == cache_key
        and _token_cache.expires_at > now + 30
    ):
        return _token_cache.token

    body = {
        "grant_type": "client_credentials",
        "client_id": client_id,
        "client_secret": client_secret,
    }
    if cfg.scopes:
        body["scope"] = " ".join(cfg.scopes)
    if cfg.audience:
        body["audience"] = cfg.audience

    data = urllib.parse.urlencode(body).encode("utf-8")
    req = urllib.request.Request(token_url, data=data, method="POST")
    req.add_header("Content-Type", "application/x-www-form-urlencoded")
    with urllib.request.urlopen(req, timeout=10) as resp:
        payload = json.loads(resp.read().decode("utf-8"))
    token = str(payload.get("access_token", "")).strip()
    if not token:
        return ""
    expires_in = int(payload.get("expires_in", 1800))
    _token_cache.token = token
    _token_cache.key = cache_key
    _token_cache.expires_at = now + expires_in
    return token


def _build_cache_key(cfg: OAuth2Config) -> str:
    scopes = " ".join(cfg.scopes or [])
    return "|".join(
        [
            (cfg.token_url or "").strip(),
            (cfg.client_id or "").strip(),
            scopes,
            (cfg.audience or "").strip(),
        ]
    )


def _env_value(key: str) -> str:
    import os

    return (os.getenv(key) or "").strip()


def _split_csv(value: str) -> List[str]:
    return [entry.strip() for entry in value.split(",") if entry.strip()]


def _normalize_oauth2_mode(mode: str) -> str:
    value = (mode or "").strip().lower()
    if value in ("", "auto", "client_credentials"):
        return "client_credentials"
    raise ValueError(f"unsupported oauth2 mode for worker sdk: {mode}")

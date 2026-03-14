import datetime
import json
import urllib.error
import urllib.request
from dataclasses import dataclass
from typing import Any, Dict, List, Optional

from .context import WorkerContext
from .oauth2 import OAuth2Config, oauth2_token_from_config, resolve_oauth2_config


@dataclass
class RuleRecord:
    id: str
    when: str
    emit: List[str]
    driver_id: str


@dataclass
class DriverRecord:
    id: str
    name: str
    config_json: str
    enabled: bool


@dataclass
class InstallationRecord:
    provider: str
    account_id: str
    account_name: str
    installation_id: str
    provider_instance_key: str
    enterprise_id: str = ""
    enterprise_slug: str = ""
    enterprise_name: str = ""
    access_token: str = ""
    refresh_token: str = ""
    expires_at: Optional[datetime.datetime] = None


@dataclass
class SCMClientRecord:
    provider: str
    api_base_url: str
    access_token: str
    provider_instance_key: str
    expires_at: Optional[datetime.datetime] = None


@dataclass
class APIClientOptions:
    base_url: str
    api_key: str = ""
    oauth2_config: Optional[OAuth2Config] = None
    tenant_id: str = ""
    timeout: float = 10.0


class RulesClient:
    def __init__(self, opts: APIClientOptions) -> None:
        self.opts = opts

    def list_rules(self, ctx: Optional[WorkerContext] = None) -> List[RuleRecord]:
        payload = _post_json(self.opts, "/cloud.v1.RulesService/ListRules", {}, ctx)
        raw_rules = _read_array(payload, "rules")
        return [
            RuleRecord(
                id=_read_string(record, "id"),
                when=_read_string(record, "when"),
                emit=_read_string_array(record, "emit"),
                driver_id=_read_string(record, "driver_id", "driverId"),
            )
            for record in raw_rules
        ]

    def get_rule(self, rule_id: str, ctx: Optional[WorkerContext] = None) -> RuleRecord:
        trimmed = (rule_id or "").strip()
        if not trimmed:
            raise ValueError("rule id is required")
        payload = _post_json(
            self.opts, "/cloud.v1.RulesService/GetRule", {"id": trimmed}, ctx
        )
        record = _read_object(payload, "rule")
        if not record:
            raise ValueError(f"rule not found: {trimmed}")
        return RuleRecord(
            id=_read_string(record, "id"),
            when=_read_string(record, "when"),
            emit=_read_string_array(record, "emit"),
            driver_id=_read_string(record, "driver_id", "driverId"),
        )


class DriversClient:
    def __init__(self, opts: APIClientOptions) -> None:
        self.opts = opts

    def list_drivers(self, ctx: Optional[WorkerContext] = None) -> List[DriverRecord]:
        payload = _post_json(self.opts, "/cloud.v1.DriversService/ListDrivers", {}, ctx)
        raw_drivers = _read_array(payload, "drivers")
        return [
            DriverRecord(
                id=_read_string(record, "id"),
                name=_read_string(record, "name"),
                config_json=_read_string(record, "config_json", "configJson"),
                enabled=_read_bool(record, "enabled"),
            )
            for record in raw_drivers
        ]

    def get_driver_by_id(
        self, driver_id: str, ctx: Optional[WorkerContext] = None
    ) -> Optional[DriverRecord]:
        trimmed = (driver_id or "").strip()
        if not trimmed:
            raise ValueError("driver id is required")
        drivers = self.list_drivers(ctx)
        for record in drivers:
            if (record.id or "").strip() == trimmed:
                return record
        return None


class EventLogsClient:
    def __init__(self, opts: APIClientOptions) -> None:
        self.opts = opts

    def update_status(
        self,
        log_id: str,
        status: str,
        error_message: str = "",
        ctx: Optional[WorkerContext] = None,
    ) -> None:
        trimmed = (log_id or "").strip()
        if not trimmed:
            raise ValueError("log id is required")
        status_val = (status or "").strip()
        if not status_val:
            raise ValueError("status is required")
        _post_json(
            self.opts,
            "/cloud.v1.EventLogsService/UpdateEventLogStatus",
            {
                "log_id": trimmed,
                "status": status_val,
                "error_message": (error_message or "").strip(),
            },
            ctx,
        )


class InstallationsClient:
    def __init__(self, opts: APIClientOptions) -> None:
        self.opts = opts

    def get_by_installation_id(
        self,
        provider: str,
        installation_id: str,
        ctx: Optional[WorkerContext] = None,
    ) -> Optional[InstallationRecord]:
        trimmed_provider = (provider or "").strip()
        trimmed_id = (installation_id or "").strip()
        if not trimmed_provider:
            raise ValueError("provider is required")
        if not trimmed_id:
            raise ValueError("installation_id is required")
        payload = _post_json(
            self.opts,
            "/cloud.v1.InstallationsService/GetInstallationByID",
            {"provider": trimmed_provider, "installation_id": trimmed_id},
            ctx,
        )
        record = _read_object(payload, "installation")
        if not record:
            return None
        return _normalize_installation(record)


class SCMClientsClient:
    def __init__(self, opts: APIClientOptions) -> None:
        self.opts = opts

    def get_scm_client(
        self,
        provider: str,
        installation_id: str,
        provider_instance_key: str = "",
        ctx: Optional[WorkerContext] = None,
    ) -> SCMClientRecord:
        trimmed_provider = (provider or "").strip()
        trimmed_id = (installation_id or "").strip()
        if not trimmed_provider:
            raise ValueError("provider is required")
        if not trimmed_id:
            raise ValueError("installation_id is required")
        payload = _post_json(
            self.opts,
            "/cloud.v1.SCMService/GetSCMClient",
            {
                "provider": trimmed_provider,
                "installation_id": trimmed_id,
                "provider_instance_key": (provider_instance_key or "").strip(),
            },
            ctx,
        )
        record = _read_object(payload, "client")
        if not record:
            raise ValueError("scm client missing in response")
        return _normalize_scm_client(record)


def resolve_endpoint(explicit: str) -> str:
    trimmed = (explicit or "").strip()
    if trimmed:
        return trimmed.rstrip("/")
    env_endpoint = _env_value("GITHOOK_ENDPOINT")
    if env_endpoint:
        return env_endpoint
    env_base = _env_value("GITHOOK_API_BASE_URL")
    if env_base:
        return env_base
    return "http://localhost:8080"


def resolve_api_key(explicit: str) -> str:
    trimmed = (explicit or "").strip()
    if trimmed:
        return trimmed
    return _env_value("GITHOOK_API_KEY")


def resolve_tenant_id(explicit: str) -> str:
    trimmed = (explicit or "").strip()
    if trimmed:
        return trimmed
    return _env_value("GITHOOK_TENANT_ID")


def _normalize_installation(record: Dict[str, Any]) -> InstallationRecord:
    return InstallationRecord(
        provider=_read_string(record, "provider"),
        account_id=_read_string(record, "account_id", "accountId"),
        account_name=_read_string(record, "account_name", "accountName"),
        installation_id=_read_string(record, "installation_id", "installationId"),
        provider_instance_key=_read_string(
            record, "provider_instance_key", "providerInstanceKey"
        ),
        enterprise_id=_read_string(record, "enterprise_id", "enterpriseId"),
        enterprise_slug=_read_string(record, "enterprise_slug", "enterpriseSlug"),
        enterprise_name=_read_string(record, "enterprise_name", "enterpriseName"),
        access_token=_read_string(record, "access_token", "accessToken"),
        refresh_token=_read_string(record, "refresh_token", "refreshToken"),
        expires_at=_parse_datetime(record, "expires_at", "expiresAt"),
    )


def _normalize_scm_client(record: Dict[str, Any]) -> SCMClientRecord:
    return SCMClientRecord(
        provider=_read_string(record, "provider"),
        api_base_url=_read_string(record, "api_base_url", "apiBaseUrl"),
        access_token=_read_string(record, "access_token", "accessToken"),
        provider_instance_key=_read_string(
            record, "provider_instance_key", "providerInstanceKey"
        ),
        expires_at=_parse_datetime(record, "expires_at", "expiresAt"),
    )


def _post_json(
    opts: APIClientOptions,
    path: str,
    body: Dict[str, Any],
    ctx: Optional[WorkerContext],
) -> Dict[str, Any]:
    base = resolve_endpoint(opts.base_url)
    if not base:
        raise ValueError("base url is required")
    url = f"{base}{path}"
    headers: Dict[str, str] = {"Content-Type": "application/json"}
    _apply_auth_headers(headers, opts, ctx)
    tenant_id = (ctx.tenant_id if ctx else "") or (opts.tenant_id or "")
    if tenant_id:
        headers["X-Tenant-ID"] = tenant_id

    data = json.dumps(body or {}).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers=headers, method="POST")
    timeout = opts.timeout if opts.timeout and opts.timeout > 0 else 10.0
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            payload = resp.read()
    except urllib.error.HTTPError as err:
        raw = err.read() if err.fp else b""
        message = raw.decode("utf-8", errors="replace")
        raise RuntimeError(f"api request failed ({err.code}): {message}") from err
    except urllib.error.URLError as err:
        raise RuntimeError(f"api request failed: {err}") from err

    if not payload:
        return {}
    try:
        decoded = json.loads(payload.decode("utf-8"))
        if isinstance(decoded, dict):
            return decoded
    except Exception:
        return {}
    return {}


def _apply_auth_headers(
    headers: Dict[str, str], opts: APIClientOptions, ctx: Optional[WorkerContext]
) -> None:
    api_key = resolve_api_key(opts.api_key)
    if api_key:
        headers["X-API-Key"] = api_key
        return
    cfg = opts.oauth2_config or resolve_oauth2_config(None)
    token = oauth2_token_from_config(ctx, cfg)
    if token:
        headers["Authorization"] = f"Bearer {token}"


def _parse_datetime(record: Dict[str, Any], *keys: str) -> Optional[datetime.datetime]:
    for key in keys:
        value = record.get(key)
        if isinstance(value, str) and value:
            try:
                normalized = value.replace("Z", "+00:00")
                dt = datetime.datetime.fromisoformat(normalized)
                if dt.tzinfo is None:
                    dt = dt.replace(tzinfo=datetime.timezone.utc)
                return dt
            except ValueError:
                continue
        if isinstance(value, dict) and "seconds" in value:
            try:
                seconds = int(value.get("seconds"))
                return datetime.datetime.fromtimestamp(
                    seconds, tz=datetime.timezone.utc
                )
            except Exception:
                continue
    return None


def _read_string(record: Dict[str, Any], *keys: str) -> str:
    for key in keys:
        value = record.get(key)
        if isinstance(value, str):
            return value
    return ""


def _read_string_array(record: Dict[str, Any], *keys: str) -> List[str]:
    for key in keys:
        value = record.get(key)
        if isinstance(value, list):
            return [str(entry) for entry in value if entry is not None]
    return []


def _read_bool(record: Dict[str, Any], *keys: str) -> bool:
    for key in keys:
        value = record.get(key)
        if isinstance(value, bool):
            return value
    return False


def _read_array(record: Dict[str, Any], *keys: str) -> List[Dict[str, Any]]:
    for key in keys:
        value = record.get(key)
        if isinstance(value, list):
            out: List[Dict[str, Any]] = []
            for entry in value:
                if isinstance(entry, dict):
                    out.append(entry)
            return out
    return []


def _read_object(record: Dict[str, Any], *keys: str) -> Optional[Dict[str, Any]]:
    for key in keys:
        value = record.get(key)
        if isinstance(value, dict):
            return value
    return None


def _env_value(key: str) -> str:
    import os

    return (os.getenv(key) or "").strip()

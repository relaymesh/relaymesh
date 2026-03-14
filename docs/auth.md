# API Authentication (OAuth2/OIDC)

relaymesh validates OAuth2/OIDC JWTs on all Connect RPC endpoints. Webhooks and OAuth callbacks remain public.

## Server configuration

Minimal server config:

```yaml
auth:
  oauth2:
    enabled: true
    issuer: https://<your-idp>/oauth2/default
    audience: api://githook
```

- `issuer` is required for auto-discovery.
- `audience` is required unless your IdP omits `aud` checks.
- If you prefer manual endpoints, set `jwks_url`, `authorize_url`, and `token_url`.

## CLI behavior

The CLI reads `auth.oauth2` from `--config` (default: `config.yaml`) and applies auth automatically:

1) If `GITHOOK_API_KEY` is set, the CLI uses `x-api-key` and skips OAuth2.
2) If OAuth2 is enabled, the CLI uses a cached access token if available.
3) If no cached token exists:
   - `mode: client_credentials` or `mode: auto` → the CLI requests a token and caches it.
   - `mode: auth_code` → run `githook auth` to login and cache a token.
4) The CLI sends `Authorization: Bearer <token>` for all RPC calls.

The token cache lives under your OS cache directory, for example:
`~/.cache/github.com/relaymesh/relaymesh/token.json`.

## CLI config examples

Client credentials:

```yaml
endpoint: http://localhost:8080
auth:
  oauth2:
    enabled: true
    issuer: https://<your-idp>/oauth2/default
    audience: api://githook
    mode: client_credentials
    client_id: ${OAUTH_CLIENT_ID}
    client_secret: ${OAUTH_CLIENT_SECRET}
```

Auth code (browser login):

```yaml
endpoint: http://localhost:8080
auth:
  oauth2:
    enabled: true
    issuer: https://<your-idp>/oauth2/default
    audience: api://githook
    mode: auth_code
    client_id: ${OAUTH_CLIENT_ID}
    client_secret: ${OAUTH_CLIENT_SECRET}
    # redirect_url defaults to https://app.example.com/auth/callback
```

Run:

```bash
githook auth
```

This opens a browser, completes login via a local loopback callback, and stores the token for later CLI calls.

## Server login endpoints

When OAuth2 is enabled, the server exposes:

- `GET /auth/login` (login redirect)
- `GET /auth/callback` (auth_code callback)

## Optional fields

- `required_scopes`: scopes enforced by the server.
- `required_groups`, `required_roles`: claims enforced by the server.
- `scopes`: defaults to `openid profile email` for auth_code.

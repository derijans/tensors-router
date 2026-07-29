# Security and Reverse Proxy

## Intended network scope

The router is intended for intranet use. Do not expose it directly to the public internet.

Prefer a loopback or private-interface bind, restrictive CIDR rules, and a VPN or authenticated reverse proxy for remote access.

## Security profiles

### Secure

`secure` is the default profile.

Inference keys authorize model APIs. Admin keys authorize management routes, benchmarks, load, unload, and shutdown. The cluster token is accepted only under `/router/v1/node/...`.

A non-loopback secure router bind requires at least one inference key and one admin key. A non-loopback secure WebUI bind requires `server.admin_token`.

Startup rejects missing router configuration files and known placeholder credential values.

### Trusted LAN

`trusted_lan` skips router inference and administration checks and skips WebUI login and CSRF checks.

It does not disable cluster authentication, CIDR checks, managed backend isolation, header filtering, body limits, or transport memory limits.

Use this profile only on a network where every permitted client is trusted with administration access.

## Credential separation

Use independent values for:

- router inference keys
- router admin keys
- WebUI admin token
- cluster token

Do not reuse example or placeholder values. Do not put credentials in public URLs, documentation, or `.kcpps` files.

## CIDR enforcement

`server.allowed_cidrs` evaluates the direct peer address. The router does not use `X-Forwarded-For` to make access-control decisions.

When a reverse proxy is used, allow the proxy address and enforce original-client policy at the proxy.

## Managed backend isolation

Managed backend URLs must resolve to loopback addresses. Backend `extra_args` cannot override managed host and port values.

The management WebUI and backend interfaces use different listeners and origins. Cross-origin backend UI access uses temporary tickets rather than forwarding management credentials.

## Reverse proxy

Example Nginx route for model APIs:

```nginx
location /v1/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_buffering off;
    proxy_read_timeout 600s;
    proxy_send_timeout 600s;
}
```

Streaming uses server-sent events. Disable response buffering and allow timeouts long enough for model loading and generation.

If the router is mounted under a path prefix, strip that prefix before forwarding.

Terminate public TLS at the reverse proxy. Keep the router on loopback or a private interface and apply authentication at both layers when the deployment requires it.

## WebUI certificates

The WebUI serves HTTPS directly. Configure certificate files for a trusted certificate, or allow it to generate a local self-signed certificate.

Use `server.cert_hosts` for browser-facing names or addresses that cannot be inferred from local interfaces. Use `server.backend_ui_public_url` when the backend UI listener has a different external origin due to NAT or proxying.

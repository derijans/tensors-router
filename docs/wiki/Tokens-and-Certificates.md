# Tokens and Certificates

Each router keeps its tokens and TLS certificates in `store_dir/credentials.sqlite` (mode `0600`). This file is separate from the router database, so `GET /router/v1/site/store/download` never includes it. Manage it with `tensors-router security ...` or the WebUI **Security** tab. Changes reach a running router within two seconds. You do not need to restart the router for token changes or certificate rotation.

## Tokens

| Role | Authorizes |
|---|---|
| `inference` | model APIs |
| `admin` | site APIs, benchmarks, load, unload, shutdown |
| `metrics` | only `GET /router/v1/metrics` (admin tokens work there as well) |
| `cluster` | only `/router/v1/node/...` (router-to-router calls) |

A generated token is shown once and stored only as a SHA-256 hash. A cluster token is also stored in readable form, because this router sends it to its peers. Every router in a cluster must hold the same cluster token. Create the token on one router. On each other router, give it with `--value-file` (CLI) or **Existing value** (WebUI).

The router does not let you delete the last token of a role that the configuration requires:

- inference and admin tokens on a non-loopback `secure` bind
- an admin token when MCP is enabled
- a cluster token on a master or slave

The same rules decide whether the router starts.

```bash
tensors-router security token create --role admin --name ops --config config.yaml
tensors-router security token show --name mesh --config config.yaml           # cluster tokens only
openssl rand -hex 32 > mesh.txt
tensors-router security token create --role cluster --name mesh --value-file mesh.txt
```

### Migration from config.yaml

The first time the router or the CLI opens the credential store, it imports these values once:

- `auth.inference_keys`, `auth.admin_keys`, `auth.metrics_keys` and the deprecated `auth.bearer_keys`
- `cluster.token`
- the `TENSORS_ROUTER_{INFERENCE,ADMIN,CLUSTER}_TOKEN[_FILE]` environment variables

The imported entries get names such as `migrated-admin-1`. After the import, the router ignores those settings and warns at every start while they are still present. Remove them from the configuration when you have checked the imported tokens.

## Certificates

| Kind | Holds a key | Purpose |
|---|---|---|
| `identity` | yes | the certificate this router serves over HTTPS and presents to peers; one is active |
| `authority` | yes | a local CA that issues identities for other routers |
| `trusted_authority` | no | a CA whose identities this router trusts, for example your own CA |
| `pinned_peer` | no | one peer certificate trusted directly, for self-signed identities |

When an identity is active at startup, the router listener serves HTTPS only. The router then requires every caller of `/router/v1/node/...` to present a client certificate that verifies against this router's trust anchors, in addition to the cluster token. Trust anchors are the trusted and local CAs, the pinned peers, and the router's own identity. Peer calls verify the server against the same anchors and never against the system roots, unless no anchor is configured. Activate an identity at runtime and the router serves it on the next handshake. A restart is needed only to switch the listener between HTTP and HTTPS. The Security tab shows when a restart is needed.

Identities need both server and client authentication usage. Their host names and IP addresses must cover the host in the URL that peers use (`cluster.public_url`, `cluster.master_url`, `cluster.slave_urls`). Change those URLs to `https://`. The router warns at startup about peer URLs that still use `http`.

### With your own CA

On each router:

```bash
tensors-router security cert import --kind trusted_authority --name my-ca --cert my-ca.pem
tensors-router security cert import --kind identity --name node1 --cert node1-chain.pem --key node1.key --activate
```

### With the built-in CA

On one router:

```bash
tensors-router security ca create --name cluster-ca
tensors-router security ca issue --ca cluster-ca --name node1 --host node1.lan --install --activate
tensors-router security ca issue --ca cluster-ca --name node2 --host node2.lan --out-cert node2.crt.pem --out-key node2.key.pem
tensors-router security cert export --name cluster-ca --out cluster-ca.pem
```

Copy `node2.*.pem` and `cluster-ca.pem` to node2 and import them there:

```bash
tensors-router security cert import --kind trusted_authority --name cluster-ca --cert cluster-ca.pem
tensors-router security cert import --kind identity --name node2 --cert node2.crt.pem --key node2.key.pem --activate
```

The CA private key never leaves the router that created it.

### Without a CA

On each router, generate a self-signed identity, export its certificate, and pin it on every peer:

```bash
tensors-router security cert generate --name node1 --host node1.lan --activate
tensors-router security cert export --name node1 --out node1.pem
# on node2:
tensors-router security cert import --kind pinned_peer --name node1 --cert node1.pem
```

## WebUI to router

A managed router (empty `router.url`) writes the certificates that verify its listener to `server.state_dir/router-trust.pem` (`serve --tls-trust-bundle`). The WebUI then connects over `https://127.0.0.1:8080` and trusts only that bundle. For an external router, set `router.url` to its `https://` address and `router.ca_file` to a bundle written by `tensors-router security trust-bundle --out`.

Private keys are stored in plain text, protected only by file permissions (`0600`, directory `0700`). Keep `store_dir` on a disk that only the router account can read.

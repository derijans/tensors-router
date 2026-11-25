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
| `identity` | yes | the certificate this router serves to cluster peers over HTTPS and presents to them as a client; one is active |
| `api_certificate` | yes | the certificate this router serves to API clients and browsers on its API port; one is active, never a trust anchor for peers |
| `authority` | yes | a local CA that issues identities for other routers |
| `trusted_authority` | no | a CA whose identities this router trusts, for example your own CA |
| `pinned_peer` | no | one peer certificate trusted directly, for self-signed identities |

When an identity is active at startup, the listener that serves cluster peers uses HTTPS. That is `server.bind`, or `cluster.bind` when it is set (see [API certificate and cluster port](#api-certificate-and-cluster-port)). The router then requires every caller of `/router/v1/node/...` to present a client certificate that verifies against this router's trust anchors, in addition to the cluster token. Trust anchors are the trusted and local CAs, the pinned peers, and the router's own identity. Peer calls verify the server against the same anchors and never against the system roots, unless no anchor is configured. Activate an identity or an API certificate at runtime and the router serves it on the next handshake. A restart is needed only to switch a listener between HTTP and HTTPS. The Security tab shows one notice per listener and says when a restart is needed.

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

### Certificate source

Every certificate shows where it came from in `source`:

| `source` | Meaning |
|---|---|
| `created` | made on this router: a generated self-signed identity or a CA |
| `supplied` | imported by you, or a signed certificate you installed for a [certificate request](#certificate-requests) of this router |
| `migrated` | imported once from certificate files named in the configuration (see [Migration](#migration)) |
| `issued` | issued by this router's own CA and installed on this router |
| `master` | signed by the cluster master's CA and installed on this router by the master (see [Managing slaves from the master](#managing-slaves-from-the-master)) |

## API certificate and cluster port

An API certificate is the certificate that API clients and browsers see on the router's API port. To let clients connect without extra trust setup, import a certificate signed by a CA your clients already trust, for example the CA of your pfSense firewall, together with its private key:

```bash
tensors-router security cert import --kind api_certificate --name api --cert api-chain.pem --key api.key --activate
```

In the WebUI, use **Import** with kind **API certificate** and turn on **Activate when the certificate is an API certificate or a router identity**. The certificate must be a leaf certificate that allows server authentication. Client authentication is not required. One API certificate and one identity can be active at the same time. **Activate** and **Delete** work as they do for identities: deleting the active one needs `--force` (CLI) or confirmation in the WebUI. An API certificate is never a trust anchor for peers.

To keep the private key on the router instead of importing it, use a [certificate request](#certificate-requests).

### Listeners

`cluster.bind` is opt-in. It is allowed only for `master` and `slave`, and it must differ from `server.bind`.

| `cluster.role` | `cluster.bind` | Listeners |
|---|---|---|
| `standalone` | not allowed | `server.bind` is the API port and serves everything |
| `master` or `slave` | empty | `server.bind` serves API clients and cluster peers with the identity |
| `master` or `slave` | set | `server.bind` is the API port and answers `404` on `/router/v1/node/...`; `cluster.bind` serves only `/router/v1/node/...` with the identity and requires a verified client certificate (mutual TLS) |

When `cluster.bind` is set, `cluster.public_url`, `cluster.master_url` and `cluster.slave_urls` must point at the `cluster.bind` port of each router. With a shared port the router keeps serving its identity there, so an active API certificate is not served. The router warns about this at startup on a master or slave, and the Security tab shows a notice.

### Which certificate a listener serves

The router decides at startup. The API port serves:

1. the active API certificate, when there is one
2. otherwise, with `security.profile: secure`, the active identity
3. otherwise plain HTTP (`trusted_lan`, or no identity)

The cluster port, and a shared port, use HTTPS with the identity whenever an identity is active, and plain HTTP otherwise. Switching a listener between HTTP and HTTPS needs a restart. Rotating a certificate does not.

### Migration

The router imports these files once and ignores them after that:

- `server.tls_cert_file` and `server.tls_key_file` in the router configuration, imported as the API certificate `migrated-router-config`. The router warns at every start while they are still set.
- a WebUI that manages its router (empty `router.url`) passes its own `server.cert_file` and `server.key_file` at launch (`serve --import-api-cert F --import-api-key F`), imported as the API certificate `migrated-webui`

An imported certificate is activated when no API certificate is active yet.

### WebUI listener

A managed router exports the active API certificate chain and its private key to `server.state_dir/api-certificate.pem` (mode `0600`, `serve --api-certificate-export F`). The WebUI serves that certificate on its listeners and reloads it when the file changes. When no API certificate is active, the router removes the file and the WebUI falls back to `server.cert_file` and `server.key_file`, or to its generated certificate. The WebUI is intended to run only on the master.

## Certificate requests

A certificate request lets a CA sign a certificate for this router while the private key stays on the router. Use it for an API certificate from the CA your clients already trust, for example the pfSense certificate manager, or for an identity from your own CA.

1. In the **Security** tab, create a certificate request. Pick the kind (**API certificate** or **Router identity**), a name, and the host names and IP addresses the certificate must cover. The router creates the key, keeps it as a pending key, and shows the request (CSR) in PEM form.
2. Sign the request with your CA. An identity needs both server and client authentication usage, an API certificate needs server authentication.
3. Install the signed certificate, with its chain, on the same router and choose whether to activate it. The router matches it to the pending key, stores it with source `supplied`, and removes the pending key.

A certificate that matches no pending key on the router is refused, so only the exact request you created can be installed. The signed certificate must meet the same rules as an imported certificate of that kind. The name must not be used by another certificate or pending request. Pending requests stay listed until a signed certificate is installed or you delete the request.

## Cluster setup wizard

On a master, the **Security** tab has a **Cluster setup** button in its header. It opens the **Cluster security setup** card. The wizard does the cluster setup for the master and every slave from the master's WebUI. The browser talks only to the master, which relays each action to the slaves (see [Managing slaves from the master](#managing-slaves-from-the-master)).

Each step shows a table with one row per router: **Router**, **State** and **Details**. The table shows the result of the last run. A router that is already done shows **Done**, and the step skips it. You can run every step again safely with **Run step**. **Check again** reloads the security summary of every router.

1. **Cluster token.** The step creates a cluster token on the master when it has none. A slave that the master can reach already holds the master's cluster token, because every node route requires it, so the step shows that slave as done. A slave that the master cannot reach shows the error. Give that slave the master's cluster token by hand once (**Create token**, role **Cluster**, **Existing value**), then click **Check again**.
2. **Certificate authority.** Pick a local CA or a trusted CA stored on the master, for example a CA you imported from pfSense. Or create a new local CA (the name field defaults to `cluster-ca`). The wizard preselects the CA that issued the master's active identity.
3. **Router identities.** For every router, the master included, the router creates the key and a certificate request, the chosen local CA signs it, and the router installs and activates the certificate. A router is done when its active identity was issued by the chosen CA.
4. **API certificates.** A checkbox per router selects the routers that get an API certificate from the chosen CA. A router that already has an active API certificate is unchecked by default and keeps it, for example a certificate imported from the configuration or the WebUI files (source `migrated`). A router is done when its active API certificate was issued by the chosen CA, or when it keeps its own.
5. **Trust.** The step installs the chosen CA as **Trusted CA** on every slave. The master sends only the CA certificate. A slave is done when it already holds a CA with the same fingerprint.
6. **Configuration.** Each router has a **Cluster port** field. The default is the current `cluster.bind` port, or else the `server.bind` port plus 10. The step shows the exact `cluster:` block to put in that router's `config.yaml`: `bind`, `public_url` (`https://` on the cluster port), `master_url` on slaves (the master's new `public_url`) and `slave_urls` on the master (the new `public_url` of every slave). It lists each setting that changes with its current and new value. Below the blocks, the table shows the security profile of each router. **Run step** switches every router that still runs `trusted_lan` to the secure profile, after one confirmation. The master switches through its local profile route, and a slave through its node route. The same blockers apply as for **Switch to secure** on a single router (see [Cluster setup in the WebUI](#cluster-setup-in-the-webui)), for example a non-loopback bind without inference and admin tokens. A router with a blocker shows **Blocked** with the reason and is not switched. The WebUI never writes cluster settings to `config.yaml` itself.
7. **Restart and health.** Restart the slaves first, then the master. The router cannot restart a slave remotely. Restart each slave on its own host: stop it, for example with `POST /router/v1/shutdown` and an admin token, then start it with its configuration. Restart the master last. For a router managed by the WebUI, use **Restart** in the top bar. **Check again** shows the health of each router from its security summary: whether it is reachable, whether each listener (API port, cluster port) serves HTTPS or HTTP or needs a restart, and the profile (secure, or restart required). A router is healthy when it is reachable, every listener serves HTTPS without a pending restart, and the profile is secure.

### With a CA that has no key on the master

A trusted CA, for example one imported from pfSense, has no private key on the master. With such a CA in step 2, **Run step** in steps 3 and 4 creates only the certificate requests. Each router row then offers **Download request** and **Install signed**. Sign each request with your CA, then install the signed certificate with **Install signed**. See [Certificate requests](#certificate-requests).

## Cluster setup in the WebUI

On a master, the [Cluster setup wizard](#cluster-setup-wizard) does these steps for the whole cluster. The manual steps below stay for reference.

Do these steps in the **Security** tab of each router's WebUI. The example uses the built-in CA on the master. With your own CA, skip steps 2 to 4 and import your CA as **Trusted CA** and each router's own certificate and key as **Router identity** instead. Once a slave holds the cluster token, the master can do the certificate steps for that slave from its own Security tab; see [Managing slaves from the master](#managing-slaves-from-the-master).

### Master

1. **Create token**, role **Cluster**, leave **Existing value** empty. Click **Show** on the new token and copy the secret.
2. **Create CA**, for example `cluster-ca`.
3. On the `cluster-ca` row click **Issue**. Name the identity `master`, list the master's host names and IP addresses, turn on **Install on this router instead of downloading** and **Activate after installing**.
4. For each slave click **Issue** on `cluster-ca` again. Name it after the slave, list the host names and IP addresses that appear in the master's `cluster.slave_urls`, and leave both toggles off. Click **Download certificate** and **Download key**. The key is not kept on the master.
5. Click **Download** on the `cluster-ca` row. This file holds only the CA certificate.
6. In `config.yaml` set `cluster.bind` to the port for cluster peers, for example `0.0.0.0:8090`. Change `cluster.public_url` and every `cluster.slave_urls` entry to `https://` and point them at the `cluster.bind` port of each router. Restart the router.

### Each slave

1. **Create token**, role **Cluster**, paste the master's secret into **Existing value**.
2. **Import** the `cluster-ca` certificate with kind **Trusted CA** and no private key.
3. **Import** the slave's certificate and key from step 4 of the master with kind **Router identity**, and turn on **Activate when the certificate is a router identity**.
4. In `config.yaml` set `cluster.bind` to the port for cluster peers. Change `cluster.public_url` and `cluster.master_url` to `https://`, with `cluster.public_url` on this slave's `cluster.bind` port and `cluster.master_url` on the master's `cluster.bind` port. Restart the router.

When a router with certificates still runs `security.profile: trusted_lan`, the Security tab warns that model and admin APIs accept requests without a token and offers **Switch to secure**. The router then rewrites `security.profile` in its `config.yaml`, and the change takes effect after a restart. The button stays hidden while the router could not start with the secure profile, for example on a non-loopback bind without inference and admin tokens. After the switch, set `router.token` in `webui.yaml` to an admin token so the WebUI keeps access.

The Security tab shows one notice per listener. A notice turns green when its listener serves HTTPS. If the WebUI talks to an external router (`router.url` set), point `router.ca_file` at a bundle from `tensors-router security trust-bundle --out` when that router switches to HTTPS.

## Managing slaves from the master

On a master, the **Security** tab has a node selector. The master's own entry shows its tokens and certificates as before. Pick a slave and the tab shows the same tables for that slave, and every action runs on the slave. The browser talks only to the master, which relays each action over the cluster connection.

From the master you can, on each slave:

- see its tokens (without secrets), certificates, pending certificate requests, listeners and security profile
- create and delete tokens
- push the cluster token: the master sends its own cluster token, and the slave stores it unless it already holds the same secret
- trust the master's CA: pick a CA or trusted CA stored on the master, and the master sends only its certificate to the slave as a **Trusted CA**. Any other CA or pinned peer certificate can be added from PEM.
- issue an identity or an API certificate from a CA on the master, for the host names and IP addresses you list, and optionally activate it
- create and delete certificate requests, and install a certificate signed by another CA, as in [Certificate requests](#certificate-requests)
- generate a self-signed identity
- activate, download and delete certificates (downloads contain the certificate only)
- switch it to the secure profile: the slave rewrites `security.profile` in its `config.yaml`, with the same blockers as on the slave itself. The request must carry a cluster token that the slave accepts, so the master keeps access after the switch. The change takes effect after a restart.

Changes to `cluster.bind` and the peer URLs in a slave's `config.yaml`, and restarts, are still done on the slave itself. `security.profile` can be switched from the master.

Private keys never leave a node. When the master issues a certificate for a slave, the slave creates the key and a certificate request, the master signs the request with its CA, and the slave installs the signed certificate with source `master`. If signing or installing fails, the master removes the request from the slave again. No node route accepts or returns a private key.

The master reaches a slave the same way it does for routing: the slave must hold the master's cluster token, and when the slave serves HTTPS to its peers it must trust the master's identity. The Security tab shows an error for a slave the master cannot reach.

### API certificate state per node

Every node reports its API certificate in the cluster inventory: whether one is active, its name, source, expiry and fingerprint. On the master, the Nodes view and the Security node selector flag every node that has no API certificate, or one that expires within 30 days.

### Who can change a slave's certificates

Whoever holds the cluster token can replace a slave's certificates and tokens. This matches what the cluster token already allows, for example writing a slave's model configurations. When `cluster.bind` is set, the slave also requires a client certificate it trusts, so a caller needs both the cluster token and an identity issued by a CA the slave trusts (or pinned on the slave). Protect the cluster token like an admin token.

## WebUI to router

A managed router (empty `router.url`) writes the certificates that verify its listeners to `server.state_dir/router-trust.pem` (`serve --tls-trust-bundle`). The bundle includes the active API certificate. The WebUI then connects over `https://127.0.0.1:8080` and verifies the router's certificate chain against that bundle only, without checking the host name, because a public API certificate usually does not list `127.0.0.1`. The managed router also exports its active API certificate for the WebUI listeners; see [WebUI listener](#webui-listener). For an external router, set `router.url` to its `https://` address and `router.ca_file` to a bundle written by `tensors-router security trust-bundle --out`. The WebUI then verifies the host name as usual.

Private keys are stored in plain text, protected only by file permissions (`0600`, directory `0700`). Keep `store_dir` on a disk that only the router account can read.

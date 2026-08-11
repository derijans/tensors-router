# Model Context Protocol

The router exposes MCP tools from a selected model through a single Streamable HTTP endpoint:

```text
POST /router/mcp?model=<public-model-id>
```

The endpoint is stateless and accepts an administrator bearer credential. It supports local models and public model IDs assembled by a cluster. The router discovers and calls tools through the selected backend, but it does not run an inference tool loop. OpenAI-compatible inference remains transparent, so the client owns the conversation, tool selection, tool execution, and final completion.

MCP is available for KoboldCpp and llama.cpp text backends. It is disabled by default.

## Enable the gateway

Both the router-wide setting and the model setting must be enabled:

```yaml
mcp:
  enabled: true
  directory: "./runtime/mcp"
```

```json
{
  "model_param": "/srv/models/model.gguf",
  "mcp_enabled": true,
  "mcp_servers": []
}
```

At least one `auth.admin_keys` credential is required whenever the router-wide MCP setting is enabled, including with the `trusted_lan` profile.

`mcp.directory` is resolved relative to the directory containing the router YAML file. It is not resolved relative to the executable or the process working directory. For example, `./runtime/mcp` in `/etc/tensors-router/config.yaml` resolves to `/etc/tensors-router/runtime/mcp`.

An absolute path remains absolute. Generic examples are:

```yaml
mcp:
  enabled: true
  directory: "/var/lib/tensors-router/mcp"
```

```yaml
mcp:
  enabled: true
  directory: "D:/tensors-router/mcp"
```

Forward slashes keep Windows YAML paths readable. Native Windows paths are handled with the platform path rules.

### Linux router fragment

```yaml
security:
  profile: "secure"

auth:
  inference_keys:
    - "replace-with-inference-key"
  admin_keys:
    - "replace-with-distinct-admin-key"

models:
  config_dir: "/etc/tensors-router/kcpps"
  file_roots:
    - "/srv/models"

backend:
  mode: "kobold"

mcp:
  enabled: true
  directory: "/var/lib/tensors-router/mcp"

kobold:
  binary_path: "/opt/tensors-router/bin/koboldcpp"
  data_dir: "/var/lib/tensors-router/kobold"
```

### Windows router fragment

```yaml
security:
  profile: "secure"

auth:
  inference_keys:
    - "replace-with-inference-key"
  admin_keys:
    - "replace-with-distinct-admin-key"

models:
  config_dir: "C:/ProgramData/tensors-router/kcpps"
  file_roots:
    - "D:/models"

backend:
  mode: "llama_sdcpp"

mcp:
  enabled: true
  directory: "D:/tensors-router/mcp"

llama:
  binary_path: "C:/Program Files/tensors-router/llama/llama-server.exe"
  data_dir: "C:/ProgramData/tensors-router/llama"
```

These are fragments to merge into the canonical [router configuration](Configuration). Replace every credential placeholder and keep administrator and inference credentials distinct.

## Model configuration format

`mcp_servers` is an array embedded in the source `.kcpps` JSON. Each entry requires a unique, nonempty `name` and a `definition` object. A definition must contain exactly one of:

- `command` for a stdio server, with optional string-array `args` and optional string-valued `env`
- `url` for a KoboldCpp HTTP or HTTPS server, with optional string-valued `headers`

The legacy `mcpfile` field is rejected. The router owns generated backend configuration and never writes a generated path into the source `.kcpps` file.

Server names, tool names, public model IDs, and configuration stems remain case-sensitive and are not lowercased or rewritten. Listings are sorted by their exact names for deterministic results. Exact duplicate server or tool names are rejected.

## KoboldCpp

KoboldCpp supports stdio servers, HTTP or HTTPS servers, multiple servers, and a mixture of transports. For example:

```json
{
  "backend_mode": "kobold",
  "model_param": "/srv/models/model.gguf",
  "mcp_enabled": true,
  "mcp_servers": [
    {
      "name": "LocalTools",
      "definition": {
        "command": "/usr/bin/node",
        "args": ["/opt/mcp/local-tools.mjs"],
        "env": {
          "DATA_ROOT": "/srv/tool-data"
        }
      }
    },
    {
      "name": "RemoteTools",
      "definition": {
        "url": "https://mcp.example.com/mcp",
        "headers": {
          "Authorization": "Bearer replace-with-server-token"
        }
      }
    }
  ]
}
```

The router uses KoboldCpp's `/mcp` JSON-RPC interface. Tool results retain MCP text, structured content, images, and error status. The configured backend version must implement the transport and content types used by its servers.

For an enabled model named `MyModel.kcpps`, the router creates a private overlay at `models.config_dir/.router-mcp/MyModel.kcpps`. The overlay points KoboldCpp to the generated `servers.json`; the original model configuration remains the base configuration.

## llama.cpp

llama.cpp supports one or more stdio servers. HTTP and HTTPS server definitions are rejected for this backend. For example:

```json
{
  "backend_mode": "llama_sdcpp",
  "model_param": "D:/models/model.gguf",
  "mcp_enabled": true,
  "mcp_servers": [
    {
      "name": "LocalTools",
      "definition": {
        "command": "C:/Program Files/nodejs/node.exe",
        "args": ["C:/ProgramData/mcp/local-tools.mjs"]
      }
    },
    {
      "name": "CaseSensitiveTools",
      "definition": {
        "command": "C:/Python313/python.exe",
        "args": ["C:/ProgramData/mcp/other-tools.py"],
        "env": {
          "DATA_ROOT": "D:/tool-data"
        }
      }
    }
  ]
}
```

The router passes the generated file to `llama-server` with `--mcp-servers-config` and maps the backend `/tools` interface to MCP. Tool calls currently return MCP text content. Backend errors become text content with the MCP error flag. The llama.cpp `/tools` interface does not currently preserve structured or image results.

## Generated layout and lifecycle

For a source configuration named `MyModel.kcpps`, the generated storage is:

```text
<mcp.directory>/
  MyModel/
    servers.json
```

The exact `MyModel` casing is preserved. `servers.json` uses the backend server configuration shape:

```json
{
  "mcpServers": {
    "CaseSensitiveName": {
      "command": "/path/to/server",
      "args": []
    }
  }
}
```

KoboldCpp also receives the private overlay described above. llama.cpp receives the absolute generated path as its `--mcp-servers-config` argument. Model loading, backend switching, configuration updates, and restarts reconcile the generated file before it is used.

If global `mcp.enabled` is `false`, the router creates no MCP gateway handler, exposes no `/router/mcp` endpoint, and creates no MCP artifacts. Previously generated artifacts for known configurations are removed during reconciliation. Setting a model's `mcp_enabled` to `false`, or omitting it, removes that model's generated directory and KoboldCpp overlay while retaining the embedded server definitions in the source file.

Use a dedicated `mcp.directory`. Do not store models, executables, source configurations, or unrelated data in a generated model-stem directory because that directory is owned by the router lifecycle.

## Connect an MCP client

Configure a Streamable HTTP MCP client with:

```text
URL: http://127.0.0.1:8080/router/mcp?model=MyModel
Authorization: Bearer <administrator-credential>
```

Only `POST` is accepted. The `model` query value is the router-visible public model ID, including any cluster disambiguation. Missing models, disabled models, and models without active MCP return `404`. A model with no available replica returns a backend availability error.

Requests and responses are bounded by `limits.max_control_body_mb`. Client cancellation propagates through the router, and a routing lease keeps the selected backend available for the request. In a cluster, the master selects an MCP-capable replica and forwards the request over the cluster-authenticated node route. Clients must not call the internal node route or receive the cluster token.

## OpenAI-compatible tool loop

The consumer owns the loop:

1. Connect to `/router/mcp?model=<public-model-id>` with an administrator credential and list tools.
2. Convert each MCP tool name, description, and input schema into an OpenAI function tool definition.
3. Send those definitions with the conversation to `/v1/chat/completions`, using the credential required for inference.
4. Execute each returned tool call through the same MCP endpoint.
5. Append the tool results to the conversation and call `/v1/chat/completions` again.
6. Continue until the model returns a final answer without tool calls.

The router does not automatically insert tool definitions, execute a model-selected tool, or feed results back into inference. This keeps the inference API transparent and lets the consumer apply approval, retry, logging, and result-size policies.

## Security boundaries

Treat every `.kcpps` file containing `mcp_servers` as secret-bearing, even when MCP is disabled. Stdio definitions can execute local commands with configured environment values. HTTP definitions can contain authorization headers. Only trusted administrators should create or modify these definitions.

Keep backend listeners on loopback, restrict the router with `server.allowed_cidrs`, and use the `secure` profile with distinct administrator, inference, and cluster credentials. Do not expose generated files through a web server or place them in a shared directory. The router creates private per-model artifact directories, rejects symlinked artifact directories, and only removes validated paths beneath its configured MCP and overlay roots.

The configured roots and their ancestors are administrator-selected trusted paths and may resolve through links or mounts. Generated per-model directories must be ordinary directories rather than links.

The MCP endpoint requires an administrator principal. Inference-only credentials cannot list or call tools. Configured MCP headers and environment values are materialized for the backend but are not returned as tool metadata by the gateway.

## Current Ollama limitation

The router does not currently provide an Ollama-specific MCP endpoint or an automatic `/api/chat` tool loop. Use `/router/mcp` for tool discovery and execution and the OpenAI-compatible API for the documented inference loop. A consumer that uses Ollama-compatible inference must adapt and orchestrate that loop itself.

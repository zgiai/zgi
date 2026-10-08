# MCP client and GitHub Connector

`api/pkg/mcpclient` is the protocol adapter for stdio and Streamable HTTP. It accepts caller-supplied server settings and credentials, negotiates the protocol, preserves raw Tools, Resources, Prompts and tool results, and supports explicit connection cleanup. It is an internal library with no standalone web endpoint or demo service.

## GitHub · MCP trial

The **GitHub · MCP** Connector (`github-mcp`) uses [GitHub's official remote MCP server](https://github.com/github/github-mcp-server#remote-github-mcp-server) at `https://api.githubcopilot.com/mcp/`. It is separate from the existing GitHub REST Connector. The existing personal Key setup encrypts and stores each user's PAT; the ordinary connection, binding, authorization and execution audit path applies.

Choose **GitHub · MCP** in the Connector catalog, enter a GitHub PAT that can read the target repository's Issues, and connect. Setup checks `get_me` and that `list_issues` and `issue_read` are available. The client sends `Authorization: Bearer <PAT>`, `X-MCP-Readonly: true` and `X-MCP-Tools: get_me,list_issues,issue_read` as described in the [server configuration](https://github.com/github/github-mcp-server/blob/main/docs/server-configuration.md). The platform exposes only two read operations: list open Issues ordered by most recently updated, and read one Issue using `issue_read` with `method=get`. The model cannot supply an MCP tool name or an `issue_read` method. Runtime checks the discovered tool schema before each call.

Bind the ready connection to a chat, select it in that chat, and ask:

> 查看 `gin-gonic/gin` 最近更新的 5 个未关闭 Issue，列出标题和链接，再详细说明其中一个。

The chat tool results include `provider: github-mcp` and `transport: mcp`, Issue numbers, titles, GitHub links and detail text. The existing Connector execution records show the two platform operation IDs `github-mcp.issue.list` and `github-mcp.issue.get`; those IDs identify the MCP route without exposing the PAT. GitHub's list response does not provide a link for each item, so the adapter builds canonical Issue links from the requested owner, repository and Issue number.

## Protocol contract and validation

Use `mcpclient.NewClient(config, options)`, `Connect(ctx)`, the connection methods for Tools, Resources, Resource Templates and Prompts, then `Close(ctx)`. `Info()` reports the negotiated version, server information and capabilities. Tool names, schemas, annotations, content blocks and unknown result fields remain available in raw form. `structuredContent` distinguishes omission from JSON `null`. Tool `isError` is separate from classified transport, protocol, auth and cancellation failures. An in-flight failure carries `MayHaveExecuted`; tool calls are not automatically retried.

The library supports Streamable HTTP JSON/SSE, stdio process launch, bounded requests, catalog events and caller-provided HTTP credentials or OAuth callbacks. It does not provide automatic model or file access.

Run focused checks from `api/`:

```sh
go test ./pkg/mcpclient/... ./internal/modules/connectors/providers/githubmcp/... ./internal/modules/connectors/catalog/... -count=1
```

`api/testdata/mcp_python_server.py` is a test fixture for interoperability with the independent Python MCP SDK. Install `mcp==2.3.0` in an isolated Python environment and set `MCP_PYTHON_EXECUTABLE` to its interpreter to run the Python interop tests. GitHub Connector tests use a local MCP HTTP fixture and do not require a PAT. Live GitHub verification requires a user-supplied PAT and a running ZGI app.

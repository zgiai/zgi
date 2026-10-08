"""Small cross-SDK MCP fixture. Run with the official Python SDK 2.3.0."""

import argparse

from mcp.server.mcpserver import MCPServer

server = MCPServer("python-interop", version="1.0.0")


@server.tool(name="Case.Tool", structured_output=True)
def echo(value: str) -> dict[str, str]:
    """Return a structured echo while preserving a mixed-case dotted tool name."""
    return {"value": value}


@server.resource("demo://readme", mime_type="text/plain")
def readme() -> str:
    """A fixed resource for cross-SDK tests."""
    return "Python MCP resource"


@server.prompt(name="summarize")
def summarize(topic: str) -> str:
    """A fixed prompt for cross-SDK tests."""
    return f"Summarize {topic}"


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--http", action="store_true")
    parser.add_argument("--port", type=int, default=8000)
    args = parser.parse_args()
    if args.http:
        server.run("streamable-http", host="127.0.0.1", port=args.port, stateless_http=True, json_response=True)
    else:
        server.run("stdio")

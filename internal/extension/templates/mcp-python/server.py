"""MCP server extension for grut."""

import json
import sys
from typing import Any


def handle_request(request: dict[str, Any]) -> dict[str, Any]:
    """Handle an incoming MCP request."""
    method = request.get("method", "")
    if method == "initialize":
        return {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "serverInfo": {"name": "mcp-server"},
        }
    return {"error": {"code": -32601, "message": "Method not found: " + method}}


def write_response(response: dict[str, Any], request_id: Any) -> None:
    """Write a JSON-RPC response to stdout."""
    response["jsonrpc"] = "2.0"
    response["id"] = request_id
    sys.stdout.write(json.dumps(response) + "\n")
    sys.stdout.flush()


def main() -> None:
    """Read JSON-RPC requests from stdin and write responses to stdout."""
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            request = json.loads(line)
        except json.JSONDecodeError:
            write_response({"error": {"code": -32700, "message": "Parse error"}}, None)
            continue
        if not isinstance(request, dict):
            write_response({"error": {"code": -32600, "message": "Invalid Request"}}, None)
            continue
        response = handle_request(request)
        write_response(response, request.get("id"))


if __name__ == "__main__":
    main()

"""MCP server extension for grut."""

import json
import sys


def handle_request(request):
    """Handle an incoming MCP request."""
    method = request.get("method", "")
    if method == "initialize":
        return {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "serverInfo": {"name": "mcp-server"},
        }
    return {"error": {"code": -32601, "message": "Method not found: " + method}}


def main():
    """Read JSON-RPC requests from stdin and write responses to stdout."""
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        request = json.loads(line)
        response = handle_request(request)
        response["jsonrpc"] = "2.0"
        response["id"] = request.get("id")
        sys.stdout.write(json.dumps(response) + "\n")
        sys.stdout.flush()


if __name__ == "__main__":
    main()

"use strict";

const readline = require("readline");

const rl = readline.createInterface({ input: process.stdin });

function handleRequest(request) {
  const method = request.method || "";
  if (method === "initialize") {
    return {
      protocolVersion: "2024-11-05",
      capabilities: {},
      serverInfo: { name: "mcp-server" },
    };
  }
  return { error: { code: -32601, message: "Method not found: " + method } };
}

rl.on("line", (line) => {
  const request = JSON.parse(line);
  const response = handleRequest(request);
  response.jsonrpc = "2.0";
  response.id = request.id;
  process.stdout.write(JSON.stringify(response) + "\n");
});

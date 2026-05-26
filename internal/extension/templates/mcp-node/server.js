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

function writeResponse(id, response) {
  response.jsonrpc = "2.0";
  response.id = id;
  process.stdout.write(JSON.stringify(response) + "\n");
}

rl.on("line", (line) => {
  let request;
  try {
    request = JSON.parse(line);
  } catch {
    writeResponse(null, { error: { code: -32700, message: "Parse error" } });
    return;
  }
  if (!request || typeof request !== "object" || Array.isArray(request)) {
    writeResponse(null, { error: { code: -32600, message: "Invalid Request" } });
    return;
  }

  const response = handleRequest(request);
  writeResponse(request.id, response);
});

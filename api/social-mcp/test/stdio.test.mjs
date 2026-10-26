import test from "node:test";
import assert from "node:assert/strict";
import { fileURLToPath } from "node:url";
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";
test("real stdio initialization lists exact native schema and returns explicit unavailable", async () => {
  const schema = {
    type: "object",
    properties: { account_id: { type: "string" } },
    additionalProperties: false,
  };
  const transport = new StdioClientTransport({
    command: process.execPath,
    args: [fileURLToPath(new URL("../server.mjs", import.meta.url))],
    env: {
      PATH: process.env.PATH,
      ZGI_SOCIAL_PROVIDER: "linkedin-social",
      ZGI_SOCIAL_TOKEN: "fixture-secret",
      ZGI_SOCIAL_CONTRACT: JSON.stringify([
        { name: "account_get", inputSchema: schema },
        { name: "message_send", inputSchema: schema },
      ]),
    },
    stderr: "pipe",
  });
  const client = new Client({ name: "test", version: "1" });
  let errors = "";
  transport.stderr.on("data", (d) => (errors += d));
  try {
    await client.connect(transport);
    const list = await client.listTools();
    assert.deepEqual(
      list.tools.map((t) => t.name),
      ["account_get"],
    );
    assert.deepEqual(list.tools[0].inputSchema, schema);
    const result = await client.callTool({
      name: "message_send",
      arguments: {},
    });
    assert.equal(result.isError, true);
    const error = JSON.parse(result.content[0].text);
    assert.equal(error.code, "provider_unavailable");
    assert.ok(!JSON.stringify(result).includes("fixture-secret"));
    assert.equal(errors, "");
  } finally {
    await client.close();
  }
});

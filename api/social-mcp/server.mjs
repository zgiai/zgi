import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  ListToolsRequestSchema,
  CallToolRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";
import { createBridge, BridgeError } from "./bridge.mjs";
try {
  const bridge = createBridge({
    provider: process.env.ZGI_SOCIAL_PROVIDER,
    token: process.env.ZGI_SOCIAL_TOKEN,
    contract: JSON.parse(process.env.ZGI_SOCIAL_CONTRACT || "[]"),
  });
  const server = new Server(
    { name: "zgi-social-mcp", version: "0.1.0" },
    { capabilities: { tools: {} } },
  );
  server.setRequestHandler(ListToolsRequestSchema, async () => ({
    tools: bridge.tools(),
  }));
  server.setRequestHandler(CallToolRequestSchema, async (r) => {
    try {
      return {
        content: [
          {
            type: "text",
            text: JSON.stringify(
              await bridge.call(r.params.name, r.params.arguments ?? {}),
            ),
          },
        ],
      };
    } catch (e) {
      return {
        isError: true,
        content: [
          {
            type: "text",
            text: JSON.stringify({
              code: e instanceof BridgeError ? e.code : "provider_unavailable",
              message:
                e instanceof BridgeError
                  ? e.message
                  : "platform operation unavailable",
              ...(e instanceof BridgeError && e.unknown
                ? { unknown: true }
                : {}),
            }),
          },
        ],
      };
    }
  });
  await server.connect(new StdioServerTransport());
} catch {
  process.stderr.write("Social MCP configuration invalid\n");
  process.exitCode = 1;
}

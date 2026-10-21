import type { MarketOffering } from '@/services/types/zgi-market';

/** The environment variable the examples read the organization's market credential from. */
export const MARKET_CREDENTIAL_ENV = 'ZGI_MARKET_KEY';

/** Shown in an example when this instance has not been told the market's public gateway. */
export const MARKET_GATEWAY_PLACEHOLDER = 'https://YOUR-MARKET-GATEWAY';

const slugPattern = /^[a-z0-9][a-z0-9._-]{0,127}$/;
const httpMethodPattern = /^(GET|POST|PUT|PATCH|DELETE)$/;
// What a declared path pattern may hold; nothing that would leave the quoted address in a shell.
const operationPathPattern = /^\/[A-Za-z0-9._~@:{}/-]{0,255}$/;
const authorizationHeader = `-H "Authorization: Bearer $${MARKET_CREDENTIAL_ENV}"`;

interface FamilyExample {
  /** Path under the offering's base address; empty for the root path. */
  path: string;
  contentType: 'application/json' | 'application/x-www-form-urlencoded';
  body: Record<string, unknown>;
}

const question = 'Hello';

// One representative request per protocol family. Paths and bodies follow the vendor protocol the
// gateway forwards unchanged; `<model>` stands for a model the offering serves.
const familyExamples: Record<string, FamilyExample> = {
  openai_chat: {
    path: 'chat/completions',
    contentType: 'application/json',
    body: { model: '<model>', messages: [{ role: 'user', content: question }] },
  },
  anthropic_messages: {
    path: 'v1/messages',
    contentType: 'application/json',
    body: { model: '<model>', max_tokens: 1024, messages: [{ role: 'user', content: question }] },
  },
  openai_embeddings: {
    path: 'embeddings',
    contentType: 'application/json',
    body: { model: '<model>', input: question },
  },
  openai_rerank: {
    path: 'rerank',
    contentType: 'application/json',
    body: { model: '<model>', query: question, documents: ['First document', 'Second document'] },
  },
  baidu_ocr: {
    path: 'rest/2.0/ocr/v1/general_basic',
    contentType: 'application/x-www-form-urlencoded',
    body: { url: 'https://example.com/image.png' },
  },
  tavily_search: {
    path: 'search',
    contentType: 'application/json',
    body: { query: question },
  },
  bocha_search: {
    path: 'v1/web-search',
    contentType: 'application/json',
    body: { query: question, count: 5 },
  },
  jina_reader: {
    path: '',
    contentType: 'application/json',
    body: { url: 'https://example.com' },
  },
};

/** The gateway origin an example uses, and whether this instance really knows it. */
export function marketGatewayOrigin(configured: string | undefined): {
  origin: string;
  configured: boolean;
} {
  const origin = (configured ?? '').trim().replace(/\/+$/, '');
  return origin
    ? { origin, configured: true }
    : { origin: MARKET_GATEWAY_PLACEHOLDER, configured: false };
}

function shellQuote(value: string): string {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}

/** The absolute gateway address of an offering. Only a well-formed slug becomes part of it. */
export function marketGatewayBaseUrl(
  origin: string,
  offering: Pick<MarketOffering, 'slug'>
): string {
  const slug = slugPattern.test(offering.slug) ? offering.slug : '<offering>';
  return `${origin}/v1/gateway/${slug}`;
}

/** The absolute MCP server address of an `mcp` listing. */
export function marketMcpServerUrl(origin: string, slug: string): string {
  return `${origin}/v1/mcp/${slugPattern.test(slug) ? slug : '<server>'}`;
}

export interface MarketApiQuickStart {
  baseUrl: string;
  /** A copyable request. */
  curl: string;
  /** False when the catalog does not carry what a concrete request needs. */
  concrete: boolean;
  /** True when the example names a `<model>` the reader has to replace. */
  needsModel: boolean;
}

/** A quick-start request for one offering, derived only from what the catalog carries. */
export function marketApiQuickStart(
  origin: string,
  offering: Pick<MarketOffering, 'slug' | 'protocol_family' | 'operations'>
): MarketApiQuickStart {
  const baseUrl = marketGatewayBaseUrl(origin, offering);
  const example = familyExamples[offering.protocol_family];
  // An `http` offering declares what may be called: the example is its first operation that
  // reads, or its first operation, with `{name}` left for the reader.
  const declared = offering.operations ?? [];
  const operation = declared.find(item => item.method === 'GET') ?? declared[0];
  if (
    !example &&
    operation &&
    httpMethodPattern.test(operation.method) &&
    operationPathPattern.test(operation.path)
  ) {
    const lines = [
      operation.method === 'GET'
        ? `curl "${baseUrl}${operation.path}" \\`
        : `curl -X ${operation.method} "${baseUrl}${operation.path}" \\`,
      `  ${authorizationHeader}`,
    ];
    if (operation.method !== 'GET' && operation.method !== 'DELETE') {
      lines[1] += ' \\';
      lines.push(`  -H "Content-Type: application/json" \\`, `  -d '{}'`);
    }
    return { baseUrl, concrete: true, needsModel: false, curl: lines.join('\n') };
  }
  if (!example) {
    return {
      baseUrl,
      concrete: false,
      needsModel: false,
      curl: [`curl -X POST "${baseUrl}/<operation path>" \\`, `  ${authorizationHeader}`].join(
        '\n'
      ),
    };
  }
  const url = example.path ? `${baseUrl}/${example.path}` : baseUrl;
  const lines = [`curl "${url}" \\`, `  ${authorizationHeader} \\`];
  if (example.contentType === 'application/json') {
    lines.push(`  -H "Content-Type: application/json" \\`);
    lines.push(`  -d ${shellQuote(JSON.stringify(example.body))}`);
  } else {
    const fields = Object.entries(example.body);
    fields.forEach(([key, value], index) => {
      const last = index === fields.length - 1;
      lines.push(`  --data-urlencode ${shellQuote(`${key}=${String(value)}`)}${last ? '' : ' \\'}`);
    });
  }
  return { baseUrl, concrete: true, needsModel: 'model' in example.body, curl: lines.join('\n') };
}

/** A client configuration that reaches the listing's MCP server with the market credential. */
export function marketMcpClientConfig(origin: string, slug: string): string {
  return JSON.stringify(
    {
      mcpServers: {
        [slugPattern.test(slug) ? slug : 'zgi-market']: {
          type: 'http',
          url: marketMcpServerUrl(origin, slug),
          headers: { Authorization: `Bearer \${${MARKET_CREDENTIAL_ENV}}` },
        },
      },
    },
    null,
    2
  );
}

/** A request that lists the tools of the listing's MCP server. */
export function marketMcpToolsListCurl(origin: string, slug: string): string {
  return [
    `curl "${marketMcpServerUrl(origin, slug)}" \\`,
    `  ${authorizationHeader} \\`,
    `  -H "Content-Type: application/json" \\`,
    `  -H "Accept: application/json, text/event-stream" \\`,
    `  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'`,
  ].join('\n');
}

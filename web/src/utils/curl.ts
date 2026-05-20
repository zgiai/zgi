/**
 * Pure TypeScript cURL parser for client-side usage.
 * - Converts a cURL command to structured HTTP request data
 * - Covers common flags: -X/--request, -H/--header, --url, -d/--data, --data-raw, --data-binary, --json, -u/--user, -G, -I/--head
 * - Handles quoted strings and basic escaping, including Windows cmd caret continuations
 *
 * Note:
 * This parser aims to support the most common API cURL use cases. Advanced features (complex multipart, cookie files,
 * proxy options, file upload via @, etc.) are out of scope for the first iteration.
 */

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH' | 'HEAD';
export interface HttpHeaderKV {
  key: string;
  value: string;
}
export interface ConvertCurlResult {
  method: HttpMethod;
  url: string;
  headers: HttpHeaderKV[];
  body?: string;
}

/** Normalize multiline cURL string */
export function normalizeCurlString(curl: string): string {
  // Trim and normalize newlines
  let s = curl.trim();
  // Replace CRLF with LF
  s = s.replace(/\r\n/g, '\n');
  // Join lines ending with backslash (bash/zsh)
  s = s.replace(/\\\n\s*/g, ' ');
  // Join lines with Windows cmd continuation ^ followed by newline
  s = s.replace(/\s*\^\s*\n\s*/g, ' ');
  // Unescape common caret escapes from "Copy as cURL (cmd)"
  s = s.replace(/\^\^/g, '^');
  s = s.replace(/\^"/g, '"');
  s = s.replace(/\^&/g, '&');
  s = s.replace(/\^</g, '<');
  s = s.replace(/\^>/g, '>');
  s = s.replace(/\^\|/g, '|');
  // Do NOT collapse spaces globally to avoid altering quoted payloads
  return s;
}

/**
 * Tokenize cURL keeping quoted segments together
 * Supports single and double quotes; handles simple escaped quotes
 */
function tokenizeCurl(input: string): string[] {
  const tokens: string[] = [];
  let i = 0;
  const n = input.length;
  while (i < n) {
    // Skip spaces
    if (input[i] === ' ') {
      i++;
      continue;
    }
    const ch = input[i];
    if (ch === '"' || ch === "'") {
      const quote = ch;
      i++; // skip quote
      let buf = '';
      while (i < n) {
        const c = input[i];
        if (c === '\\' && i + 1 < n && input[i + 1] === quote) {
          buf += quote;
          i += 2;
          continue;
        }
        if (c === quote) {
          i++;
          break;
        }
        buf += c;
        i++;
      }
      tokens.push(buf);
      continue;
    }
    // Unquoted token
    const start = i;
    while (i < n && input[i] !== ' ') i++;
    tokens.push(input.slice(start, i));
  }
  return tokens.filter(t => t.length > 0);
}

function setHeader(headers: HttpHeaderKV[], key: string, value: string) {
  const idx = headers.findIndex(h => h.key.toLowerCase() === key.toLowerCase());
  if (idx >= 0) headers[idx] = { key, value };
  else headers.push({ key, value });
}

function ensureContentType(headers: HttpHeaderKV[], value = 'application/json') {
  const exists = headers.some(h => h.key.toLowerCase() === 'content-type');
  if (!exists) headers.push({ key: 'Content-Type', value });
}

/** Basic URL check */
function isHttpUrl(token: string): boolean {
  return token.startsWith('http://') || token.startsWith('https://');
}

/**
 * Parse cURL into structured request
 */
export function parseCurlToRequest(curlRaw: string): ConvertCurlResult {
  const normalized = normalizeCurlString(curlRaw);
  const tokens = tokenizeCurl(normalized);

  let method: HttpMethod = 'GET';
  let url = '';
  const headers: HttpHeaderKV[] = [];
  const bodyParts: string[] = [];
  let useGetWithQuery = false; // -G flag

  for (let i = 0; i < tokens.length; i++) {
    const token = tokens[i];
    const next = i + 1 < tokens.length ? tokens[i + 1] : undefined;

    // Ignore command name
    if (token.toLowerCase() === 'curl') continue;

    // Method flags
    if (token === '-X' || token === '--request') {
      if (next) {
        const m = next.toUpperCase() as HttpMethod;
        method = (['GET', 'POST', 'PUT', 'DELETE', 'PATCH', 'HEAD'] as const).includes(m)
          ? m
          : 'GET';
        i++;
        continue;
      }
    }

    // --head / -I (HEAD request)
    if (token === '--head' || token === '-I') {
      method = 'HEAD';
      continue;
    }

    // Header flags
    if (token === '-H' || token === '--header') {
      if (next) {
        // Expect "Key: value" format
        const colonIdx = next.indexOf(':');
        if (colonIdx > 0) {
          const key = next.slice(0, colonIdx).trim();
          const value = next.slice(colonIdx + 1).trim();
          setHeader(headers, key, value);
        }
        i++;
        continue;
      }
    }

    // URL flag
    if (token === '--url') {
      if (next) {
        url = next;
        i++;
        continue;
      }
    }

    // Data flags
    if (
      token === '-d' ||
      token === '--data' ||
      token === '--data-raw' ||
      token === '--data-binary' ||
      token === '--data-urlencode'
    ) {
      if (next) {
        bodyParts.push(next);
        if (method === 'GET' && !useGetWithQuery) method = 'POST';
        i++;
        continue;
      }
    }

    // JSON flag
    if (token === '--json') {
      if (next) {
        bodyParts.push(next);
        ensureContentType(headers, 'application/json');
        if (method === 'GET' && !useGetWithQuery) method = 'POST';
        i++;
        continue;
      }
    }

    // Basic auth
    if (token === '-u' || token === '--user') {
      if (next) {
        // next like "user:pass"
        const [user, pass] = next.split(':');
        if (user && pass !== undefined) {
          // btoa available in browser
          const basic =
            typeof btoa === 'function'
              ? btoa(`${user}:${pass}`)
              : Buffer.from(`${user}:${pass}`).toString('base64');
          setHeader(headers, 'Authorization', `Basic ${basic}`);
        }
        i++;
        continue;
      }
    }

    // GET with query params
    if (token === '-G') {
      useGetWithQuery = true;
      method = 'GET';
      continue;
    }

    // Bare URL
    if (!token.startsWith('-') && isHttpUrl(token)) {
      if (!url) url = token;
      continue;
    }
  }

  let body = bodyParts.join('\n');

  // If -G used: move body parts to query string
  if (useGetWithQuery && bodyParts.length > 0 && url) {
    const query = bodyParts
      .map(p => p.trim())
      .filter(p => p.length > 0)
      .join('&');
    const hasQ = url.includes('?');
    url = `${url}${hasQ ? '&' : '?'}${query}`;
    body = '';
  }

  if (!url) throw new Error('URL not found in cURL');

  const result: ConvertCurlResult = { method, url, headers };
  if (body) result.body = body;
  return result;
}

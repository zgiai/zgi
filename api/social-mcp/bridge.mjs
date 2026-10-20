import { isIP } from "node:net";
import { Graph } from "@thenavidm/facebook-mcp-cli/dist/api/client.js";
import { registerAll } from "@thenavidm/facebook-mcp-cli/dist/tools/index.js";
import { LinkedInApiClient } from "./upstream/linkedin/linkedin-api.js";
const ALL = [
  "account_get",
  "account_list",
  "post_list",
  "post_get",
  "post_create",
  "comment_list",
  "comment_create",
  "comment_reply",
  "conversation_list",
  "message_list",
  "message_send",
  "metrics_get",
];
const MSG = ["conversation_list", "message_list", "message_send"];
const enc = encodeURIComponent;
export class BridgeError extends Error {
  constructor(code, message, unknown = false) {
    super(message);
    this.code = code;
    this.unknown = unknown;
  }
}
const invalid = (m) => new BridgeError("invalid_input", m);
const denied = (m) => new BridgeError("insufficient_key", m);
function req(a, k) {
  if (typeof a[k] !== "string" || !a[k].trim() || a[k].length > 10000)
    throw invalid(`${k} required`);
  return a[k];
}
function seg(v) {
  if (
    typeof v !== "string" ||
    !v ||
    v.length > 500 ||
    /[\x00-\x20/\\?#]/.test(v)
  )
    throw invalid("invalid identifier");
  return enc(v);
}
const limit = (a) =>
  Number.isFinite(a.limit)
    ? Math.min(50, Math.max(1, Math.floor(a.limit)))
    : 20;
function cursor(a) {
  if (
    a.cursor !== undefined &&
    (typeof a.cursor !== "string" || a.cursor.length > 2000)
  )
    throw invalid("invalid cursor");
  return a.cursor;
}
function interval(a) {
  const o = {};
  for (const k of ["since", "until"])
    if (a[k] !== undefined) {
      if (
        typeof a[k] !== "string" ||
        !/^\d{4}-\d\d-\d\dT/.test(a[k]) ||
        !Number.isFinite(Date.parse(a[k]))
      )
        throw invalid("invalid interval");
      o[k] = a[k];
    }
  if (o.since && o.until && Date.parse(o.since) >= Date.parse(o.until))
    throw invalid("invalid interval");
  return o;
}
function photo(v) {
  let u;
  try {
    u = new URL(v);
  } catch {
    throw invalid("invalid image URL");
  }
  const h = u.hostname.toLowerCase();
  if (
    u.protocol !== "https:" ||
    u.username ||
    u.password ||
    u.port ||
    isIP(h) ||
    h.includes(":") ||
    !h.includes(".") ||
    /(^|\.)(localhost|local|internal|test|invalid|example)$/.test(h)
  )
    throw invalid("invalid image URL");
  return u.href;
}
function image(a) {
  if (a.image_data === undefined) {
    if (a.mime_type !== undefined)
      throw invalid("mime_type requires image_data");
    return null;
  }
  if (
    typeof a.image_data !== "string" ||
    a.image_data.length > 196608 ||
    !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(
      a.image_data,
    )
  )
    throw invalid("invalid image_data");
  const b = Buffer.from(a.image_data, "base64");
  const ok =
    a.mime_type === "image/png"
      ? b.subarray(0, 8).equals(Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]))
      : a.mime_type === "image/jpeg"
        ? b[0] === 255 && b[1] === 216 && b[2] === 255
        : a.mime_type === "image/webp"
          ? b.subarray(0, 4).toString() === "RIFF" &&
            b.subarray(8, 12).toString() === "WEBP"
          : false;
  if (!ok) throw invalid("image_data does not match mime_type");
  return b;
}
function scrub(v, t, k = "") {
  if (/^(access_token|refresh_token|authorization)$/i.test(k)) return undefined;
  if (typeof v === "string") {
    if (v.includes(t)) return "[redacted]";
    if (/^https?:/i.test(v)) {
      try {
        const u = new URL(v);
        for (const key of [...u.searchParams.keys()])
          if (/token|secret|authorization/i.test(key))
            u.searchParams.delete(key);
        return u.href;
      } catch {}
    }
    return v;
  }
  if (Array.isArray(v)) return v.map((x) => scrub(x, t));
  if (v && typeof v === "object")
    return Object.fromEntries(
      Object.entries(v)
        .map(([k, x]) => [k, scrub(x, t, k)])
        .filter(([, x]) => x !== undefined),
    );
  return v;
}
// Input is validated as JSON before this scan. Quoting large integer tokens must
// never repair malformed JSON (for example an unquoted numeric property name).
function losslessIntegerJSON(text) {
  const pieces = [];
  let quoted = false,
    escaped = false,
    start = 0;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quoted) {
      if (escaped) escaped = false;
      else if (c === "\\") escaped = true;
      else if (c === '"') quoted = false;
      continue;
    }
    if (c === '"') {
      quoted = true;
      continue;
    }
    if (c !== "-" && !(c >= "0" && c <= "9")) continue;
    let end = i + 1;
    while (end < text.length && /[0-9.eE+\-]/.test(text[end])) end++;
    const number = text.slice(i, end);
    if (
      /^-?(?:0|[1-9][0-9]*)$/.test(number) &&
      !Number.isSafeInteger(Number(number))
    ) {
      pieces.push(text.slice(start, i), JSON.stringify(number));
      start = end;
    }
    i = end - 1;
  }
  pieces.push(text.slice(start));
  return pieces.join("");
}
export function createBridge({
  provider,
  token,
  contract = [],
  fetch: fetchImpl = globalThis.fetch,
  now = Date.now,
}) {
  if (
    !["x-social", "linkedin-social", "facebook-social"].includes(provider) ||
    !token ||
    typeof token !== "string"
  )
    throw invalid("provider and token required");
  const extra = {
    "x-social": ["post_search", "mention_list", "post_repost", "post_quote"],
    "linkedin-social": ["notification_list", "post_repost"],
    "facebook-social": ["mention_list", "webhook_subscribe"],
  };
  const supported = [...ALL, ...extra[provider]].filter(
    (n) => provider !== "linkedin-social" || !MSG.includes(n),
  );
  const tools = contract
    .filter((t) => supported.includes(t.name))
    .map((t) => ({
      name: t.name,
      description: t.description || t.name,
      inputSchema: t.inputSchema,
    }));
  let identity,
    acl,
    fbTools,
    writeDispatched = false;
  async function transport(input, init = {}, upload = false) {
    const u = new URL(input);
    const approved =
      provider === "x-social"
        ? u.origin === "https://api.x.com"
        : provider === "facebook-social"
          ? u.origin === "https://graph.facebook.com"
          : u.origin === "https://api.linkedin.com" ||
            (upload &&
              (u.hostname === "www.linkedin.com" ||
                u.hostname === "media.licdn.com" ||
                u.hostname.endsWith(".licdn.com")));
    if (
      u.protocol !== "https:" ||
      u.username ||
      u.password ||
      u.port ||
      !approved
    )
      throw invalid("unapproved platform URL");
    const method = init.method ?? "GET",
      write = !["GET", "HEAD"].includes(method);
    if (write) writeDispatched = true;
    try {
      const r = await fetchImpl(u, {
        ...init,
        redirect: "error",
        signal: AbortSignal.timeout(15000),
      });
      if (Number(r.headers.get("content-length")) > 1048576)
        throw Error("size");
      const reader = r.body?.getReader(),
        chunks = [];
      let n = 0;
      if (reader)
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          n += value.byteLength;
          if (n > 1048576) {
            await reader.cancel();
            throw Error("size");
          }
          chunks.push(Buffer.from(value));
        }
      let text = Buffer.concat(chunks).toString();
      if (!r.ok) {
        let vendorCode;
        if (provider === "facebook-social")
          try {
            vendorCode = JSON.parse(text)?.error?.code;
          } catch {}
        if (r.status === 401 || vendorCode === 190)
          throw new BridgeError(
            "invalid_key",
            "platform authentication failed HTTP 401",
          );
        if (r.status === 403 || vendorCode === 200 || vendorCode === 10)
          throw denied(
            r.status === 403
              ? "platform permission denied HTTP 403"
              : "platform permission denied",
          );
        if (write && r.status >= 500)
          throw new BridgeError(
            "outcome_unknown",
            "write outcome unknown",
            true,
          );
        throw new BridgeError(
          "provider_unavailable",
          `platform request failed HTTP ${r.status}`,
        );
      }
      if (text)
        try {
          JSON.parse(text);
          // Supply lossless JSON text to both response.json() and upstream
          // clients that parse response.text() themselves.
          text = losslessIntegerJSON(text);
        } catch {
          if (!(upload && method === "PUT")) throw Error("JSON");
        }
      return new Response(text || null, {
        status: r.status,
        headers: r.headers,
      });
    } catch (e) {
      if (e instanceof BridgeError) throw e;
      throw new BridgeError(
        write ? "outcome_unknown" : "provider_unavailable",
        write ? "write outcome unknown" : "platform request unavailable",
        write,
      );
    }
  }
  async function request(path, { method = "GET", query = {}, body } = {}) {
    const base =
      provider === "x-social"
        ? "https://api.x.com/2/"
        : provider === "facebook-social"
          ? "https://graph.facebook.com/v23.0/"
          : "https://api.linkedin.com/rest/";
    const u = new URL(path, base);
    for (const [k, v] of Object.entries(query))
      if (v !== undefined) u.searchParams.set(k, String(v));
    const headers = { Authorization: `Bearer ${token}` };
    if (provider === "linkedin-social")
      Object.assign(headers, {
        "LinkedIn-Version": "202609",
        "X-Restli-Protocol-Version": "2.0.0",
      });
    if (body !== undefined) headers["Content-Type"] = "application/json";
    const r = await transport(u, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    return r.status === 204 ? {} : await r.json();
  }
  const li = new LinkedInApiClient({
    version: "202609",
    getAccessToken: async () => token,
    fetch: (u, i) =>
      transport(u, i, new URL(u).origin !== "https://api.linkedin.com"),
  });
  async function me() {
    if (identity) return identity;
    if (provider === "x-social") {
      const r = await request("users/me");
      if (!r.data?.id) throw denied("account identity unavailable");
      identity = {
        id: String(r.data.id),
        name: r.data.name || r.data.username || String(r.data.id),
      };
    }
    if (provider === "facebook-social") {
      const r = await request("me", { query: { fields: "id,name,category" } });
      if (!r.id || !r.category) throw denied("Facebook Page token required");
      identity = { id: String(r.id), name: r.name || String(r.id) };
    }
    if (provider === "linkedin-social") {
      const r = await li.getUserInfo();
      if (!r.sub) throw denied("account identity unavailable");
      identity = { id: String(r.sub), name: r.name || String(r.sub) };
    }
    return identity;
  }
  async function organizations() {
    if (acl) return acl;
    const r = await request("organizationAcls", {
      query: {
        q: "roleAssignee",
        role: "ADMINISTRATOR",
        state: "APPROVED",
        count: 50,
      },
    });
    return (acl = (r.elements ?? [])
      .filter(
        (x) =>
          x.role === "ADMINISTRATOR" &&
          x.state === "APPROVED" &&
          /^urn:li:organization:\d+$/.test(x.organization),
      )
      .map((x) => x.organization));
  }
  async function account(a) {
    if (provider === "linkedin-social") {
      const id = req(a, "account_id"),
        org = /^\d+$/.test(id) ? `urn:li:organization:${id}` : id;
      if (!(await organizations()).includes(org))
        throw denied("account is not an approved managed organization");
      return org;
    }
    const m = await me();
    if (a.account_id !== undefined && a.account_id !== m.id)
      throw denied("account does not match token identity");
    return m.id;
  }
  async function liPost(a, org) {
    const p = req(a, "post_id");
    if (!/^urn:li:(share|ugcPost):\d+$/.test(p))
      throw invalid("invalid post_id");
    const r = await request(`posts/${enc(p)}`);
    if (r.author !== org) throw denied("post author does not match account");
    return r;
  }
  function fbPost(a, id) {
    const p = req(a, "post_id");
    if (!p.startsWith(`${id}_`))
      throw denied("post does not belong to account");
    return p;
  }
  async function conversation(a, id) {
    const c = req(a, "conversation_id"),
      r = await request(seg(c), { query: { fields: "id,participants" } }),
      participants = r.participants?.data ?? [];
    if (!participants.some((p) => String(p.id) === id))
      throw denied("conversation does not belong to account");
    return { id: c, participants };
  }
  async function registry() {
    if (fbTools) return fbTools;
    const m = await me(),
      cfg = {
        pages: [{ id: m.id, name: m.name, accessToken: token }],
        preferred: [m.id],
        readOnly: false,
        allowDestructive: false,
        requestTimeoutMs: 15000,
        userAgent: "ZGI-Social-MCP/0.1",
      },
      graph = new Graph(cfg);
    // Preserve upstream Graph public methods and registerAll handlers; replace only
    // transport to bound bytes, forbid redirects/retries and redact vendor errors.
    graph.request = async (_p, method, path, opts = {}) => {
      const u = new URL(`https://graph.facebook.com/v23.0/${path}`);
      for (const [k, v] of Object.entries(opts.query ?? {}))
        if (v !== undefined) u.searchParams.set(k, String(v));
      return (
        await transport(u, {
          method,
          headers: { Authorization: `Bearer ${token}` },
        })
      ).json();
    };
    fbTools = new Map();
    registerAll({ registerTool: (n, s, h) => fbTools.set(n, h) }, cfg, graph);
    return fbTools;
  }
  async function fbTool(n, a) {
    const r = await (await registry()).get(n)(a);
    if (r.isError)
      throw new BridgeError(
        "provider_unavailable",
        "platform operation unavailable",
      );
    return JSON.parse(r.content[0].text);
  }
  function xRange(a) {
    const r = interval(a);
    return {
      start_time: r.since,
      end_time: r.until,
      since_id: a.since_id,
      until_id: a.until_id,
    };
  }
  async function xCall(n, a, id) {
    if (n === "comment_create") {
      const postID = req(a, "post_id");
      const post = await request(`tweets/${seg(postID)}`, {
        query: { "tweet.fields": "author_id" },
      });
      if (post.data?.id !== postID)
        throw new BridgeError(
          "provider_unavailable",
          "requested post unavailable",
        );
      return request("tweets", {
        method: "POST",
        body: { text: req(a, "text"), reply: { in_reply_to_tweet_id: postID } },
      });
    }
    if (n === "post_repost")
      return request(`users/${seg(id)}/retweets`, {
        method: "POST",
        body: { tweet_id: req(a, "post_id") },
      });
    if (n === "post_quote")
      return request("tweets", {
        method: "POST",
        body: { text: req(a, "text"), quote_tweet_id: req(a, "post_id") },
      });
    if (n === "post_search" || n === "mention_list")
      return request(
        n === "post_search"
          ? "tweets/search/recent"
          : `users/${seg(id)}/mentions`,
        {
          query: {
            ...(n === "post_search"
              ? { query: req(a, "query"), next_token: cursor(a) }
              : { pagination_token: cursor(a) }),
            max_results: Math.max(n === "post_search" ? 10 : 5, limit(a)),
            ...xRange(a),
            "tweet.fields":
              "author_id,created_at,public_metrics,conversation_id",
          },
        },
      );
    if (n === "post_list")
      return request(`users/${seg(id)}/tweets`, {
        query: {
          max_results: Math.max(5, limit(a)),
          pagination_token: cursor(a),
          ...xRange(a),
          "tweet.fields": "created_at,public_metrics,conversation_id",
        },
      });
    if (n === "post_get")
      return request(`tweets/${seg(req(a, "post_id"))}`, {
        query: {
          "tweet.fields": "author_id,created_at,public_metrics,conversation_id",
        },
      });
    if (n === "post_create") {
      if (a.image_url) throw invalid("image_url unavailable; use image_data");
      const body = { text: req(a, "text") };
      if (image(a)) {
        const m = await request("media/upload", {
          method: "POST",
          body: { media: a.image_data, media_category: "tweet_image" },
        });
        if (!m.data?.id)
          throw new BridgeError(
            "outcome_unknown",
            "media upload result unknown",
            true,
          );
        body.media = { media_ids: [String(m.data.id)] };
      }
      return request("tweets", { method: "POST", body });
    }
    if (n === "comment_list")
      return request("tweets/search/recent", {
        query: {
          query: `conversation_id:${seg(req(a, "post_id"))}`,
          max_results: Math.max(10, limit(a)),
          next_token: cursor(a),
          ...xRange(a),
          "tweet.fields":
            "author_id,created_at,in_reply_to_user_id,conversation_id",
        },
      });
    if (n === "comment_reply") {
      const postID = req(a, "post_id"),
        commentID = req(a, "comment_id");
      const post = await request(`tweets/${seg(postID)}`, {
        query: { "tweet.fields": "conversation_id" },
      });
      const comment = await request(`tweets/${seg(commentID)}`, {
        query: { "tweet.fields": "conversation_id" },
      });
      if (
        !post.data?.conversation_id ||
        post.data.conversation_id !== comment.data?.conversation_id
      )
        throw denied("comment does not belong to requested thread");
      return request("tweets", {
        method: "POST",
        body: {
          text: req(a, "text"),
          reply: { in_reply_to_tweet_id: commentID },
        },
      });
    }
    if (n === "conversation_list")
      return request("dm_events", {
        query: {
          max_results: limit(a),
          pagination_token: cursor(a),
          "dm_event.fields":
            "dm_conversation_id,created_at,sender_id,participant_ids",
        },
      });
    if (n === "message_list")
      return request(
        `dm_conversations/${seg(req(a, "conversation_id"))}/dm_events`,
        {
          query: {
            max_results: limit(a),
            pagination_token: cursor(a),
            "dm_event.fields":
              "dm_conversation_id,created_at,sender_id,participant_ids",
          },
        },
      );
    if (n === "message_send") {
      const c = req(a, "conversation_id"),
        r = await request(`dm_conversations/${seg(c)}/dm_events`, {
          query: { max_results: 10 },
        });
      if (!r.data?.length) throw denied("existing conversation required");
      return request(`dm_conversations/${seg(c)}/messages`, {
        method: "POST",
        body: { text: req(a, "text") },
      });
    }
    if (n === "metrics_get") {
      const range = interval(a);
      if (range.since || range.until)
        throw invalid("X metrics are current snapshots; interval unsupported");
      return {
        ...(await request(
          a.post_id ? `tweets/${seg(a.post_id)}` : `users/${seg(id)}`,
          {
            query: a.post_id
              ? {
                  "tweet.fields":
                    "public_metrics,non_public_metrics,organic_metrics,promoted_metrics",
                }
              : { "user.fields": "public_metrics" },
          },
        )),
        interval: null,
        limitations: [
          "Current snapshot only; fields depend on ownership, access tier and post age. Absent metrics unavailable.",
        ],
      };
    }
  }
  async function fbCall(n, a, id) {
    if (n === "comment_create")
      return request(`${seg(fbPost(a, id))}/comments`, {
        method: "POST",
        body: { message: req(a, "text") },
      });
    if (n === "webhook_subscribe")
      return request(`${id}/subscribed_apps`, {
        method: "POST",
        body: {
          subscribed_fields: ["feed", "messages", "messaging_postbacks"],
        },
      });
    if (n === "mention_list")
      return request(`${id}/tagged`, {
        query: {
          limit: limit(a),
          after: cursor(a),
          fields: "id,message,created_time,permalink_url,from",
        },
      });
    if (n === "post_create" && a.image_data !== undefined) {
      const bytes = image(a),
        form = new FormData();
      form.set("caption", req(a, "text"));
      form.set("source", new Blob([bytes], { type: a.mime_type }), "image");
      return (
        await transport(`https://graph.facebook.com/v23.0/${id}/photos`, {
          method: "POST",
          headers: { Authorization: `Bearer ${token}` },
          body: form,
        })
      ).json();
    }
    if (n === "post_create")
      return a.image_url
        ? fbTool("create_photo_post", {
            page: id,
            url: photo(a.image_url),
            caption: req(a, "text"),
          })
        : fbTool("create_post", { page: id, message: req(a, "text") });
    if (n === "post_list")
      return request(`${id}/published_posts`, {
        query: {
          limit: limit(a),
          after: cursor(a),
          fields:
            "id,message,created_time,permalink_url,shares,reactions.summary(true).limit(0),comments.summary(true).limit(0)",
        },
      });
    if (n === "post_get")
      return request(seg(fbPost(a, id)), {
        query: {
          fields: "id,message,created_time,permalink_url,from,attachments",
        },
      });
    if (n === "comment_list")
      return request(`${seg(fbPost(a, id))}/comments`, {
        query: {
          limit: limit(a),
          after: cursor(a),
          fields: "id,message,from,created_time,parent",
        },
      });
    if (n === "comment_reply") {
      const p = fbPost(a, id),
        c = req(a, "comment_id"),
        r = await request(seg(c), { query: { fields: "id,object,parent" } });
      if (r.object?.id !== p)
        throw denied("comment parent does not match post");
      return fbTool("reply_to_comment", {
        page: id,
        comment_id: c,
        message: req(a, "text"),
      });
    }
    if (n === "conversation_list")
      return request(`${id}/conversations`, {
        query: {
          platform: "messenger",
          limit: limit(a),
          after: cursor(a),
          fields: "id,participants,updated_time",
        },
      });
    if (n === "message_list") {
      const c = await conversation(a, id);
      return request(`${seg(c.id)}/messages`, {
        query: {
          limit: limit(a),
          after: cursor(a),
          fields: "id,message,from,to,created_time",
        },
      });
    }
    if (n === "message_send") {
      const c = await conversation(a, id),
        r = await request(`${seg(c.id)}/messages`, {
          query: { limit: 50, fields: "id,from,created_time" },
        }),
        m = (r.data ?? [])
          .filter(
            (m) =>
              m.from?.id &&
              String(m.from.id) !== id &&
              c.participants.some((p) => String(p.id) === String(m.from.id)),
          )
          .sort(
            (a, b) => Date.parse(b.created_time) - Date.parse(a.created_time),
          )[0],
        age = now() - Date.parse(m?.created_time);
      if (!Number.isFinite(age) || age < 0 || age >= 86400000)
        throw denied(
          "Messenger standard response window unavailable or expired",
        );
      if (c.participants.length !== 2)
        throw denied("one-to-one conversation required");
      return request(`${id}/messages`, {
        method: "POST",
        body: {
          recipient: { id: m.from.id },
          messaging_type: "RESPONSE",
          message: { text: req(a, "text") },
        },
      });
    }
    if (n === "metrics_get") {
      const range = interval(a),
        post = a.post_id ? fbPost(a, id) : null,
        p = post || id;
      if (a.metrics === undefined) {
        if (range.since || range.until)
          throw invalid("interval requires explicit metrics");
        return {
          ...(await request(seg(p), {
            query: {
              fields: post
                ? "id,reactions.summary(true).limit(0),comments.summary(true).limit(0),shares"
                : "id,name,fan_count,followers_count",
            },
          })),
          interval: null,
          limitations: [
            "Current snapshot only; historical Insights requires explicit metrics and platform grants.",
          ],
        };
      }
      if (
        !Array.isArray(a.metrics) ||
        !a.metrics.length ||
        a.metrics.length > 10 ||
        a.metrics.some(
          (m) => typeof m !== "string" || !/^[a-z][a-z0-9_]{0,79}$/.test(m),
        )
      )
        throw invalid("invalid metrics list");
      if (post && (range.since || range.until))
        throw invalid(
          "post Insights are lifetime metrics; interval unavailable",
        );
      return {
        ...(await request(`${seg(p)}/insights`, {
          query: {
            metric: a.metrics.join(","),
            ...(post
              ? {}
              : { period: "day", since: range.since, until: range.until }),
          },
        })),
        interval: post ? null : range,
        limitations: [
          "Explicit native Insights metric names; unavailable metrics fail rather than becoming zeros.",
          ...(post
            ? ["Post Insights are lifetime values; date filtering unavailable."]
            : []),
        ],
      };
    }
  }

  async function liCall(n, a, org) {
    if (n === "comment_create") {
      await liPost(a, org);
      return li.commentOnPost({
        postUrn: a.post_id,
        actorUrn: org,
        message: req(a, "text"),
        objectUrn: a.post_id,
      });
    }
    if (n === "post_repost") {
      const parent = req(a, "post_id");
      if (!/^urn:li:(share|ugcPost):\d+$/.test(parent))
        throw invalid("invalid post_id");
      return li.createPost({
        authorUrn: org,
        commentary: a.text === undefined ? "" : req(a, "text"),
        reshareUrn: parent,
      });
    }
    if (n === "notification_list") {
      const start = a.cursor === undefined ? 0 : Number(a.cursor);
      if (!Number.isSafeInteger(start) || start < 0 || start > 100000)
        throw invalid("invalid cursor");
      const r = interval(a),
        range = [];
      if (r.since) range.push(`start:${Date.parse(r.since)}`);
      if (r.until) range.push(`end:${Date.parse(r.until)}`);
      return request("organizationalEntityNotifications", {
        query: {
          q: "criteria",
          organizationalEntity: org,
          actions:
            "List(LIKE,COMMENT,SHARE,SHARE_MENTION,ADMIN_COMMENT,COMMENT_EDIT,COMMENT_DELETE)",
          count: limit(a),
          start,
          ...(range.length ? { timeRange: `(${range.join(",")})` } : {}),
        },
      });
    }
    if (n === "post_create") {
      if (a.image_url) throw invalid("image_url unavailable; use image_data");
      let imageUrn;
      const text = req(a, "text");
      if (image(a))
        imageUrn = (
          await li.uploadImage({ ownerUrn: org, dataBase64: a.image_data })
        ).image_urn;
      return li.createPost({
        authorUrn: org,
        commentary: text,
        imageUrn,
      });
    }
    if (n === "post_list" || n === "comment_list") {
      const start = a.cursor === undefined ? 0 : Number(a.cursor);
      if (!Number.isSafeInteger(start) || start < 0 || start > 100000)
        throw invalid("invalid cursor");
      if (n === "comment_list") await liPost(a, org);
      return request(
        n === "post_list"
          ? "posts"
          : `socialActions/${enc(a.post_id)}/comments`,
        {
          query:
            n === "post_list"
              ? {
                  q: "author",
                  author: org,
                  count: limit(a),
                  start,
                  sortBy: "LAST_MODIFIED",
                }
              : { count: limit(a), start },
        },
      );
    }
    if (n === "post_get") return liPost(a, org);
    if (n === "comment_reply") {
      await liPost(a, org);
      const c = req(a, "comment_id");
      const match = c.match(/^urn:li:comment:\(urn:li:activity:\d+,(\d+)\)$/);
      const commentID = match ? match[1] : /^\d+$/.test(c) ? c : null;
      if (!commentID) throw invalid("invalid LinkedIn comment_id");
      const parent = await request(
        `socialActions/${enc(a.post_id)}/comments/${commentID}`,
      );
      if (
        String(parent.id) !== commentID ||
        !/^urn:li:comment:\(urn:li:activity:\d+,\d+\)$/.test(
          parent.commentUrn,
        ) ||
        (match && parent.commentUrn !== c)
      )
        throw denied("comment parent does not match post");
      return li.commentOnPost({
        postUrn: parent.commentUrn,
        actorUrn: org,
        message: req(a, "text"),
        objectUrn: a.post_id,
        parentComment: parent.commentUrn,
      });
    }
    if (n === "metrics_get") {
      const range = interval(a),
        query = { q: "organizationalEntity", organizationalEntity: org };
      if (a.post_id) {
        if (range.since || range.until)
          throw invalid(
            "LinkedIn per-post statistics do not support intervals",
          );
        await liPost(a, org);
        if (a.post_id.startsWith("urn:li:ugcPost:"))
          query["ugcPosts[0]"] = a.post_id;
        else query.shares = `List(${a.post_id})`;
      }
      if (range.since || range.until) {
        if (!range.since || !range.until)
          throw invalid("LinkedIn interval needs since and until");
        query.timeIntervals = `(timeRange:(start:${Date.parse(range.since)},end:${Date.parse(range.until)}),timeGranularityType:DAY)`;
      }
      return {
        ...(await request("organizationalEntityShareStatistics", { query })),
        interval: range,
        limitations: [
          "Organization share statistics; missing fields unavailable. No trial/conversion attribution.",
        ],
      };
    }
  }
  async function call(n, a = {}) {
    writeDispatched = false;
    if (!supported.includes(n))
      throw new BridgeError(
        "provider_unavailable",
        "operation unavailable for this provider",
      );
    if (!tools.some((t) => t.name === n))
      throw invalid("operation outside native contract");
    if (!a || typeof a !== "object" || Array.isArray(a))
      throw invalid("arguments must be an object");
    const fixedArgs = {
      comment_create: ["account_id", "post_id", "text"],
      post_search: [
        "account_id",
        "query",
        "limit",
        "cursor",
        "since",
        "until",
        "since_id",
        "until_id",
      ],
      mention_list:
        provider === "x-social"
          ? [
              "account_id",
              "limit",
              "cursor",
              "since",
              "until",
              "since_id",
              "until_id",
            ]
          : ["account_id", "limit", "cursor"],
      notification_list: ["account_id", "limit", "cursor", "since", "until"],
      post_repost:
        provider === "linkedin-social"
          ? ["account_id", "post_id", "text"]
          : ["account_id", "post_id"],
      post_quote: ["account_id", "post_id", "text"],
      webhook_subscribe: ["account_id"],
    };
    if (fixedArgs[n] && Object.keys(a).some((k) => !fixedArgs[n].includes(k)))
      throw invalid("unsupported argument or unsupported filter for operation");
    const filters = ["since", "until", "since_id", "until_id"].filter(
      (k) => a[k] !== undefined,
    );
    const nativeX =
      provider === "x-social" &&
      ["post_list", "post_search", "mention_list", "comment_list"].includes(n);
    if (
      filters.some(
        (k) =>
          (!nativeX && !(k === "since" || k === "until")) ||
          (!nativeX &&
            n !== "metrics_get" &&
            !(provider === "linkedin-social" && n === "notification_list")),
      )
    )
      throw invalid("unsupported filter for operation");
    interval(a);
    if (
      nativeX &&
      ((a.since !== undefined && a.since_id !== undefined) ||
        (a.until !== undefined && a.until_id !== undefined))
    )
      throw invalid("date and ID bounds are mutually exclusive");
    if (n === "post_create") {
      if (a.image_data !== undefined && a.image_url !== undefined)
        throw invalid("image_data and image_url are mutually exclusive");
      image(a);
    }
    for (const key of ["since_id", "until_id"])
      if (
        a[key] !== undefined &&
        (typeof a[key] !== "string" || !/^\d{1,19}$/.test(a[key]))
      )
        throw invalid("invalid ID filter");
    if (a.since_id && a.until_id && BigInt(a.since_id) >= BigInt(a.until_id))
      throw invalid("invalid ID interval");
    cursor(a);
    if (n === "post_search") {
      if (req(a, "query").length > 512) throw invalid("query too long");
    }
    if (
      provider === "x-social" &&
      ["post_repost", "post_quote"].includes(n) &&
      !/^\d{1,19}$/.test(req(a, "post_id"))
    )
      throw invalid("invalid post_id");
    try {
      if (n === "account_get")
        return {
          ...(await me()),
          operations: tools.map((t) => t.name),
          ...(provider === "linkedin-social"
            ? {
                unavailable: Object.fromEntries(
                  MSG.map((n) => [
                    n,
                    "Company Page messaging requires approved partner API",
                  ]),
                ),
              }
            : {}),
        };
      if (n === "account_list") {
        if (provider === "linkedin-social") {
          const entries = await organizations();
          const start = a.cursor === undefined ? 0 : Number(a.cursor);
          if (!Number.isSafeInteger(start) || start < 0 || start > 50)
            throw invalid("invalid cursor");
          const selected = entries.slice(start, start + limit(a));
          return {
            accounts: await Promise.all(
              selected.map(async (org) => {
                const r = await request(
                  `organizations/${org.split(":").at(-1)}`,
                );
                return { id: org, name: r.localizedName || org };
              }),
            ),
            ...(start + selected.length < entries.length
              ? { cursor: String(start + selected.length) }
              : {}),
            limitations: [
              "Only first 50 approved ADMINISTRATOR ACL entries selectable.",
            ],
          };
        }
        if (a.cursor) throw invalid("single account has no next page");
        return { accounts: [await me()] };
      }
      const id = await account(a);
      const data = scrub(
        await (provider === "x-social"
          ? xCall(n, a, id)
          : provider === "facebook-social"
            ? fbCall(n, a, id)
            : liCall(n, a, id)),
        token,
      );
      if (!data || typeof data !== "object") throw Error("result");
      if (n === "post_repost" && provider === "x-social") {
        if (data.data?.retweeted !== true)
          throw new BridgeError(
            "outcome_unknown",
            "write outcome unknown",
            true,
          );
      } else if (n === "webhook_subscribe") {
        if (data.success !== true)
          throw new BridgeError(
            "outcome_unknown",
            "write outcome unknown",
            true,
          );
      } else if (
        [
          "post_create",
          "comment_create",
          "post_quote",
          "post_repost",
          "comment_reply",
          "message_send",
        ].includes(n)
      ) {
        const receipt =
          provider === "x-social"
            ? data.data?.id || data.data?.dm_event_id
            : data.id || data.message_id;
        if (
          (typeof receipt !== "string" && typeof receipt !== "number") ||
          !String(receipt)
        )
          throw new BridgeError(
            "outcome_unknown",
            "write outcome unknown",
            true,
          );
      }
      if (n.endsWith("_list") || n === "post_search") {
        let next;
        if (provider === "x-social") next = data.meta?.next_token;
        if (provider === "facebook-social" && data.paging?.next)
          next = data.paging?.cursors?.after;
        if (
          provider === "linkedin-social" &&
          data.paging?.links?.some((l) => l.rel === "next")
        )
          next = String(
            Number(a.cursor ?? 0) + Number(data.paging.count ?? limit(a)),
          );
        const rows = data.data ?? data.elements ?? [];
        data.sync = {
          source_platform: provider,
          source_ids: Array.isArray(rows)
            ? rows
                .map((r) => r.id ?? r.notificationId ?? r.commentUrn)
                .filter((v) => v !== undefined)
                .map((value) => {
                  if (typeof value === "number" && !Number.isSafeInteger(value))
                    throw new BridgeError(
                      "provider_unavailable",
                      "platform source identifier is not lossless",
                    );
                  return String(value);
                })
            : [],
          next_cursor: next ?? null,
          filters: Object.fromEntries(filters.map((k) => [k, a[k]])),
          checkpoint_policy:
            "Keep original filters across every cursor page; advance a caller checkpoint only after all pages are durably processed. Native cursors may expire; reads are not snapshots.",
        };
      }
      return data;
    } catch (e) {
      if (e instanceof BridgeError) throw e;
      throw new BridgeError(
        writeDispatched ? "outcome_unknown" : "provider_unavailable",
        writeDispatched
          ? "write outcome unknown"
          : "platform operation unavailable",
        writeDispatched,
      );
    }
  }
  let pending = Promise.resolve();
  return {
    tools: () => tools,
    call: (name, args) => {
      const result = pending
        .then(() => call(name, args))
        .then((data) => {
          const clean = scrub(data, token);
          if (
            Buffer.byteLength(JSON.stringify(JSON.stringify(clean))) >
            60 * 1024
          )
            throw new BridgeError(
              writeDispatched ? "outcome_unknown" : "provider_unavailable",
              writeDispatched
                ? "write outcome unknown"
                : "result too large; request a smaller page",
              writeDispatched,
            );
          return clean;
        });
      pending = result.catch(() => {});
      return result;
    },
  };
}

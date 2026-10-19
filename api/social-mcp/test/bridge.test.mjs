import test from "node:test";
import assert from "node:assert/strict";
import { createBridge } from "../bridge.mjs";
const token = "secret-fixture-token";
const contract = [
  "account_get",
  "account_list",
  "post_list",
  "post_get",
  "post_create",
  "comment_list",
  "comment_reply",
  "conversation_list",
  "message_list",
  "message_send",
  "metrics_get",
].map((name) => ({
  name,
  description: name,
  inputSchema: { type: "object", properties: {}, additionalProperties: false },
}));
function fixture(provider, handler) {
  const calls = [];
  const fetch = async (input, init = {}) => {
    const url = new URL(input);
    calls.push({ url, init });
    const data = await handler(url, init, calls.length);
    return new Response(JSON.stringify(data?.body ?? data), {
      status: typeof data?.status === "number" ? data.status : 200,
      headers: { "content-type": "application/json", ...(data?.headers ?? {}) },
    });
  };
  return {
    bridge: createBridge({
      provider,
      token,
      contract,
      fetch,
      now: () => Date.parse("2026-10-19T12:00:00Z"),
    }),
    calls,
  };
}
test("contracts retain exact schema; LinkedIn messaging excluded and fails without network", async () => {
  const { bridge, calls } = fixture("linkedin-social", () => ({}));
  assert.deepEqual(
    bridge.tools().find((t) => t.name === "post_create").inputSchema,
    contract[4].inputSchema,
  );
  assert.ok(!bridge.tools().some((t) => t.name === "message_send"));
  await assert.rejects(
    bridge.call("message_send", { conversation_id: "c", text: "hi" }),
    /unavailable/,
  );
  assert.equal(calls.length, 0);
});
test("X identity binds account; text post sent exactly once; image fetch never attempted", async () => {
  const { bridge, calls } = fixture("x-social", (u, init) =>
    u.pathname === "/2/users/me"
      ? { data: { id: "1", name: "ZGI" } }
      : { data: { id: "2" } },
  );
  assert.equal((await bridge.call("account_get", {})).id, "1");
  await assert.rejects(
    bridge.call("post_create", { account_id: "other", text: "x" }),
    /account/,
  );
  await assert.rejects(
    bridge.call("post_create", { text: "x", image_url: "http://127.0.0.1" }),
    /image/,
  );
  await bridge.call("post_create", { text: "hello" });
  assert.equal(calls.filter((c) => c.init.method === "POST").length, 1);
  assert.deepEqual(JSON.parse(calls.at(-1).init.body), { text: "hello" });
});
test("Facebook reuses Page publishing tools and rejects person tokens/mismatched account", async () => {
  const { bridge, calls } = fixture("facebook-social", (u) =>
    u.pathname.endsWith("/me")
      ? { id: "10", name: "Page", category: "Software" }
      : { id: "10_20" },
  );
  const account = await bridge.call("account_get", {});
  assert.equal(account.id, "10");
  await bridge.call("post_create", { text: "hello" });
  assert.equal(calls.at(-1).url.pathname, "/v23.0/10/feed");
  assert.equal(calls.at(-1).url.searchParams.get("message"), "hello");
  await assert.rejects(
    bridge.call("post_list", { account_id: "99" }),
    /account/,
  );
  const person = fixture("facebook-social", () => ({
    id: "person",
    name: "Person",
  }));
  await assert.rejects(person.bridge.call("account_get", {}), /Page/);
});
test("Facebook message response checks conversation ownership and 24 hour latest inbound", async () => {
  const { bridge, calls } = fixture("facebook-social", (u) => {
    if (u.pathname.endsWith("/me"))
      return { id: "10", name: "Page", category: "Software" };
    if (u.pathname.endsWith("/c"))
      return { id: "c", participants: { data: [{ id: "10" }, { id: "20" }] } };
    if (u.pathname.endsWith("/c/messages"))
      return {
        data: [
          {
            from: { id: "20" },
            created_time: "2026-10-19T11:00:00Z",
            message: "hi",
          },
        ],
      };
    return { message_id: "sent" };
  });
  await bridge.call("message_send", { conversation_id: "c", text: "hello" });
  const send = calls.at(-1);
  assert.equal(send.url.pathname, "/v23.0/10/messages");
  assert.equal(JSON.parse(send.init.body).messaging_type, "RESPONSE");
  const old = fixture("facebook-social", (u) =>
    u.pathname.endsWith("/me")
      ? { id: "10", name: "Page", category: "Software" }
      : u.pathname.endsWith("/c")
        ? { participants: { data: [{ id: "10" }, { id: "20" }] } }
        : {
            data: [
              { from: { id: "20" }, created_time: "2026-10-17T00:00:00Z" },
            ],
          },
  );
  await assert.rejects(
    old.bridge.call("message_send", { conversation_id: "c", text: "hello" }),
    /window/,
  );
  assert.equal(old.calls.filter((c) => c.init.method === "POST").length, 0);
});
test("LinkedIn org ACL binds author and escapes commentary using vendored client", async () => {
  const { bridge, calls } = fixture("linkedin-social", (u) => {
    if (u.pathname.endsWith("/organizationAcls"))
      return {
        elements: [
          {
            organization: "urn:li:organization:7",
            role: "ADMINISTRATOR",
            state: "APPROVED",
          },
        ],
      };
    if (u.pathname === "/v2/userinfo") return { sub: "member", name: "Member" };
    if (u.pathname.endsWith("/organizations/7"))
      return { id: 7, localizedName: "ZGI" };
    return { body: {}, headers: { "x-restli-id": "urn:li:share:9" } };
  });
  await assert.rejects(
    bridge.call("post_create", { account_id: "8", text: "Hi" }),
    /approved/,
  );
  await bridge.call("post_create", { account_id: "7", text: "Hello (world)" });
  assert.equal(
    JSON.parse(calls.at(-1).init.body).author,
    "urn:li:organization:7",
  );
  assert.equal(
    JSON.parse(calls.at(-1).init.body).commentary,
    "Hello \\(world\\)",
  );
});
test("read list bounds, sanitized error/URLs and missing metrics preserved", async () => {
  const { bridge, calls } = fixture("x-social", (u) =>
    u.pathname === "/2/users/me"
      ? { data: { id: "1", name: "ZGI" } }
      : {
          data: [{ public_metrics: { like_count: 2 } }],
          paging: { next: "https://example.com/?access_token=" + token },
        },
  );
  const result = await bridge.call("post_list", { limit: 500 });
  assert.equal(calls.at(-1).url.searchParams.get("max_results"), "50");
  assert.ok(!JSON.stringify(result).includes(token));
  const bad = fixture("x-social", () => ({
    status: 403,
    body: { error: token },
  }));
  await assert.rejects(
    bad.bridge.call("account_get", {}),
    (e) => !e.message.includes(token) && e.message.includes("403"),
  );
});
test("Facebook photo URL validation rejects private, literal, credential and redirect targets", async () => {
  const { bridge, calls } = fixture("facebook-social", (u) =>
    u.pathname.endsWith("/me")
      ? { id: "10", name: "Page", category: "Software" }
      : { id: "p" },
  );
  for (const image_url of [
    "http://example.com/a",
    "https://127.0.0.1/a",
    "https://[::1]/a",
    "https://localhost/a",
    "https://a.local/a",
    "https://user:pass@example.com/a",
  ])
    await assert.rejects(
      bridge.call("post_create", { text: "x", image_url }),
      /image/,
    );
  assert.equal(calls.filter((c) => c.init.method === "POST").length, 0);
});
test("unowned LinkedIn post rejected before mutation", async () => {
  const { bridge, calls } = fixture("linkedin-social", (u) =>
    u.pathname.endsWith("/organizationAcls")
      ? {
          elements: [
            {
              organization: "urn:li:organization:7",
              role: "ADMINISTRATOR",
              state: "APPROVED",
            },
          ],
        }
      : { author: "urn:li:organization:8" },
  );
  await assert.rejects(
    bridge.call("comment_reply", {
      account_id: "7",
      post_id: "urn:li:share:9",
      comment_id: "urn:li:comment:2",
      text: "hi",
    }),
    /author/,
  );
  assert.equal(calls.filter((c) => c.init.method === "POST").length, 0);
});
test("X uploads base64 media before posting; transport errors never retry writes", async () => {
  const { bridge, calls } = fixture("x-social", (u) =>
    u.pathname === "/2/users/me"
      ? { data: { id: "1", name: "ZGI" } }
      : u.pathname === "/2/media/upload"
        ? { data: { id: "media" } }
        : { data: { id: "post" } },
  );
  const image_data = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]).toString(
    "base64",
  );
  await bridge.call("post_create", {
    text: "Photo",
    image_data,
    mime_type: "image/png",
  });
  assert.equal(calls[1].url.pathname, "/2/media/upload");
  assert.deepEqual(JSON.parse(calls[2].init.body).media, {
    media_ids: ["media"],
  });
  let posts = 0;
  const bad = createBridge({
    provider: "x-social",
    token,
    contract,
    fetch: async (u, i) => {
      if (i.method === "POST") {
        posts++;
        throw Error(token);
      }
      return new Response(JSON.stringify({ data: { id: "1", name: "ZGI" } }));
    },
  });
  await assert.rejects(
    bad.call("post_create", { text: "Hello" }),
    (e) =>
      e.code === "outcome_unknown" && e.unknown && !e.message.includes(token),
  );
  assert.equal(posts, 1);
});
test("LinkedIn reused image pipeline polls AVAILABLE and refuses untrusted upload targets", async () => {
  const png = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]).toString("base64");
  const handler = (u) => {
    if (u.pathname.endsWith("/organizationAcls"))
      return {
        elements: [
          {
            organization: "urn:li:organization:7",
            role: "ADMINISTRATOR",
            state: "APPROVED",
          },
        ],
      };
    if (u.searchParams.get("action") === "initializeUpload")
      return {
        value: {
          uploadUrl: "https://www.linkedin.com/dms-uploads/photo",
          image: "urn:li:image:1",
        },
      };
    if (u.pathname.includes("/images/")) return { status: "AVAILABLE" };
    return { body: {}, headers: { "x-restli-id": "urn:li:share:1" } };
  };
  const { bridge, calls } = fixture("linkedin-social", handler);
  await bridge.call("post_create", {
    account_id: "7",
    text: "Photo",
    image_data: png,
    mime_type: "image/png",
  });
  assert.equal(calls.filter((c) => c.init.method === "PUT").length, 1);
  assert.equal(
    JSON.parse(calls.at(-1).init.body).content.media.id,
    "urn:li:image:1",
  );
  const bad = fixture("linkedin-social", (u) =>
    u.searchParams.get("action") === "initializeUpload"
      ? {
          value: {
            uploadUrl: "https://evil.invalid/upload",
            image: "urn:li:image:1",
          },
        }
      : handler(u),
  );
  await assert.rejects(
    bad.bridge.call("post_create", {
      account_id: "7",
      text: "Photo",
      image_data: png,
      mime_type: "image/png",
    }),
    /unapproved/,
  );
  assert.equal(
    bad.calls.filter((c) => c.url.hostname === "evil.invalid").length,
    0,
  );
});
test("oversized responses and malformed JSON fail safely without token disclosure", async () => {
  for (const response of [
    () => new Response("x".repeat(1048577)),
    () => new Response("{invalid"),
  ]) {
    const bridge = createBridge({
      provider: "x-social",
      token,
      contract,
      fetch: async () => response(),
    });
    await assert.rejects(
      bridge.call("account_get", {}),
      (e) => e.code === "provider_unavailable" && !e.message.includes(token),
    );
  }
});
test("LinkedIn nested reply verifies scoped comment and includes object and parentComment", async () => {
  const urn = "urn:li:comment:(urn:li:activity:99,2)";
  const { bridge, calls } = fixture("linkedin-social", (u) =>
    u.pathname.endsWith("/organizationAcls")
      ? {
          elements: [
            {
              organization: "urn:li:organization:7",
              role: "ADMINISTRATOR",
              state: "APPROVED",
            },
          ],
        }
      : u.pathname.endsWith("/comments/2")
        ? { id: "2", commentUrn: urn }
        : u.pathname.includes("/posts/")
          ? { author: "urn:li:organization:7" }
          : { id: "3" },
  );
  await bridge.call("comment_reply", {
    account_id: "7",
    post_id: "urn:li:share:9",
    comment_id: urn,
    text: "Thanks",
  });
  assert.deepEqual(JSON.parse(calls.at(-1).init.body), {
    actor: "urn:li:organization:7",
    message: { text: "Thanks" },
    object: "urn:li:share:9",
    parentComment: urn,
  });
});
test("all providers preserve metrics raw values, intervals and missing fields", async () => {
  for (const provider of ["x-social", "facebook-social", "linkedin-social"]) {
    const { bridge, calls } = fixture(provider, (u) =>
      u.pathname.endsWith("/organizationAcls")
        ? {
            elements: [
              {
                organization: "urn:li:organization:7",
                role: "ADMINISTRATOR",
                state: "APPROVED",
              },
            ],
          }
        : u.pathname.endsWith("/me")
          ? provider === "x-social"
            ? { data: { id: "1", name: "ZGI" } }
            : { id: "1", name: "Page", category: "Software" }
          : { elements: [{ totalShareStatistics: { clickCount: 5 } }] },
    );
    const args =
      provider === "linkedin-social"
        ? {
            account_id: "7",
            since: "2026-10-01T00:00:00Z",
            until: "2026-10-02T00:00:00Z",
          }
        : {};
    const data = await bridge.call("metrics_get", args);
    assert.equal(data.elements[0].totalShareStatistics.clickCount, 5);
    assert.equal(
      data.elements[0].totalShareStatistics.impressionCount,
      undefined,
    );
    if (provider === "linkedin-social")
      assert.ok(
        calls
          .at(-1)
          .url.searchParams.get("timeIntervals")
          .includes("start:1790812800000"),
      );
  }
});
test("X replies verify requested thread before writing", async () => {
  const { bridge, calls } = fixture("x-social", (u) =>
    u.pathname === "/2/users/me"
      ? { data: { id: "1", name: "ZGI" } }
      : u.pathname === "/2/tweets/2"
        ? { data: { id: "2", conversation_id: "100" } }
        : { data: { id: "3", conversation_id: "999" } },
  );
  await assert.rejects(
    bridge.call("comment_reply", { post_id: "2", comment_id: "3", text: "hi" }),
    /thread/,
  );
  assert.equal(calls.filter((c) => c.init.method === "POST").length, 0);
});
test("LinkedIn account list paginates only verified ACL entries", async () => {
  const { bridge } = fixture("linkedin-social", (u) =>
    u.pathname.endsWith("/organizationAcls")
      ? {
          elements: [7, 8].map((n) => ({
            organization: `urn:li:organization:${n}`,
            role: "ADMINISTRATOR",
            state: "APPROVED",
          })),
        }
      : { localizedName: "Company" },
  );
  const first = await bridge.call("account_list", { limit: 1 });
  assert.equal(first.accounts.length, 1);
  assert.equal(first.cursor, "1");
  const next = await bridge.call("account_list", {
    limit: 1,
    cursor: first.cursor,
  });
  assert.equal(next.accounts[0].id, "urn:li:organization:8");
  assert.equal(next.cursor, undefined);
});
test("serialized tool result cap rejects instead of truncating valid data", async () => {
  const { bridge } = fixture("x-social", (u) =>
    u.pathname === "/2/users/me"
      ? { data: { id: "1", name: "ZGI" } }
      : { data: [{ text: "x".repeat(65000) }] },
  );
  await assert.rejects(
    bridge.call("post_list", {}),
    (e) => e.code === "provider_unavailable",
  );
});
test("successful HTTP write without platform receipt remains unknown", async () => {
  const { bridge } = fixture("x-social", (u) =>
    u.pathname === "/2/users/me" ? { data: { id: "1", name: "ZGI" } } : {},
  );
  await assert.rejects(
    bridge.call("post_create", { text: "hello" }),
    (e) => e.code === "outcome_unknown" && e.unknown,
  );
});
test("Facebook snapshot stats have no fabricated interval; explicit insights metrics validated", async () => {
  const { bridge, calls } = fixture("facebook-social", (u) =>
    u.pathname.endsWith("/me")
      ? { id: "10", name: "Page", category: "Software" }
      : u.pathname.endsWith("/insights")
        ? { data: [{ name: "page_media_view", values: [{ value: 3 }] }] }
        : { id: "10", followers_count: 5 },
  );
  const snapshot = await bridge.call("metrics_get", {});
  assert.equal(snapshot.followers_count, 5);
  assert.equal(snapshot.interval, null);
  assert.ok(!calls.at(-1).url.pathname.endsWith("/insights"));
  await assert.rejects(
    bridge.call("metrics_get", { since: "2026-10-01T00:00:00Z" }),
    /metrics/,
  );
  await assert.rejects(
    bridge.call("metrics_get", { metrics: ["invalid,fields"] }),
    /metric/,
  );
  const insight = await bridge.call("metrics_get", {
    metrics: ["page_media_view"],
    since: "2026-10-01T00:00:00Z",
    until: "2026-10-02T00:00:00Z",
  });
  assert.equal(insight.data[0].name, "page_media_view");
  assert.equal(calls.at(-1).url.searchParams.get("metric"), "page_media_view");
});

test("Facebook post Insights omit period and reject date filters", async () => {
  const { bridge, calls } = fixture("facebook-social", (u) =>
    u.pathname.endsWith("/me")
      ? { id: "10", name: "Page", category: "Software" }
      : { data: [{ name: "post_media_view", values: [{ value: 3 }] }] },
  );
  await bridge.call("metrics_get", {
    post_id: "10_20",
    metrics: ["post_media_view"],
  });
  assert.equal(calls.at(-1).url.searchParams.has("period"), false);
  await assert.rejects(
    bridge.call("metrics_get", {
      post_id: "10_20",
      metrics: ["post_media_view"],
      since: "2026-10-01T00:00:00Z",
    }),
    /lifetime/,
  );
});
test("Meta token and permission error codes map safely even with HTTP400", async () => {
  for (const [vendorCode, code] of [
    [190, "invalid_key"],
    [200, "insufficient_key"],
  ]) {
    const { bridge } = fixture("facebook-social", () => ({
      status: 400,
      body: { error: { code: vendorCode, message: token } },
    }));
    await assert.rejects(
      bridge.call("account_get", {}),
      (e) => e.code === code && !e.message.includes(token),
    );
  }
});
test("Facebook top-level and nested replies verify root object rather than parent comment", async () => {
  for (const parent of [undefined, { id: "parent-comment" }]) {
    const { bridge, calls } = fixture("facebook-social", (u) =>
      u.pathname.endsWith("/me")
        ? { id: "10", name: "Page", category: "Software" }
        : u.pathname.endsWith("/comment")
          ? {
              id: "comment",
              object: { id: "10_20" },
              ...(parent ? { parent } : {}),
            }
          : { id: "reply" },
    );
    await bridge.call("comment_reply", {
      post_id: "10_20",
      comment_id: "comment",
      text: "Thanks",
    });
    assert.equal(calls.at(-1).init.method, "POST");
  }
  const bad = fixture("facebook-social", (u) =>
    u.pathname.endsWith("/me")
      ? { id: "10", name: "Page", category: "Software" }
      : { id: "comment", object: { id: "99_20" } },
  );
  await assert.rejects(
    bad.bridge.call("comment_reply", {
      post_id: "10_20",
      comment_id: "comment",
      text: "hi",
    }),
    /match post/,
  );
  assert.equal(bad.calls.filter((c) => c.init.method === "POST").length, 0);
});
test("LinkedIn share vs UGC filter uses native contract; post stats interval rejected", async () => {
  for (const [post_id, key, value] of [
    ["urn:li:share:9", "shares", "List(urn:li:share:9)"],
    ["urn:li:ugcPost:9", "ugcPosts[0]", "urn:li:ugcPost:9"],
  ]) {
    const { bridge, calls } = fixture("linkedin-social", (u) =>
      u.pathname.endsWith("/organizationAcls")
        ? {
            elements: [
              {
                organization: "urn:li:organization:7",
                role: "ADMINISTRATOR",
                state: "APPROVED",
              },
            ],
          }
        : u.pathname.includes("/posts/")
          ? { author: "urn:li:organization:7" }
          : { elements: [] },
    );
    await bridge.call("metrics_get", { account_id: "7", post_id });
    assert.equal(calls.at(-1).url.searchParams.get(key), value);
    const before = calls.length;
    await assert.rejects(
      bridge.call("metrics_get", {
        account_id: "7",
        post_id,
        since: "2026-10-01T00:00:00Z",
        until: "2026-10-02T00:00:00Z",
      }),
      /interval/,
    );
    assert.equal(calls.length, before);
  }
});

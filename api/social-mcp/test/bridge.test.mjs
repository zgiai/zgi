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
  "comment_get",
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
        ? { data: { id: "media", media_key: "3_media" } }
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
test("X post metrics bind the returned post author to the connected account", async () => {
  const owned = fixture("x-social", (u) =>
    u.pathname === "/2/users/me"
      ? { data: { id: "1", name: "ZGI" } }
      : {
          data: {
            id: "9",
            author_id: "1",
            public_metrics: { like_count: 3 },
          },
        },
  );
  const metrics = await owned.bridge.call("metrics_get", { post_id: "9" });
  assert.equal(metrics.data.author_id, "1");
  assert.match(
    owned.calls.at(-1).url.searchParams.get("tweet.fields"),
    /author_id/,
  );

  const foreign = fixture("x-social", (u) =>
    u.pathname === "/2/users/me"
      ? { data: { id: "1", name: "ZGI" } }
      : { data: { id: "9", author_id: "2", public_metrics: {} } },
  );
  await assert.rejects(
    foreign.bridge.call("metrics_get", { post_id: "9" }),
    /author/,
  );
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
    const result = await bridge.call("metrics_get", {
      account_id: "7",
      post_id,
    });
    assert.equal(result.post_id, post_id);
    assert.equal(result.account_id, "urn:li:organization:7");
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

test("X post and comment readback include authoritative thread and media fields", async () => {
  const response = {
    data: {
      id: "20",
      text: "A reply",
      author_id: "1",
      created_at: "2026-10-19T10:00:00.000Z",
      conversation_id: "10",
      referenced_tweets: [{ type: "replied_to", id: "10" }],
      attachments: { media_keys: ["3_photo"] },
    },
    includes: {
      media: [
        {
          media_key: "3_photo",
          type: "photo",
          url: "https://pbs.twimg.com/media/photo.jpg",
        },
      ],
    },
  };
  const { bridge, calls } = fixture("x-social", (u) =>
    u.pathname === "/2/users/me"
      ? { data: { id: "1", name: "ZGI" } }
      : response,
  );
  const post = await bridge.call("post_get", { post_id: "20" });
  assert.deepEqual(
    {
      id: post.id,
      text: post.text,
      author_id: post.author_id,
      url: post.url,
      published_at: post.published_at,
      parent_id: post.parent_id,
      media: post.media,
    },
    {
      id: "20",
      text: "A reply",
      author_id: "1",
      url: "https://x.com/i/web/status/20",
      published_at: "2026-10-19T10:00:00.000Z",
      parent_id: "10",
      media: [
        { id: "3_photo", url: "https://pbs.twimg.com/media/photo.jpg" },
      ],
    },
  );
  assert.deepEqual(post.vendor, response);
  const query = calls.at(-1).url.searchParams;
  assert.match(query.get("tweet.fields"), /referenced_tweets/);
  assert.match(query.get("tweet.fields"), /attachments/);
  assert.match(query.get("expansions"), /attachments\.media_keys/);
  assert.match(query.get("media.fields"), /media_key/);

  const comment = await bridge.call("comment_get", { comment_id: "20" });
  assert.equal(comment.parent_id, "10");
  assert.equal(calls.at(-1).url.pathname, "/2/tweets/20");
  await assert.rejects(
    bridge.call("comment_get", { account_id: "other", comment_id: "20" }),
    /account/,
  );

  for (const [type, field] of [
    ["quoted", "quoted_id"],
    ["retweeted", "reposted_id"],
  ]) {
    const linked = fixture("x-social", (u) =>
      u.pathname === "/2/users/me"
        ? { data: { id: "1", name: "ZGI" } }
        : {
            data: {
              id: "21",
              text: type === "retweeted" ? "" : "Commentary",
              author_id: "1",
              created_at: "2026-10-19T10:00:00Z",
              referenced_tweets: [{ type, id: "10" }],
            },
          },
    );
    const linkedResult = await linked.bridge.call("post_get", {
      post_id: "21",
    });
    assert.equal(linkedResult[field], "10");
    assert.equal(linkedResult.parent_id, undefined);
  }
});

test("Facebook comment readback verifies the managed Page root and preserves actual parent", async () => {
  const comment = {
    id: "10_30",
    message: "Thanks",
    from: { id: "25", name: "Customer" },
    created_time: "2026-10-19T10:00:00+0000",
    permalink_url: "https://www.facebook.com/10/posts/20?comment_id=30",
    object: { id: "10_20" },
    parent: { id: "10_29" },
    attachment: {
      target: { id: "photo-1", url: "https://www.facebook.com/photo/1" },
    },
  };
  const { bridge, calls } = fixture("facebook-social", (u) =>
    u.pathname.endsWith("/me")
      ? { id: "10", name: "Page", category: "Software" }
      : comment,
  );
  const result = await bridge.call("comment_get", { comment_id: "10_30" });
  assert.equal(result.id, "10_30");
  assert.equal(result.author_id, "25");
  assert.equal(result.parent_id, "10_29");
  assert.deepEqual(result.media, [
    { id: "photo-1", url: "https://www.facebook.com/photo/1" },
  ]);
  assert.match(calls.at(-1).url.searchParams.get("fields"), /object/);
  assert.match(calls.at(-1).url.searchParams.get("fields"), /attachment/);

  const foreign = fixture("facebook-social", (u) =>
    u.pathname.endsWith("/me")
      ? { id: "10", name: "Page", category: "Software" }
      : { ...comment, object: { id: "99_20" } },
  );
  await assert.rejects(
    foreign.bridge.call("comment_get", { comment_id: "10_30" }),
    /managed Page/,
  );

  const incomplete = fixture("facebook-social", (u) =>
    u.pathname.endsWith("/me")
      ? { id: "10", name: "Page", category: "Software" }
      : { ...comment, permalink_url: undefined },
  );
  await assert.rejects(
    incomplete.bridge.call("comment_get", { comment_id: "10_30" }),
    (error) =>
      error.code === "provider_unavailable" && /url unavailable/.test(error.message),
  );
});

test("LinkedIn comment readback verifies the managed parent and returns exact vendor fields", async () => {
  const commentUrn = "urn:li:comment:(urn:li:activity:9,2)";
  const comment = {
    id: "2",
    commentUrn,
    actor: "urn:li:organization:7",
    created: { time: 1792404000000 },
    message: { text: "Thanks" },
    object: "urn:li:activity:9",
  };
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
        ? { id: "urn:li:share:9", author: "urn:li:organization:7" }
        : comment,
  );
  const result = await bridge.call("comment_get", {
    account_id: "7",
    post_id: "urn:li:share:9",
    comment_id: commentUrn,
  });
  assert.equal(result.id, commentUrn);
  assert.equal(result.author_id, "urn:li:organization:7");
  assert.equal(result.parent_id, "urn:li:activity:9");
  assert.equal(result.parent_post_id, "urn:li:share:9");
  assert.equal(result.text, "Thanks");
  assert.match(result.url, /^https:\/\/www\.linkedin\.com\/feed\/update\//);
  assert.equal(calls.at(-1).url.pathname.endsWith("/comments/2"), true);

  const foreign = fixture("linkedin-social", (u) =>
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
        ? { id: "urn:li:share:9", author: "urn:li:organization:8" }
        : comment,
  );
  await assert.rejects(
    foreign.bridge.call("comment_get", {
      account_id: "7",
      post_id: "urn:li:share:9",
      comment_id: commentUrn,
    }),
    /author/,
  );

  const inconsistent = fixture("linkedin-social", (u) =>
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
        ? { id: "urn:li:share:9", author: "urn:li:organization:7" }
        : { ...comment, object: "urn:li:activity:999" },
  );
  await assert.rejects(
    inconsistent.bridge.call("comment_get", {
      account_id: "7",
      post_id: "urn:li:share:9",
      comment_id: commentUrn,
    }),
    /parent/,
  );
});

test("LinkedIn post readback resolves exact image IDs and URLs under the managed owner", async () => {
  const post = {
    id: "urn:li:share:9",
    author: "urn:li:organization:7",
    commentary: "Photo",
    publishedAt: 1792404000000,
    content: { media: { id: "urn:li:image:1" } },
  };
  const image = {
    id: "urn:li:image:1",
    owner: "urn:li:organization:7",
    status: "AVAILABLE",
    downloadUrl: "https://media.licdn.com/dms/image/1",
  };
  const managed = (asset = image) =>
    fixture("linkedin-social", (u) =>
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
          ? post
          : asset,
    );
  const { bridge, calls } = managed();
  const result = await bridge.call("post_get", {
    account_id: "7",
    post_id: "urn:li:share:9",
  });
  assert.deepEqual(result.media, [
    { id: "urn:li:image:1", url: "https://media.licdn.com/dms/image/1" },
  ]);
  assert.deepEqual(result.vendor_media, [image]);
  assert.equal(calls.at(-1).url.pathname, "/rest/images/urn%3Ali%3Aimage%3A1");

  const foreign = managed({ ...image, owner: "urn:li:organization:8" });
  await assert.rejects(
    foreign.bridge.call("post_get", {
      account_id: "7",
      post_id: "urn:li:share:9",
    }),
    /media owner/,
  );
});

test("LinkedIn post readback keeps reshare linkage distinct from comment parents", async () => {
  const { bridge } = fixture("linkedin-social", (u) =>
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
      : {
          id: "urn:li:share:10",
          author: "urn:li:organization:7",
          commentary: "",
          publishedAt: 1792404000000,
          reshareContext: { parent: "urn:li:share:9" },
        },
  );
  const result = await bridge.call("post_get", {
    account_id: "7",
    post_id: "urn:li:share:10",
  });
  assert.equal(result.reposted_id, "urn:li:share:9");
  assert.equal(result.parent_id, undefined);
});

test("image post receipts retain canonical post IDs and uploaded media IDs", async () => {
  const png = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]).toString(
    "base64",
  );
  const x = fixture("x-social", (u) =>
    u.pathname === "/2/users/me"
      ? { data: { id: "1", name: "ZGI" } }
      : u.pathname === "/2/media/upload"
        ? { data: { id: "123", media_key: "3_123" } }
        : u.pathname === "/2/tweets/post-x"
          ? {
              data: {
                id: "post-x",
                author_id: "1",
                text: "Photo",
                created_at: "2026-10-20T02:00:00Z",
                attachments: { media_keys: ["3_123"] },
              },
              includes: {
                media: [{ media_key: "3_123", url: "https://pbs.twimg.com/media/123" }],
              },
            }
          : { data: { id: "post-x" } },
  );
  const xr = await x.bridge.call("post_create", {
    text: "Photo",
    image_data: png,
    mime_type: "image/png",
  });
  assert.deepEqual(xr.media_receipts, [{ id: "3_123" }]);
  assert.deepEqual(JSON.parse(x.calls.at(-1).init.body).media, {
    media_ids: ["123"],
  });
  const xActual = await x.bridge.call("post_get", { post_id: "post-x" });
  assert.deepEqual(xActual.media, [
    { id: "3_123", url: "https://pbs.twimg.com/media/123" },
  ]);

  const fb = fixture("facebook-social", (u) =>
    u.pathname.endsWith("/me")
      ? { id: "10", name: "Page", category: "Software" }
      : { id: "photo-fb", post_id: "10_post-fb" },
  );
  const fbr = await fb.bridge.call("post_create", {
    text: "Photo",
    image_data: png,
    mime_type: "image/png",
  });
  assert.equal(fbr.id, "10_post-fb");
  assert.deepEqual(fbr.media_receipts, [{ id: "photo-fb" }]);

  const li = fixture("linkedin-social", (u) =>
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
      : u.searchParams.get("action") === "initializeUpload"
        ? {
            value: {
              uploadUrl: "https://www.linkedin.com/dms-uploads/photo",
              image: "urn:li:image:1",
            },
          }
        : u.pathname.includes("/images/")
          ? { status: "AVAILABLE" }
          : { body: {}, headers: { "x-restli-id": "urn:li:share:1" } },
  );
  const lir = await li.bridge.call("post_create", {
    account_id: "7",
    text: "Photo",
    image_data: png,
    mime_type: "image/png",
  });
  assert.equal(lir.id, "urn:li:share:1");
  assert.deepEqual(lir.media_receipts, [{ id: "urn:li:image:1" }]);
});

const additions = [
  "post_search",
  "mention_list",
  "post_repost",
  "post_quote",
  "notification_list",
  "webhook_subscribe",
];
contract.push(
  ...additions.map((name) => ({ name, inputSchema: { type: "object" } })),
);
test("new operations are platform-specific; unsupported filters fail before identity fetch", async () => {
  for (const [provider, name] of [
    ["facebook-social", "post_search"],
    ["linkedin-social", "mention_list"],
    ["x-social", "webhook_subscribe"],
  ]) {
    const { bridge, calls } = fixture(provider, () => ({}));
    assert.ok(!bridge.tools().some((t) => t.name === name));
    await assert.rejects(bridge.call(name, {}), /unavailable/);
    assert.equal(calls.length, 0);
  }
  for (const [provider, name, args] of [
    [
      "linkedin-social",
      "post_list",
      { account_id: "7", since: "2026-10-01T00:00:00Z" },
    ],
    ["facebook-social", "mention_list", { since_id: "9" }],
    [
      "x-social",
      "message_list",
      { conversation_id: "c", until: "2026-10-01T00:00:00Z" },
    ],
  ]) {
    const { bridge, calls } = fixture(provider, () => ({}));
    await assert.rejects(bridge.call(name, args), /unsupported/);
    assert.equal(calls.length, 0);
  }
});
test("X search and mentions forward native increments with stable continuation metadata", async () => {
  const { bridge, calls } = fixture("x-social", (u) =>
    u.pathname.endsWith("/me")
      ? { data: { id: "1" } }
      : { data: [{ id: "12" }], meta: { next_token: "next", newest_id: "12" } },
  );
  const args = {
    query: "zgi -is:retweet",
    until_id: "20",
    since: "2026-10-01T00:00:00Z",
    cursor: "page",
  };
  const result = await bridge.call("post_search", args);
  assert.equal(calls.at(-1).url.pathname, "/2/tweets/search/recent");
  assert.equal(calls.at(-1).url.searchParams.get("start_time"), args.since);
  assert.equal(calls.at(-1).url.searchParams.get("until_id"), "20");
  assert.equal(result.sync.next_cursor, "next");
  assert.equal(result.sync.filters.until_id, "20");
  assert.deepEqual(result.sync.source_ids, ["12"]);
  await bridge.call("mention_list", { since_id: "9" });
  assert.equal(calls.at(-1).url.pathname, "/2/users/1/mentions");
});
test("X repost checks acting account and explicit acknowledgment; quote needs post receipt", async () => {
  for (const acknowledged of [true, false]) {
    const { bridge, calls } = fixture("x-social", (u) =>
      u.pathname.endsWith("/me")
        ? { data: { id: "1" } }
        : { data: { retweeted: acknowledged } },
    );
    await assert.rejects(
      bridge.call("post_repost", { account_id: "2", post_id: "9" }),
      /account/,
    );
    const op = bridge.call("post_repost", { post_id: "9" });
    if (acknowledged) {
      const receipt = await op;
      assert.equal(receipt.data.retweeted, true);
      assert.equal(receipt.reposted_id, "9");
    }
    else await assert.rejects(op, (e) => e.code === "outcome_unknown");
    assert.equal(calls.filter((c) => c.init.method === "POST").length, 1);
    assert.deepEqual(JSON.parse(calls.at(-1).init.body), { tweet_id: "9" });
  }
  const { bridge, calls } = fixture("x-social", (u) =>
    u.pathname.endsWith("/me") ? { data: { id: "1" } } : { data: { id: "30" } },
  );
  const quote = await bridge.call("post_quote", { post_id: "9", text: "Hi" });
  assert.equal(quote.quoted_id, "9");
  assert.deepEqual(JSON.parse(calls.at(-1).init.body), {
    text: "Hi",
    quote_tweet_id: "9",
  });
});
test("LinkedIn notifications use checked org native dates and reshare reuses upstream", async () => {
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
      : u.pathname.endsWith("/posts")
        ? { body: {}, headers: { "x-restli-id": "urn:li:share:11" } }
        : {
            elements: [
              {
                notificationId: 12,
                organizationalEntity: "urn:li:organization:7",
              },
            ],
            paging: {
              start: 0,
              count: 1,
              links: [{ rel: "next", href: "/ignored" }],
            },
          },
  );
  const result = await bridge.call("notification_list", {
    account_id: "7",
    since: "2026-10-01T00:00:00Z",
    limit: 1,
  });
  assert.equal(
    calls.at(-1).url.pathname,
    "/rest/organizationalEntityNotifications",
  );
  assert.equal(
    calls.at(-1).url.searchParams.get("timeRange"),
    "(start:1790812800000)",
  );
  assert.equal(calls.at(-1).init.headers["LinkedIn-Version"], "202609");
  assert.equal(result.sync.next_cursor, "1");
  const repost = await bridge.call("post_repost", {
    account_id: "7",
    post_id: "urn:li:share:9",
    text: "Hello (world)",
  });
  assert.equal(repost.reposted_id, "urn:li:share:9");
  assert.deepEqual(JSON.parse(calls.at(-1).init.body).reshareContext, {
    parent: "urn:li:share:9",
  });
  assert.equal(
    JSON.parse(calls.at(-1).init.body).commentary,
    "Hello \\(world\\)",
  );
});
test("Facebook tagged reads and subscription use only verified Page and fixed fields", async () => {
  const { bridge, calls } = fixture("facebook-social", (u) =>
    u.pathname.endsWith("/me")
      ? { id: "10", category: "Software" }
      : u.pathname.endsWith("/subscribed_apps")
        ? { success: true }
        : {
            data: [{ id: "20_30" }],
            paging: {
              cursors: { after: "p" },
              next: "https://untrusted.invalid/",
            },
          },
  );
  const result = await bridge.call("mention_list", {});
  assert.equal(calls.at(-1).url.pathname, "/v23.0/10/tagged");
  assert.equal(result.sync.next_cursor, "p");
  await bridge.call("webhook_subscribe", {});
  assert.deepEqual(JSON.parse(calls.at(-1).init.body), {
    subscribed_fields: ["feed", "messages", "messaging_postbacks"],
  });
  assert.equal(calls.at(-1).url.pathname, "/v23.0/10/subscribed_apps");
});
test("Facebook inline image uses bounded multipart bytes and rejects mixed media", async () => {
  const { bridge, calls } = fixture("facebook-social", (u) =>
    u.pathname.endsWith("/me")
      ? { id: "10", category: "Software" }
      : { id: "photo" },
  );
  const image_data = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]).toString(
    "base64",
  );
  await assert.rejects(
    bridge.call("post_create", {
      text: "hi",
      image_data,
      mime_type: "image/png",
      image_url: "https://cdn.public.org/a.png",
    }),
    /mutually exclusive/,
  );
  await bridge.call("post_create", {
    text: "hi",
    image_data,
    mime_type: "image/png",
  });
  const call = calls.at(-1);
  assert.equal(call.url.pathname, "/v23.0/10/photos");
  assert.ok(call.init.body instanceof FormData);
  assert.equal(call.init.body.get("caption"), "hi");
  assert.equal(call.init.body.get("source").size, 8);
  assert.equal(call.init.headers.Authorization, `Bearer ${token}`);
});
test("fixed operations reject injected subscription fields and unsupported reshare media before network", async () => {
  for (const [provider, name, args] of [
    [
      "facebook-social",
      "webhook_subscribe",
      { subscribed_fields: ["arbitrary"] },
    ],
    ["x-social", "post_repost", { post_id: "9", text: "ignored" }],
    [
      "linkedin-social",
      "post_repost",
      {
        account_id: "7",
        post_id: "urn:li:share:9",
        image_url: "https://cdn.public.org/a",
      },
    ],
  ]) {
    const { bridge, calls } = fixture(provider, () => ({}));
    await assert.rejects(bridge.call(name, args), /unsupported argument/);
    assert.equal(calls.length, 0);
  }
});
test("new publish operations never retry unknown outcomes or accept missing receipts", async () => {
  for (const [provider, name, args] of [
    ["x-social", "post_repost", { post_id: "9" }],
    ["x-social", "post_quote", { post_id: "9", text: "Hi" }],
    [
      "linkedin-social",
      "post_repost",
      { account_id: "7", post_id: "urn:li:share:9" },
    ],
    ["facebook-social", "webhook_subscribe", {}],
  ]) {
    for (const mode of ["timeout", "no-receipt"]) {
      let writes = 0;
      const bridge = createBridge({
        provider,
        token,
        contract,
        fetch: async (u, i) => {
          if (i.method === "POST") {
            writes++;
            if (mode === "timeout") throw Error("timeout");
            return new Response("{}");
          }
          return new Response(
            JSON.stringify(
              provider === "x-social"
                ? { data: { id: "1" } }
                : provider === "facebook-social"
                  ? { id: "10", category: "Software" }
                  : {
                      elements: [
                        {
                          organization: "urn:li:organization:7",
                          role: "ADMINISTRATOR",
                          state: "APPROVED",
                        },
                      ],
                    },
            ),
          );
        },
      });
      await assert.rejects(
        bridge.call(name, args),
        (e) => e.code === "outcome_unknown" && e.unknown,
      );
      assert.equal(writes, 1);
    }
  }
});
test("X refuses mutually exclusive native date and ID bounds before network", async () => {
  for (const args of [
    { query: "zgi", since: "2026-10-01T00:00:00Z", since_id: "1" },
    { query: "zgi", until: "2026-10-01T00:00:00Z", until_id: "2" },
  ]) {
    const { bridge, calls } = fixture("x-social", () => ({
      data: { id: "1" },
    }));
    await assert.rejects(
      bridge.call("post_search", args),
      /mutually exclusive/,
    );
    assert.equal(calls.length, 0);
  }
});

test("raw unsafe integer IDs remain distinct strings without changing quoted text or scientific numbers", async () => {
  const raw = String.raw`{"elements":[{"notificationId":9007199254740993},{"notificationId":9007199254740992},{"notificationId":9007199254740991}],"text":"9007199254740993 and \"escaped\" and \\ slash","quoted":"9007199254740993","decimal":1.25,"exponent":1.5e3,"scientific":9.007199254740993e15,"negative":-9007199254740993}`;
  const bridge = createBridge({
    provider: "linkedin-social",
    token,
    contract,
    fetch: async (u) =>
      new Response(
        new URL(u).pathname.endsWith("/organizationAcls")
          ? JSON.stringify({
              elements: [
                {
                  organization: "urn:li:organization:7",
                  role: "ADMINISTRATOR",
                  state: "APPROVED",
                },
              ],
            })
          : raw,
      ),
  });
  const result = await bridge.call("notification_list", { account_id: "7" });
  assert.deepEqual(result.sync.source_ids, [
    "9007199254740993",
    "9007199254740992",
    "9007199254740991",
  ]);
  assert.equal(result.elements[0].notificationId, "9007199254740993");
  assert.equal(result.elements[2].notificationId, 9007199254740991);
  assert.equal(result.text, JSON.parse(raw).text);
  assert.equal(result.quoted, "9007199254740993");
  assert.equal(result.decimal, 1.25);
  assert.equal(result.exponent, 1500);
  assert.equal(result.scientific, JSON.parse(raw).scientific);
  assert.equal(result.negative, "-9007199254740993");
  const identity = createBridge({
    provider: "linkedin-social",
    token,
    contract,
    fetch: async () => new Response('{"sub":9007199254740993}'),
  });
  assert.equal((await identity.call("account_get", {})).id, "9007199254740993");
});
test("lossless number handling never repairs invalid JSON", async () => {
  for (const raw of [
    '{"id":09007199254740993}',
    '{"id":9007199254740993x}',
    '{"id":9007199254740993,}',
    '{"id":"unterminated}',
    '{"id":9007199254740993.}',
  ]) {
    const bridge = createBridge({
      provider: "x-social",
      token,
      contract,
      fetch: async () => new Response(raw),
    });
    await assert.rejects(
      bridge.call("account_get", {}),
      (e) => e.code === "provider_unavailable",
    );
  }
});
contract.push({ name: "comment_create", inputSchema: { type: "object" } });
test("top-level comments use verified actors without existing comment IDs", async () => {
  for (const provider of ["x-social", "linkedin-social", "facebook-social"]) {
    const { bridge, calls } = fixture(provider, (u, i) => {
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
      if (u.pathname.endsWith("/me"))
        return provider === "x-social"
          ? { data: { id: "1" } }
          : { id: "10", category: "Software" };
      if (i.method === "POST")
        return provider === "x-social"
          ? { data: { id: "created" } }
          : { id: "created" };
      return provider === "x-social"
        ? { data: { id: "9", author_id: "1" } }
        : { author: "urn:li:organization:7" };
    });
    const args = {
      post_id:
        provider === "x-social"
          ? "9"
          : provider === "linkedin-social"
            ? "urn:li:share:9"
            : "10_20",
      text: "Hello",
      ...(provider === "linkedin-social" ? { account_id: "7" } : {}),
    };
    await bridge.call("comment_create", args);
    const write = calls.at(-1),
      body = JSON.parse(write.init.body);
    assert.equal(calls.filter((c) => c.init.method === "POST").length, 1);
    if (provider === "x-social")
      assert.deepEqual(body, {
        text: "Hello",
        reply: { in_reply_to_tweet_id: "9" },
      });
    if (provider === "linkedin-social")
      assert.deepEqual(body, {
        actor: "urn:li:organization:7",
        message: { text: "Hello" },
        object: "urn:li:share:9",
      });
    if (provider === "facebook-social") {
      assert.equal(write.url.pathname, "/v23.0/10_20/comments");
      assert.deepEqual(body, { message: "Hello" });
    }
  }
});
test("managed Page comments deny foreign posts and comments require native write receipts", async () => {
  for (const provider of ["facebook-social", "linkedin-social"]) {
    const { bridge, calls } = fixture(provider, (u, i) =>
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
            ? { data: { id: "1" } }
            : { id: "10", category: "Software" }
          : provider === "x-social"
            ? { data: { author_id: "other" } }
            : { author: "urn:li:organization:8" },
    );
    await assert.rejects(
      bridge.call("comment_create", {
        account_id: provider === "linkedin-social" ? "7" : undefined,
        post_id:
          provider === "x-social"
            ? "9"
            : provider === "facebook-social"
              ? "99_20"
              : "urn:li:share:9",
        text: "Hello",
      }),
      /author|belong/,
    );
    assert.equal(calls.filter((c) => c.init.method === "POST").length, 0);
  }
  const { bridge } = fixture("x-social", (u, i) =>
    u.pathname.endsWith("/me")
      ? { data: { id: "1" } }
      : i.method === "POST"
        ? {}
        : { data: { id: "9", author_id: "1" } },
  );
  await assert.rejects(
    bridge.call("comment_create", { post_id: "9", text: "Hello" }),
    (e) => e.code === "outcome_unknown",
  );
});
test("X comments allow visible foreign authors while binding the acting account", async () => {
  const { bridge, calls } = fixture("x-social", (u, i) =>
    u.pathname.endsWith("/me")
      ? { data: { id: "1" } }
      : i.method === "POST"
        ? { data: { id: "created" } }
        : { data: { id: "9", author_id: "foreign" } },
  );
  await assert.rejects(
    bridge.call("comment_create", {
      account_id: "other",
      post_id: "9",
      text: "Hi",
    }),
    /account/,
  );
  assert.equal(calls.filter((c) => c.init.method === "POST").length, 0);
  await bridge.call("comment_create", {
    account_id: "1",
    post_id: "9",
    text: "Hi",
  });
  assert.equal(calls.at(-1).init.headers.Authorization, `Bearer ${token}`);
  assert.deepEqual(JSON.parse(calls.at(-1).init.body), {
    text: "Hi",
    reply: { in_reply_to_tweet_id: "9" },
  });
  for (const data of [{ id: "different" }, {}]) {
    const invalid = fixture("x-social", (u) =>
      u.pathname.endsWith("/me") ? { data: { id: "1" } } : { data },
    );
    await assert.rejects(
      invalid.bridge.call("comment_create", { post_id: "9", text: "Hi" }),
    );
    assert.equal(
      invalid.calls.filter((c) => c.init.method === "POST").length,
      0,
    );
  }
});
test("unsafe exponent and decimal numeric source identifiers fail rather than collide", async () => {
  for (const value of ["9.007199254740993e15", "9007199254740993.0", "1.25"]) {
    const bridge = createBridge({
      provider: "linkedin-social",
      token,
      contract,
      fetch: async (u) =>
        new Response(
          new URL(u).pathname.endsWith("/organizationAcls")
            ? JSON.stringify({
                elements: [
                  {
                    organization: "urn:li:organization:7",
                    role: "ADMINISTRATOR",
                    state: "APPROVED",
                  },
                ],
              })
            : `{"elements":[{"notificationId":${value}}]}`,
        ),
    });
    await assert.rejects(
      bridge.call("notification_list", { account_id: "7" }),
      (e) => e.code === "provider_unavailable",
    );
  }
});

// Derived from 2060-io/linkedin-mcp c1cddcfaf98bf9129fc5f87d21d181f68c00a9b7. See LICENSE.
// Modification: injected transport for bounded, credential-safe requests.
import { encodeUrn } from "./helpers.js";
const DEFAULT_API_BASE = "https://api.linkedin.com";
const RESTLI_VERSION = "2.0.0";
const LITTLE_TEXT_RESERVED = /[\\|{}@[\]()<>#*_~]/g;
function escapeLittleText(s) {
  return s.replace(LITTLE_TEXT_RESERVED, (c) => `\\${c}`);
}
export function toLittleText(input) {
  let out = "";
  let last = 0;
  const hashtag = /(^|\s)#([A-Za-z0-9]+)/g;
  let m;
  while ((m = hashtag.exec(input)) !== null) {
    out += escapeLittleText(input.slice(last, m.index));
    out += m[1];
    out += `#${m[2]}`;
    last = hashtag.lastIndex;
  }
  out += escapeLittleText(input.slice(last));
  return out;
}
export class LinkedInApiClient {
  config;
  apiBase;
  constructor(config) {
    this.config = config;
    this.apiBase = config.apiBase ?? DEFAULT_API_BASE;
  }
  get restBase() {
    return `${this.apiBase}/rest`;
  }
  async headers(json) {
    const token = await this.config.getAccessToken();
    const h = {
      Authorization: `Bearer ${token}`,
      "LinkedIn-Version": this.config.version,
      "X-Restli-Protocol-Version": RESTLI_VERSION,
    };
    if (json) h["Content-Type"] = "application/json";
    return h;
  }
  async request(url, method, operation, body) {
    const init = {
      method,
      headers: await this.headers(body !== undefined),
    };
    if (body !== undefined) init.body = JSON.stringify(body);
    const response = await this.config.fetch(url, init);
    const rawText = await response.text();
    let parsed = undefined;
    if (rawText) {
      try {
        parsed = JSON.parse(rawText);
      } catch {
        parsed = rawText;
      }
    }
    if (!response.ok) {
      throw new Error(
        this.formatError(operation, response.status, parsed, rawText),
      );
    }
    return {
      status: response.status,
      ok: response.ok,
      headers: response.headers,
      body: parsed,
      rawText,
    };
  }
  formatError(operation, status, parsed, rawText) {
    const message =
      parsed && typeof parsed === "object" && "message" in parsed
        ? String(parsed.message)
        : rawText.slice(0, 300);
    if (status === 429) {
      const isShareLimit = /share|daily limit|UGC/i.test(message);
      const kind = isShareLimit ? "author share limit" : "API rate limit";
      return `${operation} rate limited (HTTP 429, ${kind}): ${message}. Try again later.`;
    }
    if (status === 401) {
      return `${operation} unauthorized (HTTP 401): ${message}. Re-authorize at /oauth/start.`;
    }
    return `${operation} failed (HTTP ${status}): ${message}`;
  }
  restliId(res) {
    return (
      res.headers.get("x-restli-id") ?? res.headers.get("x-linkedin-id") ?? ""
    );
  }
  async getUserInfo() {
    const res = await this.request(
      `${this.apiBase}/v2/userinfo`,
      "GET",
      "getUserInfo",
    );
    return res.body;
  }
  async createPost(params) {
    const body = {
      author: params.authorUrn,
      commentary: toLittleText(params.commentary),
      visibility: params.visibility ?? "PUBLIC",
      distribution: {
        feedDistribution: "MAIN_FEED",
        targetEntities: [],
        thirdPartyDistributionChannels: [],
      },
      lifecycleState: "PUBLISHED",
      isReshareDisabledByAuthor: false,
    };
    if (params.link) {
      body.content = {
        article: {
          source: params.link.url,
          ...(params.link.title
            ? {
                title: params.link.title,
              }
            : {}),
          ...(params.link.description
            ? {
                description: params.link.description,
              }
            : {}),
        },
      };
    } else if (params.imageUrn) {
      body.content = {
        media: {
          id: params.imageUrn,
          ...(params.imageAltText
            ? {
                altText: params.imageAltText,
              }
            : {}),
        },
      };
    } else if (params.multiImages && params.multiImages.length > 0) {
      body.content = {
        multiImage: {
          images: params.multiImages.map((img) => ({
            id: img.id,
            ...(img.altText
              ? {
                  altText: img.altText,
                }
              : {}),
          })),
        },
      };
    }
    if (params.reshareUrn) {
      body.reshareContext = {
        parent: params.reshareUrn,
      };
    }
    const res = await this.request(
      `${this.restBase}/posts`,
      "POST",
      "createPost",
      body,
    );
    return {
      id: this.restliId(res),
      author: params.authorUrn,
    };
  }
  async deletePost(postUrn) {
    await this.request(
      `${this.restBase}/posts/${encodeUrn(postUrn)}`,
      "DELETE",
      "deletePost",
    );
    return {
      id: postUrn,
      deleted: true,
    };
  }
  async commentOnPost(params) {
    const url = `${this.restBase}/socialActions/${encodeUrn(params.postUrn)}/comments`;
    const res = await this.request(url, "POST", "commentOnPost", {
      actor: params.actorUrn,
      message: {
        text: params.message,
      },
      ...(params.objectUrn ? { object: params.objectUrn } : {}),
      ...(params.parentComment ? { parentComment: params.parentComment } : {}),
    });
    const bodyId =
      res.body &&
      typeof res.body === "object" &&
      ("$URN" in res.body || "commentUrn" in res.body || "id" in res.body)
        ? String(res.body.$URN || res.body.commentUrn || res.body.id)
        : this.restliId(res);
    return {
      id: bodyId,
      post_urn: params.postUrn,
    };
  }
  async reactToPost(params) {
    const url = `${this.restBase}/reactions?actor=${encodeUrn(params.actorUrn)}`;
    await this.request(url, "POST", "reactToPost", {
      root: params.postUrn,
      reactionType: params.reactionType,
    });
    return {
      post_urn: params.postUrn,
      reaction: params.reactionType,
    };
  }
  async uploadImage(params) {
    const init = await this.request(
      `${this.restBase}/images?action=initializeUpload`,
      "POST",
      "initializeImageUpload",
      {
        initializeUploadRequest: {
          owner: params.ownerUrn,
        },
      },
    );
    const value = init.body.value;
    if (!value?.uploadUrl || !value?.image) {
      throw new Error(
        `initializeImageUpload: unexpected response ${init.rawText.slice(0, 300)}`,
      );
    }
    const bytes = Buffer.from(params.dataBase64, "base64");
    const token = await this.config.getAccessToken();
    const put = await this.config.fetch(value.uploadUrl, {
      method: "PUT",
      headers: {
        Authorization: `Bearer ${token}`,
      },
      body: new Uint8Array(bytes),
    });
    if (!put.ok) {
      const text = await put.text();
      throw new Error(
        `uploadImage PUT failed (HTTP ${put.status}): ${text.slice(0, 300)}`,
      );
    }
    await this.waitForImageAvailable(value.image);
    return {
      image_urn: value.image,
    };
  }
  async waitForImageAvailable(
    imageUrn,
    { attempts = 30, intervalMs = 1000 } = {},
  ) {
    for (let i = 0; i < attempts; i++) {
      const res = await this.request(
        `${this.restBase}/images/${encodeUrn(imageUrn)}`,
        "GET",
        "getImage",
      );
      const status = res.body?.status;
      if (status === "AVAILABLE") return;
      if (status && status !== "PROCESSING" && status !== "WAITING_UPLOAD") {
        throw new Error(
          `Image processing failed for ${imageUrn} (status: ${status}).`,
        );
      }
      await new Promise((resolve) => setTimeout(resolve, intervalMs));
    }
    throw new Error(
      `Image ${imageUrn} was not ready after ${attempts}s. Aborted to avoid an unrenderable post; try again.`,
    );
  }
}

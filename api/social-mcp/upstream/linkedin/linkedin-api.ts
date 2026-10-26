import { encodeUrn } from "./helpers.js";
import type { UserInfo } from "./oauth.js";

const DEFAULT_API_BASE = "https://api.linkedin.com";
const RESTLI_VERSION = "2.0.0";

// LinkedIn parses post `commentary` as "Little Text": these reserved characters
// must be backslash-escaped, or content is silently truncated at the character
// (e.g. an unescaped "(" drops everything after it). JSON.stringify adds the
// second backslash on the wire.
const LITTLE_TEXT_RESERVED = /[\\|{}@[\]()<>#*_~]/g;

function escapeLittleText(s: string): string {
  return s.replace(LITTLE_TEXT_RESERVED, (c) => `\\${c}`);
}

/**
 * Format plain user text as LinkedIn "Little Text": escape every reserved
 * character so the post isn't truncated, while leaving `#hashtags` as plain
 * hashtag elements so they remain clickable. (Do NOT use `{hashtag|#|tag}`
 * templates here — a malformed template invalidates the whole string and makes
 * LinkedIn render the raw escaped text.)
 */
export function toLittleText(input: string): string {
  let out = "";
  let last = 0;
  const hashtag = /(^|\s)#([A-Za-z0-9]+)/g;
  let m: RegExpExecArray | null;
  while ((m = hashtag.exec(input)) !== null) {
    out += escapeLittleText(input.slice(last, m.index));
    out += m[1]; // boundary (start or whitespace), never a reserved char
    out += `#${m[2]}`; // valid HashtagElement, left unescaped to stay clickable
    last = hashtag.lastIndex;
  }
  out += escapeLittleText(input.slice(last));
  return out;
}

export type Visibility = "PUBLIC" | "CONNECTIONS" | "LOGGED_IN";
export type ReactionType =
  | "LIKE"
  | "PRAISE"
  | "EMPATHY"
  | "INTEREST"
  | "APPRECIATION"
  | "ENTERTAINMENT";

export interface LinkedInApiConfig {
  version: string; // LinkedIn-Version header, YYYYMM
  getAccessToken: () => Promise<string>;
  apiBase?: string; // default https://api.linkedin.com
}

export interface CreatePostParams {
  authorUrn: string;
  commentary: string;
  visibility?: Visibility;
  link?: { url: string; title?: string; description?: string };
  imageUrn?: string;
  imageAltText?: string;
  multiImages?: Array<{ id: string; altText?: string }>;
  reshareUrn?: string;
}

interface RawResponse {
  status: number;
  ok: boolean;
  headers: Headers;
  body: unknown;
  rawText: string;
}

export class LinkedInApiClient {
  private apiBase: string;

  constructor(private config: LinkedInApiConfig) {
    this.apiBase = config.apiBase ?? DEFAULT_API_BASE;
  }

  private get restBase(): string {
    return `${this.apiBase}/rest`;
  }

  private async headers(json: boolean): Promise<Record<string, string>> {
    const token = await this.config.getAccessToken();
    const h: Record<string, string> = {
      Authorization: `Bearer ${token}`,
      "LinkedIn-Version": this.config.version,
      "X-Restli-Protocol-Version": RESTLI_VERSION,
    };
    if (json) h["Content-Type"] = "application/json";
    return h;
  }

  private async request(
    url: string,
    method: string,
    operation: string,
    body?: unknown,
  ): Promise<RawResponse> {
    const init: RequestInit = { method, headers: await this.headers(body !== undefined) };
    if (body !== undefined) init.body = JSON.stringify(body);

    const response = await fetch(url, init);
    const rawText = await response.text();
    let parsed: unknown = undefined;
    if (rawText) {
      try {
        parsed = JSON.parse(rawText);
      } catch {
        parsed = rawText;
      }
    }

    if (!response.ok) {
      throw new Error(this.formatError(operation, response.status, parsed, rawText));
    }
    return { status: response.status, ok: response.ok, headers: response.headers, body: parsed, rawText };
  }

  private formatError(operation: string, status: number, parsed: unknown, rawText: string): string {
    const message =
      parsed && typeof parsed === "object" && "message" in parsed
        ? String((parsed as { message: unknown }).message)
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

  /** The created entity URN, returned in the x-restli-id / x-linkedin-id header. */
  private restliId(res: RawResponse): string {
    return res.headers.get("x-restli-id") ?? res.headers.get("x-linkedin-id") ?? "";
  }

  // --- Identity ---

  async getUserInfo(): Promise<UserInfo> {
    const res = await this.request(`${this.apiBase}/v2/userinfo`, "GET", "getUserInfo");
    return res.body as UserInfo;
  }

  // --- Posts ---

  async createPost(params: CreatePostParams): Promise<{ id: string; author: string }> {
    const body: Record<string, unknown> = {
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
          ...(params.link.title ? { title: params.link.title } : {}),
          ...(params.link.description ? { description: params.link.description } : {}),
        },
      };
    } else if (params.imageUrn) {
      body.content = {
        media: {
          id: params.imageUrn,
          ...(params.imageAltText ? { altText: params.imageAltText } : {}),
        },
      };
    } else if (params.multiImages && params.multiImages.length > 0) {
      body.content = {
        multiImage: {
          images: params.multiImages.map((img) => ({
            id: img.id,
            ...(img.altText ? { altText: img.altText } : {}),
          })),
        },
      };
    }

    // Reshare of an existing post (commentary becomes the reshare comment).
    if (params.reshareUrn) {
      body.reshareContext = { parent: params.reshareUrn };
    }

    const res = await this.request(`${this.restBase}/posts`, "POST", "createPost", body);
    return { id: this.restliId(res), author: params.authorUrn };
  }

  async deletePost(postUrn: string): Promise<{ id: string; deleted: true }> {
    await this.request(`${this.restBase}/posts/${encodeUrn(postUrn)}`, "DELETE", "deletePost");
    return { id: postUrn, deleted: true };
  }

  // --- Engagement ---

  async commentOnPost(params: {
    postUrn: string;
    actorUrn: string;
    message: string;
  }): Promise<{ id: string; post_urn: string }> {
    const url = `${this.restBase}/socialActions/${encodeUrn(params.postUrn)}/comments`;
    const res = await this.request(url, "POST", "commentOnPost", {
      actor: params.actorUrn,
      message: { text: params.message },
    });
    const bodyId =
      res.body && typeof res.body === "object" && "$URN" in res.body
        ? String((res.body as { $URN: unknown }).$URN)
        : this.restliId(res);
    return { id: bodyId, post_urn: params.postUrn };
  }

  async reactToPost(params: {
    postUrn: string;
    actorUrn: string;
    reactionType: ReactionType;
  }): Promise<{ post_urn: string; reaction: ReactionType }> {
    const url = `${this.restBase}/reactions?actor=${encodeUrn(params.actorUrn)}`;
    await this.request(url, "POST", "reactToPost", {
      root: params.postUrn,
      reactionType: params.reactionType,
    });
    return { post_urn: params.postUrn, reaction: params.reactionType };
  }

  // --- Media ---

  /**
   * Upload an image: initialize the upload to get an upload URL + image URN,
   * then PUT the bytes. Returns the image URN for use in a post.
   */
  async uploadImage(params: {
    ownerUrn: string;
    dataBase64: string;
  }): Promise<{ image_urn: string }> {
    const init = await this.request(
      `${this.restBase}/images?action=initializeUpload`,
      "POST",
      "initializeImageUpload",
      { initializeUploadRequest: { owner: params.ownerUrn } },
    );

    const value = (init.body as { value?: { uploadUrl?: string; image?: string } }).value;
    if (!value?.uploadUrl || !value?.image) {
      throw new Error(`initializeImageUpload: unexpected response ${init.rawText.slice(0, 300)}`);
    }

    const bytes = Buffer.from(params.dataBase64, "base64");
    const token = await this.config.getAccessToken();
    const put = await fetch(value.uploadUrl, {
      method: "PUT",
      headers: { Authorization: `Bearer ${token}` },
      body: new Uint8Array(bytes),
    });
    if (!put.ok) {
      const text = await put.text();
      throw new Error(`uploadImage PUT failed (HTTP ${put.status}): ${text.slice(0, 300)}`);
    }

    // LinkedIn processes the upload asynchronously. Referencing the image in a post
    // before it is AVAILABLE produces a share that renders "This post cannot be
    // displayed", so wait for processing to finish before returning the URN.
    await this.waitForImageAvailable(value.image);

    return { image_urn: value.image };
  }

  /**
   * Poll an image's processing status until it is AVAILABLE. Throws on a terminal
   * failure or if processing does not complete within the timeout, so callers never
   * publish a post that references an unprocessed image.
   */
  private async waitForImageAvailable(
    imageUrn: string,
    { attempts = 30, intervalMs = 1000 }: { attempts?: number; intervalMs?: number } = {},
  ): Promise<void> {
    for (let i = 0; i < attempts; i++) {
      const res = await this.request(
        `${this.restBase}/images/${encodeUrn(imageUrn)}`,
        "GET",
        "getImage",
      );
      const status = (res.body as { status?: string } | undefined)?.status;
      if (status === "AVAILABLE") return;
      if (status && status !== "PROCESSING" && status !== "WAITING_UPLOAD") {
        throw new Error(`Image processing failed for ${imageUrn} (status: ${status}).`);
      }
      await new Promise((resolve) => setTimeout(resolve, intervalMs));
    }
    throw new Error(
      `Image ${imageUrn} was not ready after ${attempts}s. Aborted to avoid an unrenderable post; try again.`,
    );
  }
}

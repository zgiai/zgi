import { BaseService } from '@/lib/http/services';
import type {
  MarketActivationRequest,
  MarketActivationRequestInput,
  MarketActivationRequestPage,
  MarketActivationRequestQuery,
  MarketCallContent,
  MarketCallLogPage,
  MarketCallLogQuery,
  MarketCategory,
  MarketConnection,
  MarketCredential,
  MarketCredentialCreated,
  MarketToolBinding,
  MarketToolBindingInput,
  MarketToolCandidates,
  MarketToolSubjectType,
  MarketInstalledSkill,
  MarketListing,
  MarketListingKind,
  MarketListingPage,
  MarketListingQuery,
  MarketMeta,
  MarketSkillInstall,
  MarketSkillManifest,
  MarketSubscription,
} from './types/zgi-market';

interface MarketEnvelope<T> {
  code: number;
  message?: string;
  data: T;
}

/**
 * ZGI Market as the members of the current organization see it. This instance forwards every
 * request to the console's market proxy; the answers are the market's own envelopes.
 */
class ZgiMarketService extends BaseService {
  constructor() {
    super({
      endpoint: 'main',
      basePath: '/console/api/workspaces/current/zgi-market',
    });
  }

  async getCategories(kind?: MarketListingKind): Promise<MarketCategory[]> {
    const response = await this.request<MarketEnvelope<MarketCategory[]>>(
      'get',
      '/categories',
      undefined,
      { params: kind ? { kind } : undefined }
    );
    return response.data ?? [];
  }

  async getListings(query: MarketListingQuery): Promise<MarketListingPage> {
    const response = await this.request<MarketListingPage & { code: number }>(
      'get',
      '/listings',
      undefined,
      {
        params: {
          page: query.page,
          per_page: query.per_page,
          ...(query.kind ? { kind: query.kind } : {}),
          ...(query.category ? { category: query.category } : {}),
          ...(query.q ? { q: query.q } : {}),
        },
      }
    );
    return { data: response.data ?? [], meta: response.meta };
  }

  async getListing(kind: MarketListingKind, slug: string): Promise<MarketListing> {
    const response = await this.request<MarketEnvelope<MarketListing>>(
      'get',
      `/listings/${encodeURIComponent(kind)}/${encodeURIComponent(slug)}`
    );
    return response.data;
  }

  /** What this instance knows about the caller and the market's public gateway. */
  async getMeta(): Promise<MarketMeta> {
    const response = await this.request<MarketEnvelope<MarketMeta>>('get', '/meta');
    return response.data;
  }

  async getSubscriptions(): Promise<MarketSubscription[]> {
    const response = await this.request<MarketEnvelope<MarketSubscription[]>>(
      'get',
      '/subscriptions',
      undefined,
      { params: { per_page: 100 } }
    );
    return response.data ?? [];
  }

  async activate(listingId: number, offeringId?: number): Promise<MarketSubscription> {
    const response = await this.request<MarketEnvelope<MarketSubscription>>(
      'post',
      '/subscriptions',
      { listing_id: listingId, ...(offeringId ? { offering_id: offeringId } : {}) }
    );
    return response.data;
  }

  /** Members' requests to activate an item: an administrator reads everyone's, a member their own. */
  async getActivationRequests(
    query: MarketActivationRequestQuery = {}
  ): Promise<MarketActivationRequestPage> {
    const response = await this.request<MarketEnvelope<MarketActivationRequestPage>>(
      'get',
      '/activation-requests',
      undefined,
      {
        params: {
          ...(query.status ? { status: query.status } : {}),
          ...(query.listing_id ? { listing_id: query.listing_id } : {}),
          ...(query.mine ? { mine: 1 } : {}),
        },
      }
    );
    return response.data ?? { data: [], pending: 0 };
  }

  async requestActivation(input: MarketActivationRequestInput): Promise<MarketActivationRequest> {
    const response = await this.request<MarketEnvelope<MarketActivationRequest>>(
      'post',
      '/activation-requests',
      input
    );
    return response.data;
  }

  /** approve and reject are an administrator's; cancel is the requester's. */
  async settleActivationRequest(
    id: string,
    action: 'approve' | 'reject' | 'cancel',
    note = ''
  ): Promise<MarketActivationRequest> {
    const response = await this.request<MarketEnvelope<MarketActivationRequest>>(
      'post',
      `/activation-requests/${encodeURIComponent(id)}/${action}`,
      note ? { note } : {}
    );
    return response.data;
  }

  /** The organization skills that were installed from market listings. */
  async getInstalledSkills(): Promise<MarketInstalledSkill[]> {
    const response = await this.request<MarketEnvelope<MarketInstalledSkill[]>>(
      'get',
      '/installs/skills'
    );
    return response.data ?? [];
  }

  /**
   * Installs a skill listing into the organization: the market activates it, and this instance
   * imports its package as an organization skill.
   */
  async installSkill(slug: string, overwrite = false): Promise<MarketSkillInstall> {
    const response = await this.request<MarketEnvelope<MarketSkillInstall>>(
      'post',
      `/installs/skills/${encodeURIComponent(slug)}`,
      { overwrite }
    );
    return response.data;
  }

  /**
   * The files of a skill package. The market answers it only to an organization that activated
   * the skill; before that the request is refused.
   */
  async getSkillManifest(slug: string): Promise<MarketSkillManifest> {
    const response = await this.request<MarketEnvelope<MarketSkillManifest>>(
      'get',
      `/skills/${encodeURIComponent(slug)}/manifest`
    );
    return response.data;
  }

  /** The organization's market credentials, newest first. A secret is never among them. */
  async getCredentials(): Promise<MarketCredential[]> {
    const response = await this.request<MarketEnvelope<MarketCredential[]>>(
      'get',
      '/credentials',
      undefined,
      { params: { per_page: 100 } }
    );
    return response.data ?? [];
  }

  /** Creates a credential. The answer carries the secret, this once. */
  async createCredential(name: string): Promise<MarketCredentialCreated> {
    const response = await this.request<MarketEnvelope<MarketCredentialCreated>>(
      'post',
      '/credentials',
      { name }
    );
    return response.data;
  }

  /** Revokes a credential for good; calls made with it are refused from then on. */
  async revokeCredential(id: number): Promise<void> {
    await this.request('delete', `/credentials/${id}`);
  }

  /** The organization's connections at the market. */
  async getConnections(): Promise<MarketConnection[]> {
    const response = await this.request<MarketEnvelope<MarketConnection[]>>('get', '/connections');
    return response.data ?? [];
  }

  /**
   * Starts the OAuth authorization of a connector. The market sends the browser back to
   * returnUrl, which must be a page of this instance, after the vendor's consent.
   */
  async connect(connectorSlug: string, name: string, returnUrl: string): Promise<MarketConnection> {
    const response = await this.request<MarketEnvelope<MarketConnection>>('post', '/connections', {
      connector_slug: connectorSlug,
      name,
      return_url: returnUrl,
    });
    return response.data;
  }

  async deleteConnection(id: number): Promise<void> {
    await this.request('delete', `/connections/${id}`);
  }

  /** Forwards the vendor's redirect to the market and returns the page to open next. */
  async completeOAuth(query: string): Promise<string> {
    const response = await this.request<MarketEnvelope<{ redirect: string }>>(
      'get',
      `/connections/oauth/callback${query.startsWith('?') ? query : `?${query}`}`
    );
    return response.data.redirect;
  }

  /** The organization's market tool calls, newest first, for administrators. */
  async getCallLogs(query: MarketCallLogQuery): Promise<MarketCallLogPage> {
    const response = await this.request<MarketEnvelope<MarketCallLogPage>>(
      'get',
      '/call-logs',
      undefined,
      {
        params: {
          from: query.from,
          to: query.to,
          page: query.page ?? 1,
          per_page: query.per_page ?? 20,
          ...(query.status ? { status: query.status } : {}),
          ...(query.source ? { source: query.source } : {}),
        },
      }
    );
    return response.data;
  }

  /** What a call sent and answered, as the market kept it, by its request id. */
  async getCallContent(requestId: string): Promise<MarketCallContent> {
    const response = await this.request<MarketEnvelope<MarketCallContent>>(
      'get',
      '/calls/content',
      undefined,
      { params: { request_id: requestId } }
    );
    return response.data;
  }

  /** The activated MCP listings that may be bound, with the organization's connections. */
  async getToolCandidates(): Promise<MarketToolCandidates> {
    const response = await this.request<MarketEnvelope<MarketToolCandidates>>(
      'get',
      '/tools/candidates'
    );
    return response.data ?? { listings: [], connections: [] };
  }

  async getSubjectTools(
    subjectType: MarketToolSubjectType,
    subjectId: string
  ): Promise<MarketToolBinding> {
    const response = await this.request<MarketEnvelope<MarketToolBinding>>(
      'get',
      `/tools/subjects/${subjectType}/${encodeURIComponent(subjectId)}`
    );
    return response.data;
  }

  /** Replaces the market tools of a subject. An empty list removes them all. */
  async bindSubjectTools(
    subjectType: MarketToolSubjectType,
    subjectId: string,
    items: MarketToolBindingInput[]
  ): Promise<MarketToolBinding> {
    const response = await this.request<MarketEnvelope<MarketToolBinding>>(
      'put',
      `/tools/subjects/${subjectType}/${encodeURIComponent(subjectId)}`,
      { items }
    );
    return response.data;
  }
}

export const zgiMarketService = new ZgiMarketService();

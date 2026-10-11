import { BaseService } from '@/lib/http/services';
import type {
  MarketCategory,
  MarketListing,
  MarketListingKind,
  MarketListingPage,
  MarketListingQuery,
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
}

export const zgiMarketService = new ZgiMarketService();

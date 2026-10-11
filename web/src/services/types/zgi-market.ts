/** A text of the market in both of its languages. */
export interface MarketText {
  'zh-CN': string;
  'en-US': string;
}

export const marketListingKinds = [
  'api',
  'mcp',
  'skill',
  'plugin',
  'dataset',
  'connector',
] as const;
export type MarketListingKind = (typeof marketListingKinds)[number];

export interface MarketCategoryRef {
  id: number;
  slug: string;
  name: MarketText;
}

export interface MarketOperation {
  method: string;
  path: string;
  input_schema?: Record<string, unknown>;
  output_schema?: Record<string, unknown>;
}

export interface MarketOffering {
  id: number;
  slug: string;
  provider: string;
  tier: string;
  protocol_family: string;
  meters: string[];
  base_url?: string;
  requires_connector?: string;
  operations?: MarketOperation[];
}

/** One price of a listing. An amount is in micros of a yuan, or in milli-points when `currency` is PTS. */
export interface MarketPrice {
  offering_id: number;
  offering?: string;
  meter: string;
  unit_price_micros: number;
  per_quantity: number;
  currency: 'CNY' | 'PTS';
}

export interface MarketTool {
  name: string;
  description: MarketText;
  risk_level: 'read' | 'write' | 'destructive';
}

export interface MarketListing {
  id: number;
  kind: MarketListingKind;
  slug: string;
  primary_category: MarketCategoryRef | null;
  tags: string[];
  card: { title: MarketText; subtitle: MarketText; description: MarketText; badges: string[] };
  detail: { body: MarketText; origin_url?: string };
  offerings: MarketOffering[];
  prices?: MarketPrice[];
  mcp?: { tools: MarketTool[] } | null;
  current_version?: { label: string; published_at: string } | null;
  published_at?: string | null;
}

export interface MarketPageMeta {
  current_page: number;
  last_page: number;
  per_page: number;
  total: number;
}

export interface MarketListingPage {
  data: MarketListing[];
  meta: MarketPageMeta;
}

export interface MarketCategory {
  id: number;
  slug: string;
  name: MarketText;
  kinds: MarketListingKind[];
  listing_count?: number;
  children?: MarketCategory[];
}

export interface MarketSubscription {
  id: number;
  listing_id: number;
  listing_kind: MarketListingKind;
  listing_slug: string;
  offering_id?: number | null;
  offering_slug?: string;
  status: string;
}

export interface MarketListingQuery {
  kind?: MarketListingKind;
  category?: string;
  q?: string;
  page: number;
  per_page: number;
}

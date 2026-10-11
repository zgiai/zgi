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

/**
 * One price of a listing. An amount is in micros of a yuan, or in milli-points when `currency` is
 * PTS. `offering_slug` names the offering the price applies to and is absent for usage without one.
 */
export interface MarketPrice {
  offering_slug?: string;
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
  required_connectors?: MarketRequiredConnector[];
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

/** An organization skill that was installed from a market listing. */
export interface MarketInstalledSkill {
  slug: string;
  skill_id: string;
  name: string;
  version: string;
}

/**
 * What installing a skill listing did: the skill is installed, or its package was refused by the
 * import checks, or it would replace a skill of the organization that came from elsewhere.
 */
export interface MarketSkillInstall {
  installed: boolean;
  enabled: boolean;
  skill_id?: string;
  name?: string;
  version?: string;
  validation_errors?: string[];
  warnings?: string[];
  conflict?: string;
}

export interface MarketListingQuery {
  kind?: MarketListingKind;
  category?: string;
  q?: string;
  page: number;
  per_page: number;
}

/** A connector whose connection the organization needs before it can use a listing. */
export interface MarketRequiredConnector {
  slug: string;
  auth_type: string;
  title: MarketText;
}

/** A connection of the organization at the market. */
export interface MarketConnection {
  id: number;
  connector_slug: string;
  name: string;
  status: string;
  auth_type?: string;
  authorize_url?: string;
}

export type MarketToolRisk = 'read' | 'write' | 'destructive';

/** An activated MCP listing the organization may bind to an agent or a digital human. */
export interface MarketToolCandidate {
  listing_id: number;
  slug: string;
  title: MarketText;
  subscription_id: number;
  tools: Array<{ name: string; description: MarketText; risk_level: MarketToolRisk }>;
  required_connectors: MarketRequiredConnector[];
}

export interface MarketToolCandidates {
  listings: MarketToolCandidate[];
  connections: MarketConnection[];
}

export type MarketToolSubjectType = 'agent' | 'digital_human';

/** One MCP listing bound to a subject. */
export interface MarketToolBindingItem {
  listing_id: number;
  slug: string;
  title?: string;
  subscription_id: number;
  connection_id?: number | null;
  tools: Array<{ name: string; risk_level: MarketToolRisk }>;
}

/** The market tools of one agent or digital human. */
export interface MarketToolBinding {
  subject_type: MarketToolSubjectType;
  subject_id: string;
  items: MarketToolBindingItem[];
  updated_at: string | null;
  gateway_configured: boolean;
}

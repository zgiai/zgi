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
  /** Present for the meter `unit` only: what one unit is in that offering, such as `credits`. */
  unit_name?: string;
  unit_price_micros: number;
  per_quantity: number;
  currency: 'CNY' | 'PTS';
}

export interface MarketTool {
  name: string;
  description: MarketText;
  risk_level: 'read' | 'write' | 'destructive';
}

/** Where a listing comes from and under which terms; every member may be empty. */
export interface MarketListingDetail {
  body: MarketText;
  origin_url?: string;
  source_repository?: string;
  source_revision?: string;
  license?: string;
  review_status?: string;
}

/** A listing a plugin bundles, when it is published in this market. */
export interface MarketPluginMember {
  kind: MarketListingKind;
  slug: string;
  listing_id?: number;
  offering_slug?: string;
  title?: MarketText;
}

/** What a plugin listing packages. */
export interface MarketPluginSummary {
  manifest?: {
    name?: string;
    version?: string;
    description?: string;
    author?: string;
    formats?: string[];
  };
  components?: Partial<Record<'skills' | 'commands' | 'agents' | 'mcp_servers' | 'hooks', number>>;
  has_package?: boolean;
  requirements?: string[];
  members?: MarketPluginMember[];
}

/**
 * What the package of a skill listing is, as the market describes it to anyone who may see the
 * listing: what SKILL.md declares, the beginning of its text, and the files beside it. The market
 * caps the file list and says how many it left out.
 */
export interface MarketSkillSummary {
  name?: string;
  description?: string;
  when_to_use?: string;
  excerpt?: string;
  excerpt_truncated?: boolean;
  files: Array<{ path: string; size: number }>;
  file_count: number;
  files_omitted?: number;
  total_bytes: number;
  has_scripts: boolean;
  interpreters?: string[];
}

/** How a connector listing authenticates and which listings act through it. */
export interface MarketConnectorSummary {
  auth_type: string;
  used_by?: Array<{
    kind: MarketListingKind;
    slug: string;
    title?: MarketText;
    tools?: MarketTool[];
  }>;
}

export interface MarketListing {
  id: number;
  kind: MarketListingKind;
  slug: string;
  primary_category: MarketCategoryRef | null;
  secondary_categories?: MarketCategoryRef[];
  tags: string[];
  card: { title: MarketText; subtitle: MarketText; description: MarketText; badges: string[] };
  detail: MarketListingDetail;
  offerings: MarketOffering[];
  prices?: MarketPrice[];
  /** The most one call of each bounded offering can cost at the list prices, in milli-points. */
  price_caps?: Array<{ offering_slug: string; max_charge_micros: number }>;
  mcp?: { tools: MarketTool[] } | null;
  plugin?: MarketPluginSummary | null;
  /** Only on the detail of one skill listing, and only when the market has summarized its package. */
  skill?: MarketSkillSummary | null;
  connector?: MarketConnectorSummary | null;
  required_connectors?: MarketRequiredConnector[];
  current_version?: { version_no?: number; label: string; published_at: string } | null;
  published_at?: string | null;
}

/** What this instance tells a market page beside the market's own answers. */
export interface MarketMeta {
  /** The public origin of the market gateway; empty when this instance names none. */
  gateway_url: string;
  /** Whether the caller administers the organization. */
  admin: boolean;
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
/** Why an operation of an API offering cannot become a tool. */
export type MarketOperationReason = 'no_input_schema' | 'input_schema_not_object';

/** One operation of an activated API offering. `key` is its method and path. */
export interface MarketCandidateOperation {
  key: string;
  name: string;
  method: string;
  path: string;
  description: string;
  risk_level: MarketToolRisk;
  bindable: boolean;
  reason?: MarketOperationReason;
  input_schema?: Record<string, unknown>;
  output_schema?: Record<string, unknown>;
}

/** What a subject can be given: an MCP server, or one http offering of an API listing. */
export type MarketToolItemKind = 'mcp' | 'api';

export interface MarketToolCandidate {
  kind: MarketToolItemKind;
  listing_id: number;
  slug: string;
  title: MarketText;
  /** One activation; it identifies the candidate and its item in a binding. */
  subscription_id: number;
  /** The tools of an MCP server, or the bindable operations of an API offering by tool name. */
  tools: Array<{ name: string; description: MarketText; risk_level: MarketToolRisk }>;
  required_connectors: MarketRequiredConnector[];
  offering_slug?: string;
  operations?: MarketCandidateOperation[];
}

export interface MarketToolCandidates {
  listings: MarketToolCandidate[];
  connections: MarketConnection[];
}

export type MarketToolSubjectType = 'agent' | 'digital_human';

/** One MCP server or API offering bound to a subject. An item without a kind is an MCP server. */
export interface MarketToolBindingItem {
  kind?: MarketToolItemKind;
  listing_id: number;
  slug: string;
  title?: string;
  subscription_id: number;
  connection_id?: number | null;
  tools: Array<{ name: string; risk_level: MarketToolRisk }>;
  offering_slug?: string;
  /** Whether every bindable operation of the offering is a tool, also ones added later. */
  all_operations?: boolean;
  operations?: Array<{
    key: string;
    name: string;
    method: string;
    path: string;
    risk_level: MarketToolRisk;
  }>;
}

/** One requested item of a subject's binding. */
export interface MarketToolBindingInput {
  listing_id: number;
  connection_id?: number | null;
  /** Names the offering of an API item. */
  offering_slug?: string;
  /** Operation keys of an API item; absent for every bindable operation. */
  operations?: string[];
}

/** The market tools of one agent or digital human. */
export interface MarketToolBinding {
  subject_type: MarketToolSubjectType;
  subject_id: string;
  items: MarketToolBindingItem[];
  updated_at: string | null;
  gateway_configured: boolean;
}

/**
 * A market credential of the organization: the identity of a call on the market's gateway. The
 * secret is never part of it; `secret_hint` is its last four characters.
 */
export interface MarketCredential {
  id: number;
  name: string;
  access_key_id: string;
  secret_hint: string;
  /** `external_api` is created by an administrator; `digital_human` belongs to a tool binding. */
  source: 'external_api' | 'digital_human';
  binding_id: number | null;
  status: 'active' | 'revoked';
  expires_at: string | null;
  last_used_at: string | null;
  revoked_at: string | null;
  created_at: string;
}

/** The answer of a creation: the only time the secret is readable. */
export interface MarketCredentialCreated extends MarketCredential {
  secret_key: string;
}

/** What the package of a skill holds, as the market describes it to an organization that activated it. */
export interface MarketSkillManifest {
  version_label: string;
  sha256: string;
  size_bytes: number;
  files: Array<{ path: string; size: number; sha256: string }>;
}

/** How a market tool call ended, as this instance saw it. */
export type MarketCallStatus = 'succeeded' | 'tool_error' | 'failed' | 'refused';

/** Whether what the market charged for a call is known. */
export type MarketCallChargeState = 'pending' | 'charged' | 'none' | 'unknown';

/** The charge of one metering event of a call. */
export interface MarketCallChargeLine {
  meter: string;
  unit_name: string;
  quantity: number;
  amount_micros: number;
}

/** One market tool call this instance made for an agent or a digital human. */
export interface MarketCallLog {
  id: string;
  /** `credential` is a call made with a market credential outside this instance. */
  subject_type: MarketToolSubjectType | 'credential';
  subject_id: string;
  subject_name: string;
  item_kind: 'mcp' | 'api';
  listing_slug: string;
  listing_title: string;
  offering_slug: string;
  tool_name: string;
  /** The surface the conversation came from: console, webapp, external-api, channel, task. */
  source: string;
  user_id: string;
  user_name: string;
  external_user_id: string;
  conversation_id: string;
  message_id: string;
  app_id: string;
  credential_id: number;
  credential_name: string;
  input: string;
  output: string;
  status: MarketCallStatus;
  error_message: string;
  duration_ms: number;
  request_id: string;
  charge_state: MarketCallChargeState;
  charge_micros: number;
  charge_lines: MarketCallChargeLine[];
  created_at: string;
}

export interface MarketCallLogSummary {
  calls: number;
  succeeded: number;
  charge_micros: number;
  pending: number;
}

export interface MarketCallLogPage {
  data: MarketCallLog[];
  total: number;
  page: number;
  per_page: number;
  summary: MarketCallLogSummary;
}

/** A period in Unix seconds, [from, to), at most 31 days, and optional filters. */
export interface MarketCallLogQuery {
  from: number;
  to: number;
  status?: MarketCallStatus;
  source?: string;
  page?: number;
  per_page?: number;
}

/** One body of a call as the market kept it: text when it is UTF-8, else base64. */
export interface MarketCallBody {
  content_type: string;
  encoding: 'text' | 'base64';
  body: string;
  bytes: number;
  truncated: boolean;
}

/** What a call sent and answered, as the market kept it. */
export interface MarketCallContent {
  call_id: number;
  request_id: string;
  kept: boolean;
  request: MarketCallBody | null;
  response: MarketCallBody | null;
}

import {
  keepPreviousData,
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import { AGENT_KEYS, DIGITAL_HUMAN_KEYS } from '@/hooks/query-keys';
import { zgiMarketService } from '@/services/zgi-market.service';
import type {
  MarketCallLogQuery,
  MarketListingKind,
  MarketListingQuery,
  MarketToolBindingInput,
  MarketToolSubjectType,
} from '@/services/types/zgi-market';

const keys = {
  all: ['zgi-market'] as const,
  categories: (kind?: MarketListingKind) => ['zgi-market', 'categories', kind ?? 'all'] as const,
  listings: (query: MarketListingQuery) => ['zgi-market', 'listings', query] as const,
  listing: (kind: string, slug: string) => ['zgi-market', 'listing', kind, slug] as const,
  subscriptions: ['zgi-market', 'subscriptions'] as const,
  meta: ['zgi-market', 'meta'] as const,
  installedSkills: ['zgi-market', 'installed-skills'] as const,
  connections: ['zgi-market', 'connections'] as const,
  credentials: ['zgi-market', 'credentials'] as const,
  callLogs: (query: MarketCallLogQuery) => ['zgi-market', 'call-logs', query] as const,
  callContent: (requestId: string) => ['zgi-market', 'call-content', requestId] as const,
  skillManifest: (slug: string) => ['zgi-market', 'skill-manifest', slug] as const,
  toolCandidates: ['zgi-market', 'tool-candidates'] as const,
  subjectTools: (subjectType: string, subjectId: string) =>
    ['zgi-market', 'subject-tools', subjectType, subjectId] as const,
};

const staleTime = 60 * 1000;

export function useZgiMarketCategories(kind?: MarketListingKind) {
  return useQuery({
    queryKey: keys.categories(kind),
    queryFn: () => zgiMarketService.getCategories(kind),
    staleTime,
  });
}

export function useZgiMarketListings(query: MarketListingQuery, enabled = true) {
  return useQuery({
    queryKey: keys.listings(query),
    queryFn: () => zgiMarketService.getListings(query),
    placeholderData: keepPreviousData,
    enabled,
    staleTime,
  });
}

export function useZgiMarketListing(kind: MarketListingKind | undefined, slug: string | undefined) {
  return useQuery({
    queryKey: keys.listing(kind ?? '', slug ?? ''),
    queryFn: () => zgiMarketService.getListing(kind as MarketListingKind, slug as string),
    enabled: Boolean(kind && slug),
    staleTime,
  });
}

/** The caller's role and the market's public gateway, as this instance knows them. */
export function useZgiMarketMeta() {
  return useQuery({
    queryKey: keys.meta,
    queryFn: () => zgiMarketService.getMeta(),
    staleTime: 5 * staleTime,
  });
}

export function useZgiMarketSubscriptions() {
  return useQuery({
    queryKey: keys.subscriptions,
    queryFn: () => zgiMarketService.getSubscriptions(),
    staleTime,
  });
}

export function useActivateZgiMarketListing() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (target: { listingId: number; offeringId?: number }) =>
      zgiMarketService.activate(target.listingId, target.offeringId),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: keys.subscriptions }),
  });
}

export function useZgiMarketInstalledSkills(enabled = true) {
  return useQuery({
    queryKey: keys.installedSkills,
    queryFn: () => zgiMarketService.getInstalledSkills(),
    enabled,
    staleTime,
  });
}

/**
 * The Skill pickers of agents and digital humans keep their candidates for a while. A skill that
 * was just installed has to be there when its administrator goes on to bind it.
 */
function invalidateSkillCandidates(queryClient: QueryClient) {
  void queryClient.invalidateQueries({
    predicate: query =>
      query.queryKey[0] === AGENT_KEYS.all[0] && query.queryKey.includes('candidates'),
  });
  void queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.skillCandidates() });
}

export function useInstallZgiMarketSkill() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (target: { slug: string; overwrite?: boolean }) =>
      zgiMarketService.installSkill(target.slug, target.overwrite),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: keys.subscriptions });
      void queryClient.invalidateQueries({ queryKey: keys.installedSkills });
      // The organization's skills and every binding picker read them.
      void queryClient.invalidateQueries({ queryKey: ['aichat'] });
      invalidateSkillCandidates(queryClient);
    },
  });
}

export function useZgiMarketConnections(enabled = true) {
  return useQuery({
    queryKey: keys.connections,
    queryFn: () => zgiMarketService.getConnections(),
    enabled,
  });
}

/** The files of an activated skill's package. Disabled until the organization has the skill. */
export function useZgiMarketSkillManifest(slug: string, enabled: boolean) {
  return useQuery({
    queryKey: keys.skillManifest(slug),
    queryFn: () => zgiMarketService.getSkillManifest(slug),
    enabled,
    staleTime,
    retry: false,
  });
}

/** The organization's market tool calls for one period; administrators only. */
export function useZgiMarketCallLogs(query: MarketCallLogQuery, enabled = true) {
  return useQuery({
    queryKey: keys.callLogs(query),
    queryFn: () => zgiMarketService.getCallLogs(query),
    enabled,
    placeholderData: keepPreviousData,
  });
}

/** What a call sent and answered, as the market kept it; read when a direct call is opened. */
export function useZgiMarketCallContent(requestId: string, enabled: boolean) {
  return useQuery({
    queryKey: keys.callContent(requestId),
    queryFn: () => zgiMarketService.getCallContent(requestId),
    enabled: enabled && requestId !== '',
    staleTime,
  });
}

export function useZgiMarketCredentials(enabled = true) {
  return useQuery({
    queryKey: keys.credentials,
    queryFn: () => zgiMarketService.getCredentials(),
    enabled,
  });
}

/** Creates a credential. The secret lives in the mutation result only; nothing caches it. */
export function useCreateZgiMarketCredential() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => zgiMarketService.createCredential(name),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: keys.credentials }),
    // A retry would create a second credential whose secret nobody saw.
    retry: false,
    gcTime: 0,
  });
}

export function useRevokeZgiMarketCredential() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => zgiMarketService.revokeCredential(id),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: keys.credentials }),
  });
}

export function useZgiMarketToolCandidates(enabled = true) {
  return useQuery({
    queryKey: keys.toolCandidates,
    queryFn: () => zgiMarketService.getToolCandidates(),
    enabled,
  });
}

export function useZgiMarketSubjectTools(subjectType: MarketToolSubjectType, subjectId: string) {
  return useQuery({
    queryKey: keys.subjectTools(subjectType, subjectId),
    queryFn: () => zgiMarketService.getSubjectTools(subjectType, subjectId),
    enabled: Boolean(subjectId),
  });
}

export function useBindZgiMarketSubjectTools(
  subjectType: MarketToolSubjectType,
  subjectId: string
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (items: MarketToolBindingInput[]) =>
      zgiMarketService.bindSubjectTools(subjectType, subjectId, items),
    onSuccess: binding => {
      queryClient.setQueryData(keys.subjectTools(subjectType, subjectId), binding);
    },
  });
}

/** A market skill the organization activated but has not installed into its own skills. */
export interface PendingMarketSkill {
  slug: string;
  listingId: number;
}

/**
 * The skills the organization holds an active market subscription for and has not installed. A
 * subscription comes from activating the skill itself or a plugin it is a member of.
 */
export function useZgiMarketPendingSkills(enabled = true) {
  const subscriptions = useQuery({
    queryKey: keys.subscriptions,
    queryFn: () => zgiMarketService.getSubscriptions(),
    enabled,
    staleTime,
  });
  const installed = useZgiMarketInstalledSkills(enabled);
  const installedSlugs = new Set((installed.data ?? []).map(skill => skill.slug));
  const seen = new Set<string>();
  const pending: PendingMarketSkill[] = [];
  for (const subscription of subscriptions.data ?? []) {
    if (
      subscription.listing_kind !== 'skill' ||
      subscription.status !== 'active' ||
      installedSlugs.has(subscription.listing_slug) ||
      seen.has(subscription.listing_slug)
    ) {
      continue;
    }
    seen.add(subscription.listing_slug);
    pending.push({ slug: subscription.listing_slug, listingId: subscription.listing_id });
  }
  return {
    pending,
    installed: installed.data ?? [],
    isLoading: enabled && (subscriptions.isPending || installed.isPending),
    isError: subscriptions.isError || installed.isError,
  };
}

/** What installing several market skills came to. */
export interface MarketSkillBatchResult {
  installed: string[];
  failed: Array<{ slug: string; reason: 'conflict' | 'refused' | 'error'; detail: string }>;
}

/**
 * Installs market skills one after another, so a package that is refused does not stop the rest
 * and each one is reported by name.
 */
export function useInstallZgiMarketSkills() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (slugs: string[]): Promise<MarketSkillBatchResult> => {
      const result: MarketSkillBatchResult = { installed: [], failed: [] };
      for (const slug of slugs) {
        try {
          const outcome = await zgiMarketService.installSkill(slug);
          if (outcome.installed) {
            result.installed.push(slug);
          } else if (outcome.conflict) {
            result.failed.push({ slug, reason: 'conflict', detail: outcome.conflict });
          } else {
            result.failed.push({
              slug,
              reason: 'refused',
              detail: outcome.validation_errors?.[0] ?? '',
            });
          }
        } catch (error) {
          result.failed.push({
            slug,
            reason: 'error',
            detail: error instanceof Error ? error.message : '',
          });
        }
      }
      return result;
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: keys.subscriptions });
      void queryClient.invalidateQueries({ queryKey: keys.installedSkills });
      void queryClient.invalidateQueries({ queryKey: ['aichat'] });
      invalidateSkillCandidates(queryClient);
    },
  });
}

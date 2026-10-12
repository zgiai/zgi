import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { zgiMarketService } from '@/services/zgi-market.service';
import type {
  MarketListingKind,
  MarketListingQuery,
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
    mutationFn: (items: Array<{ listing_id: number; connection_id?: number | null }>) =>
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
    },
  });
}

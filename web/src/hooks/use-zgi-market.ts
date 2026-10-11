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

export function useZgiMarketListings(query: MarketListingQuery) {
  return useQuery({
    queryKey: keys.listings(query),
    queryFn: () => zgiMarketService.getListings(query),
    placeholderData: keepPreviousData,
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

export function useBindZgiMarketSubjectTools(subjectType: MarketToolSubjectType, subjectId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (items: Array<{ listing_id: number; connection_id?: number | null }>) =>
      zgiMarketService.bindSubjectTools(subjectType, subjectId, items),
    onSuccess: binding => {
      queryClient.setQueryData(keys.subjectTools(subjectType, subjectId), binding);
    },
  });
}

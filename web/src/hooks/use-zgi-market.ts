import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { zgiMarketService } from '@/services/zgi-market.service';
import type { MarketListingKind, MarketListingQuery } from '@/services/types/zgi-market';

const keys = {
  all: ['zgi-market'] as const,
  categories: (kind?: MarketListingKind) => ['zgi-market', 'categories', kind ?? 'all'] as const,
  listings: (query: MarketListingQuery) => ['zgi-market', 'listings', query] as const,
  listing: (kind: string, slug: string) => ['zgi-market', 'listing', kind, slug] as const,
  subscriptions: ['zgi-market', 'subscriptions'] as const,
  installedSkills: ['zgi-market', 'installed-skills'] as const,
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

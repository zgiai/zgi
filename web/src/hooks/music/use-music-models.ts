'use client';

import { useQuery } from '@tanstack/react-query';
import { MUSIC_KEYS } from '@/hooks/query-keys';
import { modelService } from '@/services/model.service';
import { useOrganizationStore } from '@/store/organization-store';

export function useMusicModels() {
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const organizationId = currentOrganization?.id ?? '';
  const query = useQuery({
    queryKey: MUSIC_KEYS.models(organizationId),
    queryFn: () => modelService.getAvailableModels({ use_case: 'music-gen' }),
    enabled: Boolean(organizationId),
    staleTime: 60 * 1000,
    retry: false,
  });

  return {
    ...query,
    models: query.data?.data.items.filter(model => model.endpoints.music_generation === true) ?? [],
  };
}

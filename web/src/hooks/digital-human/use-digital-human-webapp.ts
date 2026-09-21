'use client';

import { useQuery } from '@tanstack/react-query';
import { digitalHumanWebAppService } from '@/services/digital-human-webapp.service';

export function useDigitalHumanWebAppConfig(webAppId: string) {
  return useQuery({
    queryKey: ['digital-human-webapp', webAppId, 'config'],
    queryFn: () => digitalHumanWebAppService.getConfig(webAppId),
    enabled: Boolean(webAppId),
    retry: false,
    staleTime: 30_000,
  });
}

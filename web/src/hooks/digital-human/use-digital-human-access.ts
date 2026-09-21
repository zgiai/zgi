'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { digitalHumanService } from '@/services/digital-human.service';
import type { DigitalHumanAccessSettings } from '@/services/types/digital-human';

const accessKey = (id: string) => ['digital-human', id, 'access'] as const;
const apiKeysKey = (id: string) => ['digital-human', id, 'api-keys'] as const;

export function useDigitalHumanAccess(id: string, enabled = true) {
  return useQuery({
    queryKey: accessKey(id),
    queryFn: () => digitalHumanService.getAccessSettings(id),
    enabled: enabled && Boolean(id),
    staleTime: 10_000,
  });
}

export function useDigitalHumanAPIKeys(id: string, enabled = true) {
  return useQuery({
    queryKey: apiKeysKey(id),
    queryFn: () => digitalHumanService.listAPIKeys(id),
    enabled: enabled && Boolean(id),
    staleTime: 10_000,
  });
}

export function useDigitalHumanAccessMutations(id: string) {
  const queryClient = useQueryClient();
  const refreshKeys = () => queryClient.invalidateQueries({ queryKey: apiKeysKey(id) });
  const updateAccess = useMutation({
    mutationFn: (request: Partial<DigitalHumanAccessSettings>) =>
      digitalHumanService.updateAccessSettings(id, request),
    onSuccess: data => queryClient.setQueryData(accessKey(id), data),
  });
  const createAPIKey = useMutation({
    mutationFn: (request: { name: string; expires_at?: string }) =>
      digitalHumanService.createAPIKey(id, request),
    onSuccess: refreshKeys,
  });
  const updateAPIKey = useMutation({
    mutationFn: ({
      keyId,
      request,
    }: {
      keyId: string;
      request: { name?: string; status?: 'active' | 'revoked'; expires_at?: string };
    }) => digitalHumanService.updateAPIKey(id, keyId, request),
    onSuccess: refreshKeys,
  });
  const deleteAPIKey = useMutation({
    mutationFn: (keyId: string) => digitalHumanService.deleteAPIKey(id, keyId),
    onSuccess: refreshKeys,
  });
  return { updateAccess, createAPIKey, updateAPIKey, deleteAPIKey };
}

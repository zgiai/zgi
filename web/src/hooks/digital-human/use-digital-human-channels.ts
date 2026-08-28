'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { DIGITAL_HUMAN_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { digitalHumanService } from '@/services/digital-human.service';
import { integrationService } from '@/services/integration.service';
import type {
  CreateDigitalHumanChannelInstallationRequest,
  UpdateDigitalHumanChannelInstallationRequest,
} from '@/services/types/digital-human';
import { getErrorMessage } from '@/utils/error-notifications';

function channelMutationError(error: unknown, fallback: string) {
  toast.error(getErrorMessage(error) || fallback);
}

export function useDigitalHumanChannels(id: string, auditEnabled = true) {
  const installations = useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.channelInstallations(id),
    queryFn: () => digitalHumanService.listChannelInstallations(id),
    enabled: Boolean(id),
    staleTime: 10_000,
    refetchInterval: 5_000,
  });
  const events = useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.channelEvents(id),
    queryFn: () => digitalHumanService.listChannelEvents(id, { page: 1, page_size: 50 }),
    enabled: Boolean(id) && auditEnabled,
    refetchInterval: 5_000,
  });
  const deliveries = useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.channelDeliveries(id),
    queryFn: () => digitalHumanService.listChannelDeliveries(id, { page: 1, page_size: 50 }),
    enabled: Boolean(id) && auditEnabled,
    refetchInterval: query => {
      const active = query.state.data?.data?.items.some(item =>
        ['pending', 'processing'].includes(item.status)
      );
      return active ? 2_000 : 5_000;
    },
  });
  return { installations, events, deliveries };
}

export function useDigitalHumanChannelMutations(id: string) {
  const t = useT('digitalHumans');
  const queryClient = useQueryClient();
  const invalidateConfiguration = () =>
    queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.channelInstallations(id) });
  const invalidateActivity = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.channelEvents(id) }),
      queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.channelDeliveries(id) }),
    ]);

  const createInstallation = useMutation({
    mutationFn: (request: CreateDigitalHumanChannelInstallationRequest) =>
      digitalHumanService.createChannelInstallation(id, request),
    onSuccess: () => {
      toast.success(t('channels.messages.installationCreated'));
      return invalidateConfiguration();
    },
    onError: error => channelMutationError(error, t('channels.errors.installation')),
  });
  const updateInstallation = useMutation({
    mutationFn: ({
      installationId,
      request,
    }: {
      installationId: string;
      request: UpdateDigitalHumanChannelInstallationRequest;
    }) => digitalHumanService.updateChannelInstallation(id, installationId, request),
    onSuccess: () => {
      toast.success(t('channels.messages.installationUpdated'));
      return invalidateConfiguration();
    },
    onError: error => channelMutationError(error, t('channels.errors.installation')),
  });
  const updateCredential = useMutation({
    mutationFn: ({
      installationId,
      expectedRevision,
      credentials,
    }: {
      installationId: string;
      expectedRevision: number;
      credentials: Record<string, string>;
    }) =>
      digitalHumanService.updateChannelInstallationCredential(id, installationId, {
        expected_revision: expectedRevision,
        credentials,
      }),
    onSuccess: () => invalidateConfiguration(),
    onError: error => channelMutationError(error, t('channels.errors.installation')),
  });
  const verifyInstallation = useMutation({
    mutationFn: (installationId: string) =>
      digitalHumanService.verifyChannelInstallation(id, installationId),
    onSuccess: () => invalidateConfiguration(),
    onError: error => channelMutationError(error, t('channels.errors.verification')),
  });
  const startConnectionTest = useMutation({
    mutationFn: ({
      installationId,
      conversationType,
    }: {
      installationId: string;
      conversationType: 'direct' | 'group';
    }) => digitalHumanService.startChannelConnectionTest(id, installationId, conversationType),
    onSuccess: () => invalidateConfiguration(),
    onError: error => channelMutationError(error, t('channels.errors.verification')),
  });
  const deleteInstallation = useMutation({
    mutationFn: (installationId: string) =>
      digitalHumanService.deleteChannelInstallation(id, installationId),
    onSuccess: () => {
      toast.success(t('channels.messages.installationDeleted'));
      return invalidateConfiguration();
    },
    onError: error => channelMutationError(error, t('channels.errors.installationDelete')),
  });
  const retryDelivery = useMutation({
    mutationFn: (deliveryId: string) => digitalHumanService.retryChannelDelivery(id, deliveryId),
    onSuccess: () => {
      toast.success(t('channels.messages.deliveryRetried'));
      return invalidateActivity();
    },
    onError: error => channelMutationError(error, t('channels.errors.deliveryRetry')),
  });

  return {
    createInstallation,
    updateInstallation,
    updateCredential,
    verifyInstallation,
    startConnectionTest,
    deleteInstallation,
    retryDelivery,
  };
}

export function useDigitalHumanChannelCredentials(integrationIDs: string[], enabled = true) {
  const normalizedIntegrationIDs = [...new Set(integrationIDs)].sort();
  return useQuery({
    queryKey: [...DIGITAL_HUMAN_KEYS.all, 'channel-credentials', normalizedIntegrationIDs],
    queryFn: async () => {
      const responses = await Promise.all(
        normalizedIntegrationIDs.map(integrationID =>
          integrationService.getAllConnections({ integration_id: integrationID })
        )
      );
      return responses.flatMap(response => response.data.items ?? response.data.data ?? []);
    },
    staleTime: 30_000,
    retry: false,
    enabled: enabled && normalizedIntegrationIDs.length > 0,
  });
}

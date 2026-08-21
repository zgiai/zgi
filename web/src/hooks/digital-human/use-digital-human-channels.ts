'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { DIGITAL_HUMAN_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { digitalHumanService } from '@/services/digital-human.service';
import type {
  CreateDigitalHumanChannelBindingRequest,
  CreateDigitalHumanChannelInstallationRequest,
  UpdateDigitalHumanChannelInstallationRequest,
} from '@/services/types/digital-human';
import { getErrorMessage } from '@/utils/error-notifications';

function channelMutationError(error: unknown, fallback: string) {
  toast.error(getErrorMessage(error) || fallback);
}

export function useDigitalHumanChannels(
  id: string,
  auditEnabled = true,
  discoveryEnabled = true
) {
  const installations = useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.channelInstallations(id),
    queryFn: () => digitalHumanService.listChannelInstallations(id),
    enabled: Boolean(id),
    staleTime: 10_000,
  });
  const bindings = useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.channelBindings(id),
    queryFn: () => digitalHumanService.listChannelBindings(id),
    enabled: Boolean(id),
    staleTime: 10_000,
  });
  const discoveries = useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.channelDiscoveries(id),
    queryFn: () => digitalHumanService.listChannelDiscoveries(id),
    enabled: Boolean(id) && discoveryEnabled,
    refetchInterval: 3_000,
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
  return { installations, bindings, discoveries, events, deliveries };
}

export function useDigitalHumanChannelMutations(id: string) {
  const t = useT('digitalHumans');
  const queryClient = useQueryClient();
  const invalidateConfiguration = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.channelInstallations(id) }),
      queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.channelBindings(id) }),
      queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.channelDiscoveries(id) }),
    ]);
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
  const deleteInstallation = useMutation({
    mutationFn: ({
      installationId,
      removeBindings,
    }: {
      installationId: string;
      removeBindings: boolean;
    }) => digitalHumanService.deleteChannelInstallation(id, installationId, removeBindings),
    onSuccess: () => {
      toast.success(t('channels.messages.installationDeleted'));
      return invalidateConfiguration();
    },
    onError: error => channelMutationError(error, t('channels.errors.installationDelete')),
  });
  const createBinding = useMutation({
    mutationFn: (request: CreateDigitalHumanChannelBindingRequest) =>
      digitalHumanService.createChannelBinding(id, request),
    onSuccess: () => {
      toast.success(t('channels.messages.bindingCreated'));
      return invalidateConfiguration();
    },
    onError: error => channelMutationError(error, t('channels.errors.binding')),
  });
  const deleteBinding = useMutation({
    mutationFn: (bindingId: string) => digitalHumanService.deleteChannelBinding(id, bindingId),
    onSuccess: () => {
      toast.success(t('channels.messages.bindingDeleted'));
      return invalidateConfiguration();
    },
    onError: error => channelMutationError(error, t('channels.errors.bindingDelete')),
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
    deleteInstallation,
    createBinding,
    deleteBinding,
    retryDelivery,
  };
}

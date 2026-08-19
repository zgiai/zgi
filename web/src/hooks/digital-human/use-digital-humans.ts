'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { isAxiosError } from 'axios';
import { toast } from 'sonner';
import { DIGITAL_HUMAN_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { digitalHumanService } from '@/services/digital-human.service';
import type {
  CreateDigitalHumanRequest,
  CreateDigitalHumanRoleTemplateRequest,
  DigitalHumanBindingType,
  DigitalHumanMemoryListParams,
  DigitalHumanMemoryProposalRequest,
  DigitalHumanResourceBindingInput,
  DigitalHumanRoleAssignmentInput,
  ManualDigitalHumanInvocationRequest,
  PublishDigitalHumanRequest,
  UpdateDigitalHumanDraftRequest,
  UpdateDigitalHumanRequest,
} from '@/services/types/digital-human';
import { getErrorMessage } from '@/utils/error-notifications';

function mutationError(error: unknown, fallback: string) {
  toast.error(getErrorMessage(error) || fallback);
}

function isRevisionConflict(error: unknown) {
  if (!isAxiosError(error)) return false;
  const code = (error.response?.data as { code?: string } | undefined)?.code;
  return error.response?.status === 409 || code === 'digital_human.revision.conflict';
}

export function useDigitalHumans(enabled = true, search = '') {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.list({ search }),
    queryFn: () => digitalHumanService.list({ page: 1, page_size: 100, search }),
    enabled,
    staleTime: 15_000,
  });
}

export function useDigitalHuman(id: string, enabled = true) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.detail(id),
    queryFn: () => digitalHumanService.get(id),
    enabled: enabled && Boolean(id),
    staleTime: 10_000,
  });
}

export function useDigitalHumanRoleTemplates(enabled = true) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.roleTemplates(),
    queryFn: () => digitalHumanService.listRoleTemplates({ page: 1, page_size: 100 }),
    enabled,
    staleTime: 15_000,
  });
}

export function useDigitalHumanRoleTemplate(id: string, enabled = true) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.roleTemplate(id),
    queryFn: () => digitalHumanService.getRoleTemplate(id),
    enabled: enabled && Boolean(id),
    staleTime: 10_000,
  });
}

export function useDigitalHumanDraftRoles(id: string, enabled = true) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.draftRoles(id),
    queryFn: () => digitalHumanService.getDraftRoles(id),
    enabled: enabled && Boolean(id),
  });
}

export function useDigitalHumanDraftBindings(id: string, enabled = true) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.draftBindings(id),
    queryFn: () => digitalHumanService.getDraftBindings(id),
    enabled: enabled && Boolean(id),
  });
}

export function useDigitalHumanBindingCandidates(
  id: string,
  params: {
    type: DigitalHumanBindingType;
    query?: string;
    parent_resource_id?: string;
    limit?: number;
  },
  enabled = true
) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.bindingCandidates(id, params),
    queryFn: () => digitalHumanService.listBindingCandidates(id, params),
    enabled:
      enabled &&
      Boolean(id) &&
      (params.type !== 'database_table' || Boolean(params.parent_resource_id)),
    staleTime: 15_000,
  });
}

export function useDigitalHumanRuns(id: string, enabled = true) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.runs(id),
    queryFn: () => digitalHumanService.listRuns(id, { page: 1, page_size: 100 }),
    enabled: enabled && Boolean(id),
    refetchInterval: query => {
      const active = query.state.data?.data?.items.some(run =>
        ['queued', 'running', 'canceling'].includes(run.status)
      );
      return active ? 1_500 : false;
    },
  });
}

export function useDigitalHumanRun(id: string, runId: string, enabled = true) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.run(id, runId),
    queryFn: () => digitalHumanService.getRun(id, runId),
    enabled: enabled && Boolean(id) && Boolean(runId),
    refetchInterval: query => {
      const status = query.state.data?.data?.status;
      return status && ['queued', 'running', 'canceling'].includes(status) ? 1_500 : false;
    },
  });
}

export function useDigitalHumanMemories(
  id: string,
  params: DigitalHumanMemoryListParams = {},
  enabled = true
) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.memories(id, params),
    queryFn: () => digitalHumanService.listMemories(id, params),
    enabled: enabled && Boolean(id),
    staleTime: 10_000,
  });
}

export function useDigitalHumanMutations(id?: string) {
  const t = useT('digitalHumans');
  const queryClient = useQueryClient();
  const invalidateDetail = async (digitalHumanId: string) => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.detail(digitalHumanId) }),
      queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.lists() }),
      queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.draftRoles(digitalHumanId) }),
      queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.draftBindings(digitalHumanId) }),
    ]);
  };
  const handleConflict = (error: unknown, digitalHumanId: string, fallback: string) => {
    if (!isRevisionConflict(error)) {
      mutationError(error, fallback);
      return;
    }
    toast.error(t('errors.conflict'));
    void invalidateDetail(digitalHumanId);
  };

  const create = useMutation({
    mutationFn: (request: CreateDigitalHumanRequest) => digitalHumanService.create(request),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.lists() }),
    onError: error => mutationError(error, t('errors.create')),
  });
  const update = useMutation({
    mutationFn: ({
      digitalHumanId,
      request,
    }: {
      digitalHumanId: string;
      request: UpdateDigitalHumanRequest;
    }) => digitalHumanService.update(digitalHumanId, request),
    onSuccess: (_, variables) => invalidateDetail(variables.digitalHumanId),
    onError: (error, variables) =>
      handleConflict(error, variables.digitalHumanId, t('errors.save')),
  });
  const updateDraft = useMutation({
    mutationFn: ({
      digitalHumanId,
      request,
    }: {
      digitalHumanId: string;
      request: UpdateDigitalHumanDraftRequest;
    }) => digitalHumanService.updateDraft(digitalHumanId, request),
    onSuccess: (_, variables) => invalidateDetail(variables.digitalHumanId),
    onError: (error, variables) =>
      handleConflict(error, variables.digitalHumanId, t('errors.save')),
  });
  const publish = useMutation({
    mutationFn: ({
      digitalHumanId,
      request,
    }: {
      digitalHumanId: string;
      request: PublishDigitalHumanRequest;
    }) => digitalHumanService.publish(digitalHumanId, request),
    onSuccess: (_, variables) => invalidateDetail(variables.digitalHumanId),
    onError: (error, variables) =>
      handleConflict(error, variables.digitalHumanId, t('errors.publish')),
  });
  const createRole = useMutation({
    mutationFn: (request: CreateDigitalHumanRoleTemplateRequest) =>
      digitalHumanService.createRoleTemplate(request),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.roleTemplates() }),
    onError: error => mutationError(error, t('errors.role')),
  });
  const publishRole = useMutation({
    mutationFn: ({ roleId, request }: { roleId: string; request: PublishDigitalHumanRequest }) =>
      digitalHumanService.publishRoleTemplate(roleId, request),
    onSuccess: (_, variables) =>
      Promise.all([
        queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.roleTemplates() }),
        queryClient.invalidateQueries({
          queryKey: DIGITAL_HUMAN_KEYS.roleTemplate(variables.roleId),
        }),
      ]),
    onError: error => mutationError(error, t('errors.role')),
  });
  const replaceRoles = useMutation({
    mutationFn: ({
      digitalHumanId,
      expectedDraftLockVersion,
      items,
    }: {
      digitalHumanId: string;
      expectedDraftLockVersion: number;
      items: DigitalHumanRoleAssignmentInput[];
    }) => digitalHumanService.replaceDraftRoles(digitalHumanId, expectedDraftLockVersion, items),
    onSuccess: (_, variables) => invalidateDetail(variables.digitalHumanId),
    onError: (error, variables) =>
      handleConflict(error, variables.digitalHumanId, t('errors.role')),
  });
  const replaceBindings = useMutation({
    mutationFn: ({
      digitalHumanId,
      expectedDraftLockVersion,
      items,
    }: {
      digitalHumanId: string;
      expectedDraftLockVersion: number;
      items: DigitalHumanResourceBindingInput[];
    }) => digitalHumanService.replaceDraftBindings(digitalHumanId, expectedDraftLockVersion, items),
    onSuccess: (_, variables) => invalidateDetail(variables.digitalHumanId),
    onError: (error, variables) =>
      handleConflict(error, variables.digitalHumanId, t('errors.binding')),
  });
  const invoke = useMutation({
    mutationFn: ({
      digitalHumanId,
      request,
    }: {
      digitalHumanId: string;
      request: ManualDigitalHumanInvocationRequest;
    }) => digitalHumanService.submitInvocation(digitalHumanId, request),
    onSuccess: (_, variables) =>
      queryClient.invalidateQueries({
        queryKey: DIGITAL_HUMAN_KEYS.runs(variables.digitalHumanId),
      }),
    onError: error => mutationError(error, t('errors.run')),
  });
  const cancelRun = useMutation({
    mutationFn: ({ digitalHumanId, runId }: { digitalHumanId: string; runId: string }) =>
      digitalHumanService.cancelRun(digitalHumanId, runId),
    onSuccess: (_, variables) =>
      Promise.all([
        queryClient.invalidateQueries({
          queryKey: DIGITAL_HUMAN_KEYS.runs(variables.digitalHumanId),
        }),
        queryClient.invalidateQueries({
          queryKey: DIGITAL_HUMAN_KEYS.run(variables.digitalHumanId, variables.runId),
        }),
      ]),
    onError: error => mutationError(error, t('errors.run')),
  });
  const applyMemoryProposal = useMutation({
    mutationFn: ({
      digitalHumanId,
      request,
    }: {
      digitalHumanId: string;
      request: DigitalHumanMemoryProposalRequest;
    }) => digitalHumanService.applyMemoryProposal(digitalHumanId, request),
    onSuccess: (_, variables) =>
      queryClient.invalidateQueries({
        queryKey: DIGITAL_HUMAN_KEYS.memories(variables.digitalHumanId),
      }),
    onError: error => mutationError(error, t('memory.errors.mutate')),
  });
  const undoMemory = useMutation({
    mutationFn: ({
      digitalHumanId,
      operationId,
      expectedRevision,
    }: {
      digitalHumanId: string;
      operationId: string;
      expectedRevision: number;
    }) => digitalHumanService.undoMemory(digitalHumanId, operationId, expectedRevision),
    onSuccess: (_, variables) =>
      queryClient.invalidateQueries({
        queryKey: DIGITAL_HUMAN_KEYS.memories(variables.digitalHumanId),
      }),
    onError: error => mutationError(error, t('memory.errors.undo')),
  });
  const updateMemoryState = useMutation({
    mutationFn: ({
      digitalHumanId,
      expectedMemoryEpoch,
      autoExtractionEnabled,
    }: {
      digitalHumanId: string;
      expectedMemoryEpoch: number;
      autoExtractionEnabled: boolean;
    }) =>
      digitalHumanService.updateMemoryState(
        digitalHumanId,
        expectedMemoryEpoch,
        autoExtractionEnabled
      ),
    onSuccess: (_, variables) =>
      queryClient.invalidateQueries({
        queryKey: DIGITAL_HUMAN_KEYS.memories(variables.digitalHumanId),
      }),
    onError: error => mutationError(error, t('memory.errors.fuse')),
  });

  return {
    id,
    create,
    update,
    updateDraft,
    publish,
    createRole,
    publishRole,
    replaceRoles,
    replaceBindings,
    invoke,
    cancelRun,
    applyMemoryProposal,
    undoMemory,
    updateMemoryState,
  };
}

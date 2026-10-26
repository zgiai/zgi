'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { isAxiosError } from 'axios';
import { toast } from 'sonner';
import { DIGITAL_HUMAN_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { digitalHumanService } from '@/services/digital-human.service';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { useWorkspaceStore } from '@/store/workspace-store';
import {
  queriedConnectorDraft,
  savedConnectorDraft,
} from '@/components/digital-human/digital-human-connector-model';
import { isActiveDigitalHumanRunStatus } from '@/components/digital-human/digital-human-run-presentation';
import type { ApiResponseData } from '@/services/types/common';
import type {
  CreateDigitalHumanRequest,
  CreateDigitalHumanPersonaTemplateRequest,
  DigitalHumanBindingType,
  DigitalHumanMemoryListParams,
  DigitalHumanMemoryProposalRequest,
  DigitalHumanMemoryRescopeRequest,
  DigitalHumanMemoryVisibilityRequest,
  DigitalHumanResourceBindingInput,
  DraftResourceBindings,
  ManualDigitalHumanInvocationRequest,
  PublishDigitalHumanRequest,
  RestoreDigitalHumanVersionRequest,
  UpdateDigitalHumanDraftRequest,
  UpdateDigitalHumanRequest,
  UpdateDigitalHumanPersonaTemplateRequest,
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

/**
 * Identity of the active workspace context. A response that resolves after the
 * account, organization, or workspace switched must never land in the cache.
 */
function digitalHumanRequestScope() {
  return JSON.stringify([
    useAuthStore.getState().user?.id,
    useOrganizationStore.getState().currentOrganization?.id,
    useWorkspaceStore.getState().currentWorkspace?.id,
  ]);
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

export function useDigitalHumanVersions(id: string, enabled = true) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.versions(id),
    queryFn: () => digitalHumanService.listVersions(id, { page: 1, page_size: 100 }),
    enabled: enabled && Boolean(id),
    staleTime: 10_000,
  });
}

export function useDigitalHumanVersionRestorePreview(
  id: string,
  versionId: string,
  enabled = true
) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.versionRestorePreview(id, versionId),
    queryFn: () => digitalHumanService.previewVersionRestore(id, versionId),
    enabled: enabled && Boolean(id) && Boolean(versionId),
    staleTime: 0,
  });
}

export function useDigitalHumanPersonaTemplates(
  params: { page: number; pageSize: number; search?: string },
  enabled = true
) {
  const queryParams = {
    page: params.page,
    page_size: params.pageSize,
    search: params.search?.trim() || undefined,
  };
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.personaTemplates(queryParams),
    queryFn: () => digitalHumanService.listPersonaTemplates(queryParams),
    enabled,
    staleTime: 15_000,
  });
}

export function useDigitalHumanDraftBindings(id: string, enabled = true) {
  const queryClient = useQueryClient();
  const queryKey = DIGITAL_HUMAN_KEYS.draftBindings(id);
  return useQuery({
    queryKey,
    queryFn: async () => {
      const scope = digitalHumanRequestScope();
      const started =
        queryClient.getQueryData<ApiResponseData<DraftResourceBindings>>(queryKey)?.data
          .draft_version_id;
      const response = await digitalHumanService.getDraftBindings(id);
      if (scope !== digitalHumanRequestScope()) throw new Error('Digital human scope changed');
      const cached = queryClient.getQueryData<ApiResponseData<DraftResourceBindings>>(queryKey);
      // A delayed GET from an older draft generation must not overwrite a newer one.
      return { ...response, data: queriedConnectorDraft(response.data, cached?.data, started) };
    },
    enabled: enabled && Boolean(id),
  });
}

/**
 * Skills a new digital human may bind, fetched before creation.
 *
 * It uses the same server-side eligibility gate as the draft editor, so the
 * creation form can never offer a skill the configuration page would hide.
 */
export function useDigitalHumanSkillCandidates(enabled = true) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.skillCandidates(),
    queryFn: () => digitalHumanService.listSkillCandidates(),
    enabled,
    staleTime: 30_000,
  });
}

/**
 * Owner Connector connections a new digital human may attach, fetched before
 * creation.
 *
 * Query and limit are passed into the service and into the query key, so a
 * server search and the full 512-candidate page are requested and cached
 * independently.
 */
export function useDigitalHumanConnectorCandidates(
  params: { query?: string; limit?: number } = {},
  enabled = true
) {
  const queryParams = {
    query: params.query?.trim() || undefined,
    limit: params.limit,
  };
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.connectorCandidates(queryParams),
    queryFn: () => digitalHumanService.listConnectorCandidates(queryParams),
    enabled,
    staleTime: 30_000,
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

export function useDigitalHumanRuns(
  id: string,
  enabled = true,
  // Narrows runs to who asked for them.
  requester?: { 'requester_kind[]'?: string[]; requester_id?: string }
) {
  return useQuery({
    queryKey: requester ? [...DIGITAL_HUMAN_KEYS.runs(id), requester] : DIGITAL_HUMAN_KEYS.runs(id),
    queryFn: () => digitalHumanService.listRuns(id, { page: 1, page_size: 100, ...requester }),
    enabled: enabled && Boolean(id),
    refetchInterval: query => {
      const active = query.state.data?.data?.items.some(run =>
        isActiveDigitalHumanRunStatus(run.status)
      );
      return active ? 1_500 : 10_000;
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
      return status && isActiveDigitalHumanRunStatus(status) ? 1_500 : false;
    },
  });
}

export function useDigitalHumanRunRuntimeLog(
  id: string,
  runId: string,
  enabled = true,
  pollWhilePending = false
) {
  return useQuery({
    queryKey: DIGITAL_HUMAN_KEYS.runRuntimeLog(id, runId),
    queryFn: () => digitalHumanService.getRunRuntimeLog(id, runId),
    enabled: enabled && Boolean(id) && Boolean(runId),
    refetchInterval: query => {
      // A log that cannot be read stays unreadable; do not keep asking.
      if (query.state.status === 'error') return false;
      const status = query.state.data?.data?.detail?.status;
      return pollWhilePending || (status && isActiveDigitalHumanRunStatus(status)) ? 1_500 : false;
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
    onSuccess: (_, variables) =>
      Promise.all([
        invalidateDetail(variables.digitalHumanId),
        queryClient.invalidateQueries({
          queryKey: DIGITAL_HUMAN_KEYS.versions(variables.digitalHumanId),
        }),
      ]),
    onError: (error, variables) =>
      handleConflict(error, variables.digitalHumanId, t('errors.publish')),
  });
  const restoreVersion = useMutation({
    mutationFn: ({
      digitalHumanId,
      request,
    }: {
      digitalHumanId: string;
      request: RestoreDigitalHumanVersionRequest;
    }) => digitalHumanService.restoreVersion(digitalHumanId, request),
    onSuccess: (_, variables) =>
      Promise.all([
        invalidateDetail(variables.digitalHumanId),
        queryClient.invalidateQueries({
          queryKey: DIGITAL_HUMAN_KEYS.versions(variables.digitalHumanId),
        }),
      ]),
    onError: (error, variables) => {
      const code = isAxiosError(error)
        ? (error.response?.data as { code?: string } | undefined)?.code
        : undefined;
      if (code === 'digital_human.version.restore_impact_changed') {
        toast.error(t('versionHistory.impactChanged'));
        void queryClient.invalidateQueries({
          queryKey: DIGITAL_HUMAN_KEYS.versionRestorePreview(
            variables.digitalHumanId,
            variables.request.version_id
          ),
        });
        return;
      }
      handleConflict(error, variables.digitalHumanId, t('versionHistory.restoreFailed'));
    },
  });
  const createPersonaTemplate = useMutation({
    mutationFn: (request: CreateDigitalHumanPersonaTemplateRequest) =>
      digitalHumanService.createPersonaTemplate(request),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.personaTemplates() }),
    onError: error => mutationError(error, t('errors.role')),
  });
  const updatePersonaTemplate = useMutation({
    mutationFn: ({
      templateId,
      request,
    }: {
      templateId: string;
      request: UpdateDigitalHumanPersonaTemplateRequest;
    }) => digitalHumanService.updatePersonaTemplate(templateId, request),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.personaTemplates() }),
    onError: error => mutationError(error, t('errors.role')),
  });
  const deletePersonaTemplate = useMutation({
    mutationFn: ({
      templateId,
      expectedRevision,
    }: {
      templateId: string;
      expectedRevision: number;
    }) => digitalHumanService.deletePersonaTemplate(templateId, expectedRevision),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.personaTemplates() }),
    onError: error => mutationError(error, t('errors.role')),
  });
  const replaceBindings = useMutation({
    mutationKey: id ? DIGITAL_HUMAN_KEYS.replaceDraftBindings(id) : undefined,
    onMutate: () => ({ scope: digitalHumanRequestScope() }),
    mutationFn: ({
      digitalHumanId,
      expectedDraftLockVersion,
      expectedDraftVersionId,
      items,
    }: {
      digitalHumanId: string;
      expectedDraftLockVersion: number;
      expectedDraftVersionId?: string;
      items: DigitalHumanResourceBindingInput[];
    }) =>
      digitalHumanService.replaceDraftBindings(
        digitalHumanId,
        expectedDraftLockVersion,
        items,
        expectedDraftVersionId
      ),
    onSuccess: async (response, variables, context) => {
      if (context?.scope !== digitalHumanRequestScope()) return;
      const queryKey = DIGITAL_HUMAN_KEYS.draftBindings(variables.digitalHumanId);
      // Install the canonical response without downgrading a newer draft that
      // was published or saved while this request was in flight.
      queryClient.setQueryData<ApiResponseData<DraftResourceBindings>>(queryKey, previous => ({
        ...response,
        data: savedConnectorDraft(response.data, previous?.data),
      }));
      await invalidateDetail(variables.digitalHumanId);
    },
    onError: (error, variables, context) => {
      if (context?.scope !== digitalHumanRequestScope()) return;
      handleConflict(error, variables.digitalHumanId, t('errors.binding'));
    },
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
  const rescopeMemory = useMutation({
    mutationFn: ({
      digitalHumanId,
      memoryId,
      request,
    }: {
      digitalHumanId: string;
      memoryId: string;
      request: DigitalHumanMemoryRescopeRequest;
    }) => digitalHumanService.rescopeMemory(digitalHumanId, memoryId, request),
    onSuccess: (_, variables) =>
      queryClient.invalidateQueries({
        queryKey: DIGITAL_HUMAN_KEYS.memories(variables.digitalHumanId),
      }),
    onError: error => mutationError(error, t('memory.errors.mutate')),
  });
  const setMemoryVisibility = useMutation({
    mutationFn: ({
      digitalHumanId,
      memoryId,
      request,
    }: {
      digitalHumanId: string;
      memoryId: string;
      request: DigitalHumanMemoryVisibilityRequest;
    }) => digitalHumanService.setMemoryVisibility(digitalHumanId, memoryId, request),
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
  const repairMemory = useMutation({
    mutationFn: ({ digitalHumanId }: { digitalHumanId: string }) =>
      digitalHumanService.repairMemory(digitalHumanId),
    onSuccess: (_, variables) =>
      queryClient.invalidateQueries({
        queryKey: DIGITAL_HUMAN_KEYS.memories(variables.digitalHumanId),
      }),
    onError: error => mutationError(error, t('memory.errors.repair')),
  });

  return {
    id,
    create,
    update,
    updateDraft,
    publish,
    restoreVersion,
    createPersonaTemplate,
    updatePersonaTemplate,
    deletePersonaTemplate,
    replaceBindings,
    invoke,
    cancelRun,
    applyMemoryProposal,
    rescopeMemory,
    setMemoryVisibility,
    undoMemory,
    updateMemoryState,
    repairMemory,
  };
}

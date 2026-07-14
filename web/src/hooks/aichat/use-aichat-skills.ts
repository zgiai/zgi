import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { AGENT_KEYS, AICHAT_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n/translations';
import { aichatService } from '@/services/aichat.service';
import type {
  AIChatConfirmImportSkillRequest,
  AIChatSkillOrganizationConfig,
  AIChatSkillPreference,
} from '@/services/types/aichat';
import type { AgentBindingMutationConfirmation } from '@/services/types/common';
import { getAgentResourceBoundImpact } from '@/utils/agent-resource-bound';

interface UpdateAIChatSkillConfigVariables {
  payload: AIChatSkillOrganizationConfig;
  silent?: boolean;
}

interface UpdateAIChatSkillPreferenceVariables {
  payload: AIChatSkillPreference;
  silent?: boolean;
}

/**
 * @hook useAIChatSkills
 * @description Load available AIChat Skill V2 metadata for the console chat selector.
 */
export function useAIChatSkills(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: AICHAT_KEYS.skills(),
    queryFn: async () => {
      const response = await aichatService.listSkills();
      return response.data ?? [];
    },
    enabled: options?.enabled ?? true,
    retry: false,
  });
}

/**
 * @hook useAIChatSkill
 * @description Load one AIChat Skill metadata item for detail displays.
 */
export function useAIChatSkill(id: string | null | undefined) {
  return useQuery({
    queryKey: AICHAT_KEYS.skill(id ?? ''),
    queryFn: async () => {
      const response = await aichatService.getSkill(id ?? '');
      return response.data;
    },
    enabled: Boolean(id),
    retry: false,
  });
}

/**
 * @hook useAIChatSkillConfig
 * @description Load organization-level enabled AIChat Skill ids.
 */
export function useAIChatSkillConfig() {
  return useQuery({
    queryKey: AICHAT_KEYS.skillConfig(),
    queryFn: async () => {
      const response = await aichatService.getSkillConfig();
      return response.data;
    },
    retry: false,
  });
}

/**
 * @hook useUpdateAIChatSkillConfig
 * @description Replace organization-level AIChat Skill configuration.
 */
export function useUpdateAIChatSkillConfig() {
  const queryClient = useQueryClient();
  const t = useT('dashboard');

  return useMutation({
    mutationFn: ({ payload }: UpdateAIChatSkillConfigVariables) =>
      aichatService.updateSkillConfig(payload),
    onSuccess: async (response, variables) => {
      if (variables.silent) {
        return;
      }

      if (response.data) {
        queryClient.setQueryData(AICHAT_KEYS.skillConfig(), response.data);
      }

      await Promise.all([
        queryClient.invalidateQueries({ queryKey: AICHAT_KEYS.skills() }),
        queryClient.invalidateQueries({ queryKey: AICHAT_KEYS.skillConfig() }),
        queryClient.invalidateQueries({ queryKey: AGENT_KEYS.all }),
      ]);
      toast.success(t('organization.aichatSkills.messages.saved'));
    },
    onError: (error, variables) => {
      if (variables.silent) {
        return;
      }

      toast.error(
        error instanceof Error ? error.message : t('organization.aichatSkills.messages.saveFailed')
      );
    },
  });
}

export function useSkillCatalog() {
  return useAIChatSkills();
}

export function useOrganizationSkillPolicy() {
  return useAIChatSkillConfig();
}

export function useAIChatSkillPreference(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: AICHAT_KEYS.skillPreference(),
    queryFn: async () => {
      const response = await aichatService.getSkillPreference();
      return response.data;
    },
    enabled: options?.enabled ?? true,
    retry: false,
  });
}

export function useUpdateAIChatSkillPreference() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ payload }: UpdateAIChatSkillPreferenceVariables) =>
      aichatService.updateSkillPreference(payload),
    onSuccess: async response => {
      queryClient.setQueryData(AICHAT_KEYS.skillPreference(), response.data);
      await queryClient.invalidateQueries({ queryKey: AICHAT_KEYS.skills() });
    },
  });
}

/**
 * @hook usePreviewImportAIChatSkill
 * @description Validate a custom AIChat Skill zip package before publishing it.
 */
export function usePreviewImportAIChatSkill() {
  const t = useT('dashboard');

  return useMutation({
    mutationFn: (file: File) => aichatService.previewImportSkill(file),
    onError: error => {
      toast.error(
        error instanceof Error
          ? error.message
          : t('organization.aichatSkills.messages.previewFailed')
      );
    },
  });
}

/**
 * @hook useConfirmImportAIChatSkill
 * @description Publish a previously previewed custom organization AIChat Skill.
 */
export function useConfirmImportAIChatSkill() {
  const queryClient = useQueryClient();
  const t = useT('dashboard');

  return useMutation({
    mutationFn: (payload: AIChatConfirmImportSkillRequest) =>
      aichatService.confirmImportSkill(payload),
    onSuccess: async response => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: AICHAT_KEYS.skills() }),
        queryClient.invalidateQueries({ queryKey: AICHAT_KEYS.skillConfig() }),
      ]);
      toast.success(
        t('organization.aichatSkills.messages.imported', {
          skill: response.data.name || response.data.skill_id,
        })
      );
    },
    onError: error => {
      toast.error(
        error instanceof Error
          ? error.message
          : t('organization.aichatSkills.messages.importFailed')
      );
    },
  });
}

/**
 * @hook useCancelImportAIChatSkillPreview
 * @description Best-effort cleanup for a previously previewed custom AIChat Skill package.
 */
export function useCancelImportAIChatSkillPreview() {
  return useMutation({
    mutationFn: (importId: string) => aichatService.cancelImportSkillPreview(importId),
  });
}

/**
 * @hook useDeleteAIChatSkill
 * @description Delete an organization custom AIChat Skill.
 */
export function useDeleteAIChatSkill() {
  const queryClient = useQueryClient();
  const t = useT('dashboard');

  return useMutation({
    mutationFn: ({
      id,
      confirmation,
    }: {
      id: string;
      confirmation?: AgentBindingMutationConfirmation;
    }) => aichatService.deleteSkill(id, confirmation),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: AICHAT_KEYS.skills() }),
        queryClient.invalidateQueries({ queryKey: AICHAT_KEYS.skillConfig() }),
        queryClient.invalidateQueries({ queryKey: AGENT_KEYS.all }),
      ]);
      toast.success(t('organization.aichatSkills.messages.deleted'));
    },
    onError: error => {
      if (getAgentResourceBoundImpact(error)) return;
      toast.error(
        error instanceof Error
          ? error.message
          : t('organization.aichatSkills.messages.deleteFailed')
      );
    },
  });
}

'use client';

import { useCallback } from 'react';
import { useRouter } from 'next/navigation';
import { useLocale } from 'next-intl';
import { toast } from 'sonner';
import { parse, stringify } from 'yaml';
import { useImportWorkflow } from '@/hooks/workflow/use-workflow-import-export';
import { useT } from '@/i18n';
import { withBasePath } from '@/lib/config';
import type { WorkflowImportResult } from '@/services/types/workflow';
import { getAgentDetailEditHref } from '@/utils/agent-detail-routes';
import { uploadAppAvatarPreset } from '@/components/common/icon-input/avatar-preset-upload';
import { getRandomAppAvatar } from '@/components/common/icon-input/avatar-presets';
import type { AgentTemplate, AgentTemplateLocale } from './types';

interface TemplateYamlDocument {
  app?: {
    icon?: string;
    icon_type?: string;
  };
  workflow?: {
    graph?: {
      nodes?: Array<{
        id?: string;
        data?: Record<string, unknown>;
      }>;
    };
  };
}

function resolveTemplateLocale(locale: string): AgentTemplateLocale {
  return locale.startsWith('zh') ? 'zh-Hans' : 'en-US';
}

function resolveTemplateYamlPath(template: AgentTemplate, locale: string): string {
  const templateLocale = resolveTemplateLocale(locale);
  return template.localizedYamlPaths?.[templateLocale] ?? template.yamlPath;
}

function hydrateTemplateYaml(parsed: TemplateYamlDocument, avatarImageId: string): string {
  const nodes = parsed?.workflow?.graph?.nodes;
  if (Array.isArray(nodes)) {
    for (const node of nodes) {
      if (!node?.data || node.data.type !== 'llm') continue;
      node.data.prompt_source = 'inline';
      delete node.data.prompt_reference;
    }
  }

  parsed.app ??= {};
  parsed.app.icon = avatarImageId;
  parsed.app.icon_type = 'image';

  return stringify(parsed);
}

function resolveTemplateRouteKind(template: AgentTemplate): string {
  return template.kind === 'agent' ? 'agent' : 'workflow';
}

export function useCreateAgentFromTemplate() {
  const router = useRouter();
  const locale = useLocale();
  const t = useT();
  const { importWorkflow, isImporting } = useImportWorkflow();

  const createFromTemplate = useCallback(
    async (template: AgentTemplate, workspaceId: string): Promise<WorkflowImportResult> => {
      let fileContent: string;

      try {
        const yamlPath = resolveTemplateYamlPath(template, locale);
        const response = await fetch(withBasePath(yamlPath), { cache: 'no-store' });
        if (!response.ok) {
          throw new Error(`Template asset request failed with ${response.status}`);
        }
        fileContent = await response.text();
      } catch (error) {
        toast.error(t('agents.templates.templateUnavailable'));
        throw error;
      }

      let parsedYaml: TemplateYamlDocument;
      try {
        parsedYaml = parse(fileContent) as TemplateYamlDocument;
      } catch (error) {
        toast.error(t('agents.templates.templateUnavailable'));
        throw error;
      }

      let avatarImageId: string;
      try {
        const uploadedAvatar = await uploadAppAvatarPreset(getRandomAppAvatar());
        if (!uploadedAvatar.imageId) throw new Error('Avatar upload did not return a file ID');
        avatarImageId = uploadedAvatar.imageId;
      } catch (error) {
        toast.error(t('common.iconInput.avatarLibraryDialog.uploadFailed'));
        throw error;
      }

      const hydratedYaml = hydrateTemplateYaml(parsedYaml, avatarImageId);

      const file = new File([hydratedYaml], `${template.id}.yml`, { type: 'application/x-yaml' });
      const response = await importWorkflow({ file, workspaceId });
      const agentId = response.data.agent_id;
      router.push(getAgentDetailEditHref(agentId, resolveTemplateRouteKind(template)));
      return response.data;
    },
    [importWorkflow, locale, router, t]
  );

  return {
    createFromTemplate,
    isCreatingFromTemplate: isImporting,
  };
}

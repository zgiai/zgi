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
import type { AgentTemplate, AgentTemplateLocale, AgentTemplatePromptBinding } from './types';

function resolveTemplateLocale(locale: string): AgentTemplateLocale {
  return locale.startsWith('zh') ? 'zh-Hans' : 'en-US';
}

function resolveTemplateYamlPath(template: AgentTemplate, locale: string): string {
  const templateLocale = resolveTemplateLocale(locale);
  return template.localizedYamlPaths?.[templateLocale] ?? template.yamlPath;
}

function resolvePromptIdForLocale(
  binding: AgentTemplatePromptBinding,
  locale: AgentTemplateLocale
): string | null {
  return (
    binding.promptIdsByLocale[locale] ??
    binding.promptIdsByLocale['en-US'] ??
    binding.promptIdsByLocale['zh-Hans'] ??
    null
  );
}

function injectPromptBindingsIntoTemplateYaml(
  yamlText: string,
  template: AgentTemplate,
  locale: AgentTemplateLocale
): string {
  if (!template.defaultPromptBindings?.length) {
    return yamlText;
  }

  const parsed = parse(yamlText) as {
    workflow?: {
      graph?: {
        nodes?: Array<{
          id?: string;
          data?: Record<string, unknown>;
        }>;
      };
    };
  };

  const nodes = parsed?.workflow?.graph?.nodes;
  if (!Array.isArray(nodes) || nodes.length === 0) {
    return yamlText;
  }

  for (const binding of template.defaultPromptBindings) {
    const promptId = resolvePromptIdForLocale(binding, locale);
    if (!promptId) continue;

    for (const node of nodes) {
      if (!node?.id || !binding.nodeIds.includes(node.id)) continue;
      if (!node.data || node.data.type !== 'llm') continue;

      node.data = {
        ...node.data,
        prompt_source: 'managed',
        prompt_reference: {
          prompt_id: promptId,
          prompt_name: binding.fallbackTitle,
          label: 'production',
          locale,
          source: 'official',
        },
      };
    }
  }

  return stringify(parsed);
}

export function useCreateAgentFromTemplate() {
  const router = useRouter();
  const locale = useLocale();
  const t = useT();
  const { importWorkflow, isImporting } = useImportWorkflow();

  const createFromTemplate = useCallback(
    async (template: AgentTemplate, workspaceId: string): Promise<WorkflowImportResult> => {
      let fileContent: string;
      const templateLocale = resolveTemplateLocale(locale);

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

      const hydratedYaml = injectPromptBindingsIntoTemplateYaml(fileContent, template, templateLocale);
      const file = new File([hydratedYaml], `${template.id}.yml`, { type: 'application/x-yaml' });
      const response = await importWorkflow({ file, workspaceId });
      const agentId = response.data.agent_id;
      router.push(`/console/agents/${agentId}/workflow`);
      return response.data;
    },
    [importWorkflow, locale, router, t]
  );

  return {
    createFromTemplate,
    isCreatingFromTemplate: isImporting,
  };
}

'use client';

import { useQuery } from '@tanstack/react-query';
import { Eye } from 'lucide-react';
import { toast } from 'sonner';
import { MemoryInstructionsField, MemorySettingRow } from '@/components/memory/memory-settings';
import { Button } from '@/components/ui/button';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import agentService from '@/services/agent.service';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';
import { AGENT_MEMORY_LIMITS } from '../utils';

interface AgentRuntimeMemorySectionProps {
  agentId: string;
  open: boolean;
  agentMemoryEnabled: boolean;
  agentMemoryAutoExtractionEnabled: boolean;
  agentMemoryInstructions: string;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeAgentMemoryEnabled: (value: boolean) => void;
  onChangeAgentMemoryAutoExtractionEnabled: (value: boolean) => void;
  onChangeAgentMemoryInstructions: (value: string) => void;
  onOpenMemoryValues: () => void;
}

// Agent memory has no configured types (ADR 0017): the builder describes what
// to remember in the memory instructions, and the model organizes each user's
// memory files itself.
export function AgentRuntimeMemorySection({
  agentId,
  open,
  agentMemoryEnabled,
  agentMemoryAutoExtractionEnabled,
  agentMemoryInstructions,
  readOnly = false,
  onToggleSection,
  onChangeAgentMemoryEnabled,
  onChangeAgentMemoryAutoExtractionEnabled,
  onChangeAgentMemoryInstructions,
  onOpenMemoryValues,
}: AgentRuntimeMemorySectionProps) {
  const t = useT('agents.agentRuntime');
  const { locale } = useLocale();
  const templatesQuery = useQuery({
    queryKey: AGENT_KEYS.memoryTemplates(agentId, locale),
    queryFn: () => agentService.getAgentMemoryTemplates(agentId, locale),
    enabled: Boolean(agentId) && open,
    staleTime: 10 * 60 * 1000,
  });
  const memoryTemplates = templatesQuery.data?.data?.catalog.templates ?? [];
  const automaticAvailable = templatesQuery.data?.data?.automatic_extraction_available !== false;

  return (
    <RuntimeSection
      title={t('sections.memory')}
      section="memory"
      open={open}
      onToggle={onToggleSection}
    >
      <div className="space-y-4">
        <MemorySettingRow
          id="agent-memory-enabled"
          title={t('memory.agentTitle')}
          description={t('memory.agentDescription')}
          checked={agentMemoryEnabled}
          disabled={readOnly}
          onCheckedChange={onChangeAgentMemoryEnabled}
        />
        {agentMemoryEnabled && (
          <>
            {/* Without automatic memory in the deployment there is nothing to choose. */}
            {automaticAvailable ? (
              <MemorySettingRow
                id="agent-memory-auto"
                title={t('memory.autoExtractionTitle')}
                description={t('memory.autoExtractionSummary')}
                hint={t('memory.autoExtractionDescription')}
                checked={agentMemoryAutoExtractionEnabled}
                disabled={readOnly}
                onCheckedChange={onChangeAgentMemoryAutoExtractionEnabled}
              />
            ) : null}
            <MemoryInstructionsField
              value={agentMemoryInstructions}
              onChange={onChangeAgentMemoryInstructions}
              maxLength={AGENT_MEMORY_LIMITS.maxInstructionsChars}
              templates={memoryTemplates}
              placeholder={t('memory.instructionsPlaceholder')}
              disabled={readOnly}
              onTemplateApplied={() => toast.success(t('memory.templateApplied'))}
            />
            <div className="flex flex-wrap items-center justify-between gap-x-2 gap-y-1 border-t pt-2.5">
              <span className="text-xs text-muted-foreground">{t('memory.tryHint')}</span>
              <Button
                type="button"
                variant="link"
                size="xs"
                className="h-auto px-0 text-primary"
                onClick={onOpenMemoryValues}
              >
                <Eye className="size-3" />
                {t('memory.viewValues')}
              </Button>
            </div>
          </>
        )}
      </div>
    </RuntimeSection>
  );
}

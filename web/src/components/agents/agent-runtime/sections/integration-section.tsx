'use client';

import { AlertCircle, PlugZap, Trash2 } from 'lucide-react';
import { safeIntegrationDisplayText } from '@/components/integrations/display-utils';
import { useIntegrationMetadata } from '@/components/integrations/metadata-i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { integrationCatalogItems, useIntegrationCatalog } from '@/hooks';
import { useT } from '@/i18n';
import type { AgentBindingHealth } from '@/services/types/agent';
import type {
  AgentIntegrationConnectionBinding,
  AgentIntegrationConnectionCandidate,
} from '@/services/types/integration';
import { AgentRuntimeResourceCard, AgentRuntimeResourceSection } from '../resource-section';
import type { AgentConfigSection } from '../types';

interface AgentRuntimeIntegrationSectionProps {
  open: boolean;
  bindings: AgentIntegrationConnectionBinding[];
  candidatesByConnectionID: Map<string, AgentIntegrationConnectionCandidate>;
  isLoading: boolean;
  bindingHealth?: AgentBindingHealth;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onOpenDialog: () => void;
  onChangeBindings: (value: AgentIntegrationConnectionBinding[]) => void;
}

export function AgentRuntimeIntegrationSection({
  open,
  bindings,
  candidatesByConnectionID,
  isLoading,
  bindingHealth,
  readOnly = false,
  onToggleSection,
  onOpenDialog,
  onChangeBindings,
}: AgentRuntimeIntegrationSectionProps) {
  const t = useT('agents.agentRuntime');
  const integrationMetadata = useIntegrationMetadata();
  const catalogQuery = useIntegrationCatalog(true, 'shared');
  const catalogByIntegration = new Map(
    integrationCatalogItems(catalogQuery.data?.data).map(item => [
      item.integration_id || item.id,
      item,
    ])
  );

  return (
    <AgentRuntimeResourceSection
      title={t('sections.integrations')}
      section="integrations"
      open={open}
      count={bindings.length}
      addLabel={t('integration.add')}
      helpText={t('integration.helpText')}
      emptyText={t('integration.emptySelected')}
      isLoading={isLoading}
      onToggleSection={onToggleSection}
      onAdd={onOpenDialog}
      readOnly={readOnly}
    >
      <div className="space-y-2">
        {bindings.map(binding => {
          const candidate = candidatesByConnectionID.get(binding.connection_id);
          const unavailable = !candidate && !isLoading;
          const label = safeIntegrationDisplayText(
            candidate?.name,
            unavailable
              ? t('integration.unavailableConnection')
              : t('integration.unnamedConnection')
          );
          const catalogItem = catalogByIntegration.get(binding.integration_id);
          const integrationName = catalogItem
            ? integrationMetadata.providerName(catalogItem, t('integration.unknownExternalApp'))
            : t('integration.unknownExternalApp');
          const actionsByID = new Map(
            catalogItem?.actions.map(action => [action.id, action]) ?? []
          );
          return (
            <AgentRuntimeResourceCard
              key={`${binding.integration_id}:${binding.connection_id}`}
              icon={
                unavailable ? <AlertCircle className="size-4" /> : <PlugZap className="size-4" />
              }
              title={label}
              description={
                unavailable
                  ? t('integration.unavailableDescription')
                  : t('integration.connectionDescription', {
                      integration: integrationName,
                      count: binding.allowed_action_ids.length,
                    })
              }
              error={unavailable || candidate?.status !== 'active'}
              healthItem={bindingHealth?.items.find(
                item =>
                  item.binding_type === 'integration_connection' &&
                  item.resource_id === binding.connection_id
              )}
              action={
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  isIcon
                  className="size-8 shrink-0 text-muted-foreground hover:text-destructive"
                  aria-label={t('integration.remove', { name: label })}
                  disabled={readOnly}
                  onClick={() =>
                    onChangeBindings(
                      bindings.filter(item => item.connection_id !== binding.connection_id)
                    )
                  }
                >
                  <Trash2 className="size-4" />
                </Button>
              }
            >
              <div className="mt-2 flex flex-wrap gap-1">
                {binding.allowed_action_ids.map(actionId => {
                  const action = actionsByID.get(actionId);
                  return (
                    <Badge key={actionId} variant="outline" className="text-[10px]">
                      {action
                        ? integrationMetadata.actionName(action, t('integration.unknownAction'))
                        : t('integration.unknownAction')}
                    </Badge>
                  );
                })}
              </div>
            </AgentRuntimeResourceCard>
          );
        })}
      </div>
    </AgentRuntimeResourceSection>
  );
}

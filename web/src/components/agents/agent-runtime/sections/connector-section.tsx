'use client';

import { Trash2 } from 'lucide-react';
import { ConnectorIcon } from '@/components/connectors/connector-icon';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  connectorCatalogItems,
  personalConnectionItems,
  useConnectorCatalog,
  usePersonalConnections,
} from '@/hooks';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import type { AgentBindingHealth, AgentConnectorBinding } from '@/services/types/agent';
import { AgentRuntimeResourceCard, AgentRuntimeResourceSection } from '../resource-section';
import type { AgentConfigSection } from '../types';

interface AgentRuntimeConnectorSectionProps {
  open: boolean;
  connectorBindings: AgentConnectorBinding[];
  menuOpen: boolean;
  bindingHealth?: AgentBindingHealth;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onOpenDialog: (anchorElement: HTMLElement) => void;
  onChangeConnectorBindings: (value: AgentConnectorBinding[]) => void;
}

export function AgentRuntimeConnectorSection({
  open,
  connectorBindings,
  menuOpen,
  bindingHealth,
  readOnly = false,
  onToggleSection,
  onOpenDialog,
  onChangeConnectorBindings,
}: AgentRuntimeConnectorSectionProps) {
  const t = useT('agents.agentRuntime');
  const connectorT = useT('connectors');
  const { locale } = useLocale();
  const shouldLoadConnectorBindings = connectorBindings.length > 0;
  const connectorCatalogQuery = useConnectorCatalog(undefined, shouldLoadConnectorBindings);
  const connectorConnectionsQuery = usePersonalConnections(undefined, shouldLoadConnectorBindings);
  const connectorCatalogByID = new Map(
    connectorCatalogItems(connectorCatalogQuery.data?.data).map(item => [
      item.id,
      localizeConnectorCatalogItem(item, locale),
    ])
  );
  const connectorConnectionsByID = new Map(
    personalConnectionItems(connectorConnectionsQuery.data?.data).map(item => [item.id, item])
  );
  const connectorBindingsLoading =
    shouldLoadConnectorBindings &&
    (connectorCatalogQuery.isLoading || connectorConnectionsQuery.isLoading);
  return (
    <AgentRuntimeResourceSection
      title={connectorT('title')}
      section="connectors"
      open={open}
      count={connectorBindings.length}
      addLabel={connectorT('picker.title')}
      helpText={connectorT('description')}
      emptyText={connectorT('sections.empty')}
      isLoading={connectorBindingsLoading}
      onToggleSection={onToggleSection}
      onAdd={onOpenDialog}
      addMenuOpen={menuOpen}
      readOnly={readOnly}
    >
      <div className="space-y-2">
        {connectorBindings.map(binding => {
              const connector = connectorCatalogByID.get(binding.connector_id);
              const connection = connectorConnectionsByID.get(binding.connection_id);
              const unavailable = !connector || !connection || connection.state !== 'ready';
              const title = connection?.name ?? connector?.name ?? binding.connector_id;
              const description = connection?.external_display_name
                ? connectorT('picker.connectedAs', { name: connection.external_display_name })
                : (connector?.description ?? connectorT('status.unavailable'));
              const classification = connector?.classification ?? 'connector';
              const status = connection?.state ?? 'unavailable';

              return (
                <AgentRuntimeResourceCard
                  key={`${binding.connector_id}:${binding.connection_id}`}
                  icon={
                    <ConnectorIcon
                      connectorId={binding.connector_id}
                      name={title}
                      icon={connector?.icon}
                      classification={classification}
                      className="size-8 border-0 shadow-none"
                    />
                  }
                  title={title}
                  description={description}
                  error={unavailable}
                  healthItem={bindingHealth?.items.find(
                    item =>
                      item.binding_type === 'connector_connection' &&
                      item.resource_id === binding.connection_id
                  )}
                  action={
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      isIcon
                      className="size-8 shrink-0 text-muted-foreground hover:text-destructive"
                      aria-label={t('connector.remove', { name: title })}
                      disabled={readOnly}
                      onClick={() =>
                        onChangeConnectorBindings(
                          connectorBindings.filter(
                            item =>
                              item.connector_id !== binding.connector_id ||
                              item.connection_id !== binding.connection_id
                          )
                        )
                      }
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  }
                >
                  <div className="flex flex-wrap gap-1">
                    <Badge variant="outline" className="text-[10px]">
                      {classification === 'data_source'
                        ? connectorT('classification.dataSource')
                        : connectorT('classification.connector')}
                    </Badge>
                    <Badge
                      variant={
                        status === 'ready'
                          ? 'success'
                          : status === 'reconnect_required'
                            ? 'warning'
                            : 'subtle'
                      }
                      className="text-[10px]"
                    >
                      {status === 'ready'
                        ? connectorT('status.ready')
                        : status === 'reconnect_required'
                          ? connectorT('status.reconnectRequired')
                          : connectorT('status.unavailable')}
                    </Badge>
                  </div>
                </AgentRuntimeResourceCard>
              );
            })}

      </div>
    </AgentRuntimeResourceSection>
  );
}

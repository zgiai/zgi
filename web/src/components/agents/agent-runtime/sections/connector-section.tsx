'use client';

import { personalConnectionDisplayName } from '@/components/connectors/connections/personal-connections-model';
import { Trash2 } from 'lucide-react';
import { useQuery } from '@tanstack/react-query';
import { toast } from 'sonner';
import { ConnectorIcon } from '@/components/connectors/connector-icon';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import {
  connectorCatalogItems,
  connectorTargetBindingItems,
  personalConnectionItems,
  useConnectorCatalog,
  usePersonalConnections,
  useConnectorTargetBindings,
  useUpdateConnectorTargetBindingHostDelegation,
} from '@/hooks';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import type { AgentBindingHealth, AgentConnectorBinding } from '@/services/types/agent';
import { agentService } from '@/services/agent.service';
import type { ConnectorHostAudience } from '@/services/types/connector';
import { getErrorMessage } from '@/utils/error-notifications';
import { AgentRuntimeResourceCard, AgentRuntimeResourceSection } from '../resource-section';
import type { AgentConfigSection } from '../types';

interface AgentRuntimeConnectorSectionProps {
  agentId: string;
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
  agentId,
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
  const publishedVersionsQuery = useQuery({
    queryKey: ['agent-runtime', agentId, 'published-versions'],
    queryFn: () => agentService.getPublishedVersions(agentId),
    enabled: Boolean(agentId),
    staleTime: 30_000,
    retry: false,
  });
  const currentPublishedVersionId = publishedVersionsQuery.data?.data?.data?.find(
    version => version.is_current
  )?.version_uuid;
  const publishedTarget = currentPublishedVersionId
    ? {
        type: 'agent' as const,
        id: agentId,
        scope: 'published' as const,
        version_id: currentPublishedVersionId,
      }
    : undefined;
  const publishedBindingsQuery = useConnectorTargetBindings(
    publishedTarget,
    Boolean(publishedTarget)
  );
  const updateHostDelegation = useUpdateConnectorTargetBindingHostDelegation();
  const publishedBindings = connectorTargetBindingItems(publishedBindingsQuery.data?.data);
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
      helpText={connectorT('sections.connectorsDescription')}
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
          const title = connection
            ? personalConnectionDisplayName(
                connection,
                connector?.name ?? binding.connector_id,
                locale
              )
            : (connector?.name ?? binding.connector_id);
          const description = connection?.external_display_name
            ? connectorT('picker.connectedAs', { name: connection.external_display_name })
            : (connector?.description ?? connectorT('status.unavailable'));
          const classification = connector?.classification ?? 'connector';
          const status = connection?.state ?? 'unavailable';
          const publishedBinding = publishedBindings.find(
            item =>
              item.connector_id === binding.connector_id &&
              item.connection_id === binding.connection_id
          );
          const hostAudiences = new Set<ConnectorHostAudience>(
            publishedBinding?.host_audiences ?? []
          );
          const audienceOptions: Array<{ value: ConnectorHostAudience; label: string }> = [
            {
              value: 'workspace',
              label: connectorT('picker.authorization.hostAudienceWorkspace'),
            },
            { value: 'api', label: connectorT('picker.authorization.hostAudienceApi') },
            { value: 'channel', label: connectorT('picker.authorization.hostAudienceChannel') },
          ];

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
              <div className="mt-3 flex flex-wrap items-center gap-2">
                <span className="text-xs font-medium text-muted-foreground">
                  {connectorT('picker.authorization.modeLabel')}
                </span>
                <Select
                  value={binding.authorization_mode === 'direct' ? 'direct' : 'ask_each_time'}
                  disabled={readOnly}
                  onValueChange={value =>
                    onChangeConnectorBindings(
                      connectorBindings.map(item =>
                        item === binding
                          ? {
                              ...item,
                              authorization_mode: value === 'direct' ? 'direct' : 'ask_each_time',
                            }
                          : item
                      )
                    )
                  }
                >
                  <SelectTrigger className="h-8 w-auto min-w-36 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="ask_each_time">
                      {connectorT('picker.authorization.askShort')}
                    </SelectItem>
                    <SelectItem value="direct">
                      {connectorT('picker.authorization.directShort')}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              {publishedBinding ? (
                <div className="mt-3 flex flex-wrap gap-3">
                  {audienceOptions.map(option => (
                    <label key={option.value} className="flex items-center gap-2 text-xs">
                      <Checkbox
                        checked={hostAudiences.has(option.value)}
                        disabled={readOnly || updateHostDelegation.isPending}
                        onCheckedChange={async checked => {
                          const next = new Set(hostAudiences);
                          if (checked === true) next.add(option.value);
                          else next.delete(option.value);
                          try {
                            await updateHostDelegation.mutateAsync({
                              bindingId: publishedBinding.id,
                              revision: publishedBinding.revision,
                              audiences: Array.from(next),
                            });
                            toast.success(
                              connectorT('picker.authorization.hostDelegationSaved')
                            );
                          } catch (error) {
                            toast.error(
                              getErrorMessage(error) ||
                                connectorT('picker.authorization.hostDelegationSaveFailed')
                            );
                          }
                        }}
                      />
                      {option.label}
                    </label>
                  ))}
                </div>
              ) : null}
            </AgentRuntimeResourceCard>
          );
        })}
      </div>
    </AgentRuntimeResourceSection>
  );
}

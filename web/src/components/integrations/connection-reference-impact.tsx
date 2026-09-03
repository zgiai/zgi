'use client';

import Link from 'next/link';
import { Bot, ExternalLink, Unlink, Workflow } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import type {
  IntegrationCatalogItem,
  IntegrationConnectionDeleteImpact,
  IntegrationConnectionReference,
} from '@/services/types/integration';
import { useIntegrationMetadata } from './metadata-i18n';

interface WorkspaceReferenceGroup {
  id: string;
  name: string;
  references: IntegrationConnectionReference[];
}

interface ConnectionReferenceImpactProps {
  impact: IntegrationConnectionDeleteImpact;
  provider?: IntegrationCatalogItem;
  detachPending: boolean;
  onDetachAll: () => void;
}

function scopeKey(draft: boolean, published: boolean): 'draft' | 'published' | 'both' {
  if (draft && published) return 'both';
  return published ? 'published' : 'draft';
}

function groupReferencesByWorkspace(
  references: IntegrationConnectionReference[],
  unavailableWorkspace: string
): WorkspaceReferenceGroup[] {
  const groups = new Map<string, WorkspaceReferenceGroup>();
  references.forEach(reference => {
    const workspaceID = reference.workspace_id.trim();
    const key = workspaceID || `unavailable:${reference.asset_id}`;
    const current = groups.get(key) ?? {
      id: key,
      name: reference.workspace_name.trim() || unavailableWorkspace,
      references: [],
    };
    current.references.push(reference);
    groups.set(key, current);
  });
  return Array.from(groups.values())
    .map(group => ({
      ...group,
      references: group.references
        .slice()
        .sort((left, right) =>
          (left.name || left.asset_id).localeCompare(right.name || right.asset_id)
        ),
    }))
    .sort((left, right) => left.name.localeCompare(right.name));
}

export function ConnectionReferenceImpact({
  impact,
  provider,
  detachPending,
  onDetachAll,
}: ConnectionReferenceImpactProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const actionByID = new Map((provider?.actions ?? []).map(action => [action.id, action]));
  const actionName = (actionID: string) => {
    const action = actionByID.get(actionID);
    return action ? metadata.actionName(action) : metadata.actionNameByID(actionID);
  };
  const groups = groupReferencesByWorkspace(
    impact.references ?? [],
    t('delete.references.unavailableWorkspace')
  );

  if (groups.length === 0) return null;

  return (
    <span className="block space-y-3 text-left font-normal text-foreground">
      <span className="flex items-center justify-between gap-3">
        <span className="text-sm font-semibold">{t('delete.references.title')}</span>
        <span className="text-xs text-muted-foreground">
          {t('delete.references.summary', {
            agents: impact.bound_agent_count,
            workflows: impact.bound_workflow_count,
          })}
        </span>
      </span>
      <span className="block max-h-72 space-y-3 overflow-y-auto pr-1">
        {groups.map(group => (
          <span key={group.id} className="block rounded-lg border bg-background p-3">
            <span className="block truncate text-xs font-semibold text-muted-foreground">
              {t('delete.references.workspace', { name: group.name })}
            </span>
            <span className="mt-2 block space-y-2">
              {group.references.map(reference => {
                const isWorkflow = reference.asset_type === 'workflow';
                const assetName =
                  reference.name.trim() ||
                  t(
                    isWorkflow
                      ? 'delete.references.unavailableWorkflow'
                      : 'delete.references.unavailableAgent'
                  );
                const href = isWorkflow
                  ? `/console/workflows/${reference.asset_id}`
                  : `/console/agents/${reference.asset_id}`;
                return (
                  <span
                    key={`${reference.asset_type}:${reference.asset_id}`}
                    className="block rounded-md bg-muted/40 p-2.5"
                  >
                    <span className="flex items-start gap-2">
                      <span className="mt-0.5 text-muted-foreground">
                        {isWorkflow ? <Workflow className="size-4" /> : <Bot className="size-4" />}
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="flex flex-wrap items-center gap-1.5">
                          <span className="truncate text-sm font-medium">{assetName}</span>
                          <Badge variant="outline" className="text-[10px]">
                            {t(
                              isWorkflow
                                ? 'delete.references.assetType.workflow'
                                : 'delete.references.assetType.agent'
                            )}
                          </Badge>
                          <Badge variant="secondary" className="text-[10px]">
                            {t(
                              `delete.references.scope.${scopeKey(
                                reference.draft_referenced,
                                reference.published_referenced
                              )}`
                            )}
                          </Badge>
                        </span>
                      </span>
                      <Button asChild type="button" variant="ghost" size="sm" isIcon>
                        <Link href={href} target="_blank" rel="noopener noreferrer">
                          <ExternalLink className="size-3.5" />
                          <span className="sr-only">
                            {t('delete.references.openAsset', { name: assetName })}
                          </span>
                        </Link>
                      </Button>
                    </span>
                    {reference.actions.length > 0 ? (
                      <span className="mt-2 flex flex-wrap gap-1 pl-6">
                        {reference.actions.map(actionID => (
                          <Badge key={actionID} variant="outline" className="text-[10px]">
                            {actionName(actionID)}
                          </Badge>
                        ))}
                      </span>
                    ) : null}
                    {isWorkflow && reference.nodes?.length ? (
                      <span className="mt-2 block space-y-1 border-t pt-2 pl-6">
                        {reference.nodes.map(node => (
                          <span
                            key={`${node.node_id}:${node.action_id}`}
                            className="flex flex-wrap items-center gap-1.5 text-xs"
                          >
                            <span className="font-medium">
                              {node.node_name.trim() || t('delete.references.unavailableNode')}
                            </span>
                            <span className="text-muted-foreground">
                              {actionName(node.action_id)}
                            </span>
                            <Badge variant="outline" className="text-[10px]">
                              {t(
                                `delete.references.scope.${scopeKey(
                                  node.draft_referenced,
                                  node.published_referenced
                                )}`
                              )}
                            </Badge>
                          </span>
                        ))}
                      </span>
                    ) : null}
                  </span>
                );
              })}
            </span>
          </span>
        ))}
      </span>
      {impact.impact_token ? (
        <span className="block rounded-lg border border-destructive/25 bg-destructive/[0.03] p-3">
          <span className="block text-xs leading-5 text-muted-foreground">
            {t('delete.references.detachDescription')}
          </span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="mt-2 border-destructive/30 text-destructive hover:bg-destructive/5 hover:text-destructive"
            disabled={detachPending}
            onClick={onDetachAll}
          >
            <Unlink className="size-3.5" />
            {t('delete.references.detachAll')}
          </Button>
        </span>
      ) : null}
    </span>
  );
}

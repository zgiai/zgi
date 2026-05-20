'use client';

import React from 'react';
import type { VariableAggregatorNodeData } from './config';
import { useT } from '@/i18n';
import ValueBadge from '../../ui/value-badge';
import OutputVariablesView from '../../common/output-variables-view';
import { useNodeOutputVariables } from '../../hooks';

interface AggregatorContentProps {
  nodeId: string;
  data: VariableAggregatorNodeData;
}

const VariableAggregatorContent: React.FC<AggregatorContentProps> = ({ nodeId, data }) => {
  const t = useT('nodes');
  const outputVariables = useNodeOutputVariables(nodeId);
  const groupEnabled = Boolean(data.advanced_settings?.group_enabled);
  const variables = Array.isArray(data.variables) ? data.variables : [];
  const groups = Array.isArray(data.advanced_settings?.groups) ? data.advanced_settings.groups : [];
  const visibleVariables = variables.slice(0, 3);
  const visibleGroups = groups.slice(0, 2);

  return (
    <div className="space-y-2">
      <div className="text-xs font-medium text-primary">
        {groupEnabled
          ? t('variableAggregator.content.groupTitle')
          : t('variableAggregator.content.normalTitle')}
      </div>

      {!groupEnabled ? (
        visibleVariables.length > 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {visibleVariables.map((selector, index) => (
              <ValueBadge
                key={`${selector[0]}::${selector[1]}::${index}`}
                selector={[selector[0], selector[1]]}
                currentNodeId={nodeId}
              />
            ))}
            {variables.length > visibleVariables.length ? (
              <span className="text-[10px] text-muted-foreground">
                +{variables.length - visibleVariables.length}
              </span>
            ) : null}
          </div>
        ) : (
          <div className="text-xs text-muted-foreground">
            {t('variableAggregator.manager.noVariables')}
          </div>
        )
      ) : (
        <div className="space-y-1">
          {visibleGroups.map(group => {
            const variableCount = Array.isArray(group.variables) ? group.variables.length : 0;
            return (
              <div
                key={group.groupId}
                className="flex items-center justify-between gap-2 rounded-md border bg-background/70 px-2 py-1 text-xs"
              >
                <span className="truncate font-medium">
                  {group.group_name || t('variableAggregator.content.groupTitle')}
                </span>
                <span className="shrink-0 text-muted-foreground">{variableCount}</span>
              </div>
            );
          })}
          {groups.length > visibleGroups.length ? (
            <div className="text-[11px] text-muted-foreground">
              +{groups.length - visibleGroups.length}
            </div>
          ) : null}
        </div>
      )}

      <OutputVariablesView variant="compact" variables={outputVariables} maxItems={2} />
    </div>
  );
};

export default VariableAggregatorContent;

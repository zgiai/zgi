'use client';

import React from 'react';
import type { AssignerNodeData, AssignerNodeOperation } from './config';
import ValueBadge from '../../ui/value-badge';
import { useT, type NodesKey } from '@/i18n';
import { useResolvedVariableReference } from '../../hooks';

export interface AssignerContentProps {
  nodeId: string;
  data: AssignerNodeData;
}

interface AssignerSummaryTextProps {
  nodeId: string;
  operation: AssignerNodeOperation;
}

const AssignerSummaryText: React.FC<AssignerSummaryTextProps> = ({ nodeId, operation }) => {
  const t = useT();
  const value = useResolvedVariableReference({
    selector: Array.isArray(operation.value) ? (operation.value as string[]) : undefined,
    currentNodeId: nodeId,
  });

  const mathActionKey = (op: AssignerNodeOperation['operation']) => {
    switch (op) {
      case '+=':
        return 'add';
      case '-=':
        return 'subtract';
      case '*=':
        return 'multiply';
      case '/=':
        return 'divide';
      default:
        return 'add';
    }
  };

  const formatLiteralValue = (input: unknown) => {
    if (input === undefined || input === null || input === '') {
      return t('nodes.assigner.preview.pendingValue');
    }
    if (typeof input === 'string') return `"${input}"`;
    if (typeof input === 'number' || typeof input === 'boolean') return String(input);
    try {
      const text = JSON.stringify(input);
      return text.length > 40 ? `${text.slice(0, 37)}...` : text;
    } catch {
      return String(input);
    }
  };

  if (operation.operation === 'clear') {
    return <>{t('nodes.assigner.content.summary.clear')}</>;
  }

  if (operation.operation === 'set') {
    return (
      <>
        {t('nodes.assigner.content.summary.setConstant', {
          value: formatLiteralValue(operation.value),
        })}
      </>
    );
  }

  if (operation.operation === 'over-write') {
    return (
      <>
        {t('nodes.assigner.content.summary.setVariable', {
          value: value?.displayText ?? t('nodes.assigner.preview.pendingValue'),
        })}
      </>
    );
  }

  return (
    <>
      {t('nodes.assigner.content.summary.math', {
        action: t(`nodes.assigner.preview.actions.${mathActionKey(operation.operation)}` as NodesKey),
        value:
          operation.input_type === 'variable'
            ? (value?.displayText ?? t('nodes.assigner.preview.pendingValue'))
            : formatLiteralValue(operation.value),
      })}
    </>
  );
};

// Content renderer for Assigner node. Layout is provided by CustomNode.
const AssignerContent: React.FC<AssignerContentProps> = ({ nodeId, data }) => {
  const items = Array.isArray(data.items) ? data.items : [];
  const t = useT();

  const renderTargetBadge = (operation: AssignerNodeData['items'][number]) => {
    const selector = operation.variable_selector;
    if (Array.isArray(selector) && selector.length >= 2) {
      return <ValueBadge selector={selector} currentNodeId={nodeId} />;
    }

    return (
      <span className="text-xs text-muted-foreground">{t('nodes.assigner.content.unselected')}</span>
    );
  };

  return (
    <div className="space-y-2">
      {items.length === 0 ? (
        <div className="text-xs text-muted-foreground h-8 rounded-md flex items-center border px-2">
          {t('nodes.assigner.content.empty')}
        </div>
      ) : (
        items.map((operation, index) => (
          <div key={index} className="rounded-md border px-2 py-1.5">
            <div className="max-w-full truncate inline-flex items-center">
              {renderTargetBadge(operation)}
            </div>
            <div className="mt-1 text-xs text-muted-foreground truncate">
              <AssignerSummaryText nodeId={nodeId} operation={operation} />
            </div>
          </div>
        ))
      )}
    </div>
  );
};

export default AssignerContent;

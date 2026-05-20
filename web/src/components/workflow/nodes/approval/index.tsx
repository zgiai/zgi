import React from 'react';
import { Position } from '@xyflow/react';
import { Clock } from 'lucide-react';

import { Badge } from '@/components/ui/badge';
import CustomHandle from '../../ui/custom-handle';
import ValueBadge from '@/components/workflow/ui/value-badge';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';

import {
  APPROVAL_TIMEOUT_HANDLE,
  normalizeApprovalNodeData,
  type ApprovalAction,
  type ApprovalField,
  type ApprovalNodeData,
} from './config';

export interface ApprovalContentProps {
  nodeId: string;
  data: ApprovalNodeData;
}

function getActionVariant(action: ApprovalAction): 'default' | 'destructive' {
  return action.style === 'danger' ? 'destructive' : 'default';
}

const APPROVAL_CONTENT_TOKEN_PATTERN = /\{\{#([^#]+?)#\}\}/g;
const APPROVAL_OUTPUT_TOKEN_PREFIX = '$output.';

interface ApprovalSpecialTokenBadgeProps {
  sourceTitle: string;
  displayPath?: string;
  invalid?: boolean;
}

function ApprovalSpecialTokenBadge({
  sourceTitle,
  displayPath,
  invalid,
}: ApprovalSpecialTokenBadgeProps) {
  return (
    <Badge
      variant="outline"
      aria-invalid={invalid || undefined}
      className={cn(
        'mx-0.5 max-w-full rounded-sm bg-background align-baseline',
        invalid && 'border-destructive'
      )}
      title={displayPath ? `${sourceTitle} (${displayPath})` : sourceTitle}
    >
      <span className="min-w-1 truncate break-all">{sourceTitle}</span>
      {displayPath ? (
        <span className="min-w-1 overflow-hidden break-all text-xs text-highlight">
          ({displayPath})
        </span>
      ) : null}
    </Badge>
  );
}

function renderApprovalContent(
  input: string,
  fields: ApprovalField[],
  nodeId: string,
  t: ReturnType<typeof useT<'nodes'>>
) {
  if (!input) return <span className="text-muted-foreground">{t('approval.preview.emptyContent')}</span>;

  const parts: Array<string | React.ReactNode> = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = APPROVAL_CONTENT_TOKEN_PATTERN.exec(input)) !== null) {
    const [full, inner] = match;
    const start = match.index;
    const end = start + full.length;
    if (start > lastIndex) {
      parts.push(input.slice(lastIndex, start));
    }

    if (inner.startsWith(APPROVAL_OUTPUT_TOKEN_PREFIX)) {
      const fieldKey = inner.slice(APPROVAL_OUTPUT_TOKEN_PREFIX.length);
      const field = fields.find(item => item.key === fieldKey);
      parts.push(
        <ApprovalSpecialTokenBadge
          key={`approval-output-${start}`}
          sourceTitle={t('approval.preview.formOutput')}
          displayPath={field?.label || fieldKey}
          invalid={!field}
        />
      );
    } else if (inner === 'url') {
      parts.push(
        <ApprovalSpecialTokenBadge
          key={`approval-url-${start}`}
          sourceTitle={t('approval.emailDialog.urlVariable')}
        />
      );
    } else if (inner.includes('.')) {
      parts.push(
        <ValueBadge
          key={`approval-value-${start}`}
          template={full}
          className="mx-0.5 align-baseline"
          currentNodeId={nodeId}
        />
      );
    } else {
      parts.push(full);
    }

    lastIndex = end;
  }

  if (lastIndex < input.length) {
    parts.push(input.slice(lastIndex));
  }

  return parts.length > 0 ? parts : t('approval.preview.emptyContent');
}

/**
 * @component ApprovalContent
 * @category Feature
 * @status Beta
 * @description Compact workflow canvas preview for human approval nodes with action handles.
 * @usage Rendered by CustomNode for approval workflow nodes.
 * @example
 * <ApprovalContent nodeId={nodeId} data={data} />
 */
export default function ApprovalContent({ nodeId, data }: ApprovalContentProps) {
  const t = useT('nodes');
  const normalized = normalizeApprovalNodeData(data);
  const actions = normalized.approval.actions;
  const content = normalized.approval.content.trim();

  return (
    <div className="mt-1">
      <CustomHandle type="target" position={Position.Left} id="target" style={{ top: -18 }} />

      <div className="space-y-2">
        <div
          className="nowheel mt-1 max-h-[160px] min-h-8 overflow-y-auto rounded-md bg-muted/70 p-1.5 text-xs leading-relaxed text-secondary-foreground break-words whitespace-pre-wrap scrollbar-thin"
          onWheelCapture={event => event.stopPropagation()}
        >
          {renderApprovalContent(content, normalized.approval.fields, nodeId, t)}
        </div>

        <div>
          {actions.map((action, index) => (
            <div key={`${action.id}-${index}`} className="relative py-2 text-xs border-t space-y-1">
              <div className="relative text-end">
                {action?.id && (
                  <span
                    className={cn(
                      'font-mono font-semibold max-w-44 overflow-hidden text-ellipsis text-end'
                    )}
                  >
                    {action.id}
                  </span>
                )}
                <CustomHandle
                  type="source"
                  position={Position.Right}
                  id={action.id}
                  variant={getActionVariant(action)}
                  style={{ top: '50%', right: -15 }}
                />
              </div>
              <div className="truncate font-medium max-w-44 overflow-hidden text-ellipsis">
                {action.label.trim() ? action.label : t('approval.preview.emptyAction')}
              </div>
            </div>
          ))}
          <div className="relative flex items-center justify-end py-2 text-xs border-t">
            <span className="flex min-w-0 items-center gap-1">
              <Clock className="size-3 shrink-0" />
              <span className="font-mono font-semibold">{t('approval.preview.timeout')}</span>
            </span>
            <CustomHandle
              type="source"
              position={Position.Right}
              id={APPROVAL_TIMEOUT_HANDLE}
              variant="destructive"
              style={{ top: '50%', right: -15 }}
            />
          </div>
        </div>
      </div>
    </div>
  );
}

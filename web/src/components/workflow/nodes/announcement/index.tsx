import React from 'react';
import { Position } from '@xyflow/react';
import { Clock, Megaphone } from 'lucide-react';

import CustomHandle from '../../ui/custom-handle';
import ValueBadge from '@/components/workflow/ui/value-badge';
import { useT } from '@/i18n';
import { normalizeAnnouncementNodeData, type AnnouncementNodeData } from './config';

export interface AnnouncementContentProps {
  nodeId: string;
  data: AnnouncementNodeData;
}

const CONTENT_TOKEN_PATTERN = /\{\{#([^#]+?)#\}\}/g;

function renderAnnouncementContent(
  input: string,
  nodeId: string,
  t: ReturnType<typeof useT<'nodes'>>
) {
  if (!input) {
    return <span className="text-muted-foreground">{t('announcement.preview.emptyContent')}</span>;
  }

  const parts: Array<string | React.ReactNode> = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = CONTENT_TOKEN_PATTERN.exec(input)) !== null) {
    const [full, inner] = match;
    const start = match.index;
    const end = start + full.length;
    if (start > lastIndex) {
      parts.push(input.slice(lastIndex, start));
    }
    if (inner.includes('.')) {
      parts.push(
        <ValueBadge
          key={`announcement-value-${start}`}
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
  return parts.length > 0 ? parts : t('announcement.preview.emptyContent');
}

export default function AnnouncementContent({ nodeId, data }: AnnouncementContentProps) {
  const t = useT('nodes');
  const normalized = normalizeAnnouncementNodeData(data);
  const content = normalized.announcement.content.trim();

  return (
    <div className="mt-1">
      <CustomHandle type="target" position={Position.Left} id="target" style={{ top: -18 }} />
      <div className="space-y-2">
        <div
          className="nowheel mt-1 max-h-[160px] min-h-8 overflow-y-auto rounded-md bg-muted/70 p-1.5 text-xs leading-relaxed text-secondary-foreground break-words whitespace-pre-wrap scrollbar-thin"
          onWheelCapture={event => event.stopPropagation()}
        >
          {renderAnnouncementContent(content, nodeId, t)}
        </div>
        <div className="relative flex items-center justify-between border-t py-2 text-xs">
          <span className="flex min-w-0 items-center gap-1">
            <Megaphone className="size-3 shrink-0" />
            <span className="font-medium">{t('announcement.preview.publicLink')}</span>
          </span>
          <span className="flex min-w-0 items-center gap-1 text-muted-foreground">
            <Clock className="size-3 shrink-0" />
            <span>{t(`announcement.timeout.${normalized.timeout.unit}`)}</span>
          </span>
          <CustomHandle
            type="source"
            position={Position.Right}
            id="source"
            style={{ top: '50%', right: -15 }}
          />
        </div>
      </div>
    </div>
  );
}

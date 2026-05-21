import React from 'react';

import ValueBadge from '@/components/workflow/ui/value-badge';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
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

function PreviewField({
  label,
  value,
  children,
  className,
}: {
  label: string;
  value?: string;
  children?: React.ReactNode;
  className?: string;
}) {
  return (
    <div className={cn('space-y-1.5 rounded-md border bg-background/80 px-2 py-1.5', className)}>
      <div className="text-[10px] font-medium uppercase text-muted-foreground">{label}</div>
      <div
        className="nowheel max-h-[84px] overflow-y-auto text-xs leading-relaxed text-foreground break-words whitespace-pre-wrap scrollbar-thin"
        title={value}
        onWheelCapture={event => event.stopPropagation()}
      >
        {children}
      </div>
    </div>
  );
}

export default function AnnouncementContent({ nodeId, data }: AnnouncementContentProps) {
  const t = useT('nodes');
  const normalized = normalizeAnnouncementNodeData(data);
  const title = normalized.announcement.title.trim();
  const content = normalized.announcement.content.trim();
  const expiration = t('announcement.preview.expirationValue', {
    duration: normalized.timeout.duration,
    unit: t(`announcement.timeout.${normalized.timeout.unit}`),
  });

  return (
    <div className="mt-1 space-y-2">
      <PreviewField label={t('announcement.preview.title')} value={title}>
        <span className="font-medium">{renderAnnouncementContent(title, nodeId, t)}</span>
      </PreviewField>
      <PreviewField label={t('announcement.preview.content')} value={content}>
        {renderAnnouncementContent(content, nodeId, t)}
      </PreviewField>
      <PreviewField label={t('announcement.preview.expiration')} value={expiration}>
        <span className="font-medium text-muted-foreground">{expiration}</span>
      </PreviewField>
    </div>
  );
}

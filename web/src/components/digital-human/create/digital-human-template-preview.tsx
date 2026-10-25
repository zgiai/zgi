'use client';

import type { ReactNode } from 'react';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import type { IconValue } from '@/components/common/icon-input/types';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent } from '@/components/ui/card';
import { useT } from '@/i18n';

/** Shared avatar renderer for the gallery cards, the live preview and the success dialog. */
export function DigitalHumanAvatar({
  icon,
  fallbackName,
  size = 'md',
}: {
  icon: IconValue;
  fallbackName: string;
  size?: 'sm' | 'md' | 'lg' | 'xl';
}) {
  const text =
    icon.type === 'text' ? icon.icon || fallbackName.trim().slice(0, 2).toUpperCase() : '';
  return (
    <IconPreview
      icon={text}
      iconType={icon.type}
      src={icon.type === 'image' ? icon.iconUrl || '' : ''}
      iconBackground={icon.type === 'text' ? icon.iconBackground : undefined}
      editable={false}
      size={size}
    />
  );
}

// With limit, the list stays on one line and counts the tags it leaves out,
// so a fixed-height slot never cuts a second row in half.
export function DigitalHumanTagList({ tags, limit }: { tags: readonly string[]; limit?: number }) {
  if (tags.length === 0) return null;
  const shown = limit ? tags.slice(0, limit) : tags;
  const hidden = tags.length - shown.length;
  return (
    <div className={limit ? 'flex min-w-0 flex-nowrap gap-1.5' : 'flex flex-wrap gap-1.5'}>
      {shown.map(tag => (
        <span
          key={tag}
          className="truncate rounded-md bg-muted px-2 py-0.5 text-[11px] leading-5 text-muted-foreground"
        >
          {tag}
        </span>
      ))}
      {hidden > 0 ? (
        <span className="shrink-0 rounded-md bg-muted px-2 py-0.5 text-[11px] leading-5 text-muted-foreground">
          +{hidden}
        </span>
      ) : null}
    </div>
  );
}

/**
 * Read-only bullet list of display-ready strings.
 *
 * It renders whatever items it is given — skill labels or connection resource
 * labels — and never parses metadata or adds interaction.
 */
export function DigitalHumanCapabilityList({ items }: { items: readonly string[] }) {
  if (items.length === 0) return null;
  return (
    <ul className="space-y-1.5">
      {items.map((item, index) => (
        <li
          key={`${index}:${item}`}
          className="flex items-center gap-2 text-sm leading-6"
        >
          <span className="size-1.5 shrink-0 rounded-full bg-success" aria-hidden />
          <span className="min-w-0 break-words">{item}</span>
        </li>
      ))}
    </ul>
  );
}

/**
 * Live preview of what creation will produce.
 *
 * It only reflects the fields written into the create request. Capability
 * sources, bindings, and connection state are not shown in this preview.
 */
export function DigitalHumanTemplatePreview({
  icon,
  name,
  templateTitle,
  responsibility,
  tags,
}: {
  icon: IconValue;
  name: string;
  templateTitle: string;
  responsibility: string;
  tags: readonly string[];
}) {
  const t = useT('digitalHumans');
  return (
    <Card className="border border-border/80 shadow-sm">
      <CardContent className="flex flex-col items-center gap-3 p-5 text-center">
        <DigitalHumanAvatar icon={icon} fallbackName={name || templateTitle} size="lg" />
        <div className="w-full min-w-0">
          <p className="truncate text-sm font-semibold">{name.trim() || '—'}</p>
          <p className="mt-0.5 truncate text-xs text-muted-foreground">{templateTitle}</p>
        </div>
        <Badge
          variant="outline"
          className="border-success/40 bg-success/10 text-[11px] font-normal text-success"
        >
          {t('createFlow.preview.published')}
        </Badge>
        <div className="w-full border-t pt-3 text-left">
          <p className="text-xs font-medium text-muted-foreground">
            {t('createFlow.detail.responsibility')}
          </p>
          <p className="mt-1 line-clamp-4 whitespace-pre-line text-xs leading-5">
            {responsibility.trim() || t('createFlow.preview.noResponsibility')}
          </p>
        </div>
        {tags.length > 0 ? (
          <div className="w-full border-t pt-3">
            <DigitalHumanTagList tags={tags} />
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

/** Small titled block used by the read-only template detail drawer. */
export function DigitalHumanDetailSection({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section className="space-y-2">
      <h3 className="text-xs font-medium text-muted-foreground">{title}</h3>
      {children}
    </section>
  );
}

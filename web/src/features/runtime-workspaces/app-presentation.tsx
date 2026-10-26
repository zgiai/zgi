'use client';

import * as React from 'react';
import { Bot, Brain, CheckCircle2, Eye, FileText, Tags } from 'lucide-react';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import { ICON_BG, ICON_TEXT } from '@/lib/config';
import type { RuntimeApp, RuntimeCatalogTag } from '@/services/types/runtime-workspace';

export type RuntimeAppPresentation = Pick<RuntimeApp, 'name' | 'icon' | 'icon_type' | 'icon_url'>;

export function formatShortDate(value?: string | null): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }).format(date);
}

/**
 * @component RuntimeAppIcon
 * @category Feature
 * @status Beta
 * @description The icon of an exposed app: its image, or its emoji and
 * background, or its initials.
 * @usage Used in the External access app list, detail, and picker.
 * @example
 * <RuntimeAppIcon app={app} />
 */
export function RuntimeAppIcon({ app }: { app: RuntimeAppPresentation }) {
  let icon = app.name.slice(0, 2).toUpperCase() || ICON_TEXT;
  let background = ICON_BG;
  try {
    const parsed = JSON.parse(app.icon || '{}');
    icon = parsed?.icon || icon;
    background = parsed?.icon_background || background;
  } catch {
    background = ICON_BG;
  }
  return (
    <IconPreview
      icon={icon}
      iconType={app.icon_type}
      src={app.icon_type === 'image' ? app.icon_url || '' : ''}
      iconBackground={app.icon_type === 'image' ? '' : background}
      editable={false}
      size="sm"
    />
  );
}

/**
 * @component CapabilityBadges
 * @category Feature
 * @status Beta
 * @description What /connect/v1 serves for an app, as small badges.
 * @usage Used in the External access app detail and picker.
 * @example
 * <CapabilityBadges capabilities={['chat', 'files']} />
 */
export function CapabilityBadges({ capabilities }: { capabilities: string[] }) {
  const t = useT('workspace');
  const icons: Record<string, React.ElementType> = {
    files: FileText,
    memory: Brain,
    vision: Eye,
    chat: Bot,
  };
  return (
    <div className="flex flex-wrap gap-1.5">
      {capabilities
        .filter(capability => capability !== 'conversations')
        .map(capability => {
          const Icon = icons[capability] ?? CheckCircle2;
          return (
            <Badge key={capability} variant="subtle" className="gap-1 text-[11px] font-normal">
              <Icon className="size-3" />
              {t(`runtime.apps.capabilities.${capability}` as never)}
            </Badge>
          );
        })}
    </div>
  );
}

/**
 * @component CatalogTagBadges
 * @category Feature
 * @status Beta
 * @description An app's catalog tags, the first few shown and the rest
 * counted.
 * @usage Used in the External access app detail and picker.
 * @example
 * <CatalogTagBadges tags={app.tags} />
 */
export function CatalogTagBadges({ tags = [] }: { tags?: RuntimeCatalogTag[] }) {
  const t = useT('workspace');
  if (tags.length === 0) return null;
  const visibleTags = tags.slice(0, 3);
  const hiddenCount = tags.length - visibleTags.length;
  return (
    <div
      className="flex min-h-6 items-center gap-1 overflow-hidden"
      aria-label={t('runtime.apps.categories')}
      title={tags.map(tag => tag.name).join(', ')}
    >
      <Tags className="size-3.5 shrink-0 text-muted-foreground" />
      {visibleTags.map(tag => (
        <Badge
          key={tag.id}
          variant="outline"
          className="max-w-28 truncate rounded-full border-primary/20 bg-primary/5 px-2 py-0 text-[10px] font-normal text-primary"
        >
          {tag.name}
        </Badge>
      ))}
      {hiddenCount > 0 ? (
        <Badge
          variant="outline"
          className="rounded-full px-1.5 py-0 text-[10px] font-normal text-muted-foreground"
        >
          +{hiddenCount}
        </Badge>
      ) : null}
    </div>
  );
}

const unavailableReasons = [
  'agent_not_found',
  'source_workspace_archived',
  'agent_not_published',
  'digital_human_not_published',
  'agent_disabled',
  'digital_human_api_disabled',
  'agent_not_authorized',
  'binding_disabled',
] as const;
type UnavailableReason = (typeof unavailableReasons)[number];

/**
 * @component AppAvailability
 * @category Feature
 * @status Beta
 * @description Whether an exposed app can be called now, and why not.
 * @usage Used in the External access app list and detail.
 * @example
 * <AppAvailability app={app} />
 */
export function AppAvailability({
  app,
}: {
  app: Pick<RuntimeApp, 'callable' | 'unavailable_reason'>;
}) {
  const t = useT('workspace');
  if (app.callable !== false) {
    return <Badge variant="success">{t('runtime.apps.active')}</Badge>;
  }
  const reason = app.unavailable_reason;
  return (
    <Badge variant="warning">
      {reason && (unavailableReasons as readonly string[]).includes(reason)
        ? t(`runtime.apps.unavailableReasons.${reason as UnavailableReason}`)
        : t('runtime.apps.unavailable')}
    </Badge>
  );
}

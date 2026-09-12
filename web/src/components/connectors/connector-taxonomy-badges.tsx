'use client';

import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import type { ConnectorCatalogItem } from '@/services/types/connector';
import { connectorCategory, connectorTags } from './connector-metadata';

export function ConnectorTaxonomyBadges({
  item,
  maxTags = 3,
  className,
}: {
  item: Pick<ConnectorCatalogItem, 'category' | 'tags'>;
  maxTags?: number;
  className?: string;
}) {
  const t = useT('connectors');
  const category = connectorCategory(item);
  const tags = connectorTags(item);
  const visibleTags = tags.slice(0, Math.max(0, maxTags));
  const hiddenCount = Math.max(0, tags.length - visibleTags.length);
  if (visibleTags.length === 0 && category === 'other' && !className) {
    return <Badge variant="subtle">{t(`categories.${category}`)}</Badge>;
  }
  return (
    <div className={className ?? 'flex min-w-0 flex-wrap items-center gap-1.5'}>
      <Badge variant="outline">{t(`categories.${category}`)}</Badge>
      {visibleTags.map(tag => (
        <Badge key={tag} variant="subtle">
          {t(`tags.${tag}`)}
        </Badge>
      ))}
      {hiddenCount > 0 ? (
        <Badge
          variant="subtle"
          title={tags
            .slice(visibleTags.length)
            .map(tag => t(`tags.${tag}`))
            .join('、')}
        >
          {t('tags.more', { count: hiddenCount })}
        </Badge>
      ) : null}
    </div>
  );
}

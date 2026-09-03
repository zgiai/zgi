'use client';

import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import type { ConnectorCatalogItem, ConnectorClassification } from '@/services/types/connector';
import { ConnectorCatalogRow } from './connector-catalog-row';

interface ConnectorCatalogSectionProps {
  classification: ConnectorClassification;
  items: ConnectorCatalogItem[];
}

export function ConnectorCatalogSection({ classification, items }: ConnectorCatalogSectionProps) {
  const t = useT('connectors');
  const isDataSource = classification === 'data_source';
  return (
    <section aria-labelledby={`connector-section-${classification}`}>
      <div className="mb-3 flex items-end justify-between gap-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h2 id={`connector-section-${classification}`} className="text-lg font-semibold">
              {isDataSource ? t('sections.dataSources') : t('sections.connectors')}
            </h2>
            <Badge variant="subtle">{items.length}</Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {isDataSource
              ? t('sections.dataSourcesDescription')
              : t('sections.connectorsDescription')}
          </p>
        </div>
      </div>
      {items.length > 0 ? (
        <div className="grid auto-rows-fr gap-4 lg:grid-cols-2">
          {items.map(item => (
            <ConnectorCatalogRow key={item.id} item={item} />
          ))}
        </div>
      ) : (
        <div className="rounded-xl border border-dashed bg-background/60 px-5 py-8 text-center text-sm text-muted-foreground">
          {t('sections.empty')}
        </div>
      )}
    </section>
  );
}

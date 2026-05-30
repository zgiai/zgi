'use client';

import type { ComponentType } from 'react';
import { AlertCircle, Box, Database, Hash, Layers3 } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import type {
  FileAssetArtifactState,
  FileAssetVectorStatus,
  FileDetailProcessing,
} from '@/services/types/file';

interface FileIndexInfoPanelProps {
  artifactState?: FileAssetArtifactState;
  processing?: FileDetailProcessing;
  vectorStatus?: string;
  enabled: boolean;
}

function getVectorBadgeVariant(status?: string) {
  switch (status) {
    case 'ready':
      return 'success' as const;
    case 'indexing':
      return 'info' as const;
    case 'failed':
      return 'destructive' as const;
    case 'none':
    default:
      return 'subtle' as const;
  }
}

function IndexMetric({
  label,
  value,
  icon: Icon,
}: {
  label: string;
  value?: string | number | null;
  icon: ComponentType<{ className?: string }>;
}) {
  return (
    <div className="rounded-md border border-border bg-background p-4">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <Icon className="h-4 w-4" />
        <span>{label}</span>
      </div>
      <div className="mt-3 text-lg font-semibold text-foreground">
        {value === undefined || value === null || value === '' ? '-' : value}
      </div>
    </div>
  );
}

export function FileIndexInfoPanel({
  artifactState,
  processing,
  vectorStatus,
  enabled,
}: FileIndexInfoPanelProps) {
  const t = useT('files');
  const resolvedVectorStatus = vectorStatus || artifactState?.vector_status || 'none';
  const vectorStatusLabel = (() => {
    switch (resolvedVectorStatus as FileAssetVectorStatus | string) {
      case 'indexing':
        return t('detail.vectorStatus.indexing');
      case 'ready':
        return t('detail.vectorStatus.ready');
      case 'failed':
        return t('detail.vectorStatus.failed');
      case 'none':
      default:
        return t('detail.vectorStatus.none');
    }
  })();

  if (!enabled) {
    return (
      <Alert>
        <AlertCircle className="h-4 w-4" />
        <AlertTitle>{t('detail.index.notReadyTitle')}</AlertTitle>
        <AlertDescription>{t('detail.index.notReadyDescription')}</AlertDescription>
      </Alert>
    );
  }

  return (
    <div className="space-y-4">
      <div className="rounded-md border border-border bg-background p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold text-foreground">{t('detail.index.title')}</h2>
            <p className="mt-1 text-sm text-muted-foreground">{t('detail.index.description')}</p>
          </div>
          <Badge variant={getVectorBadgeVariant(resolvedVectorStatus)}>{vectorStatusLabel}</Badge>
        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <IndexMetric
          label={t('detail.embeddingProvider')}
          value={artifactState?.embedding_provider}
          icon={Database}
        />
        <IndexMetric
          label={t('detail.embeddingModel')}
          value={artifactState?.embedding_model}
          icon={Box}
        />
        <IndexMetric
          label={t('detail.embeddingDimension')}
          value={artifactState?.embedding_dimension}
          icon={Hash}
        />
        <IndexMetric
          label={t('detail.embeddingCount')}
          value={processing?.embedding_count}
          icon={Layers3}
        />
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <IndexMetric label={t('detail.chunkCount')} value={processing?.chunk_count} icon={Layers3} />
        <IndexMetric
          label={t('detail.generationNo')}
          value={processing?.summary.generation_no}
          icon={Hash}
        />
      </div>
    </div>
  );
}

'use client';

import { useT } from '@/i18n';
import { AlertTriangle, FileX, RotateCcw, Pause, Play, Info } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { cn } from '@/lib/utils';

interface DocumentStatusProps {
  document: {
    id: string;
    name: string;
    status?: string;
    display_status?: string;
    indexing_status?: string;
    error?: string;
    auto_disabled?: boolean;
    indexing_progress?: number;
  };
  onRetry?: (documentId: string) => void;
  onToggleEnabled?: (documentId: string, enabled: boolean) => void;
  className?: string;
}

// Auto-disabled document component
export function AutoDisabledDocument({
  document,
  onToggleEnabled,
  className,
}: DocumentStatusProps) {
  const t = useT();

  const handleEnable = () => {
    onToggleEnabled?.(document.id, true);
  };

  return (
    <Card className={cn('border-orange-200 bg-orange-50/50', className)}>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-2">
            <Pause className="h-4 w-4 text-orange-600" />
            <CardTitle className="text-sm font-medium">
              {t('datasets.documentStatusView.autoDisabled')}
            </CardTitle>
          </div>
          <Badge variant="outline" className="bg-orange-100 text-orange-700 border-orange-300">
            {t('datasets.status.disabled')}
          </Badge>
        </div>
        <CardDescription className="text-xs">{document.name}</CardDescription>
      </CardHeader>
      <CardContent className="pt-0">
        <Alert className="mb-3 border-orange-200 bg-orange-50">
          <Info className="h-3 w-3" />
          <AlertDescription className="text-xs">
            {t('datasets.documentStatusView.autoDisabledReason')}
          </AlertDescription>
        </Alert>
        <div className="flex gap-2">
          <Button size="sm" variant="outline" onClick={handleEnable} className="h-7 text-xs">
            <Play className="h-3 w-3 mr-1" />
            {t('datasets.documentActions.enable')}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

// Index failed document component
export function IndexFailed({
  document,
  onRetry,
  onToggleEnabled,
  className,
}: DocumentStatusProps) {
  const t = useT();

  const handleRetry = () => {
    onRetry?.(document.id);
  };

  const handleDisable = () => {
    onToggleEnabled?.(document.id, false);
  };

  return (
    <Card className={cn('border-red-200 bg-red-50/50', className)}>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-2">
            <AlertTriangle className="h-4 w-4 text-red-600" />
            <CardTitle className="text-sm font-medium">
              {t('datasets.documentStatusView.indexFailed')}
            </CardTitle>
          </div>
          <Badge variant="destructive" className="bg-red-100 text-red-700 border-red-300">
            {t('datasets.status.error')}
          </Badge>
        </div>
        <CardDescription className="text-xs">{document.name}</CardDescription>
      </CardHeader>
      <CardContent className="pt-0">
        {document.error && (
          <Alert variant="destructive" className="mb-3 border-red-200 bg-red-50">
            <FileX className="h-3 w-3" />
            <AlertDescription className="text-xs">{document.error}</AlertDescription>
          </Alert>
        )}
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={handleRetry}
            className="h-7 text-xs border-red-300 hover:bg-red-50"
          >
            <RotateCcw className="h-3 w-3 mr-1" />
            {t('datasets.documentActions.retry')}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={handleDisable}
            className="h-7 text-xs text-muted-foreground"
          >
            <Pause className="h-3 w-3 mr-1" />
            {t('datasets.documentActions.disable')}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

// Processing document component
export function ProcessingDocument({ document, className }: DocumentStatusProps) {
  const t = useT();

  return (
    <Card className={cn('border-blue-200 bg-blue-50/50', className)}>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div className="flex items-center gap-2">
            <div className="h-4 w-4 border-2 border-blue-600 border-t-transparent rounded-full animate-spin" />
            <CardTitle className="text-sm font-medium">
              {t('datasets.documentStatusView.processing')}
            </CardTitle>
          </div>
          <Badge variant="outline" className="bg-blue-100 text-blue-700 border-blue-300">
            {t('datasets.status.processing')}
          </Badge>
        </div>
        <CardDescription className="text-xs">{document.name}</CardDescription>
      </CardHeader>
      <CardContent className="pt-0">
        <div className="space-y-2">
          <div className="flex items-center justify-between text-xs">
            <span className="text-muted-foreground">
              {t('datasets.documentStatusView.progress')}
            </span>
            <span className="font-medium">{document.indexing_progress || 0}%</span>
          </div>
          <div className="w-full bg-blue-100 rounded-full h-1.5">
            <div
              className="bg-blue-600 h-1.5 rounded-full transition-all duration-300"
              style={{ width: `${document.indexing_progress || 0}%` }}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            {t('datasets.documentStatusView.processingDescription')}
          </p>
        </div>
      </CardContent>
    </Card>
  );
}

// Generic document status component that renders appropriate status
export function DocumentStatus(props: DocumentStatusProps) {
  const { document } = props;

  if (document.auto_disabled) {
    return <AutoDisabledDocument {...props} />;
  }

  const status = document.display_status ?? document.indexing_status ?? document.status;

  if (status === 'error' || status === 'failed') {
    return <IndexFailed {...props} />;
  }

  if (status === 'processing' || status === 'indexing') {
    return <ProcessingDocument {...props} />;
  }

  return null;
}

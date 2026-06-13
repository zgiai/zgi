'use client';

import React from 'react';
import { AlertCircle, LoaderCircle, RefreshCcw } from 'lucide-react';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  UniversalFilePreviewContent,
  type UniversalFilePreviewDescriptor,
} from '@/components/files/universal-file-preview-dialog';

export type FileEvidenceTab = 'original' | 'text';

export interface FileEvidenceViewerLabels {
  title: string;
  tabs: {
    original: string;
    text: string;
  };
  originalPreviewAlt: string;
  loadingOriginalPreview: string;
  originalPreviewUnavailable: string;
  promptLoadFailedTitle: string;
  retryPrompt: string;
  fileErrorTitle: string;
  fileWarningTitle: string;
  fileErrorDetails: string;
  retryFileParse: string;
  noPreview: string;
}

export interface FileEvidenceViewerProps {
  tab: FileEvidenceTab;
  onTabChange: (tab: FileEvidenceTab) => void;
  labels: FileEvidenceViewerLabels;
  original?: {
    enabled: boolean;
    file?: UniversalFilePreviewDescriptor | null;
    url?: string;
    loading?: boolean;
    error?: string;
  };
  promptIssue?: {
    message: string;
    loading?: boolean;
    onRetry?: () => void;
  };
  loading?: {
    active: boolean;
    title: string;
    description: string;
    render?: React.ReactNode;
  };
  error?: {
    message: string;
    hint?: string;
    onRetryFileParse?: () => void;
  };
  warning?: {
    message: string;
    hint?: string;
  };
  content?: string;
  highlights?: string[];
  headerActions?: React.ReactNode;
}

export function FileEvidenceViewer({
  tab,
  onTabChange,
  labels,
  original,
  promptIssue,
  loading,
  error,
  warning,
  content,
  highlights,
  headerActions,
}: FileEvidenceViewerProps) {
  const originalEnabled = Boolean(original?.enabled);
  return (
    <Tabs
      value={tab}
      onValueChange={value => onTabChange(value as FileEvidenceTab)}
      className="flex h-full min-h-0 flex-col"
    >
      <div className="flex items-center gap-2 border-b px-3 py-2 text-sm font-medium">
        <span className="shrink-0">{labels.title}</span>
        <TabsList className="h-8 rounded-md p-0.5">
          <TabsTrigger
            value="original"
            disabled={!originalEnabled}
            className="h-6 rounded px-2 text-xs"
          >
            {labels.tabs.original}
          </TabsTrigger>
          <TabsTrigger value="text" className="h-6 rounded px-2 text-xs">
            {labels.tabs.text}
          </TabsTrigger>
        </TabsList>
        {headerActions ? (
          <div className="ml-auto flex min-w-0 shrink-0 items-center gap-2">{headerActions}</div>
        ) : null}
      </div>

      <TabsContent value="original" className="m-0 min-h-0 flex-1 overflow-auto p-3">
        <OriginalEvidencePreview original={original} labels={labels} />
      </TabsContent>

      <TabsContent value="text" className="m-0 min-h-0 flex-1 overflow-auto p-3">
        <TextEvidencePanel
          labels={labels}
          promptIssue={promptIssue}
          loading={loading}
          error={error}
          warning={warning}
          content={content}
          highlights={highlights}
        />
      </TabsContent>
    </Tabs>
  );
}

function OriginalEvidencePreview({
  original,
  labels,
}: {
  original?: FileEvidenceViewerProps['original'];
  labels: FileEvidenceViewerLabels;
}) {
  return (
    <div className="h-full min-h-[420px] overflow-hidden rounded-md border bg-muted/20">
      <UniversalFilePreviewContent
        file={original?.file ?? null}
        previewUrl={original?.url}
        isLoading={original?.loading}
        error={original?.error || (!original?.enabled ? labels.originalPreviewUnavailable : null)}
        className="min-h-[420px]"
      />
    </div>
  );
}

function TextEvidencePanel({
  labels,
  promptIssue,
  loading,
  error,
  warning,
  content,
  highlights,
}: {
  labels: FileEvidenceViewerLabels;
  promptIssue?: FileEvidenceViewerProps['promptIssue'];
  loading?: FileEvidenceViewerProps['loading'];
  error?: FileEvidenceViewerProps['error'];
  warning?: FileEvidenceViewerProps['warning'];
  content?: string;
  highlights?: string[];
}) {
  if (promptIssue) {
    return (
      <Alert variant="destructive" className="max-w-3xl">
        <AlertCircle className="h-4 w-4" />
        <AlertDescription>
          <div className="font-medium">{labels.promptLoadFailedTitle}</div>
          <div className="mt-1 break-words">{promptIssue.message}</div>
          {promptIssue.onRetry ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="mt-3"
              disabled={promptIssue.loading}
              onClick={promptIssue.onRetry}
            >
              {labels.retryPrompt}
            </Button>
          ) : null}
        </AlertDescription>
      </Alert>
    );
  }

  if (loading?.active && !content) {
    return (
      loading.render ?? (
        <div className="flex h-full min-h-[420px] items-center justify-center">
          <div className="w-full max-w-md rounded-lg border border-dashed border-primary/30 bg-primary/5 px-6 py-8 text-center">
            <div className="mt-1 flex items-center justify-center gap-2 text-sm font-medium text-foreground">
              <LoaderCircle className="h-4 w-4 animate-spin text-primary" />
              <span>{loading.title}</span>
            </div>
            <div className="mt-2 text-sm leading-6 text-muted-foreground">
              {loading.description}
            </div>
          </div>
        </div>
      )
    );
  }

  return (
    <div className="space-y-3">
      {error ? (
        <div
          role="alert"
          className="max-w-3xl rounded-md border border-warning/35 bg-warning/5 p-4 text-sm"
        >
          <div className="flex gap-3">
            <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-warning/15 text-warning">
              <AlertCircle className="h-4 w-4" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="font-medium text-foreground">{labels.fileErrorTitle}</div>
              {error.hint ? (
                <div className="mt-1 text-sm leading-6 text-muted-foreground">{error.hint}</div>
              ) : null}
              <div className="mt-3 flex flex-wrap gap-2">
                {error.onRetryFileParse ? (
                  <Button type="button" size="sm" variant="outline" onClick={error.onRetryFileParse}>
                    <RefreshCcw className="h-4 w-4" />
                    {labels.retryFileParse}
                  </Button>
                ) : null}
              </div>
              {error.message ? (
                <details className="mt-3 text-xs text-muted-foreground">
                  <summary className="cursor-pointer select-none">{labels.fileErrorDetails}</summary>
                  <div className="mt-2 whitespace-pre-wrap break-words rounded border bg-background/80 p-2 font-mono leading-5">
                    {error.message}
                  </div>
                </details>
              ) : null}
            </div>
          </div>
        </div>
      ) : null}

      {warning ? (
        <Alert className="max-w-3xl border-warning/40 bg-warning/10 text-warning">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>
            <div className="font-medium">{labels.fileWarningTitle}</div>
            <div className="mt-1 break-words">{warning.message}</div>
            {warning.hint ? <div className="mt-2 text-xs leading-5">{warning.hint}</div> : null}
          </AlertDescription>
        </Alert>
      ) : null}

      {content ? (
        <MarkdownViewer content={content} highlights={highlights} />
      ) : (
        <div className="text-sm text-muted-foreground">{labels.noPreview}</div>
      )}
    </div>
  );
}

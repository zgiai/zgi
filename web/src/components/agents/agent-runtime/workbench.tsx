'use client';

import { useEffect, useRef, useState } from 'react';
import { AgentRuntimeHeader } from './header';
import { AgentRuntimeOrchestrationPanel } from './orchestration-panel';
import { AgentRuntimePreviewPanel } from './preview-panel';
import { AgentRuntimePromptPanel } from './prompt-panel';
import { AgentRuntimeVersionPopover } from './published-versions-dialog';
import { Sheet, SheetContent, SheetDescription, SheetTitle } from '@/components/ui/sheet';
import { cn } from '@/lib/utils';
import type { AgentRuntimePageModel } from './hooks/use-agent-runtime-page-model';

interface AgentRuntimeWorkbenchProps {
  model: AgentRuntimePageModel;
}

const TWO_COLUMN_MIN_WIDTH = 760;
const INLINE_PREVIEW_MIN_WIDTH = 1280;

function useMeasuredWidth() {
  const ref = useRef<HTMLDivElement | null>(null);
  const [width, setWidth] = useState(0);

  useEffect(() => {
    const node = ref.current;
    if (!node) return;

    const measure = () => setWidth(Math.floor(node.getBoundingClientRect().width));
    measure();

    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', measure);
      return () => window.removeEventListener('resize', measure);
    }

    const observer = new ResizeObserver(entries => {
      const entry = entries[0];
      setWidth(Math.floor(entry?.contentRect.width ?? node.getBoundingClientRect().width));
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  return { ref, width };
}

export function AgentRuntimeWorkbench({ model }: AgentRuntimeWorkbenchProps) {
  const { ref: bodyRef, width: bodyWidth } = useMeasuredWidth();
  const canUseTwoColumns = bodyWidth >= TWO_COLUMN_MIN_WIDTH;
  const showDraftPreview = model.preview.canUseDraftPreview;
  const showInlinePreview = showDraftPreview && bodyWidth >= INLINE_PREVIEW_MIN_WIDTH;
  const { previewSheetOpen, setPreviewSheetOpen } = model;

  useEffect(() => {
    if (showInlinePreview && previewSheetOpen) {
      setPreviewSheetOpen(false);
    }
  }, [previewSheetOpen, setPreviewSheetOpen, showInlinePreview]);

  const renderPreviewPanel = (surfaceMode: 'inline' | 'sheet' = 'inline') => (
    <AgentRuntimePreviewPanel
      {...model.preview}
      surfaceMode={surfaceMode}
      onClose={() => setPreviewSheetOpen(false)}
    />
  );

  return (
    <>
      <AgentRuntimeHeader
        {...model.header}
        showPreviewAction={showDraftPreview && !showInlinePreview}
        isPreviewOpen={previewSheetOpen}
        versionControl={<AgentRuntimeVersionPopover {...model.version} />}
      />

      <div ref={bodyRef} className="min-h-0 flex-1 overflow-y-auto lg:overflow-hidden">
        <div
          className={cn(
            'grid min-h-full min-w-0 grid-cols-1',
            canUseTwoColumns &&
              'lg:h-full lg:min-h-0 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] lg:divide-x',
            showInlinePreview &&
              'lg:grid-cols-[minmax(0,0.95fr)_minmax(0,0.95fr)_minmax(0,1.2fr)]'
          )}
        >
          <div
            className={cn(
              'h-[45vh] min-h-[360px] min-w-0 border-b',
              canUseTwoColumns && 'lg:h-full lg:min-h-0 lg:border-b-0'
            )}
          >
            <AgentRuntimePromptPanel className="h-full" {...model.prompt} />
          </div>
          <div className={cn('min-h-0 min-w-0', canUseTwoColumns && 'lg:h-full')}>
            <AgentRuntimeOrchestrationPanel
              className={cn('min-h-0', canUseTwoColumns && 'lg:h-full')}
              scrollAreaClassName={cn(
                'overflow-visible',
                canUseTwoColumns && 'lg:overflow-hidden'
              )}
              scrollViewportClassName={cn(
                'h-auto w-full rounded-[inherit]',
                canUseTwoColumns && 'lg:h-full'
              )}
              {...model.orchestration}
            />
          </div>
          {showInlinePreview ? (
            <div className="flex min-w-0 overflow-hidden">{renderPreviewPanel()}</div>
          ) : null}
        </div>
      </div>

      {showDraftPreview && !showInlinePreview ? (
        <Sheet open={previewSheetOpen} onOpenChange={setPreviewSheetOpen}>
          <SheetContent
            side="right"
            showClose={false}
            className="flex h-full min-h-0 w-[min(720px,100vw)] max-w-none flex-col p-0 sm:max-w-none"
          >
            <SheetTitle className="sr-only">{model.t('preview.title')}</SheetTitle>
            <SheetDescription className="sr-only">{model.t('preview.description')}</SheetDescription>
            {renderPreviewPanel('sheet')}
          </SheetContent>
        </Sheet>
      ) : null}
    </>
  );
}

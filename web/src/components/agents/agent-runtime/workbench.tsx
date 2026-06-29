'use client';

import { AgentRuntimeHeader } from './header';
import { AgentRuntimeOrchestrationPanel } from './orchestration-panel';
import { AgentRuntimePreviewPanel } from './preview-panel';
import { AgentRuntimePromptPanel } from './prompt-panel';
import { AgentRuntimeVersionPopover } from './published-versions-dialog';
import { Sheet, SheetContent, SheetDescription, SheetTitle } from '@/components/ui/sheet';
import type { AgentRuntimePageModel } from './hooks/use-agent-runtime-page-model';

interface AgentRuntimeWorkbenchProps {
  model: AgentRuntimePageModel;
}

export function AgentRuntimeWorkbench({ model }: AgentRuntimeWorkbenchProps) {
  const showDraftPreview = model.preview.canUseDraftPreview;
  const renderPreviewPanel = (surfaceMode: 'inline' | 'sheet' = 'inline') => (
    <AgentRuntimePreviewPanel
      {...model.preview}
      surfaceMode={surfaceMode}
      onClose={() => model.setPreviewSheetOpen(false)}
    />
  );

  return (
    <>
      <AgentRuntimeHeader
        {...model.header}
        versionControl={<AgentRuntimeVersionPopover {...model.version} />}
      />

      <div className="min-h-0 flex-1 overflow-y-auto lg:overflow-hidden">
        <div
          className={
            showDraftPreview
              ? 'grid min-h-full grid-cols-1 lg:h-full lg:min-h-0 lg:grid-cols-[minmax(320px,1fr)_minmax(360px,1fr)] lg:divide-x 2xl:grid-cols-[minmax(320px,0.95fr)_minmax(320px,0.95fr)_minmax(440px,1.2fr)]'
              : 'grid min-h-full grid-cols-1 lg:h-full lg:min-h-0 lg:grid-cols-[minmax(320px,1fr)_minmax(360px,1fr)] lg:divide-x'
          }
        >
          <div className="h-[45vh] min-h-[360px] border-b lg:h-full lg:min-h-0 lg:border-b-0">
            <AgentRuntimePromptPanel className="h-full" {...model.prompt} />
          </div>
          <div className="min-h-0 lg:h-full">
            <AgentRuntimeOrchestrationPanel
              className="min-h-0 lg:h-full"
              scrollAreaClassName="overflow-visible lg:overflow-hidden"
              scrollViewportClassName="h-auto w-full rounded-[inherit] lg:h-full"
              {...model.orchestration}
            />
          </div>
          {model.isTwoXlViewport && showDraftPreview ? (
            <div className="hidden min-w-0 overflow-hidden 2xl:flex">{renderPreviewPanel()}</div>
          ) : null}
        </div>
      </div>

      {!model.isTwoXlViewport && showDraftPreview ? (
        <Sheet open={model.previewSheetOpen} onOpenChange={model.setPreviewSheetOpen}>
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

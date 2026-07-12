import React, { useEffect, useMemo } from 'react';
import { useReactFlow } from '@xyflow/react';
import { Button } from '@/components/ui/button';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  Mouse,
  Touchpad,
  ZoomIn,
  ZoomOut,
  Maximize2,
  RotateCcw,
  RotateCw,
  PlusIcon,
  StickyNote,
} from 'lucide-react';
import { useWorkflowStore } from '../store';
import { useCreateNodeModal } from '../hooks/use-create-node-modal';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { getSavedWorkflowInteractionMode, saveWorkflowInteractionMode } from '@/utils/ui-local';

/**
 * Bottom toolbar for workflow editor interactions
 * - Input mode toggle: Mouse vs Trackpad
 * - View controls: Zoom out, Fit view, Zoom in
 * - History: Undo/Redo
 */
const WorkflowBottomToolbar: React.FC = () => {
  const rf = useReactFlow();
  const t = useT('agents');

  // Use fine-grained selectors to minimize re-renders
  const interactionMode = useWorkflowStore.use.interactionMode();
  const setInteractionMode = useWorkflowStore.use.setInteractionMode();
  const undo = useWorkflowStore.use.undo();
  const redo = useWorkflowStore.use.redo();
  const historyPast = useWorkflowStore.use.historyPast();
  const historyFuture = useWorkflowStore.use.historyFuture();
  const addNode = useWorkflowStore.use.addNode();

  const canUndo = historyPast.length > 0;
  const canRedo = historyFuture.length > 0;
  const { openModal } = useCreateNodeModal();

  const handleFitView = () => rf.fitView({ padding: 0.2, duration: 200 });
  const handleZoomIn = () => rf.zoomIn({ duration: 150 });
  const handleZoomOut = () => rf.zoomOut({ duration: 150 });

  const handleOpenAddNode: React.MouseEventHandler<HTMLButtonElement> = event => {
    openModal(undefined, null, { x: event.clientX, y: event.clientY });
  };

  const handleAddNote = () => {
    const center = rf.screenToFlowPosition({
      x: window.innerWidth / 2,
      y: window.innerHeight / 2,
    });
    addNode({ type: 'note', text: '' }, center);
  };

  const groupValue = useMemo(() => interactionMode, [interactionMode]);

  // Restore last selected interaction mode on mount; fallback handled by store default
  useEffect(() => {
    const saved = getSavedWorkflowInteractionMode();
    if (saved) {
      setInteractionMode(saved);
    }
  }, [setInteractionMode]);

  return (
    <div className="absolute bottom-4 left-1/2 -translate-x-1/2 z-10 flex items-center gap-2">
      <div className="flex items-center gap-2 rounded-lg border bg-background backdrop-blur px-2 py-1 shadow-sm">
        {/* Mode toggle */}
        <ToggleGroup
          type="single"
          value={groupValue}
          onValueChange={v => {
            if (v === 'mouse' || v === 'trackpad') {
              setInteractionMode(v);
              saveWorkflowInteractionMode(v);
            }
          }}
          className=""
        >
          <Tooltip>
            <TooltipTrigger asChild>
              <ToggleGroupItem value="mouse" aria-label={t('workflow.toolbar.mouse')}>
                <Mouse className={cn('h-4 w-4', groupValue === 'mouse' ? 'text-highlight' : '')} />
              </ToggleGroupItem>
            </TooltipTrigger>
            <TooltipContent>{t('workflow.toolbar.mouseTooltip')}</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <ToggleGroupItem value="trackpad" aria-label={t('workflow.toolbar.trackpad')}>
                <Touchpad
                  className={cn('h-4 w-4', groupValue === 'trackpad' ? 'text-highlight' : '')}
                />
              </ToggleGroupItem>
            </TooltipTrigger>
            <TooltipContent>{t('workflow.toolbar.trackpadTooltip')}</TooltipContent>
          </Tooltip>
        </ToggleGroup>

        {/* Divider */}
        <div className="w-px h-5 bg-gray-200" />

        {/* Zoom controls */}
        <div className="flex items-center gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                isIcon
                onClick={handleZoomOut}
                aria-label={t('workflow.toolbar.zoomOut')}
              >
                <ZoomOut className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('workflow.toolbar.zoomOut')}</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                isIcon
                onClick={handleFitView}
                aria-label={t('workflow.toolbar.fitView')}
              >
                <Maximize2 className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('workflow.toolbar.fitView')}</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                isIcon
                onClick={handleZoomIn}
                aria-label={t('workflow.toolbar.zoomIn')}
              >
                <ZoomIn className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('workflow.toolbar.zoomIn')}</TooltipContent>
          </Tooltip>
        </div>

        {/* Divider */}
        <div className="w-px h-5 bg-gray-200" />

        {/* History */}
        <div className="flex items-center gap-1">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                isIcon
                onClick={undo}
                disabled={!canUndo}
                aria-label={t('workflow.toolbar.undo')}
              >
                <RotateCcw className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('workflow.toolbar.undoHotkey')}</TooltipContent>
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                isIcon
                onClick={redo}
                disabled={!canRedo}
                aria-label={t('workflow.toolbar.redo')}
              >
                <RotateCw className="h-4 w-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t('workflow.toolbar.redoHotkey')}</TooltipContent>
          </Tooltip>
        </div>

        {/* Divider */}
        <div className="w-px h-5 bg-gray-200" />

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              isIcon
              onClick={handleAddNote}
              aria-label={t('workflow.toolbar.addNoteNode')}
            >
              <StickyNote className="h-4 w-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('workflow.toolbar.addNoteNodeTooltip')}</TooltipContent>
        </Tooltip>
      </div>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="default"
            isIcon
            onClick={handleOpenAddNode}
            aria-label={t('workflow.toolbar.addNode')}
          >
            <PlusIcon size={20} />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t('workflow.toolbar.addNodeTooltip')}</TooltipContent>
      </Tooltip>
    </div>
  );
};

export default WorkflowBottomToolbar;

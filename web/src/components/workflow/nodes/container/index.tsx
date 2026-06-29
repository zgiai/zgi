import React, { useCallback } from 'react';
import type { WorkflowNode, ContainerBaseData } from '../../store/type';
import { useWorkflowStoreBase, useWorkflowStore } from '../../store';
import { useCreateNodeModal } from '../../hooks/use-create-node-modal';
import { Button } from '@/components/ui/button';
import { Plus } from 'lucide-react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

interface ContainerContentProps {
  id: string;
  data: ContainerBaseData;
}

const ContainerContent: React.FC<ContainerContentProps> = ({ id, data }) => {
  const { openModal } = useCreateNodeModal();
  const t = useT();
  const mode = useWorkflowStore.use.mode();
  const canEdit = useWorkflowStore.use.canEdit();
  const isReadOnly = mode === 'history' || !canEdit;

  const onlyHasStart = useWorkflowStoreBase(
    useCallback(
      state => {
        if (!data?.start_node_id) return false;
        const children = (state.nodes as WorkflowNode[]).filter(
          n => (n as unknown as { parentId?: string })?.parentId === id
        );
        if (!children || children.length === 0) return false;
        return children.length === 1 && children[0].id === data.start_node_id;
      },
      [id, data?.start_node_id]
    )
  );

  const setDragOverContainerId = useWorkflowStore.use.setDragOverContainerId();

  const handleAddClick: React.MouseEventHandler<HTMLButtonElement> = e => {
    e.stopPropagation();
    if (isReadOnly) return;
    openModal(
      undefined,
      {
        nodeId: data.start_node_id,
        handleId: 'source',
        handleType: 'source',
      },
      { x: e.clientX, y: e.clientY }
    );
  };

  const handleDragOver: React.DragEventHandler = e => {
    e.preventDefault();
    e.stopPropagation();
    if (isReadOnly) {
      e.dataTransfer.dropEffect = 'none';
      return;
    }
    // No need to check node type here, the global overlay will handle it
    e.dataTransfer.dropEffect = 'copy';
    setDragOverContainerId(id);
  };

  const handleDragEnter: React.DragEventHandler = e => {
    e.preventDefault();
    e.stopPropagation();
    if (isReadOnly) return;
    setDragOverContainerId(id);
  };

  const handleDragLeave: React.DragEventHandler = e => {
    e.preventDefault();
    e.stopPropagation();
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    if (
      e.clientX < rect.left ||
      e.clientX > rect.right ||
      e.clientY < rect.top ||
      e.clientY > rect.bottom
    ) {
      setDragOverContainerId(null);
    }
  };

  return (
    <div
      className="relative w-full grow overflow-hidden rounded-b-[11px] bg-slate-50/50 dark:bg-slate-900/50 border-t border-slate-200/60 dark:border-slate-800/60 shadow-[inset_0_2px_4px_rgba(0,0,0,0.02)]"
      data-role="container-canvas"
      data-node-id={id}
      onDragOver={handleDragOver}
      onDragEnter={handleDragEnter}
      onDragLeave={handleDragLeave}
    >
      <div className="absolute inset-0 bg-[linear-gradient(to_right,#e5e7eb_1px,transparent_1px),linear-gradient(to_bottom,#e5e7eb_1px,transparent_1px)] bg-[size:20px_20px]" />
      <div className="absolute inset-0">
        {onlyHasStart && !isReadOnly && (
          <div className="absolute top-[13px] left-0 pointer-events-none flex items-center pl-[52px] overflow-visible">
            {/* Dashed Line - Matches system edge style */}
            <svg className="w-[80px] h-0.5 shrink-0 overflow-visible">
              <path
                d="M 2 1 L 80 1"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeDasharray="5,5"
                className="text-[var(--edge-default)] opacity-60 dark:opacity-40"
              />
            </svg>

            {/* High-Guidance Add Node Button */}
            <Button
              variant="default"
              size="sm"
              className={cn(
                'pointer-events-auto relative group flex items-center gap-2 h-9 px-4 ml-[-2px]',
                'bg-indigo-600 hover:bg-indigo-700 text-white border-transparent',
                'shadow-[0_4px_12px_-2px_rgba(79,70,229,0.4)] dark:shadow-[0_4px_12px_-2px_rgba(0,0,0,0.5)]',
                'transition-all duration-300 rounded-full scale-100 hover:scale-105 active:scale-95'
              )}
              onClick={handleAddClick}
            >
              <div className="flex items-center justify-center w-5 h-5 rounded-full bg-white/20 group-hover:bg-white/30 transition-colors duration-200">
                <Plus className="w-3.5 h-3.5 text-white" />
              </div>
              <span className="text-sm font-semibold tracking-wide">
                {t('agents.workflow.addNode')}
              </span>

              {/* Pulse effect for guidance - larger and more premium */}
              <div className="absolute inset-0 rounded-full animate-pulse bg-indigo-500/20 -z-10 scale-125" />
            </Button>
          </div>
        )}
      </div>
    </div>
  );
};

export default ContainerContent;

import React from 'react';
import WorkflowRunNodesList, {
  type WorkflowRunNodeListItem,
} from '@/components/workflow/ui/workflow-run-nodes-list';
import { useT } from '@/i18n';
import { Workflow, Play } from 'lucide-react';
import { cn } from '@/lib/utils';

interface ExecutionTabProps {
  items: WorkflowRunNodeListItem[];
  showDetail?: boolean;
  showHeader?: boolean;
  emptyTitle?: string;
  emptyDescription?: string;
  className?: string;
}

/**
 * ExecutionTab - shows the execution list content
 */
const ExecutionTab: React.FC<ExecutionTabProps> = ({
  items,
  showDetail = true,
  showHeader = true,
  emptyTitle,
  emptyDescription,
  className,
}) => {
  const t = useT('agents');
  return items.length > 0 ? (
    <div className={cn('flex-1 flex flex-col min-h-0 bg-background/40', className)}>
      {showHeader ? (
        <div className="px-3 py-2 border-b flex items-center justify-between bg-muted/30">
          <div className="flex items-center gap-1.5">
            <Workflow className="w-5 h-5 text-emerald-600" />
            <span className="text-sm font-semibold">{t('workflow.execution')}</span>
          </div>
          <div className="text-xs text-muted-foreground">
            {items.length} {t('workflow.steps', { count: items.length })}
          </div>
        </div>
      ) : null}
      <div className="min-h-0 flex-1 overflow-auto p-4 scrollbar-thin">
        <WorkflowRunNodesList showDetail={showDetail} items={items} />
      </div>
    </div>
  ) : (
    <div className={cn('h-full flex flex-col items-center justify-center gap-4 py-12', className)}>
      <div className="flex size-12 items-center justify-center rounded-xl bg-primary/5">
        <Play className="size-6 text-primary/55" />
      </div>
      <div className="text-center space-y-1">
        <p className="text-sm font-medium text-muted-foreground">
          {emptyTitle || t('workflow.executionEmptyHint')}
        </p>
        <p className="mx-auto max-w-[280px] text-balance text-xs leading-5 text-muted-foreground">
          {emptyDescription || t('workflow.runTip')}
        </p>
      </div>
    </div>
  );
};

export default ExecutionTab;

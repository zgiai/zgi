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
  className?: string;
}

/**
 * ExecutionTab - shows the execution list content
 */
const ExecutionTab: React.FC<ExecutionTabProps> = ({ items, showDetail = true, className }) => {
  const t = useT('agents');
  return items.length > 0 ? (
    <div className={cn('flex-1 flex flex-col min-h-0 bg-background/40', className)}>
      <div className="px-3 py-2 border-b flex items-center justify-between bg-muted/30">
        <div className="flex items-center gap-1.5">
          <Workflow className="w-5 h-5 text-emerald-600" />
          <span className="text-sm font-semibold">{t('workflow.execution')}</span>
        </div>
        <div className="text-xs text-muted-foreground">
          {items.length} {t('workflow.steps', { count: items.length })}
        </div>
      </div>
      <div className="flex-1 overflow-auto p-4 scrollbar-thin">
        <WorkflowRunNodesList showDetail={showDetail} items={items} />
      </div>
    </div>
  ) : (
    <div className={cn('h-full flex flex-col items-center justify-center gap-4 py-12', className)}>
      <div className="w-16 h-16 rounded-2xl bg-gradient-to-br from-primary/5 to-primary/10 flex items-center justify-center animate-in zoom-in-75 duration-500">
        <Play className="w-8 h-8 text-primary/40" />
      </div>
      <div className="text-center space-y-1">
        <p className="font-medium text-muted-foreground/80">{t('workflow.executionEmptyHint')}</p>
        <p className="text-xs text-muted-foreground/60 max-w-[200px] mx-auto text-balance">
          {t('workflow.runTip')}
        </p>
      </div>
    </div>
  );
};

export default ExecutionTab;

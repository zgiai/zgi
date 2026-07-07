import { History, RotateCcw, X } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';

import WorkflowRunsDropdown from '../../workflow-runs-dropdown';

interface PanelHeaderProps {
  agentId: string;
  query: { triggered_from: 'debugging' };
  canViewRuntimeLogs: boolean;
  onSelectDebugRun: (runId: string) => void;
  onReset: () => void;
  onClose: () => void;
}

export function PanelHeader({
  agentId,
  query,
  canViewRuntimeLogs,
  onSelectDebugRun,
  onReset,
  onClose,
}: PanelHeaderProps) {
  const t = useT();

  return (
    <div className="flex items-center justify-between border-b border-border/50 px-3 py-2">
      <div className="font-medium">{t('agents.workflow.debugTitle')}</div>
      <div className="flex items-center gap-2">
        {canViewRuntimeLogs ? (
          <WorkflowRunsDropdown
            agentId={agentId}
            query={query}
            icon={<History size={14} />}
            tooltipLabel={t('agents.workflow.debugRuns')}
            dropdownLabel={t('agents.workflow.debugRuns')}
            triggerText={t('agents.workflow.debugRuns')}
            triggerVariant="outline"
            triggerSize="xs"
            triggerClassName="h-7"
            refreshOnOpen
            onSelect={onSelectDebugRun}
          />
        ) : null}
        <Button variant="ghost" isIcon onClick={onReset} aria-label={t('common.reset')}>
          <RotateCcw size={16} className="text-primary" />
        </Button>
        <Button variant="ghost" isIcon onClick={onClose} aria-label={t('common.close')}>
          <X size={16} className="text-primary" />
        </Button>
      </div>
    </div>
  );
}

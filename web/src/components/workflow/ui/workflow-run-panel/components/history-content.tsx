import React from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import ExecutionTab from '@/components/workflow/ui/workflow-run-panel/components/workflow-run-panel-execution';
import DetailsTab from '@/components/workflow/ui/workflow-run-panel/components/workflow-run-panel-details';
import type { WorkflowRunNodeListItem } from '@/components/workflow/ui/workflow-run-nodes-list';
import type { WorkflowFinishedData, HistoryResult } from '../types';
import Results from './results';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n';

interface HistoryContentProps {
  activeTab: 'details' | 'execution' | 'results' | 'inputs';
  setActiveTab: (t: 'details' | 'execution' | 'results' | 'inputs') => void;
  loading: boolean;
  summary: WorkflowFinishedData | null;
  items: WorkflowRunNodeListItem[];
  result: HistoryResult;
}

const HistoryContent: React.FC<HistoryContentProps> = ({
  activeTab,
  setActiveTab,
  loading,
  summary,
  items,
  result,
}) => {
  const t = useT();

  if (loading) {
    return (
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <Skeleton className="h-5 w-24" />
          <Skeleton className="h-6 w-20" />
        </div>
        <div className="grid grid-cols-2 gap-2">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full col-span-2" />
        </div>
        <Skeleton className="h-6 w-20" />
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-6 w-20" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  return (
    <Tabs
      value={activeTab}
      onValueChange={v => setActiveTab(v as 'details' | 'execution' | 'results')}
      className="h-full flex flex-col"
    >
      <div className="px-4 pt-4 shrink-0">
        <TabsList className="w-full">
          <TabsTrigger className="flex-1" value="details">
            {t('agents.workflow.runDetails')}
          </TabsTrigger>
          <TabsTrigger className="flex-1" value="execution">
            {t('agents.workflow.execution')}
          </TabsTrigger>
          <TabsTrigger className="flex-1" value="results">
            {t('agents.workflow.results')}
          </TabsTrigger>
        </TabsList>
      </div>

      <TabsContent
        value="details"
        className="h-0 grow min-h-0 overflow-y-auto px-4 pb-4 mt-3 outline-none"
      >
        <DetailsTab runSummary={summary} />
      </TabsContent>

      <TabsContent
        value="execution"
        className="h-0 grow min-h-0 overflow-hidden px-4 pb-4 mt-3 outline-none data-[state=active]:flex data-[state=active]:flex-col"
      >
        <ExecutionTab items={items} className="h-full" />
      </TabsContent>

      <TabsContent
        value="results"
        className="h-0 grow min-h-0 overflow-hidden px-4 pb-4 mt-3 outline-none data-[state=active]:flex data-[state=active]:flex-col"
      >
        <Results
          mode="history"
          historyResult={result}
          emptyText={t('agents.workflow.noOutputYet')}
        />
      </TabsContent>
    </Tabs>
  );
};

export default HistoryContent;

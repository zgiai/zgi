import React from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import ExecutionTab from '@/components/workflow/ui/workflow-run-panel/components/workflow-run-panel-execution';
import DetailsTab from '@/components/workflow/ui/workflow-run-panel/components/workflow-run-panel-details';
import type { WorkflowRunNodeListItem } from '@/components/workflow/ui/workflow-run-nodes-list';
import type { WorkflowFinishedData, HistoryResult } from '../types';
import Results from './results';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';

type VisibleHistoryTab = 'details' | 'execution' | 'results';

interface HistoryContentProps {
  activeTab: 'details' | 'execution' | 'results' | 'inputs';
  setActiveTab: (t: 'details' | 'execution' | 'results' | 'inputs') => void;
  loading: boolean;
  summary: WorkflowFinishedData | null;
  items: WorkflowRunNodeListItem[];
  result: HistoryResult;
  navigationVariant?: 'default' | 'compact';
  visibleTabs?: VisibleHistoryTab[];
}

const HistoryContent: React.FC<HistoryContentProps> = ({
  activeTab,
  setActiveTab,
  loading,
  summary,
  items,
  result,
  navigationVariant = 'default',
  visibleTabs = ['details', 'execution', 'results'],
}) => {
  const t = useT();
  const availableTabs: VisibleHistoryTab[] = visibleTabs.length > 0 ? visibleTabs : ['execution'];
  const effectiveTab = availableTabs.includes(activeTab as VisibleHistoryTab)
    ? (activeTab as VisibleHistoryTab)
    : availableTabs[0];
  const showNavigation = availableTabs.length > 1;
  const isCompact = navigationVariant === 'compact';

  if (loading) {
    return (
      <div className="space-y-3 p-4">
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
      value={effectiveTab}
      onValueChange={v => setActiveTab(v as 'details' | 'execution' | 'results')}
      className="h-full flex flex-col"
    >
      {showNavigation ? (
        <div className={cn('shrink-0 px-4', isCompact ? 'border-b py-2.5' : 'pt-4')}>
          <TabsList className={cn(isCompact ? 'h-8 w-fit' : 'w-full')}>
            {availableTabs.includes('details') ? (
              <TabsTrigger
                className={cn(isCompact ? 'h-7 px-3 text-xs' : 'flex-1')}
                value="details"
              >
                {t('agents.workflow.runOverview')}
              </TabsTrigger>
            ) : null}
            {availableTabs.includes('execution') ? (
              <TabsTrigger
                className={cn(isCompact ? 'h-7 px-3 text-xs' : 'flex-1')}
                value="execution"
              >
                {t('agents.workflow.nodeDetails')}
              </TabsTrigger>
            ) : null}
            {availableTabs.includes('results') ? (
              <TabsTrigger
                className={cn(isCompact ? 'h-7 px-3 text-xs' : 'flex-1')}
                value="results"
              >
                {t('agents.workflow.finalResult')}
              </TabsTrigger>
            ) : null}
          </TabsList>
        </div>
      ) : null}

      {availableTabs.includes('details') ? (
        <TabsContent
          value="details"
          className="h-0 grow min-h-0 overflow-y-auto px-4 pb-4 mt-3 outline-none"
        >
          <DetailsTab runSummary={summary} />
        </TabsContent>
      ) : null}

      {availableTabs.includes('execution') ? (
        <TabsContent
          value="execution"
          className="h-0 grow min-h-0 overflow-hidden px-4 pb-4 mt-3 outline-none data-[state=active]:flex data-[state=active]:flex-col"
        >
          <ExecutionTab
            items={items}
            showHeader={!isCompact || !showNavigation}
            className="h-full"
          />
        </TabsContent>
      ) : null}

      {availableTabs.includes('results') ? (
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
      ) : null}
    </Tabs>
  );
};

export default HistoryContent;

'use client';

import React, { useMemo, useState, useCallback, useEffect, useRef } from 'react';
import { useT } from '@/i18n';
import {
  Search,
  Trash2,
  Upload,
  ChevronDown,
  Play,
  CheckCircle,
  Clock,
  XCircle,
  Info,
  Pause,
} from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { useParams, useSearchParams } from 'next/navigation';
import { useRandomQuestions } from '@/hooks/dataset/use-document-detail';
import { useDataset } from '@/hooks/dataset/use-datasets';
import { datasetService } from '@/services';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { QuestionImportDialog } from '@/components/datasets/question-import-dialog';
import BatchResultPanel from '@/components/datasets/batch-testing/components/result-panel';
import SaveRecordDialog from '@/components/datasets/batch-testing/components/save-record-dialog';
import { toast } from 'sonner';
import { useSaveBatchHitTestingRecord } from '@/hooks/dataset/use-batch-hit-testing';
import type { BatchTestData, ResultElement } from './type';
import { withBasePath } from '@/lib/config';
import { normalizeDatasetSearchMethod } from '@/utils/dataset/retrieval-config';
interface BatchTestingProps {
  onSearch?: (keyword: string) => void;
}

export default function BatchTesting(props: BatchTestingProps) {
  const { onSearch } = props;
  const t = useT('datasets');
  const params = useParams<{ datasetId: string }>();
  const searchParams = useSearchParams();
  const taskId = searchParams.get('taskId');
  const datasetId = (params?.datasetId as string) || undefined;
  const { data: datasetData } = useDataset(datasetId);
  const dataset = datasetData?.data;
  // Check if dataset supports QA mode (qa_model doc_form)
  const isPreQaExtension = dataset?.doc_form === 'qa_model';
  const [sampleLimit, setSampleLimit] = useState<number>(10);
  const [importOpen, setImportOpen] = useState(false);
  const [importedQuestions, setImportedQuestions] = useState<
    Array<{ question: string; id: string }>
  >([]);
  const [deletedRandomQuestions, setDeletedRandomQuestions] = useState<Set<string>>(new Set());
  const [isTesting, setIsTesting] = useState(false);
  const [testStatus, setTestStatus] = useState<Record<string, string>>({});
  const [currentTaskId, setCurrentTaskId] = useState<string | null>(null);
  const autoStartTriggeredRef = useRef(false);
  const [shouldAutoStart, setShouldAutoStart] = useState(false);

  const { data: randomQuestionsResp, isLoading: isRandomLoading } = useRandomQuestions(
    datasetId,
    sampleLimit,
    isPreQaExtension && !taskId // Only load random questions when not in retest mode
  );

  const [keyword, setKeyword] = useState('');
  const [activeQuestion, setActiveQuestion] = useState<string | null>(null);
  const [saveDialogOpen, setSaveDialogOpen] = useState(false);

  const [batchHitTestingData, setBatchHitTestingData] = useState<BatchTestData | null>(null);

  // Hook for saving batch test record
  const saveRecordMutation = useSaveBatchHitTestingRecord(datasetId || '', currentTaskId || '');
  const [isSearching, setIsSearching] = useState(false);

  // Parse CSV file content
  const parseCSV = useCallback((csvContent: string): Array<{ question: string; id: string }> => {
    const lines = csvContent.split('\n').filter(line => line.trim());
    if (lines.length === 0) return [];

    // Skip header if it exists
    const startIndex = lines[0].toLowerCase().includes('question') ? 1 : 0;
    const questions = lines.slice(startIndex).map((line, index) => ({
      question: line.trim(),
      id: `imported-${Date.now()}-${index}`,
    }));

    return questions.filter(q => q.question.length > 0);
  }, []);

  // Merge random questions with imported questions
  const allQuestions = useMemo(() => {
    // If in retest mode (taskId exists), only show imported questions from that task
    if (taskId) {
      return importedQuestions;
    }

    const randomQuestions =
      randomQuestionsResp?.data?.data
        ?.map((q: { question: string }, idx: number) => ({
          question: q.question,
          id: `random-${idx}`,
        }))
        .filter((q: { question: string; id: string }) => !deletedRandomQuestions.has(q.question)) ||
      [];

    return [...randomQuestions, ...importedQuestions];
  }, [taskId, randomQuestionsResp?.data?.data, importedQuestions, deletedRandomQuestions]);

  const activeResultData = useMemo(() => {
    return batchHitTestingData?.results?.find(
      (result: { query: string }) => result.query === activeQuestion
    );
  }, [activeQuestion, batchHitTestingData]);
  // Filter questions based on search keyword
  const filteredQuestions = useMemo(() => {
    if (!keyword.trim()) return allQuestions;

    const searchTerm = keyword.toLowerCase();
    return allQuestions.filter(q => q.question.toLowerCase().includes(searchTerm));
  }, [allQuestions, keyword]);

  const handleSearchSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      onSearch?.(keyword.trim());
    },
    [keyword, onSearch]
  );

  // Polling function for batch test status
  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const startPolling = useCallback(
    (taskId: string) => {
      if (!datasetId) return;

      if (pollIntervalRef.current) clearInterval(pollIntervalRef.current);

      const pollInterval = setInterval(async () => {
        try {
          const response = await datasetService.getBatchHitTestingStatus(datasetId, taskId);
          const statusData = response.data as BatchTestData;
          // Persist raw response for right panel rendering
          setBatchHitTestingData(response.data as BatchTestData);

          // Update status for all questions based on results array
          // Compute next status purely
          setTestStatus(prev => {
            const nextStatus = { ...prev };
            statusData.results.forEach(result => {
              nextStatus[result.query] = result.status;
            });
            return nextStatus;
          });

          // Perform side effects based on latest statusData outside setState
          type StatusValue = ResultElement['status'];
          type StatusMap = Record<string, StatusValue>;
          const nextStatus = statusData.results.reduce<StatusMap>((acc, r) => {
            acc[r.query] = r.status as StatusValue;
            return acc;
          }, {} as StatusMap);

          // Auto-select first completed if none selected and any completed
          if (!activeQuestion) {
            const firstCompleted = filteredQuestions.find(
              q => nextStatus[q.question] === 'completed'
            );
            if (firstCompleted) {
              setActiveQuestion(firstCompleted.question);
              setIsSearching(false);
            }
          }

          // If all filtered questions completed, stop polling and notify
          const allCompleted =
            filteredQuestions.length > 0 &&
            filteredQuestions.every(q => nextStatus[q.question] === 'completed');
          if (allCompleted) {
            clearInterval(pollInterval);
            pollIntervalRef.current = null;
            setIsTesting(false);
            toast.success(t('hitTesting.batchTestCompleted'), {
              description: t('hitTesting.allQuestionsCompleted'),
            });
          }
        } catch (error) {
          console.error('Failed to get batch test status:', error);
          clearInterval(pollInterval);
          pollIntervalRef.current = null;
          toast.error(t('hitTesting.fetchStatusFailed'), {
            description: t('hitTesting.checkResultsManually'),
          });
        }
      }, 2000); // Poll every 2 seconds

      pollIntervalRef.current = pollInterval;

      // Cleanup function to clear interval when component unmounts or task changes
      return () => {
        clearInterval(pollInterval);
        pollIntervalRef.current = null;
      };
    },
    [datasetId, filteredQuestions, toast, activeQuestion]
  );

  // Delete single question
  const handleDeleteQuestion = useCallback((questionId: string, questionText: string) => {
    if (questionId.startsWith('imported-')) {
      // Delete imported question
      setImportedQuestions(prev => prev.filter(q => q.id !== questionId));
    } else {
      // Delete random question by adding to deleted set
      setDeletedRandomQuestions(prev => new Set(prev).add(questionText));
    }
  }, []);

  // Batch hit testing function
  const handleBatchTesting = useCallback(async () => {
    if (filteredQuestions.length === 0) {
      toast.warning(t('hitTesting.addQuestionsFirst'));
      return;
    }

    if (!datasetId || !dataset?.retrieval_config) {
      toast.error(t('hitTesting.datasetInfoIncomplete'));
      return;
    }

    setIsTesting(true);
    setIsSearching(true);
    try {
      const retrievalConfig = dataset.retrieval_config;
      const normalizedSearchMethod = normalizeDatasetSearchMethod(
        retrievalConfig.search_method,
        Boolean(dataset.enable_graph_flow)
      );
      const response = await datasetService.asyncBatchHitTesting(datasetId, {
        dataset_ids: [datasetId],
        queries: filteredQuestions.map(item => item.question),
        retrieval_model: {
          search_method: normalizedSearchMethod,
          reranking_enable: retrievalConfig.reranking_enable,
          reranking_model: retrievalConfig.reranking_model,
          top_k: retrievalConfig.top_k,
          score_threshold_enabled: retrievalConfig.score_threshold_enabled,
          score_threshold: retrievalConfig.score_threshold,
        },
      });

      toast.success(t('hitTesting.batchTestStarted'), {
        description: t('hitTesting.taskIdInfo', { taskId: response.data.task_id }),
      });

      // Start polling for test status
      setCurrentTaskId(response.data.task_id);
      startPolling(response.data.task_id);
    } catch (error) {
      console.error('Batch testing failed:', error);
      setIsTesting(false);
      toast.error(t('hitTesting.batchTestFailed'), {
        description: t('hitTesting.checkNetworkRetry'),
      });
    }
  }, [
    filteredQuestions,
    datasetId,
    dataset?.enable_graph_flow,
    dataset?.retrieval_config,
    toast,
    startPolling,
    t,
  ]);

  // Get status icon for a question
  const getStatusIcon = useCallback(
    (question: string) => {
      const status = testStatus[question];

      switch (status) {
        case 'completed':
          return <CheckCircle className="h-4 w-4 text-green-500" />;
        case 'processing':
        case 'running':
          return <Clock className="h-4 w-4 text-blue-500 animate-spin" />;
        case 'failed':
        case 'error':
          return <XCircle className="h-4 w-4 text-red-500" />;
        default:
          return <Clock className="h-4 w-4 text-gray-400" />;
      }
    },
    [testStatus]
  );

  // Calculate test progress
  const testProgress = useMemo(() => {
    if (!currentTaskId || filteredQuestions.length === 0) {
      return { completed: 0, total: 0, percentage: 0 };
    }

    const completed = filteredQuestions.filter(q => testStatus[q.question] === 'completed').length;
    const total = filteredQuestions.length;
    const percentage = total > 0 ? Math.round((completed / total) * 100) : 0;

    return { completed, total, percentage };
  }, [currentTaskId, filteredQuestions, testStatus]);

  // Handle view test results
  const handleViewResults = useCallback(() => {
    if (!currentTaskId || !datasetId) {
      toast.warning(t('hitTesting.tip'), {
        description: t('hitTesting.noTestResults'),
      });
      return;
    }
    window.location.href = withBasePath(
      `/console/dataset/${datasetId}/batch-testing/${currentTaskId}`
    );
  }, [currentTaskId, datasetId, toast, t]);

  // Handle save test record
  const handleSaveRecord = useCallback(() => {
    if (!currentTaskId) {
      toast.warning(t('hitTesting.tip'), {
        description: t('hitTesting.noTestRecords'),
      });
      return;
    }

    setSaveDialogOpen(true);
  }, [currentTaskId, toast, t]);

  // Handle save record with batch name
  const handleSaveRecordWithName = useCallback(
    (batchName: string) => {
      if (!datasetId || !currentTaskId) return;

      saveRecordMutation.mutate(
        { batch_name: batchName },
        {
          onSuccess: () => {
            toast.success(t('hitTesting.saveSuccess'), {
              description: t('hitTesting.recordSavedSuccessfully'),
            });
            setSaveDialogOpen(false);
          },
          onError: error => {
            console.error('Save record failed:', error);
            toast.error(t('hitTesting.saveFailed'), {
              description: t('hitTesting.retryLater'),
            });
          },
        }
      );
    },
    [datasetId, currentTaskId, saveRecordMutation, toast, t]
  );

  // Reset deleted random questions when sample limit changes
  useEffect(() => {
    setDeletedRandomQuestions(new Set());
  }, [sampleLimit]);

  // Cleanup polling when component unmounts
  useEffect(() => {
    return () => {
      // Cleanup is handled in startPolling function
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current);
        pollIntervalRef.current = null;
      }
    };
  }, []);

  // Handle retest from report page
  useEffect(() => {
    if (!taskId || !datasetId || autoStartTriggeredRef.current) return;

    const loadQuestionsAndStart = async () => {
      try {
        autoStartTriggeredRef.current = true;
        const response = await datasetService.getBatchHitTestingStatus(datasetId, taskId);
        const results = response.data?.results ?? [];

        if (results.length === 0) {
          toast.error(t('hitTesting.cannotLoadQuestions'), {
            description: t('hitTesting.historyRecordNotFound'),
          });
          return;
        }

        // Load questions from task results
        const questions = results.map((r: { query: string }, idx: number) => ({
          question: r.query,
          id: `retest-${Date.now()}-${idx}`,
        }));

        setImportedQuestions(questions);

        // Set flag to trigger auto start after questions are loaded
        setShouldAutoStart(true);
      } catch (error) {
        console.error('Failed to load retest questions:', error);
        toast.error(t('hitTesting.loadFailed'), {
          description: t('hitTesting.cannotLoadHistoryQuestions'),
        });
      }
    };

    loadQuestionsAndStart();
  }, [taskId, datasetId, toast, t]);

  // Auto start testing after questions are loaded
  useEffect(() => {
    if (
      shouldAutoStart &&
      filteredQuestions.length > 0 &&
      !isTesting &&
      dataset?.retrieval_config
    ) {
      setShouldAutoStart(false);
      handleBatchTesting();
    }
  }, [shouldAutoStart, filteredQuestions, isTesting, dataset, handleBatchTesting]);

  // Stop batch testing
  const handleStopTesting = useCallback(async () => {
    if (!datasetId || !currentTaskId) return;
    try {
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current);
        pollIntervalRef.current = null;
      }
      await datasetService.stopBatchHitTestingTask(datasetId, currentTaskId);
      toast.success(t('hitTesting.testingStopped'));
      setCurrentTaskId(null);
      setIsTesting(false);
    } catch (error) {
      console.error('Stop batch testing failed:', error);
      toast.error(t('hitTesting.stopFailed'));
    }
  }, [datasetId, currentTaskId, toast, t]);

  return (
    <div className="flex h-full flex-col">
      {/* Top toolbar */}
      <div className="w-full border-b p-4 flex items-center gap-2 justify-end">
        {!taskId && (
          <Button variant="outline" size="sm" onClick={() => setImportOpen(true)}>
            <Upload className="h-4 w-4 mr-1" /> {t('hitTesting.batchImportQuestions')}
          </Button>
        )}
        {isPreQaExtension && !taskId && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="secondary" size="sm">
                {t('hitTesting.randomSample')} <ChevronDown className="h-4 w-4 ml-1" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuItem
                onClick={() => setSampleLimit(10)}
                disabled={
                  !!(randomQuestionsResp?.data?.total && 10 > randomQuestionsResp.data.total)
                }
              >
                {t('hitTesting.randomSample10')}
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => setSampleLimit(30)}
                disabled={
                  !!(randomQuestionsResp?.data?.total && 30 > randomQuestionsResp.data.total)
                }
              >
                {t('hitTesting.randomSample30')}
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => setSampleLimit(50)}
                disabled={
                  !!(randomQuestionsResp?.data?.total && 50 > randomQuestionsResp.data.total)
                }
              >
                {t('hitTesting.randomSample50')}
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => setSampleLimit(100)}
                disabled={
                  !!(randomQuestionsResp?.data?.total && 100 > randomQuestionsResp.data.total)
                }
              >
                {t('hitTesting.randomSample100')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}

        {isTesting ? (
          <Button size="sm" onClick={handleStopTesting}>
            <Pause className="h-4 w-4 mr-1" /> {t('hitTesting.stopTest')}
          </Button>
        ) : (
          <Button
            size="sm"
            onClick={handleBatchTesting}
            disabled={isTesting || filteredQuestions.length === 0}
          >
            <Play className="h-4 w-4 mr-1" />
            {t('hitTesting.startTest')}
          </Button>
        )}
      </div>

      <div className="flex flex-1">
        {/* Left sidebar */}
        <div className="w-[30%] border-r h-full flex flex-col">
          {/* Search */}
          <div className="p-4">
            <form onSubmit={handleSearchSubmit} className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder={t('hitTesting.searchQuestions')}
                value={keyword}
                onChange={e => setKeyword(e.target.value)}
                className="pl-9"
              />
            </form>
          </div>
          <div className="px-4 pb-2 flex items-center gap-2">
            <span className="text-sm ">{t('hitTesting.batchDelete')}</span>

            <Tooltip>
              <TooltipTrigger asChild>
                <div className=" text-sm text-muted-foreground ">
                  <Info className="h-4 w-4" />
                </div>
              </TooltipTrigger>
              <TooltipContent>
                <p>{t('hitTesting.questionListDescription')}</p>
              </TooltipContent>
            </Tooltip>
          </div>
          {/* Tools */}
          <div className="px-4 space-y-4">
            {/* Random questions list */}
            <div className=" space-y-2">
              {isPreQaExtension && isRandomLoading && (
                <div className="px-2 text-xs text-muted-foreground">{t('common.loading')}</div>
              )}
              {!isRandomLoading && (
                <>
                  {filteredQuestions.length === 0 ? (
                    <div className="mt-24 px-2 text-xs text-muted-foreground leading-6">
                      {isPreQaExtension
                        ? keyword.trim()
                          ? t('hitTesting.noMatchingQuestions')
                          : t('hitTesting.noQuestions')
                        : t('hitTesting.noQuestionsGenerated')}
                    </div>
                  ) : (
                    <div className="max-h-[calc(100vh-280px)] overflow-auto pr-2 space-y-2">
                      {filteredQuestions.map(q => (
                        <div
                          key={q.id}
                          className="flex items-center gap-2 text-sm text-foreground border rounded-md p-2 cursor-pointer hover:bg-muted/50"
                          onClick={() => {
                            if (testStatus[q.question] === 'completed') {
                              setActiveQuestion(q.question);
                            }
                          }}
                        >
                          <div className="flex items-center gap-2 flex-1">
                            <span className="flex-none">
                              {currentTaskId && getStatusIcon(q.question)}
                            </span>
                            <div className="leading-5 line-clamp-2">{q.question}</div>
                          </div>
                          <div className="flex items-center gap-2">
                            {q.id.startsWith('imported-') && (
                              <span className="text-xs text-blue-600 bg-blue-50 px-1.5 py-0.5 rounded">
                                {t('hitTesting.import')}
                              </span>
                            )}
                            <Button
                              variant="ghost"
                              isIcon
                              onClick={() => handleDeleteQuestion(q.id, q.question)}
                              className="h-6 w-6 p-0 hover:bg-destructive/10 hover:text-destructive flex-none"
                            >
                              <Trash2 className="h-3 w-3" />
                            </Button>
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                </>
              )}
            </div>
          </div>
        </div>
        {/* Right content */}

        <BatchResultPanel
          query={activeQuestion}
          resultData={activeResultData}
          isSearching={isSearching}
        />
      </div>

      {/* Progress Footer */}
      {currentTaskId && (
        <div className="border-t bg-white px-4 py-3 flex items-center justify-between">
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <div className="w-32 h-2 bg-gray-200 rounded-full overflow-hidden">
                <div
                  className="h-full bg-blue-500 transition-all duration-300"
                  style={{ width: `${testProgress.percentage}%` }}
                />
              </div>
              <span className="text-sm text-muted-foreground">
                {t('hitTesting.completed')} {testProgress.completed}/{testProgress.total}
              </span>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={handleViewResults}
              disabled={testProgress.completed === 0}
            >
              {t('hitTesting.viewTestResults')}
            </Button>
            <Button size="sm" onClick={handleSaveRecord} disabled={testProgress.completed === 0}>
              {t('hitTesting.saveTestRecord')}
            </Button>
          </div>
        </div>
      )}

      {/* Question Import Dialog */}
      <QuestionImportDialog
        open={importOpen}
        onClose={() => setImportOpen(false)}
        onImport={async (file: File) => {
          try {
            const csvContent = await file.text();
            const questions = parseCSV(csvContent);

            if (questions.length === 0) {
              toast.error(t('hitTesting.noValidQuestionsInCsv'));
              return;
            }

            setImportedQuestions(prev => [...prev, ...questions]);
            toast.success(t('hitTesting.importSuccess'), {
              description: t('hitTesting.importedQuestionsCount', { count: questions.length }),
            });
            setImportOpen(false);
          } catch (error) {
            console.error('Failed to parse CSV file:', error);
            toast.error(t('hitTesting.fileParsingFailed'));
          }
        }}
      />

      {/* Save Record Dialog */}
      <SaveRecordDialog
        open={saveDialogOpen}
        onOpenChange={setSaveDialogOpen}
        onSave={handleSaveRecordWithName}
        isLoading={saveRecordMutation.isPending}
      />
    </div>
  );
}

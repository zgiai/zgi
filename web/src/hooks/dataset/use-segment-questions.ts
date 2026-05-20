'use client';

import { useState, useCallback } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { datasetService } from '@/services';
import type { SegmentQuestion } from '@/services/types/dataset';
import { DATASET_KEYS } from '@/hooks/query-keys';

interface UseSegmentQuestionsProps {
  datasetId: string;
  documentId: string;
}

/**
 * Hook for managing segment questions
 * Handles question CRUD, generation, and batch import
 */
export function useSegmentQuestions({ datasetId, documentId }: UseSegmentQuestionsProps) {
  const t = useT('datasets');
  const queryClient = useQueryClient();

  // Questions state - stored per segment
  const [questionsData, setQuestionsData] = useState<Record<string, SegmentQuestion[]>>({});
  const [questionsLoading, setQuestionsLoading] = useState<Record<string, boolean>>({});

  // Fetch questions for a specific segment using React Query
  const handleFetchQuestions = useCallback(
    async (segmentId: string) => {
      setQuestionsLoading(prev => ({ ...prev, [segmentId]: true }));
      try {
        const response = await queryClient.fetchQuery({
          queryKey: DATASET_KEYS.segmentQuestions(datasetId, documentId, segmentId),
          queryFn: () =>
            datasetService.getQuestions(datasetId, documentId, segmentId, {
              page: 1,
              limit: 20,
            }),
          staleTime: 30000, // 30 seconds
        });
        const normalized: SegmentQuestion[] = (response?.data.data || [])
          .filter(q => q && q.id && q.question)
          .map(q => ({ id: q.id as string, question: q.question as string }));
        setQuestionsData(prev => ({ ...prev, [segmentId]: normalized }));
      } catch (error) {
        console.error('Failed to fetch questions:', error);
      } finally {
        setQuestionsLoading(prev => ({ ...prev, [segmentId]: false }));
      }
    },
    [queryClient, datasetId, documentId]
  );

  // Add question mutation
  const addQuestionMutation = useMutation({
    mutationFn: ({ segmentId, question }: { segmentId: string; question: string }) =>
      datasetService.addQuestion(datasetId, documentId, segmentId, { question }),
    onSuccess: (_data, variables) => {
      toast.success(t('questions.addSuccess'));
      queryClient.invalidateQueries({
        queryKey: DATASET_KEYS.segmentQuestions(datasetId, documentId, variables.segmentId),
      });
      handleFetchQuestions(variables.segmentId);
    },
    onError: () => {
      toast.error(t('questions.addFail'));
    },
  });

  // Update question mutation
  const updateQuestionMutation = useMutation({
    mutationFn: ({
      segmentId,
      questionId,
      question,
    }: {
      segmentId: string;
      questionId: string;
      question: string;
    }) => datasetService.updateQuestion(datasetId, documentId, segmentId, questionId, { question }),
    onSuccess: (_data, variables) => {
      toast.success(t('questions.updateSuccess'));
      queryClient.invalidateQueries({
        queryKey: DATASET_KEYS.segmentQuestions(datasetId, documentId, variables.segmentId),
      });
      handleFetchQuestions(variables.segmentId);
    },
    onError: () => {
      toast.error(t('questions.updateFail'));
    },
  });

  // Delete question mutation
  const deleteQuestionMutation = useMutation({
    mutationFn: ({ segmentId, questionId }: { segmentId: string; questionId: string }) =>
      datasetService.deleteQuestion(datasetId, documentId, segmentId, questionId),
    onSuccess: (_data, variables) => {
      toast.success(t('questions.deleteSuccess'));
      queryClient.invalidateQueries({
        queryKey: DATASET_KEYS.segmentQuestions(datasetId, documentId, variables.segmentId),
      });
      handleFetchQuestions(variables.segmentId);
    },
    onError: () => {
      toast.error(t('questions.deleteFail'));
    },
  });

  // Generate questions mutation
  const generateQuestionsMutation = useMutation({
    mutationFn: ({
      segmentId,
      model,
    }: {
      segmentId: string;
      model?: { provider: string; name: string };
    }) => datasetService.generateQuestions(datasetId, documentId, segmentId, model),
    onSuccess: (_data, variables) => {
      toast.success(t('questions.generateSuccess'));
      // Invalidate cache and refetch
      queryClient.invalidateQueries({
        queryKey: DATASET_KEYS.segmentQuestions(datasetId, documentId, variables.segmentId),
      });
      handleFetchQuestions(variables.segmentId);
    },
    onError: () => {
      toast.error(t('questions.generateFail'));
    },
  });

  // Batch import questions mutation
  const batchImportQuestionsMutation = useMutation({
    mutationFn: ({
      segmentId,
      questions,
    }: {
      segmentId: string;
      questions: Array<{ question: string }>;
    }) => datasetService.batchImportQuestions(datasetId, documentId, segmentId, { questions }),
    onSuccess: (_data, variables) => {
      toast.success(t('questions.importSuccess'));
      // Invalidate cache and refetch
      queryClient.invalidateQueries({
        queryKey: DATASET_KEYS.segmentQuestions(datasetId, documentId, variables.segmentId),
      });
      handleFetchQuestions(variables.segmentId);
    },
    onError: () => {
      toast.error(t('questions.importFail'));
    },
  });

  // Actions
  const addQuestion = useCallback(
    (segmentId: string, question: string) => {
      return addQuestionMutation.mutateAsync({ segmentId, question });
    },
    [addQuestionMutation]
  );

  const updateQuestion = useCallback(
    (segmentId: string, questionId: string, question: string) => {
      return updateQuestionMutation.mutateAsync({ segmentId, questionId, question });
    },
    [updateQuestionMutation]
  );

  const deleteQuestion = useCallback(
    (segmentId: string, questionId: string) => {
      return deleteQuestionMutation.mutateAsync({ segmentId, questionId });
    },
    [deleteQuestionMutation]
  );

  const generateQuestions = useCallback(
    (segmentId: string, model?: { provider: string; name: string }) => {
      return generateQuestionsMutation.mutateAsync({ segmentId, model });
    },
    [generateQuestionsMutation]
  );

  const batchImportQuestions = useCallback(
    (segmentId: string, questions: Array<{ question: string }>) => {
      return batchImportQuestionsMutation.mutateAsync({ segmentId, questions });
    },
    [batchImportQuestionsMutation]
  );

  return {
    // Data
    questionsData,
    questionsLoading,
    setQuestionsData,

    // Actions
    handleFetchQuestions,
    addQuestion,
    updateQuestion,
    deleteQuestion,
    generateQuestions,
    batchImportQuestions,

    // Loading states
    isAddingQuestion: addQuestionMutation.isPending,
    isUpdatingQuestion: updateQuestionMutation.isPending,
    isDeletingQuestion: deleteQuestionMutation.isPending,
    isGeneratingQuestions: generateQuestionsMutation.isPending,
    isImportingQuestions: batchImportQuestionsMutation.isPending,
  };
}

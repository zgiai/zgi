'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { workflowService } from '@/services/workflow.service';
import type { WorkflowExportVersion, WorkflowImportResult } from '@/services/types/workflow';
import type { ApiResponseData } from '@/services/types/common';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { AGENT_KEYS } from '@/hooks/query-keys';

interface ExportWorkflowParams {
  agentId: string;
  version?: WorkflowExportVersion;
  fileName?: string;
}

interface ImportWorkflowParams {
  file: File;
  workspaceId?: string;
}

function normalizeExportFileName(fileName: string): string {
  return fileName.toLowerCase().endsWith('.yml') || fileName.toLowerCase().endsWith('.yaml')
    ? fileName
    : `${fileName}.yml`;
}

function downloadWorkflowFile(blob: Blob, fileName: string): void {
  const url = window.URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = normalizeExportFileName(fileName);
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  window.URL.revokeObjectURL(url);
}

export function useExportWorkflow() {
  const t = useT('agents');

  const { mutateAsync, isPending } = useMutation<Blob, Error, ExportWorkflowParams>({
    mutationFn: ({ agentId, version = 'draft' }) => workflowService.exportWorkflow(agentId, version),
    onError: error => {
      const message = error.message || t('workflow.exportFailed');
      toast.error(message);
    },
    onSuccess: (blob, variables) => {
      const exportVersion = variables.version ?? 'draft';
      const fallbackFileName = `workflow-${variables.agentId}-${exportVersion}.yml`;
      downloadWorkflowFile(blob, variables.fileName ?? fallbackFileName);
      toast.success(t('workflow.exportSuccess'));
    },
  });

  return {
    exportWorkflow: mutateAsync,
    isExporting: isPending,
  };
}

export function useImportWorkflow() {
  const t = useT('agents');
  const queryClient = useQueryClient();

  const { mutateAsync, isPending, data, reset } = useMutation<
    ApiResponseData<WorkflowImportResult>,
    Error,
    ImportWorkflowParams
  >({
    mutationFn: ({ file, workspaceId }) => workflowService.importWorkflow(file, workspaceId),
    onSuccess: response => {
      queryClient.invalidateQueries({ queryKey: AGENT_KEYS.lists() });
      const warningCount = Array.isArray(response.data.warnings) ? response.data.warnings.length : 0;
      if (warningCount > 0) {
        toast.success(t('workflow.importWithWarnings', { count: warningCount }));
        return;
      }
      toast.success(t('workflow.importSuccess'));
    },
    onError: error => {
      const message = error.message || t('workflow.importFailed');
      toast.error(message);
    },
  });

  return {
    importWorkflow: mutateAsync,
    isImporting: isPending,
    importResult: data?.data ?? null,
    resetImportState: reset,
  };
}

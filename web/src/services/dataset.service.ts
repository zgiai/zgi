import { BaseService } from '@/lib/http/services';
import type {
  Dataset,
  DatasetList,
  DocumentList,
  DocumentDetail,
  DocumentMetadataDetail,
  SegmentDetail,
  SegmentsResponse,
  CreateSegmentRequest,
  CreateSegmentResponse,
  UpdateSegmentRequest,
  ChildChunkDetail,
  ChildSegmentsResponse,
  CreateChildSegmentRequest,
  UpdateChildSegmentRequest,
  BatchImportResponse,
  BatchImportStatusResponse,
  HitTestingResponse,
  HitTestingRequest,
  ExternalHitTestingRequest,
  ExternalDatasetHitTestingResponse,
  HitTestingHistoryResponse,
  ProcessRuleResponse,
  DocumentIndexingStatusResponse,
  DocumentListParams,
  QuestionsResponse,
  ErrorDocsResponse,
  RandomQuestionsResponse,
  BatchHitTestingRequest,
  BatchHitTestingResponse,
  DatasetGraph,
  CreateDatasetRequest,
  UpdateDatasetRequest,
  DocumentExtractionStrategiesResponse,
  DocumentExtractionStrategy,
  DatasetFileCandidateFilter,
  DatasetFileCandidateEmbeddingResult,
  DatasetFileCandidateList,
  DatasetFileRefCreateResult,
  DatasetFileRefList,
  DatasetFileRefView,
} from './types/dataset';
import type {
  AgentBindingMutationConfirmation,
  AgentResourceBoundImpact,
  ApiResponseData,
} from './types/common';
import type { FileProcessingRequestView } from './types/file';
import type {
  BatchHitTestingReportResponse,
  BatchHitTestingStatusResponse,
} from '@/components/datasets/batch-testing/type';

/**
 * DatasetService
 * ---------------------------------------------------------------------------
 * Handles all knowledge base (dataset) related APIs.
 */
class DatasetService extends BaseService {
  constructor() {
    super({
      endpoint: 'main',
      basePath: '/console/api',
    });
  }

  /* ------------------------------------------------------------------------ */
  /* Dataset list & detail                                                    */
  /* ------------------------------------------------------------------------ */

  /**
   * Get dataset list (workspace scope)
   * GET /console/api/datasets
   */
  getDatasets(params?: {
    page?: number;
    limit?: number;
    ids?: string[];
    keyword?: string;
    tag_ids?: string[];
    include_all?: boolean;
    workspace_id?: string;
  }): Promise<ApiResponseData<DatasetList>> {
    return this.request('get', '/datasets', undefined, { params });
  }

  /**
   * Create dataset
   * POST /console/api/datasets
   */
  createDataset(data: CreateDatasetRequest): Promise<ApiResponseData<Dataset>> {
    return this.request('post', '/datasets', data);
  }

  /**
   * Get dataset detail
   * GET /console/api/datasets/{dataset_id}
   */
  getDataset(datasetId: string): Promise<ApiResponseData<Dataset>> {
    return this.request('get', `/datasets/${datasetId}`);
  }

  /**
   * Update dataset
   * PATCH /console/api/datasets/{dataset_id}
   */
  updateDataset(datasetId: string, data: UpdateDatasetRequest): Promise<ApiResponseData<Dataset>> {
    return this.request('patch', `/datasets/${datasetId}`, data);
  }

  /**
   * Delete dataset
   * DELETE /console/api/datasets/{dataset_id}
   */
  deleteDataset(
    datasetId: string,
    confirmation?: AgentBindingMutationConfirmation
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request('delete', `/datasets/${datasetId}`, undefined, {
      params: confirmation,
    });
  }

  previewDatasetDeleteImpact(
    datasetId: string
  ): Promise<ApiResponseData<AgentResourceBoundImpact | null>> {
    return this.request('get', `/datasets/${datasetId}/delete-impact`);
  }

  /* ------------------------------------------------------------------------ */
  /* Document operations                                                      */
  /* ------------------------------------------------------------------------ */

  /**
   * Get document list in a dataset
   * GET /console/api/datasets/{dataset_id}/documents
   */
  getDocuments(
    datasetId: string,
    params?: DocumentListParams
  ): Promise<ApiResponseData<DocumentList>> {
    return this.request('get', `/datasets/${datasetId}/documents`, undefined, { params });
  }

  /**
   * Upload document to an existing dataset
   */
  createDocumentsInDataset(
    datasetId: string,
    data: {
      type: string;
      file_ids: string[];
      extraction_strategy?: DocumentExtractionStrategy;
      extraction_fallback_enabled?: boolean;
    }
  ): Promise<ApiResponseData<unknown>> {
    return this.request('post', `/datasets/${datasetId}/documents`, data);
  }

  /**
   * List file assets that can be copied into a dataset.
   * GET /console/api/datasets/{dataset_id}/file-candidates
   */
  getDatasetFileCandidates(
    datasetId: string,
    params?: {
      filter?: DatasetFileCandidateFilter;
      keyword?: string;
      page?: number;
      limit?: number;
    }
  ): Promise<ApiResponseData<DatasetFileCandidateList>> {
    return this.request('get', `/datasets/${datasetId}/file-candidates`, undefined, { params });
  }

  /**
   * List file asset sync refs for a dataset.
   * GET /console/api/datasets/{dataset_id}/file-refs
   */
  getDatasetFileRefs(
    datasetId: string,
    params?: {
      sync_status?: string;
      page?: number;
      limit?: number;
    }
  ): Promise<ApiResponseData<DatasetFileRefList>> {
    return this.request('get', `/datasets/${datasetId}/file-refs`, undefined, { params });
  }

  /**
   * Copy selected file assets into a dataset by creating sync refs.
   * POST /console/api/datasets/{dataset_id}/file-refs
   */
  createDatasetFileRefs(
    datasetId: string,
    assetIds: string[]
  ): Promise<ApiResponseData<DatasetFileRefCreateResult>> {
    return this.request('post', `/datasets/${datasetId}/file-refs`, { asset_ids: assetIds });
  }

  /**
   * Generate file asset embeddings for the current dataset embedding model.
   * POST /console/api/datasets/{dataset_id}/file-candidates/{asset_id}/embeddings
   */
  generateDatasetFileCandidateEmbeddings(
    datasetId: string,
    assetId: string
  ): Promise<ApiResponseData<DatasetFileCandidateEmbeddingResult>> {
    return this.request('post', `/datasets/${datasetId}/file-candidates/${assetId}/embeddings`);
  }

  /**
   * Get a candidate embedding generation task.
   * GET /console/api/datasets/{dataset_id}/file-candidates/{asset_id}/embedding-tasks/{request_id}
   */
  getDatasetFileCandidateEmbeddingTask(
    datasetId: string,
    assetId: string,
    requestId: string
  ): Promise<ApiResponseData<FileProcessingRequestView>> {
    return this.request(
      'get',
      `/datasets/${datasetId}/file-candidates/${assetId}/embedding-tasks/${requestId}`
    );
  }

  /**
   * Retry a failed or pending file asset sync ref.
   * POST /console/api/datasets/{dataset_id}/file-refs/{ref_id}/sync/retry
   */
  retryDatasetFileRefSync(
    datasetId: string,
    refId: string
  ): Promise<ApiResponseData<DatasetFileRefCreateResult['items'][number]>> {
    return this.request('post', `/datasets/${datasetId}/file-refs/${refId}/sync/retry`);
  }

  /**
   * Remove a file asset ref and its current dataset document.
   * DELETE /console/api/datasets/{dataset_id}/file-refs/{ref_id}
   */
  deleteDatasetFileRef(
    datasetId: string,
    refId: string
  ): Promise<ApiResponseData<DatasetFileRefView>> {
    return this.request('delete', `/datasets/${datasetId}/file-refs/${refId}`);
  }

  /**
   * Get currently available document extraction strategies.
   */
  getDocumentExtractionStrategies(): Promise<
    ApiResponseData<DocumentExtractionStrategiesResponse>
  > {
    return this.request('get', '/datasets/extraction-strategies');
  }

  /**
   * Get document detail
   */
  getDocumentDetail(
    datasetId: string,
    documentId: string,
    metadata: 'without' | 'only' = 'without'
  ): Promise<ApiResponseData<DocumentDetail>> {
    return this.request('get', `/datasets/${datasetId}/documents/${documentId}`, undefined, {
      params: { metadata },
    });
  }

  /**
   * Get document metadata only
   */
  getDocumentMetadata(
    datasetId: string,
    documentId: string
  ): Promise<ApiResponseData<DocumentMetadataDetail>> {
    return this.request('get', `/datasets/${datasetId}/documents/${documentId}`, undefined, {
      params: { metadata: 'only' },
    });
  }

  /* ------------------------------------------------------------------------ */
  /* Document state management                                                */
  /* ------------------------------------------------------------------------ */

  /**
   * Batch enable documents
   */
  batchEnableDocuments(
    datasetId: string,
    documentIds: string[]
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    const params = new URLSearchParams();
    documentIds.forEach(id => params.append('document_id', id));
    return this.request('patch', `/datasets/${datasetId}/documents/status/enable/batch?${params}`);
  }

  /**
   * Batch disable documents
   */
  batchDisableDocuments(
    datasetId: string,
    documentIds: string[]
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    const params = new URLSearchParams();
    documentIds.forEach(id => params.append('document_id', id));
    return this.request('patch', `/datasets/${datasetId}/documents/status/disable/batch?${params}`);
  }

  /**
   * Archive documents
   */
  archiveDocuments(
    datasetId: string,
    documentIds: string[]
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    const params = new URLSearchParams();
    documentIds.forEach(id => params.append('document_id', id));
    return this.request('patch', `/datasets/${datasetId}/documents/status/archive/batch?${params}`);
  }

  /**
   * Unarchive documents
   */
  unarchiveDocuments(
    datasetId: string,
    documentIds: string[]
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    const params = new URLSearchParams();
    documentIds.forEach(id => params.append('document_id', id));
    return this.request(
      'patch',
      `/datasets/${datasetId}/documents/status/un_archive/batch?${params}`
    );
  }

  /**
   * Delete documents
   */
  deleteDocuments(
    datasetId: string,
    documentId: string
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request('delete', `/datasets/${datasetId}/documents/${documentId}`);
  }

  /**
   * Batch delete documents
   */
  batchDeleteDocuments(
    datasetId: string,
    documentIds: string[]
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    const params = new URLSearchParams();
    documentIds.forEach(id => params.append('document_id', id));
    return this.request('delete', `/datasets/${datasetId}/documents?${params.toString()}`);
  }

  /* ------------------------------------------------------------------------ */
  /* Segment management                                                       */
  /* ------------------------------------------------------------------------ */

  /**
   * Get segments list
   */
  getSegments(
    datasetId: string,
    documentId: string,
    params: {
      page: number;
      limit: number;
      keyword?: string;
      enabled?: boolean | 'all';
    }
  ): Promise<ApiResponseData<SegmentsResponse>> {
    return this.request(
      'get',
      `/datasets/${datasetId}/documents/${documentId}/segments`,
      undefined,
      {
        params,
      }
    );
  }

  /**
   * Create segment
   */
  createSegment(
    datasetId: string,
    documentId: string,
    data: CreateSegmentRequest
  ): Promise<ApiResponseData<CreateSegmentResponse>> {
    return this.request('post', `/datasets/${datasetId}/documents/${documentId}/segments`, data);
  }

  /**
   * Update segment
   */
  updateSegment(
    datasetId: string,
    documentId: string,
    segmentId: string,
    data: UpdateSegmentRequest
  ): Promise<ApiResponseData<SegmentDetail>> {
    return this.request(
      'patch',
      `/datasets/${datasetId}/documents/${documentId}/segments/${segmentId}`,
      data
    );
  }

  /**
   * Batch enable segments
   */
  batchEnableSegments(
    datasetId: string,
    documentId: string,
    segmentIds: string[]
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    const params = new URLSearchParams();
    segmentIds.forEach(id => params.append('segment_id', id));
    return this.request(
      'patch',
      `/datasets/${datasetId}/documents/${documentId}/segment/enable?${params}`
    );
  }

  /**
   * Batch disable segments
   */
  batchDisableSegments(
    datasetId: string,
    documentId: string,
    segmentIds: string[]
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    const params = new URLSearchParams();
    segmentIds.forEach(id => params.append('segment_id', id));
    return this.request(
      'patch',
      `/datasets/${datasetId}/documents/${documentId}/segment/disable?${params}`
    );
  }

  /**
   * Batch delete segments
   */
  batchDeleteSegments(
    datasetId: string,
    documentId: string,
    segmentIds: string[]
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    const params = new URLSearchParams();
    segmentIds.forEach(id => params.append('segment_id', id));
    return this.request(
      'delete',
      `/datasets/${datasetId}/documents/${documentId}/segments?${params}`
    );
  }

  /* ------------------------------------------------------------------------ */
  /* Batch import                                                             */
  /* ------------------------------------------------------------------------ */

  /**
   * Batch import segments
   */
  batchImportSegments(
    datasetId: string,
    documentId: string,
    file: FormData
  ): Promise<ApiResponseData<BatchImportResponse>> {
    return this.request(
      'post',
      `/datasets/${datasetId}/documents/${documentId}/segments/batch_import`,
      file,
      {
        headers: {
          'Content-Type': 'multipart/form-data',
        },
      }
    );
  }

  /**
   * Get batch import status
   */
  getBatchImportStatus(jobId: string): Promise<ApiResponseData<BatchImportStatusResponse>> {
    return this.request('get', `/datasets/batch_import_status/${jobId}`);
  }

  /**
   * Delete single segment
   */
  deleteSegment(
    datasetId: string,
    documentId: string,
    segmentId: string
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request(
      'delete',
      `/datasets/${datasetId}/documents/${documentId}/segments/${segmentId}`
    );
  }

  /**
   * Enable single segment
   */
  enableSegment(
    datasetId: string,
    documentId: string,
    segmentId: string
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request(
      'patch',
      `/datasets/${datasetId}/documents/${documentId}/segments/${segmentId}/enable`
    );
  }

  /**
   * Disable single segment
   */
  disableSegment(
    datasetId: string,
    documentId: string,
    segmentId: string
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request(
      'patch',
      `/datasets/${datasetId}/documents/${documentId}/segments/${segmentId}/disable`
    );
  }

  /**
   * Get questions list for a segment
   */
  getQuestions(
    datasetId: string,
    documentId: string,
    segmentId: string,
    params: {
      page: number;
      limit: number;
    }
  ): Promise<ApiResponseData<QuestionsResponse>> {
    return this.request(
      'get',
      `/datasets/${datasetId}/documents/${documentId}/segments/${segmentId}/questions`,
      undefined,
      { params }
    );
  }

  /**
   * Add a question to a segment
   */
  addQuestion(
    datasetId: string,
    documentId: string,
    segmentId: string,
    data: {
      question: string;
    }
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request(
      'post',
      `/datasets/${datasetId}/documents/${documentId}/segments/${segmentId}/questions`,
      data
    );
  }

  /**
   * Update a question
   */
  updateQuestion(
    datasetId: string,
    documentId: string,
    segmentId: string,
    questionId: string,
    data: {
      question: string;
    }
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request(
      'put',
      `/datasets/${datasetId}/documents/${documentId}/segments/${segmentId}/questions/${questionId}`,
      data
    );
  }

  /**
   * Delete a question
   */
  deleteQuestion(
    datasetId: string,
    documentId: string,
    segmentId: string,
    questionId: string
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request(
      'delete',
      `/datasets/${datasetId}/documents/${documentId}/segments/${segmentId}/questions/${questionId}`
    );
  }

  /**
   * Batch generate questions for a segment
   */
  generateQuestions(
    datasetId: string,
    documentId: string,
    segmentId: string,
    model?: { provider: string; name: string }
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request(
      'post',
      `/datasets/${datasetId}/documents/${documentId}/segments/${segmentId}/questions/generate`,
      { model }
    );
  }

  /**
   * Batch import questions for a segment
   */
  batchImportQuestions(
    datasetId: string,
    documentId: string,
    segmentId: string,
    data: {
      questions: Array<{ question: string }>;
    }
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request(
      'post',
      `/datasets/${datasetId}/documents/${documentId}/segments/${segmentId}/questions/batch`,
      data
    );
  }

  /* ------------------------------------------------------------------------ */
  /* Child segments management                                                */
  /* ------------------------------------------------------------------------ */

  /**
   * Get child segments list
   */
  getChildSegments(
    datasetId: string,
    documentId: string,
    segmentId: string,
    params: {
      page: number;
      limit: number;
      keyword?: string;
    }
  ): Promise<ApiResponseData<ChildSegmentsResponse>> {
    return this.request(
      'get',
      `/datasets/${datasetId}/documents/${documentId}/segments/${segmentId}/child_chunks`,
      undefined,
      { params }
    );
  }

  /**
   * Create child segment
   */
  createChildSegment(
    datasetId: string,
    documentId: string,
    segmentId: string,
    data: CreateChildSegmentRequest
  ): Promise<ApiResponseData<ChildChunkDetail>> {
    return this.request(
      'post',
      `/datasets/${datasetId}/documents/${documentId}/segments/${segmentId}/child_chunks`,
      data
    );
  }

  /**
   * Update child segment
   */
  updateChildSegment(
    datasetId: string,
    documentId: string,
    segmentId: string,
    childChunkId: string,
    data: UpdateChildSegmentRequest
  ): Promise<ApiResponseData<ChildChunkDetail>> {
    return this.request(
      'patch',
      `/datasets/${datasetId}/documents/${documentId}/segments/${segmentId}/child_chunks/${childChunkId}`,
      data
    );
  }

  /**
   * Delete child segment
   */
  deleteChildSegment(
    datasetId: string,
    documentId: string,
    segmentId: string,
    childChunkId: string
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request(
      'delete',
      `/datasets/${datasetId}/documents/${documentId}/segments/${segmentId}/child_chunks/${childChunkId}`
    );
  }

  /* ------------------------------------------------------------------------ */
  /* Indexing & search                                                        */
  /* ------------------------------------------------------------------------ */

  /**
   * Estimate indexing cost
   */
  estimateIndexing(
    data: Record<string, unknown>
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request('post', '/datasets/indexing-estimate', data);
  }

  /**
   * Hit testing for a dataset
   */
  hitTesting(
    datasetId: string,
    data: HitTestingRequest
  ): Promise<ApiResponseData<HitTestingResponse>> {
    return this.request('post', `/datasets/${datasetId}/hit-testing`, data);
  }

  /**
   * External dataset hit testing
   */
  externalHitTesting(
    datasetId: string,
    data: ExternalHitTestingRequest
  ): Promise<ApiResponseData<ExternalDatasetHitTestingResponse>> {
    return this.request('post', `/datasets/${datasetId}/external-hit-testing`, data);
  }

  /**
   * Batch hit testing for a dataset
   */
  asyncBatchHitTesting(
    datasetId: string,
    data: BatchHitTestingRequest
  ): Promise<ApiResponseData<BatchHitTestingResponse>> {
    return this.request('post', `/datasets/${datasetId}/async-batch-hit-testing`, data);
  }

  /**
   * Get default process rule
   */
  getProcessRule(documentId?: string): Promise<ApiResponseData<ProcessRuleResponse>> {
    const url = documentId
      ? `/datasets/process-rule?document_id=${documentId}`
      : '/datasets/process-rule';
    return this.request('get', url);
  }

  /**
   * Get indexing status of a document
   */
  getIndexingStatusById(
    datasetId: string,
    documentId: string
  ): Promise<ApiResponseData<DocumentIndexingStatusResponse>> {
    return this.request('get', `/datasets/${datasetId}/documents/${documentId}/indexing-status`);
  }

  /**
   * Alias for getIndexingStatusById to match some hook usages
   */
  getDocumentIndexingStatus(
    datasetId: string,
    documentId: string
  ): Promise<ApiResponseData<DocumentIndexingStatusResponse>> {
    return this.getIndexingStatusById(datasetId, documentId);
  }

  /**
   * Get random questions for a dataset
   */
  getRandomQuestions(
    datasetId: string,
    limit: number = 3
  ): Promise<ApiResponseData<RandomQuestionsResponse>> {
    return this.request('get', `/datasets/${datasetId}/queries`, undefined, { params: { limit } });
  }

  /**
   * Get error documents in a dataset
   */
  getErrorDocs(datasetId: string): Promise<ApiResponseData<ErrorDocsResponse>> {
    return this.request('get', `/datasets/${datasetId}/error-docs`);
  }

  /**
   * Retry error documents
   */
  retryErrorDocs(
    datasetId: string,
    data: { document_ids: string[] }
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request('post', `/datasets/${datasetId}/error-docs/retry`, data);
  }

  /**
   * Get hit testing records (history)
   */
  getHitTestingRecords(
    datasetId: string,
    params: unknown
  ): Promise<ApiResponseData<HitTestingHistoryResponse>> {
    return this.request('get', `/datasets/${datasetId}/queries`, undefined, { params });
  }

  /**
   * Get batch hit testing status
   */
  getBatchHitTestingStatus(
    datasetId: string,
    taskId: string
  ): Promise<ApiResponseData<BatchHitTestingStatusResponse>> {
    return this.request('get', `/datasets/${datasetId}/batch-hit-testing/${taskId}`);
  }

  /**
   * Stop batch hit testing task
   */
  stopBatchHitTestingTask(
    datasetId: string,
    taskId: string
  ): Promise<ApiResponseData<Record<string, unknown>>> {
    return this.request('post', `/datasets/${datasetId}/batch-hit-testing/${taskId}/stop`, {});
  }

  /**
   * Save batch hit testing record
   */
  saveBatchHitTestingRecord(
    datasetId: string,
    taskId: string,
    data: unknown
  ): Promise<ApiResponseData<unknown>> {
    return this.request('post', `/datasets/${datasetId}/batch-hit-testing/${taskId}/save`, data);
  }

  /**
   * Get batch hit testing report
   */
  getBatchHitTestingReport(
    datasetId: string,
    taskId: string
  ): Promise<ApiResponseData<BatchHitTestingReportResponse>> {
    return this.request('get', `/datasets/${datasetId}/batch-hit-testing/${taskId}/report`);
  }

  /**
   * Get dataset graph data
   * GET /console/api/datasets/{dataset_id}/graph
   */
  getDatasetGraph(datasetId: string): Promise<ApiResponseData<DatasetGraph>> {
    return this.request('get', `/datasets/${datasetId}/graph`);
  }

  /**
   * Vector retrieval for dataset
   * POST /console/api/datasets/{dataset_id}/retrieve/vector
   */
  vectorRetrieval(
    datasetId: string,
    data: HitTestingRequest
  ): Promise<ApiResponseData<HitTestingResponse>> {
    return this.request('post', `/datasets/${datasetId}/retrieve/vector`, data);
  }

  /**
   * Graph retrieval for dataset
   * POST /console/api/datasets/{dataset_id}/retrieve/graph
   */
  graphRetrieval(
    datasetId: string,
    data: HitTestingRequest
  ): Promise<ApiResponseData<HitTestingResponse>> {
    return this.request('post', `/datasets/${datasetId}/retrieve/graph`, data);
  }
}

// Export singleton instance
export const datasetService = new DatasetService();
export default datasetService;

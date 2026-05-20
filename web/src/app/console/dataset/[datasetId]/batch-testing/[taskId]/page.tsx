'use client';

import React, { useCallback, useState } from 'react';
import { useParams } from 'next/navigation';
import { useT } from '@/i18n';
import {
  useBatchHitTestingReport,
  useBatchHitTestingStatus,
} from '@/hooks/dataset/use-batch-hit-testing';
import { Card } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { useDataset } from '@/hooks/dataset/use-datasets';
import { RetrievalDetailDialog } from '@/components/datasets/batch-testing/components/retrieval-detail-dialog';
import type { ResultElement } from '@/components/datasets/batch-testing/type';
import { withBasePath } from '@/lib/config';

export default function BatchTestingReportPage() {
  const t = useT();
  const { datasetId, taskId } = useParams<{ datasetId: string; taskId: string }>();
  const [selectedResult, setSelectedResult] = useState<ResultElement | null>(null);
  const [isDialogOpen, setIsDialogOpen] = useState(false);

  const { data: report, isLoading: isReportLoading } = useBatchHitTestingReport(datasetId, taskId);
  const { data: statusData, isLoading: isStatusLoading } = useBatchHitTestingStatus(
    datasetId,
    taskId
  );
  const { data: datasetData } = useDataset(datasetId);
  const dataset = datasetData?.data;

  const handleViewDetail = useCallback((result: ResultElement) => {
    setSelectedResult(result);
    setIsDialogOpen(true);
  }, []);

  const handleExport = useCallback(() => {
    const results = statusData?.results ?? [];

    // Create Excel-compatible HTML table
    const summaryRows = [
      [
        t('datasets.hitTesting.testQuestion'),
        t('datasets.hitTesting.status'),
        t('datasets.hitTesting.top1RecallMethod'),
        t('datasets.hitTesting.top1Similarity'),
        t('datasets.hitTesting.top1SourceFile'),
      ],
    ];

    const dataRows = results.map(r => {
      const top = r?.result?.records?.[0];
      const matchType = top?.match_type as 'original' | 'question' | undefined;
      const status = r.status as 'completed' | 'failed' | 'error' | 'pending' | 'processing';
      return [
        r.query,
        t(`datasets.hitTesting.batchStatus.${status}`) || t('datasets.common.notAvailable'),
        matchType
          ? t(`datasets.hitTesting.matchTypes.${matchType}`)
          : t('datasets.common.notAvailable'),
        top?.score != null ? top.score.toFixed(2) : t('datasets.common.notAvailable'),
        top?.segment?.document?.name ?? t('datasets.common.notAvailable'),
      ];
    });

    const allRows = [...summaryRows, ...dataRows];

    // Convert to HTML table for Excel
    const tableHtml = `
      <html xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:x="urn:schemas-microsoft-com:office:excel">
        <head>
          <meta charset="utf-8" />
          <!--[if gte mso 9]>
          <xml>
            <x:ExcelWorkbook>
              <x:ExcelWorksheets>
                <x:ExcelWorksheet>
                  <x:Name>${t('datasets.hitTesting.batchTestingReport')}</x:Name>
                  <x:WorksheetOptions><x:DisplayGridlines/></x:WorksheetOptions>
                </x:ExcelWorksheet>
              </x:ExcelWorksheets>
            </x:ExcelWorkbook>
          </xml>
          <![endif]-->
        </head>
        <body>
          <table border="1">
            ${allRows.map(row => `<tr>${row.map(cell => `<td>${cell}</td>`).join('')}</tr>`).join('')}
          </table>
        </body>
      </html>
    `;

    const blob = new Blob(['\ufeff', tableHtml], {
      type: 'application/vnd.ms-excel;charset=utf-8;',
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `${dataset?.name || ''}${t('datasets.hitTesting.batchTestingReport')}.xls`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  }, [statusData, dataset?.name, t]);

  const handleRetest = useCallback(() => {
    const url = withBasePath(`/console/dataset/${datasetId}/batch-testing?taskId=${taskId}`);
    window.location.href = url;
  }, [datasetId, taskId]);

  return (
    <div className="p-6 space-y-6 bg-gray-50 h-full">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">{t('datasets.hitTesting.batchTestingReport')}</h2>
      </div>

      {/* Metrics */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <Card className="p-4">
          <div className="text-sm text-muted-foreground">
            {t('datasets.hitTesting.totalQuestions')}
          </div>
          <div className="text-2xl font-semibold mt-2">
            {report?.total_queries || 0}{' '}
            <span className="text-sm font-normal">{t('datasets.hitTesting.questionsUnit')}</span>
          </div>
        </Card>
        <Card className="p-4">
          <div className="text-sm text-muted-foreground">
            {t('datasets.hitTesting.retrievalSuccessRate')}
          </div>
          <div className="text-2xl font-semibold mt-2 text-green-500">
            {report?.retrieval_success_rate || 0} <span className="text-sm font-normal">%</span>
          </div>
        </Card>
        <Card className="p-4">
          <div className="text-sm text-muted-foreground">
            {t('datasets.hitTesting.averageResponseTime')}
          </div>
          <div className="text-2xl font-semibold mt-2">
            {((report?.average_response_time || 0) / 1000).toFixed(2)}{' '}
            <span className="text-sm font-normal">{t('datasets.hitTesting.secondsUnit')}</span>
          </div>
        </Card>
        <Card className="p-4">
          <div className="text-sm text-muted-foreground">
            {t('datasets.hitTesting.questionMatchContribution')}
          </div>
          <div className="text-2xl font-semibold mt-2">
            {report?.question_match_rate || 0} <span className="text-sm font-normal">%</span>
          </div>
        </Card>
      </div>

      {/* Table */}
      <div className="border rounded-md bg-white">
        <div className="px-4 py-3 border-b text-sm font-medium">
          {t('datasets.hitTesting.detailedResultsList')}
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead className="bg-muted/40">
              <tr>
                <th className="text-left px-4 py-2 w-16">
                  {t('datasets.hitTesting.serialNumber')}
                </th>
                <th className="text-left px-4 py-2">{t('datasets.hitTesting.testQuestion')}</th>
                <th className="text-left px-4 py-2 w-24">{t('datasets.hitTesting.status')}</th>
                <th className="text-left px-4 py-2 w-48">
                  {t('datasets.hitTesting.top1RecallMethod')}
                </th>
                <th className="text-left px-4 py-2 w-48">
                  {t('datasets.hitTesting.top1Similarity')}
                </th>
                <th className="text-left px-4 py-2 w-48">
                  {t('datasets.hitTesting.top1SourceFile')}
                </th>
                <th className="text-left px-4 py-2 w-28">{t('datasets.hitTesting.operation')}</th>
              </tr>
            </thead>
            <tbody>
              {isReportLoading && isStatusLoading ? (
                <tr>
                  <td className="px-4 py-4 text-muted-foreground" colSpan={7}>
                    {t('common.loading')}
                  </td>
                </tr>
              ) : (
                (statusData?.results ?? []).map((r, idx: number) => {
                  const status = r.status as
                    | 'completed'
                    | 'failed'
                    | 'error'
                    | 'pending'
                    | 'processing';
                  const matchType = r?.result?.records[0]?.match_type as
                    | 'original'
                    | 'question'
                    | undefined;
                  return (
                    <tr key={r.query} className="border-t">
                      <td className="px-4 py-2">{idx + 1}</td>
                      <td className="px-4 py-2">{r.query}</td>
                      <td className="px-4 py-2">
                        {r.status === 'completed' ? (
                          <span className="text-green-600">
                            {t(`datasets.hitTesting.batchStatus.${status}`) ||
                              t('datasets.common.notAvailable')}
                          </span>
                        ) : r.status === 'failed' || r.status === 'error' ? (
                          <span className="text-red-600">
                            {t(`datasets.hitTesting.batchStatus.${status}`) ||
                              t('datasets.common.notAvailable')}
                          </span>
                        ) : (
                          <span className="text-muted-foreground">
                            {t(`datasets.hitTesting.batchStatus.${status}`) ||
                              t('datasets.common.notAvailable')}
                          </span>
                        )}
                      </td>
                      <td className="px-4 py-2">
                        {matchType
                          ? t(`datasets.hitTesting.matchTypes.${matchType}`)
                          : t('datasets.common.notAvailable')}
                      </td>
                      <td className="px-4 py-2">
                        {(r?.result?.records[0]?.score || 0).toFixed(2) ||
                          t('datasets.common.notAvailable')}
                      </td>
                      <td className="px-4 py-2 truncate">
                        {r?.result?.records[0]?.segment?.document?.name ||
                          t('datasets.common.notAvailable')}
                      </td>
                      <td className="px-4 py-2">
                        <Button
                          variant="link"
                          className="px-0 text-[var(--text-highlight)]"
                          onClick={() => handleViewDetail(r as ResultElement)}
                        >
                          {t('datasets.hitTesting.viewDetail')}
                        </Button>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Footer actions */}
      <div className="flex items-center justify-end gap-3">
        <Button variant="outline" onClick={handleRetest}>
          {t('datasets.hitTesting.retest')}
        </Button>
        <Button onClick={handleExport}>{t('datasets.hitTesting.exportReport')}</Button>
      </div>

      {/* Retrieval Detail Dialog */}
      <RetrievalDetailDialog
        open={isDialogOpen}
        onOpenChange={setIsDialogOpen}
        resultData={selectedResult}
      />
    </div>
  );
}

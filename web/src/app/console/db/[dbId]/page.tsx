'use client';

import { useState } from 'react';
import { useDb } from '@/hooks';
import { useParams, useRouter } from 'next/navigation';
import { useT } from '@/i18n';
import { useDbTables } from '@/hooks/db/use-db-tables';
import { DbTableFormDialog } from '@/components/db/table-form-dialog';
import { Button } from '@/components/ui/button';
import { Card, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { Plus, Table2, Search, ArrowRight, ScrollText, FileSpreadsheet } from 'lucide-react';

import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';

export default function DbOverviewPage() {
  const { dbId } = useParams();
  const router = useRouter();
  const t = useT();

  // Permissions
  const { hasPermission } = useAccountPermissions();
  const canManage = hasPermission('database.manage');
  const canAiQuery = hasPermission('database.ai_query');

  const { data: dbDetail, isLoading: isDbLoading } = useDb(dbId as string);
  const { tables, isLoading: isTablesLoading } = useDbTables(dbId as string);

  // Create table dialog state
  const [createOpen, setCreateOpen] = useState(false);

  const isLoading = isDbLoading || isTablesLoading;
  const hasTables = tables.length > 0;

  const handleOpenCreate = () => {
    if (!canManage) return;
    setCreateOpen(true);
  };

  const handleImportExcel = () => {
    if (!canManage) return;
    router.push(`/console/db/${dbId}/import-excel`);
  };

  const handleViewLogs = () => {
    router.push(`/console/db/${dbId}/record`);
  };

  const handleDataSearch = () => {
    if (!canAiQuery) return;
    router.push(`/console/db/${dbId}/search`);
  };

  if (isLoading) {
    return (
      <div className="p-6 space-y-6">
        <div className="space-y-2">
          <Skeleton className="h-8 w-48" />
          <Skeleton className="h-4 w-96" />
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          <Skeleton className="h-32" />
          <Skeleton className="h-32" />
          <Skeleton className="h-32" />
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6">
      {/* Header */}
      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <h1 className="text-2xl font-bold">{dbDetail?.data?.name}</h1>
          {hasTables && (
            <span className="text-sm text-muted-foreground bg-muted px-2 py-0.5 rounded-full">
              {t('dbs.overview.tablesCount', { count: tables.length })}
            </span>
          )}
        </div>
        {dbDetail?.data?.description && (
          <p className="text-muted-foreground">{dbDetail.data.description}</p>
        )}
      </div>

      {/* Empty State - No Tables */}
      {!hasTables && (
        <Card className="border-dashed">
          <div className="flex flex-col items-center justify-center py-12 px-6 text-center">
            <div className="rounded-full bg-muted p-4 mb-4">
              <Table2 className="h-8 w-8 text-muted-foreground" />
            </div>
            <h3 className="text-lg font-semibold mb-2">{t('dbs.overview.noTables')}</h3>
            <p className="text-muted-foreground mb-6 max-w-md">{t('dbs.overview.noTablesDesc')}</p>
            {canManage && (
              <div className="flex flex-wrap justify-center gap-2">
                <Button onClick={handleOpenCreate} size="lg">
                  <Plus className="h-4 w-4 mr-2" />
                  {t('dbs.overview.createFirstTable')}
                </Button>
                <Button onClick={handleImportExcel} size="lg" variant="outline">
                  <FileSpreadsheet className="h-4 w-4 mr-2" />
                  {t('dbs.excelImport.entry')}
                </Button>
              </div>
            )}
          </div>
        </Card>
      )}

      {/* Quick Actions - Has Tables */}
      {hasTables && (
        <div className="space-y-4">
          <h2 className="text-lg font-semibold">{t('dbs.overview.quickActions')}</h2>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {/* Data Search Card */}
            {canAiQuery && (
              <Card
                className="cursor-pointer hover:bg-accent/50 transition-colors group"
                onClick={handleDataSearch}
              >
                <CardHeader>
                  <div className="flex items-start justify-between">
                    <div className="rounded-lg bg-highlight/10 p-2.5 mb-2">
                      <Search className="h-5 w-5 text-highlight" />
                    </div>
                    <ArrowRight className="h-4 w-4 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity" />
                  </div>
                  <CardTitle className="text-base">{t('dbs.overview.dataSearch')}</CardTitle>
                  <CardDescription>{t('dbs.overview.dataSearchDesc')}</CardDescription>
                </CardHeader>
              </Card>
            )}

            {/* View Logs Card */}
            <Card
              className="cursor-pointer hover:bg-accent/50 transition-colors group"
              onClick={handleViewLogs}
            >
              <CardHeader>
                <div className="flex items-start justify-between">
                  <div className="rounded-lg bg-amber-500/10 p-2.5 mb-2">
                    <ScrollText className="h-5 w-5 text-amber-500" />
                  </div>
                  <ArrowRight className="h-4 w-4 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity" />
                </div>
                <CardTitle className="text-base">{t('dbs.features.logs')}</CardTitle>
                <CardDescription>{t('dbs.overview.viewLogsDesc')}</CardDescription>
              </CardHeader>
            </Card>

            {/* Create Table Card */}
            {canManage && (
              <Card
                className="cursor-pointer hover:bg-accent/50 transition-colors group"
                onClick={handleImportExcel}
              >
                <CardHeader>
                  <div className="flex items-start justify-between">
                    <div className="rounded-lg bg-highlight/10 p-2.5 mb-2">
                      <FileSpreadsheet className="h-5 w-5 text-highlight" />
                    </div>
                    <ArrowRight className="h-4 w-4 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity" />
                  </div>
                  <CardTitle className="text-base">{t('dbs.excelImport.entry')}</CardTitle>
                  <CardDescription>{t('dbs.excelImport.entryDesc')}</CardDescription>
                </CardHeader>
              </Card>
            )}

            {canManage && (
              <Card
                className="cursor-pointer hover:bg-accent/50 transition-colors group"
                onClick={handleOpenCreate}
              >
                <CardHeader>
                  <div className="flex items-start justify-between">
                    <div className="rounded-lg bg-green-500/10 p-2.5 mb-2">
                      <Plus className="h-5 w-5 text-green-500" />
                    </div>
                    <ArrowRight className="h-4 w-4 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity" />
                  </div>
                  <CardTitle className="text-base">{t('dbs.createTable')}</CardTitle>
                  <CardDescription>{t('dbs.overview.createTableDesc')}</CardDescription>
                </CardHeader>
              </Card>
            )}
          </div>
        </div>
      )}

      <DbTableFormDialog
        dbId={dbId as string}
        mode="create"
        open={createOpen}
        onOpenChange={setCreateOpen}
        tables={tables}
      />
    </div>
  );
}

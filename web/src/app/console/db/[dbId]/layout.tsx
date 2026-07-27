'use client';

import React from 'react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { cn } from '@/lib/utils';
import {
  ChevronDown,
  FileSpreadsheet,
  Loader2,
  MoreHorizontal,
  Pencil,
  Trash2,
  Plus,
  Search,
  ScrollText,
  Settings,
  ShieldAlert,
  Table,
} from 'lucide-react';
import { useDb, useDbTables, useDeleteDbTable, useWorkspaceMismatch } from '@/hooks';
import { useT } from '@/i18n';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { WorkspaceMismatchGuard } from '@/components/common/workspace-mismatch-guard';
import { DbTableFormDialog } from '@/components/db/table-form-dialog';
import { getSidebarCollapsed, saveSidebarCollapsed } from '@/utils/ui-local';
import { ICON_BG, ICON_TEXT } from '@/lib/config';
import type { DbTable } from '@/services/types/db';
import { ResourceSidebar, ResourceSidebarHeader } from '@/components/common/resource-sidebar';
import { EditDbDialog } from '@/components/db/dialog';
import { ErrorBoundary } from '@/components/error-boundary';
import { AgentResourceBoundDialog } from '@/components/common/agent-resource-bound-dialog';
import type { AgentResourceBoundImpact } from '@/services/types/common';
import { getAgentResourceBoundImpact } from '@/utils/agent-resource-bound';
import { dbService } from '@/services/db.service';
import { toast } from 'sonner';
import {
  DATABASE_PERMISSION_ACTIONS,
  DATABASE_TABLE_METADATA_PERMISSION_CODES,
  DATABASE_VISIBLE_PERMISSION_CODES,
} from '@/constants/permissions';

interface LayoutProps {
  children: React.ReactNode;
  // In client components, params is a Promise and should be unwrapped with React.use
  params: Promise<{ dbId: string }>;
}

export default function DbLayout({ children, params }: LayoutProps) {
  const pathname = usePathname();
  const { dbId } = React.use(params);

  // Permissions
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canView = hasAnyPermission(DATABASE_VISIBLE_PERMISSION_CODES);
  const canUpdateDatabase = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.update);
  const canManageSchema = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.schemaManage);
  const canImportExcel = hasAnyPermission([
    ...DATABASE_PERMISSION_ACTIONS.importAnalyze,
    ...DATABASE_PERMISSION_ACTIONS.importExecute,
  ]);
  const canOpenRecords = hasAnyPermission([
    ...DATABASE_PERMISSION_ACTIONS.recordView,
    ...DATABASE_PERMISSION_ACTIONS.recordCreate,
    ...DATABASE_PERMISSION_ACTIONS.recordUpdate,
    ...DATABASE_PERMISSION_ACTIONS.recordDelete,
  ]);
  const canOpenSchema = hasAnyPermission([
    ...DATABASE_PERMISSION_ACTIONS.schemaView,
    ...DATABASE_PERMISSION_ACTIONS.schemaManage,
  ]);
  const canAiQuery = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.aiQueryRead);
  const canViewOperationLogs = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.operationLogsView);
  const canViewTableMetadata = hasAnyPermission(DATABASE_TABLE_METADATA_PERMISSION_CODES);

  const { data: dbDetail, isLoading: isDbLoading } = useDb(dbId, {
    enabled: canView,
  });

  // Check workspace mismatch for sidebar navigation control
  const { isMismatch } = useWorkspaceMismatch(dbDetail?.data?.workspace_id || '');

  const t = useT();
  const router = useRouter();

  const [dbMenuOpen, setDbMenuOpen] = React.useState<boolean>(true);
  const [tableQuery, setTableQuery] = React.useState('');
  const [isCollapsed, setIsCollapsed] = React.useState<boolean>(() =>
    getSidebarCollapsed('db', true)
  );
  const [tableDialog, setTableDialog] = React.useState<{
    mode: 'create' | 'edit';
    table?: DbTable;
  } | null>(null);
  const [createMethodOpen, setCreateMethodOpen] = React.useState(false);
  const [editDbOpen, setEditDbOpen] = React.useState(false);
  const [deleteTarget, setDeleteTarget] = React.useState<{ id: string; name: string } | null>(null);
  const [bindingImpact, setBindingImpact] = React.useState<AgentResourceBoundImpact | null>(null);
  const [isCheckingDeleteImpact, setIsCheckingDeleteImpact] = React.useState(false);

  const { tables, isLoading } = useDbTables(dbId, {
    enabled: canViewTableMetadata && !isMismatch,
  });
  const visibleTables = React.useMemo(() => {
    const query = tableQuery.trim().toLocaleLowerCase();
    if (!query) return tables;

    return tables.filter(table => {
      const label = table.name || table.table_name || '';
      return label.toLocaleLowerCase().includes(query);
    });
  }, [tableQuery, tables]);
  const deleteMutation = useDeleteDbTable(dbId);

  const deleteTable = async (impact?: AgentResourceBoundImpact) => {
    if (!canManageSchema || !deleteTarget) return;
    const target = deleteTarget;
    try {
      await deleteMutation.mutateAsync({
        id: target.id,
        confirmation: impact
          ? { agent_binding_action: 'unbind', impact_token: impact.impact_token }
          : undefined,
      });
      setDeleteTarget(null);
      setBindingImpact(null);
      if (pathname.split('/').includes(target.id)) {
        router.push(`/console/db/${dbId}`);
      }
    } catch (error) {
      const nextImpact = getAgentResourceBoundImpact(error);
      if (!nextImpact) return;
      setBindingImpact(nextImpact);
    }
  };

  const requestDeleteTable = async (target: { id: string; name: string }) => {
    if (!canManageSchema || isCheckingDeleteImpact) return;
    setIsCheckingDeleteImpact(true);
    try {
      const response = await dbService.previewDbTableDeleteImpact(dbId, target.id);
      setDeleteTarget(target);
      if (response.data) setBindingImpact(response.data);
    } catch {
      toast.error(t('common.agentResourceBound.previewFailed'));
    } finally {
      setIsCheckingDeleteImpact(false);
    }
  };

  React.useEffect(() => {
    saveSidebarCollapsed('db', isCollapsed);
  }, [isCollapsed]);

  const toggleCollapse = () => setIsCollapsed(prev => !prev);

  const onOpenCreate = () => {
    if (!canManageSchema) return;
    setCreateMethodOpen(true);
  };

  const onOpenEdit = (table: DbTable) => {
    if (!canManageSchema) return;
    setTableDialog({ mode: 'edit', table });
  };

  const itemActive = (href: string) => pathname === href;
  const db = dbDetail?.data;
  const iconType = db?.icon_type;
  let textIcon = (db?.name || '').slice(0, 2).toUpperCase() || ICON_TEXT;
  const iconBackground = db?.icon_background || ICON_BG;
  const imgSrc = iconType === 'image' ? db?.icon_url || '' : undefined;

  if (iconType === 'text' && db?.icon) {
    textIcon = db.icon;
  }

  // Access Loading State
  if (isPermissionsLoading) {
    return (
      <div className="flex h-full w-full items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  // Access Denied State
  if (!canView) {
    return (
      <div className="flex flex-col items-center justify-center h-full w-full p-4 text-center">
        <ShieldAlert className="h-12 w-12 text-muted-foreground mb-4" />
        <h2 className="text-xl font-semibold mb-2">{t('common.accessDenied')}</h2>
        <p className="text-muted-foreground max-w-md">{t('common.unauthorizedDescription')}</p>
      </div>
    );
  }

  return (
    <WorkspaceMismatchGuard
      isLoading={isDbLoading}
      targetWorkspaceId={dbDetail?.data?.workspace_id || ''}
      targetWorkspaceName={dbDetail?.data?.workspace?.name}
    >
      <>
        <div className="flex w-full h-full overflow-hidden">
          <ResourceSidebar
            isCollapsed={isCollapsed}
            onToggleCollapse={toggleCollapse}
            expandLabel={t('navigation.expand')}
            collapseLabel={t('navigation.collapse')}
            isNavigationHidden={isMismatch}
            expandedWidthClassName="w-64"
            header={
              <ResourceSidebarHeader
                isCollapsed={isCollapsed}
                isLoading={isDbLoading}
                iconType={iconType}
                iconSrc={imgSrc}
                icon={textIcon}
                iconBackground={iconBackground}
                name={db?.name || t('dbs.noName')}
                description={db?.description || ''}
                backHref="/console/db"
                backLabel={t('dbs.backToDatabaseList')}
              />
            }
          >
            <nav className="flex min-h-0 flex-1 flex-col px-2 py-2">
              {canViewTableMetadata && (
                <div className={cn('flex min-h-0 flex-1 flex-col', isCollapsed && 'flex-none')}>
                  <div
                    className={cn(
                      'flex w-full items-center rounded-lg text-xs font-medium transition-colors',
                      pathname === `/console/db/${dbId}` ||
                        pathname.startsWith(`/console/db/${dbId}/table`)
                        ? 'bg-primary/10 text-primary'
                        : 'hover:bg-primary/5 hover:text-primary',
                      isCollapsed && 'justify-center'
                    )}
                  >
                    <Link
                      href={`/console/db/${dbId}`}
                      className={cn(
                        'flex min-w-0 grow items-center px-2.5 py-2',
                        isCollapsed && 'grow-0 justify-center px-0'
                      )}
                    >
                      <Table className="h-4 w-4 shrink-0" />
                      <span
                        className={cn(
                          'ml-2 min-w-0 grow truncate text-left transition-all duration-300',
                          isCollapsed && 'ml-0 w-0 grow-0 overflow-hidden opacity-0'
                        )}
                      >
                        {t('dbs.tables')}
                        {!isLoading && !isCollapsed ? (
                          <span className="ml-1.5 font-normal text-muted-foreground">
                            {tables.length}
                          </span>
                        ) : null}
                      </span>
                    </Link>
                    {!isCollapsed && canManageSchema ? (
                      <button
                        type="button"
                        onClick={onOpenCreate}
                        className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary"
                        aria-label={t('dbs.createTable')}
                        title={t('dbs.createTable')}
                      >
                        <Plus className="h-4 w-4" />
                      </button>
                    ) : null}
                    {!isCollapsed ? (
                      <button
                        type="button"
                        onClick={() => setDbMenuOpen(prev => !prev)}
                        className="mr-1 flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-primary/10 hover:text-primary"
                        aria-label={dbMenuOpen ? t('navigation.collapse') : t('navigation.expand')}
                      >
                        <ChevronDown
                          className={cn(
                            'h-4 w-4 transition-transform',
                            dbMenuOpen ? 'rotate-0' : '-rotate-90'
                          )}
                        />
                      </button>
                    ) : null}
                  </div>

                  {dbMenuOpen && !isCollapsed ? (
                    <div className="mt-2 flex min-h-0 flex-1 flex-col">
                      {tables.length > 5 ? (
                        <label className="relative mb-2 block">
                          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                          <input
                            type="search"
                            value={tableQuery}
                            onChange={event => setTableQuery(event.target.value)}
                            placeholder={t('dbs.tableNavigation.searchPlaceholder')}
                            aria-label={t('dbs.tableNavigation.searchPlaceholder')}
                            className="h-8 w-full rounded-md border border-border bg-background pl-8 pr-2 text-xs outline-none transition-colors placeholder:text-muted-foreground focus:border-primary/50 focus:ring-2 focus:ring-primary/10"
                          />
                        </label>
                      ) : null}

                      <div className="min-h-0 flex-1 space-y-1 overflow-y-auto pr-0.5">
                        {isLoading ? (
                          <>
                            <Skeleton className="h-9 w-full" />
                            <Skeleton className="h-9 w-full" />
                            <Skeleton className="h-9 w-full" />
                            <Skeleton className="h-9 w-full" />
                          </>
                        ) : null}
                        {!isLoading && visibleTables.length === 0 ? (
                          <div className="px-2 py-6 text-center text-xs text-muted-foreground">
                            {tableQuery
                              ? t('dbs.tableNavigation.noSearchResults')
                              : t('dbs.tableNavigation.noTables')}
                          </div>
                        ) : null}
                        {!isLoading &&
                          visibleTables.map((table, index) => {
                            const label = table.name || table.table_name;
                            const tableId = String(table.id || '');
                            const tableKey =
                              tableId || `${table.table_name || label || 'table'}-${index}`;
                            const tableRootHref = tableId
                              ? `/console/db/${dbId}/table/${tableId}`
                              : '';
                            const href = canOpenRecords
                              ? tableRootHref
                              : canOpenSchema && tableRootHref
                                ? `${tableRootHref}/structure`
                                : '';
                            const active =
                              Boolean(tableRootHref) &&
                              (itemActive(tableRootHref) ||
                                pathname.startsWith(tableRootHref + '/'));
                            return (
                              <div
                                key={tableKey}
                                className="group relative flex w-full min-w-0 items-center overflow-hidden"
                              >
                                {href ? (
                                  <Link
                                    href={href}
                                    className={cn(
                                      'flex h-9 w-full min-w-0 items-center rounded-md px-2 pr-9 text-xs text-secondary-foreground transition-colors',
                                      active
                                        ? 'bg-primary/10 text-primary'
                                        : 'hover:bg-primary/5 hover:text-primary'
                                    )}
                                    title={label}
                                  >
                                    <Table className="mr-2 h-3.5 w-3.5 shrink-0 opacity-70" />
                                    <span className="min-w-0 flex-1 truncate text-left">
                                      {label}
                                    </span>
                                  </Link>
                                ) : (
                                  <span
                                    className={cn(
                                      'flex h-9 w-full min-w-0 items-center rounded-md px-2 pr-9 text-xs text-muted-foreground',
                                      active && 'bg-primary/10 text-primary'
                                    )}
                                    title={label}
                                  >
                                    <Table className="mr-2 h-3.5 w-3.5 shrink-0 opacity-70" />
                                    <span className="min-w-0 flex-1 truncate text-left">
                                      {label}
                                    </span>
                                  </span>
                                )}
                                {canManageSchema && (
                                  <DropdownMenu>
                                    <DropdownMenuTrigger asChild>
                                      <button
                                        data-no-nav
                                        className={cn(
                                          'absolute right-1.5 top-1/2 inline-flex h-6 w-6 -translate-y-1/2 items-center justify-center rounded-md transition-opacity',
                                          'pointer-events-none opacity-0 group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100 data-[state=open]:pointer-events-auto data-[state=open]:opacity-100',
                                          active && 'pointer-events-auto opacity-100',
                                          active
                                            ? 'text-primary hover:text-primary hover:bg-primary/10'
                                            : 'text-muted-foreground hover:text-primary hover:bg-primary/5'
                                        )}
                                        onClick={e => {
                                          e.preventDefault();
                                          e.stopPropagation();
                                        }}
                                        aria-label={t('dbs.actions.more')}
                                      >
                                        <MoreHorizontal className="h-3.5 w-3.5" />
                                      </button>
                                    </DropdownMenuTrigger>
                                    <DropdownMenuContent align="end">
                                      <DropdownMenuItem inset onSelect={() => onOpenEdit(table)}>
                                        <Pencil className="h-4 w-4" />
                                        {t('dbs.actions.edit')}
                                      </DropdownMenuItem>
                                      <DropdownMenuItem
                                        variant="destructive"
                                        inset
                                        disabled={isCheckingDeleteImpact}
                                        onSelect={() =>
                                          void requestDeleteTable({
                                            id: String(table.id),
                                            name: label,
                                          })
                                        }
                                      >
                                        <Trash2 className="h-4 w-4 text-destructive" />
                                        {t('dbs.actions.delete')}
                                      </DropdownMenuItem>
                                    </DropdownMenuContent>
                                  </DropdownMenu>
                                )}
                              </div>
                            );
                          })}
                      </div>
                    </div>
                  ) : null}
                </div>
              )}

              <div
                className={cn(
                  'mt-2 flex flex-col gap-1 border-t border-border pt-2',
                  isCollapsed && 'mt-0 border-t-0 pt-0'
                )}
              >
                {canAiQuery && (
                  <Link
                    href={`/console/db/${dbId}/search`}
                    className={cn(
                      'flex w-full items-center rounded-md px-2.5 py-2 text-xs font-medium transition-colors',
                      pathname === `/console/db/${dbId}/search` ||
                        pathname.startsWith(`/console/db/${dbId}/search/`)
                        ? 'bg-primary/10 text-primary'
                        : 'hover:bg-primary/5 hover:text-primary',
                      isCollapsed && 'justify-center px-0'
                    )}
                  >
                    <Search className="h-4 w-4 shrink-0" />
                    <span
                      className={cn(
                        'ml-2 truncate transition-all duration-300',
                        isCollapsed && 'ml-0 w-0 overflow-hidden opacity-0'
                      )}
                    >
                      {t('dbs.features.dataQuery')}
                    </span>
                  </Link>
                )}
                {canViewOperationLogs && (
                  <Link
                    href={`/console/db/${dbId}/record`}
                    className={cn(
                      'flex w-full items-center rounded-md px-2.5 py-2 text-xs font-medium transition-colors',
                      pathname === `/console/db/${dbId}/record` ||
                        pathname.startsWith(`/console/db/${dbId}/record/`)
                        ? 'bg-primary/10 text-primary'
                        : 'hover:bg-primary/5 hover:text-primary',
                      isCollapsed && 'justify-center px-0'
                    )}
                  >
                    <ScrollText className="h-4 w-4 shrink-0" />
                    <span
                      className={cn(
                        'ml-2 truncate transition-all duration-300',
                        isCollapsed && 'ml-0 w-0 overflow-hidden opacity-0'
                      )}
                    >
                      {t('dbs.features.logs')}
                    </span>
                  </Link>
                )}
                {canUpdateDatabase && !isMismatch && db && (
                  <button
                    type="button"
                    onClick={() => setEditDbOpen(true)}
                    className={cn(
                      'flex w-full items-center rounded-md px-2.5 py-2 text-xs font-medium transition-colors hover:bg-primary/5 hover:text-primary',
                      isCollapsed && 'justify-center px-0'
                    )}
                    title={isCollapsed ? t('dbs.databaseSettings') : undefined}
                  >
                    <Settings className="h-4 w-4 shrink-0" />
                    <span
                      className={cn(
                        'ml-2 truncate transition-all duration-300',
                        isCollapsed && 'ml-0 w-0 overflow-hidden opacity-0'
                      )}
                    >
                      {t('dbs.databaseSettings')}
                    </span>
                  </button>
                )}
              </div>
            </nav>
          </ResourceSidebar>

          {/* Content */}
          <div className="flex-1 h-full overflow-hidden">
            <ErrorBoundary key={pathname}>{children}</ErrorBoundary>
          </div>
        </div>

        <Dialog open={createMethodOpen} onOpenChange={setCreateMethodOpen}>
          <DialogContent size="md">
            <DialogHeader>
              <DialogTitle>{t('dbs.createMethod.title')}</DialogTitle>
              <DialogDescription>{t('dbs.createMethod.description')}</DialogDescription>
            </DialogHeader>
            <div className="grid gap-3 px-6 pb-6 sm:grid-cols-2">
              <button
                type="button"
                className="flex min-h-28 flex-col items-start gap-2 rounded-lg border p-4 text-left transition-colors hover:border-primary hover:bg-muted/50"
                onClick={() => {
                  setCreateMethodOpen(false);
                  setTableDialog({ mode: 'create' });
                }}
              >
                <Table className="h-5 w-5 text-primary" />
                <span className="font-medium">{t('dbs.createMethod.manual')}</span>
                <span className="text-sm text-muted-foreground">
                  {t('dbs.createMethod.manualDescription')}
                </span>
              </button>
              {canImportExcel && (
                <button
                  type="button"
                  className="flex min-h-28 flex-col items-start gap-2 rounded-lg border p-4 text-left transition-colors hover:border-primary hover:bg-muted/50"
                  onClick={() => {
                    setCreateMethodOpen(false);
                    router.push(`/console/db/${dbId}/import-excel`);
                  }}
                >
                  <FileSpreadsheet className="h-5 w-5 text-primary" />
                  <span className="font-medium">{t('dbs.createMethod.excel')}</span>
                  <span className="text-sm text-muted-foreground">
                    {t('dbs.createMethod.excelDescription')}
                  </span>
                </button>
              )}
            </div>
          </DialogContent>
        </Dialog>

        <DbTableFormDialog
          dbId={dbId}
          mode={tableDialog?.mode ?? 'create'}
          open={Boolean(tableDialog)}
          onOpenChange={open => {
            if (!open) setTableDialog(null);
          }}
          table={tableDialog?.table}
          tables={tables}
        />
        <EditDbDialog open={editDbOpen} onOpenChange={setEditDbOpen} db={db} />

        {/* Delete Table Confirmation Dialog */}
        <ConfirmDialog
          variant="danger"
          open={Boolean(deleteTarget) && !bindingImpact}
          onOpenChange={open => {
            if (!open) setDeleteTarget(null);
          }}
          title={deleteTarget ? t('dbs.deleteConfirmTitle', { name: deleteTarget.name }) : ''}
          description={t('dbs.deleteTableConfirmDescription')}
          confirmText={t('common.confirm')}
          cancelText={t('common.cancel')}
          loading={deleteMutation.isPending}
          onConfirm={() => void deleteTable()}
        />
        <AgentResourceBoundDialog
          open={Boolean(bindingImpact)}
          impact={bindingImpact}
          loading={deleteMutation.isPending}
          onOpenChange={open => {
            if (!open) {
              setBindingImpact(null);
              setDeleteTarget(null);
            }
          }}
          onConfirm={() => {
            if (bindingImpact) void deleteTable(bindingImpact);
          }}
        />
      </>
    </WorkspaceMismatchGuard>
  );
}

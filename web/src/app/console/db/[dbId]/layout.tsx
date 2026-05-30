'use client';

import React from 'react';
import Link from 'next/link';
import { usePathname, useRouter } from 'next/navigation';
import { cn } from '@/lib/utils';
import {
  ChevronDown,
  Loader2,
  MoreHorizontal,
  Pencil,
  Trash2,
  Plus,
  Search,
  ScrollText,
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
import { Skeleton } from '@/components/ui/skeleton';
import { WorkspaceMismatchGuard } from '@/components/common/workspace-mismatch-guard';
import { DbTableFormDialog } from '@/components/db/table-form-dialog';
import { getSidebarCollapsed, saveSidebarCollapsed } from '@/utils/ui-local';
import { ICON_BG, ICON_TEXT } from '@/lib/config';
import type { DbTable } from '@/services/types/db';
import { ResourceSidebar, ResourceSidebarHeader } from '@/components/common/resource-sidebar';
import { EditDbDialog } from '@/components/db/dialog';
import { ErrorBoundary } from '@/components/error-boundary';

interface LayoutProps {
  children: React.ReactNode;
  // In client components, params is a Promise and should be unwrapped with React.use
  params: Promise<{ dbId: string }>;
}

export default function DbLayout({ children, params }: LayoutProps) {
  const pathname = usePathname();
  const { dbId } = React.use(params);

  // Permissions
  const { hasPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canView = hasPermission('database.view');
  const canManage = hasPermission('database.manage');
  const canAiQuery = hasPermission('database.ai_query');

  const { data: dbDetail, isLoading: isDbLoading } = useDb(dbId, {
    enabled: canView,
  });

  // Check workspace mismatch for sidebar navigation control
  const { isMismatch } = useWorkspaceMismatch(dbDetail?.data?.workspace_id || '');

  const t = useT();
  const router = useRouter();

  const [dbMenuOpen, setDbMenuOpen] = React.useState<boolean>(true);
  const [isCollapsed, setIsCollapsed] = React.useState<boolean>(() =>
    getSidebarCollapsed('db', true)
  );
  const [tableDialog, setTableDialog] = React.useState<{
    mode: 'create' | 'edit';
    table?: DbTable;
  } | null>(null);
  const [editDbOpen, setEditDbOpen] = React.useState(false);
  const [deleteTarget, setDeleteTarget] = React.useState<{ id: string; name: string } | null>(null);

  const { tables, isLoading } = useDbTables(dbId, {
    enabled: canView,
  });
  const deleteMutation = useDeleteDbTable(dbId);

  React.useEffect(() => {
    saveSidebarCollapsed('db', isCollapsed);
  }, [isCollapsed]);

  const toggleCollapse = () => setIsCollapsed(prev => !prev);

  const onOpenCreate = () => {
    if (!canManage) return;
    setTableDialog({ mode: 'create' });
  };

  const onOpenEdit = (table: DbTable) => {
    if (!canManage) return;
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
    <>
      <div className="flex w-full h-full overflow-hidden">
        <ResourceSidebar
          isCollapsed={isCollapsed}
          onToggleCollapse={toggleCollapse}
          expandLabel={t('navigation.expand')}
          collapseLabel={t('navigation.collapse')}
          expandedWidthClassName="w-72"
          isNavigationHidden={isMismatch}
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
              iconActionLabel={t('dbs.edit')}
              onIconClick={canManage && !isMismatch && db ? () => setEditDbOpen(true) : undefined}
            />
          }
        >
          <nav className="flex flex-1 flex-col gap-0.5 px-1 py-1.5">
            {/* Tables toggle */}
            <button
              type="button"
              onClick={() => {
                if (isCollapsed) {
                  setIsCollapsed(false);
                  setDbMenuOpen(true);
                } else {
                  setDbMenuOpen(prev => !prev);
                }
              }}
              className={cn(
                'flex items-center rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors',
                pathname.startsWith(`/console/db/${dbId}/table`)
                  ? 'bg-primary/10 text-primary'
                  : 'hover:bg-primary/5 hover:text-primary',
                isCollapsed && 'justify-center px-0'
              )}
            >
              <Table className="h-4 w-4 shrink-0" />
              <span
                className={cn(
                  'truncate ml-1.5 grow text-left transition-all duration-300',
                  isCollapsed && 'ml-0 w-0 grow-0 overflow-hidden opacity-0'
                )}
              >
                {t('dbs.tables')}
              </span>
              {!isCollapsed && (
                <ChevronDown
                  className={cn(
                    'h-4 w-4 transition-transform shrink-0',
                    dbMenuOpen ? 'rotate-0' : 'rotate-90'
                  )}
                />
              )}
            </button>
            {/* Table list */}
            {dbMenuOpen && !isCollapsed && (
              <div className="pl-3 space-y-0.5">
                {/* Create table */}
                {canManage && (
                  <button
                    type="button"
                    onClick={e => {
                      e.preventDefault();
                      onOpenCreate();
                    }}
                    className={cn(
                      'flex items-center rounded-md h-7 px-2 text-xs transition-colors w-full text-secondary-foreground',
                      'hover:bg-primary/5 hover:text-primary'
                    )}
                  >
                    <Plus className="h-4 w-4" />
                    <span className="ml-1 truncate">{t('dbs.createTable')}</span>
                  </button>
                )}
                {isLoading && (
                  <>
                    <Skeleton className="h-7 w-full" />
                    <Skeleton className="h-7 w-full" />
                    <Skeleton className="h-7 w-full" />
                    <Skeleton className="h-7 w-full" />
                  </>
                )}
                {!isLoading &&
                  tables.map((table, index) => {
                    const label = table.name || table.table_name;
                    const tableId = String(table.id || '');
                    const tableKey = tableId || `${table.table_name || label || 'table'}-${index}`;
                    const href = tableId ? `/console/db/${dbId}/table/${tableId}` : '#';
                    const active = itemActive(href) || pathname.startsWith(href + '/');
                    return (
                      <div
                        key={tableKey}
                        className="w-full flex items-center justify-center gap-1 group relative"
                      >
                        <Link
                          href={href}
                          className={cn(
                            'block h-7 grow cursor-pointer truncate rounded-md pl-2 pr-6 text-ellipsis text-xs leading-7 text-secondary-foreground overflow-hidden',
                            active
                              ? 'bg-primary/10 text-primary'
                              : 'hover:bg-primary/5 hover:text-primary'
                          )}
                          title={label}
                        >
                          {label}
                        </Link>
                        {/* Actions dropdown replacing edit button */}
                        {canManage && (
                          <DropdownMenu>
                            <DropdownMenuTrigger asChild>
                              <button
                                data-no-nav
                                className={cn(
                                  'absolute top-1/2 right-1 -translate-y-1/2',
                                  'h-5 w-5 inline-flex items-center justify-center rounded-md transition-opacity pointer-events-none opacity-0 group-hover:pointer-events-auto group-hover:opacity-100 data-[state=open]:pointer-events-auto data-[state=open]:opacity-100',
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
                                onSelect={() =>
                                  setDeleteTarget({ id: String(table.id), name: label })
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
            )}

            {/* Features */}
            {canAiQuery && (
              <Link
                href={`/console/db/${dbId}/search`}
                className={cn(
                  'flex items-center rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors',
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
                    'truncate ml-1.5 transition-all duration-300',
                    isCollapsed && 'ml-0 w-0 overflow-hidden opacity-0'
                  )}
                >
                  {t('dbs.features.dataQuery')}
                </span>
              </Link>
            )}
            <Link
              href={`/console/db/${dbId}/record`}
              className={cn(
                'flex items-center rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors',
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
                  'truncate ml-1.5 transition-all duration-300',
                  isCollapsed && 'ml-0 w-0 overflow-hidden opacity-0'
                )}
              >
                {t('dbs.features.logs')}
              </span>
            </Link>
          </nav>
        </ResourceSidebar>

        {/* Content */}
        <div className="flex-1 h-full overflow-hidden">
          <WorkspaceMismatchGuard
            isLoading={isDbLoading}
            targetWorkspaceId={dbDetail?.data?.workspace_id || ''}
            targetWorkspaceName={dbDetail?.data?.workspace?.name}
          >
            <ErrorBoundary key={pathname}>{children}</ErrorBoundary>
          </WorkspaceMismatchGuard>
        </div>
      </div>

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
        open={Boolean(deleteTarget)}
        onOpenChange={open => {
          if (!open) setDeleteTarget(null);
        }}
        title={deleteTarget ? t('dbs.deleteConfirmTitle', { name: deleteTarget.name }) : ''}
        description={t('dbs.deleteTableConfirmDescription')}
        confirmText={t('common.confirm')}
        cancelText={t('common.cancel')}
        loading={deleteMutation.isPending}
        onConfirm={() => {
          if (!deleteTarget) return;
          deleteMutation.mutate(deleteTarget.id, {
            onSuccess: () => {
              setDeleteTarget(null);
              if (pathname.split('/').includes(deleteTarget.id)) router.push(`/console/db/${dbId}`);
            },
          });
        }}
      />
    </>
  );
}

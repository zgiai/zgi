'use client';

import React, { useMemo, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { RefreshCw, Plus, Search, Database } from 'lucide-react';
import DbCard from '@/components/db/card';
import { CreateDbDialog, EditDbDialog } from '@/components/db/dialog';
import { useDbsBasic } from '@/hooks/db/use-dbs';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { SkeletonGrid } from '@/components/datasets/page/skeleton-grid';
import { DbEmptyElement, DbEmptySearchResults } from '@/components/db/empty-element';

import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useCurrentWorkspace, useWorkspaceStore } from '@/store/workspace-store';
import { ShieldAlert } from 'lucide-react';
import { PersonalSpaceEmptyState } from '@/components/common/personal-space-empty-state';
import type { Db } from '@/services/types/db';

export default function DbPage() {
  const t = useT();
  const currentWorkspace = useCurrentWorkspace();
  const isOrganizationMode = useWorkspaceStore(state => state.isOrganizationMode);

  // Permissions
  const { hasPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canView = hasPermission('database.view');
  const canManage = hasPermission('database.manage');

  const [searchKeyword, setSearchKeyword] = useState('');
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [selectedDb, setSelectedDb] = useState<Db | undefined>(undefined);

  const {
    dbs,
    isLoading: isDbsLoading,
    isFetching,
    refetch,
  } = useDbsBasic(
    {
      workspace_id: currentWorkspace?.id,
    },
    {
      enabled: canView,
    }
  );

  const isLoading = isDbsLoading || isPermissionsLoading;

  const allDbs = useMemo(() => dbs || [], [dbs]);
  const filteredDbs = useMemo(
    () =>
      (allDbs || []).filter(db =>
        searchKeyword.trim() ? db.name.toLowerCase().includes(searchKeyword.toLowerCase()) : true
      ),
    [allDbs, searchKeyword]
  );

  const scrollRef = useRef<HTMLDivElement | null>(null);

  const openCreate = () => {
    if (!canManage) return;
    setSelectedDb(undefined);
    setCreateDialogOpen(true);
  };

  const openEdit = (db: Db) => {
    if (!canManage) return;
    setSelectedDb(db);
    setEditDialogOpen(true);
  };

  // Access Denied State
  if (!isPermissionsLoading && !canView) {
    return (
      <div className="flex flex-col items-center justify-center h-full p-4 text-center">
        <ShieldAlert className="h-12 w-12 text-muted-foreground mb-4" />
        <h2 className="text-xl font-semibold mb-2">{t('common.accessDenied')}</h2>
        <p className="text-muted-foreground max-w-md">{t('common.unauthorizedDescription')}</p>
      </div>
    );
  }

  return (
    <div
      className="p-4 sm:p-6 lg:p-8 space-y-6 sm:space-y-8 lg:space-y-9 flex flex-col h-full overflow-y-auto"
      ref={scrollRef}
    >
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <h1 className="text-2xl font-bold">{t('dbs.title')}</h1>
          <Button
            isIcon
            variant="ghost"
            className="size-7 rounded-sm hover:bg-muted"
            onClick={() => {
              Promise.resolve(refetch()).then(() => {
                toast.success(t('common.refreshSuccess'));
              });
            }}
            disabled={isFetching}
          >
            <RefreshCw size={16} className={`${isFetching ? 'animate-spin' : ''} h-4 w-4`} />
          </Button>
        </div>

        <div className="flex gap-3 items-center">
          <div className="relative max-w-md">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder={t('dbs.search.placeholder')}
              value={searchKeyword}
              onChange={e => setSearchKeyword(e.target.value)}
              className="pl-9"
            />
          </div>
          {canManage && (
            <Button onClick={openCreate}>
              <Plus size={16} />
              <span className="text-sm font-normal">{t('dbs.create')}</span>
            </Button>
          )}
        </div>
      </div>

      {/* Unified Skeleton Grid on first load */}
      {isLoading ? (
        <SkeletonGrid showFolderSkeletons={false} showDatasetSkeletons isRootView />
      ) : (
        <>
          {!filteredDbs.length ? (
            searchKeyword.trim() ? (
              <DbEmptySearchResults
                query={searchKeyword}
                onClearSearch={() => setSearchKeyword('')}
              />
            ) : isOrganizationMode && !canManage ? (
              <PersonalSpaceEmptyState
                moduleType="databases"
                icon={<Database className="w-8 h-8 text-muted-foreground" />}
              />
            ) : (
              <DbEmptyElement
                type="generic"
                actions={
                  canManage
                    ? [
                        {
                          label: t('dbs.create'),
                          icon: <Plus className="h-4 w-4" />,
                          onClick: openCreate,
                        },
                      ]
                    : []
                }
              />
            )
          ) : (
            <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-5 gap-3 sm:gap-4 md:gap-6 lg:gap-8 xl:gap-10 2xl:gap-12">
              {filteredDbs.map(db => (
                <DbCard
                  key={db.id}
                  db={db}
                  onEdit={() => openEdit(db)}
                  onDeleted={() => void refetch()}
                />
              ))}
            </div>
          )}
        </>
      )}

      <CreateDbDialog open={createDialogOpen} onOpenChange={setCreateDialogOpen} />
      <EditDbDialog
        open={editDialogOpen}
        db={selectedDb}
        onOpenChange={open => {
          setEditDialogOpen(open);
          if (!open) {
            setSelectedDb(undefined);
          }
        }}
      />
    </div>
  );
}

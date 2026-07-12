'use client';

import React, { useMemo, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { RefreshCw, Plus, Search } from 'lucide-react';
import DbCard from '@/components/db/card';
import { CreateDbDialog, EditDbDialog } from '@/components/db/dialog';
import { useDbsBasic } from '@/hooks/db/use-dbs';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import { SkeletonGrid } from '@/components/datasets/page/skeleton-grid';
import { DbEmptyElement, DbEmptySearchResults } from '@/components/db/empty-element';

import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { ShieldAlert } from 'lucide-react';
import type { Db } from '@/services/types/db';
import {
  DATABASE_PERMISSION_ACTIONS,
  DATABASE_VISIBLE_PERMISSION_CODES,
} from '@/constants/permissions';

export default function DbPage() {
  const t = useT();
  const currentWorkspace = useCurrentWorkspace();

  // Permissions
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canView = hasAnyPermission(DATABASE_VISIBLE_PERMISSION_CODES);
  const canCreateDatabase = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.create);
  const canUpdateDatabase = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.update);

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
    if (!canCreateDatabase) return;
    setSelectedDb(undefined);
    setCreateDialogOpen(true);
  };

  const openEdit = (db: Db) => {
    if (!canUpdateDatabase) return;
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
      className="flex h-full flex-col space-y-6 overflow-y-auto p-4 @md/console:space-y-8 @md/console:p-6 @5xl/console:space-y-9 @5xl/console:p-8"
      ref={scrollRef}
    >
      {/* Header */}
      <div className="mb-4 flex flex-col gap-4 @3xl/console:flex-row @3xl/console:items-center @3xl/console:justify-between">
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

        <div className="flex w-full flex-col gap-3 @3xl/console:w-auto @3xl/console:flex-row @3xl/console:items-center">
          <div className="relative w-full @3xl/console:max-w-md">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder={t('dbs.search.placeholder')}
              value={searchKeyword}
              onChange={e => setSearchKeyword(e.target.value)}
              className="pl-9"
            />
          </div>
          {canCreateDatabase && (
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
            ) : (
              <DbEmptyElement
                type="generic"
                actions={
                  canCreateDatabase
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
            <div className="grid grid-cols-[repeat(auto-fill,minmax(13rem,1fr))] gap-4">
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

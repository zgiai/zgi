'use client';

import { useEffect, useMemo, useState } from 'react';
import { Check, Database, Search } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { useDbsBasic } from '@/hooks/db/use-dbs';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { AgentDatabaseBinding } from '@/services/types/agent';
import type { Db } from '@/services/types/db';

interface AgentRuntimeDatabaseDialogProps {
  open: boolean;
  workspaceId?: string;
  bindings: AgentDatabaseBinding[];
  onOpenChange: (open: boolean) => void;
  onConfirmDatabases: (dbIds: string[]) => void;
}

export function AgentRuntimeDatabaseDialog({
  open,
  workspaceId,
  bindings,
  onOpenChange,
  onConfirmDatabases,
}: AgentRuntimeDatabaseDialogProps) {
  const t = useT('agents.agentRuntime');
  const [selectedDbIds, setSelectedDbIds] = useState<string[]>([]);
  const [dbSearch, setDbSearch] = useState('');
  const { dbs, isLoading } = useDbsBasic(
    { workspace_id: workspaceId },
    { enabled: open && Boolean(workspaceId) }
  );

  useEffect(() => {
    if (!open) return;
    setDbSearch('');
    setSelectedDbIds(bindings.map(binding => binding.data_source_id));
  }, [bindings, open]);

  useEffect(() => {
    if (!open || isLoading) return;
    const scopedDbIds = new Set(dbs.map(db => db.id));
    setSelectedDbIds(current => {
      const next = current.filter(dbId => scopedDbIds.has(dbId));
      return next.length === current.length ? current : next;
    });
  }, [dbs, isLoading, open]);

  const filteredDbs = useMemo(() => {
    const keyword = dbSearch.trim().toLowerCase();
    if (!keyword) return dbs;
    return dbs.filter(db =>
      [db.name, db.description, db.provider, db.schema_name]
        .filter(Boolean)
        .some(value => String(value).toLowerCase().includes(keyword))
    );
  }, [dbSearch, dbs]);

  const selectedCounts = useMemo(
    () => new Map(bindings.map(binding => [binding.data_source_id, binding.table_ids.length])),
    [bindings]
  );

  const toggleDatabase = (dbId: string, checked: boolean) => {
    setSelectedDbIds(current =>
      checked
        ? Array.from(new Set([...current, dbId]))
        : current.filter(selectedId => selectedId !== dbId)
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <div className="flex items-start justify-between gap-3">
            <div>
              <DialogTitle>{t('database.dialogTitle')}</DialogTitle>
              <DialogDescription>{t('database.dialogDescription')}</DialogDescription>
            </div>
            <Badge variant="subtle" className="mt-0.5 shrink-0">
              {t('database.selectedDatabasesCount', { count: selectedDbIds.length })}
            </Badge>
          </div>
        </DialogHeader>
        <DialogBody className="max-h-[560px]">
          <div className="space-y-3">
            <Input
              value={dbSearch}
              onChange={event => setDbSearch(event.target.value)}
              placeholder={t('database.searchDatabase')}
              leftIcon={<Search className="size-4" />}
            />
            {isLoading ? (
              <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
                {Array.from({ length: 4 }).map((_, index) => (
                  <Skeleton key={index} className="h-32 w-full rounded-lg" />
                ))}
              </div>
            ) : filteredDbs.length === 0 ? (
              <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
                {t('database.noDatabases')}
              </div>
            ) : (
              <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
                {filteredDbs.map(db => (
                  <DatabaseOption
                    key={db.id}
                    db={db}
                    selected={selectedDbIds.includes(db.id)}
                    selectedCount={selectedCounts.get(db.id) ?? 0}
                    onSelect={toggleDatabase}
                  />
                ))}
              </div>
            )}
          </div>
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            {t('database.cancel')}
          </Button>
          <Button
            type="button"
            disabled={isLoading || !workspaceId}
            onClick={() => {
              const scopedDbIds = new Set(dbs.map(db => db.id));
              onOpenChange(false);
              onConfirmDatabases(selectedDbIds.filter(dbId => scopedDbIds.has(dbId)));
            }}
          >
            {t('database.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DatabaseOption({
  db,
  selected,
  selectedCount,
  onSelect,
}: {
  db: Db;
  selected: boolean;
  selectedCount: number;
  onSelect: (id: string, checked: boolean) => void;
}) {
  const t = useT('agents.agentRuntime');
  const label = db.name || db.schema_name || t('database.unnamedDatabase');
  const meta = db.schema_name || db.provider;

  return (
    <button
      type="button"
      className={cn(
        'flex min-h-32 cursor-pointer flex-col rounded-lg border bg-background p-4 text-left transition-colors hover:border-primary/50 hover:bg-muted/30',
        selected ? 'border-primary bg-primary/5' : ''
      )}
      onClick={() => onSelect(db.id, !selected)}
    >
      <span className="flex items-start gap-3">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-lg border bg-muted text-primary">
          <Database className="size-5" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-semibold">{label}</span>
          {meta ? (
            <span className="mt-1 inline-flex rounded border bg-muted/40 px-1.5 py-0.5 text-[11px] text-muted-foreground">
              {meta}
            </span>
          ) : null}
        </span>
        <span
          className={cn(
            'flex size-5 shrink-0 items-center justify-center rounded-full border',
            selected ? 'border-primary bg-primary text-primary-foreground' : 'bg-background'
          )}
        >
          {selected ? <Check className="size-3.5" /> : null}
        </span>
      </span>
      <span className="mt-3 line-clamp-2 text-xs leading-5 text-muted-foreground">
        {db.description || t('database.noDescription')}
      </span>
      {selectedCount > 0 ? (
        <Badge variant="subtle" className="mt-3 w-fit">
          {t('database.selectedTablesCount', { count: selectedCount })}
        </Badge>
      ) : null}
    </button>
  );
}

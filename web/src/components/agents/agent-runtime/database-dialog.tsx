'use client';

import { useEffect, useMemo, useState } from 'react';
import { Database, Search } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
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
  bindings: AgentDatabaseBinding[];
  onOpenChange: (open: boolean) => void;
  onConfirmDatabases: (dbIds: string[]) => void;
}

export function AgentRuntimeDatabaseDialog({
  open,
  bindings,
  onOpenChange,
  onConfirmDatabases,
}: AgentRuntimeDatabaseDialogProps) {
  const t = useT('agents.agentRuntime');
  const [selectedDbIds, setSelectedDbIds] = useState<string[]>([]);
  const [dbSearch, setDbSearch] = useState('');
  const { dbs, isLoading } = useDbsBasic({}, { enabled: open });

  useEffect(() => {
    if (!open) return;
    setDbSearch('');
    setSelectedDbIds(bindings.map(binding => binding.data_source_id));
  }, [bindings, open]);

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
                  <Skeleton key={index} className="h-24 w-full" />
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
            onClick={() => {
              onOpenChange(false);
              onConfirmDatabases(selectedDbIds);
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

  return (
    <button
      type="button"
      className={cn(
        'flex min-h-24 w-full items-start gap-3 rounded-md border bg-background p-3 text-left transition-colors hover:border-primary/50 hover:bg-muted/30',
        selected && 'border-primary/80 bg-primary/[0.04] shadow-sm'
      )}
      onClick={() => onSelect(db.id, !selected)}
    >
      <Checkbox
        checked={selected}
        onCheckedChange={value => onSelect(db.id, value === true)}
        onClick={event => event.stopPropagation()}
        className="mt-1"
      />
      <span className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-muted text-primary">
        <Database className="size-4" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium">
          {db.name || db.schema_name || t('database.unnamedDatabase')}
        </span>
        {db.description || db.schema_name ? (
          <span className="mt-1.5 line-clamp-2 text-xs leading-5 text-muted-foreground">
            {db.description || db.schema_name}
          </span>
        ) : null}
        {selectedCount > 0 ? (
          <Badge variant="subtle" className="mt-2 h-5 px-1.5 text-[10px]">
            {t('database.selectedTablesCount', { count: selectedCount })}
          </Badge>
        ) : null}
      </span>
    </button>
  );
}

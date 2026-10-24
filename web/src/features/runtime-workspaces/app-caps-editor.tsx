'use client';

import * as React from 'react';
import { Plus, Search, SlidersHorizontal, Trash2 } from 'lucide-react';
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
import { Pagination } from '@/components/ui/pagination';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { RuntimeAppIcon } from '@/features/runtime-workspaces/app-presentation';
import { useT } from '@/i18n';
import type { RuntimeApp } from '@/services/types/runtime-workspace';

const capWindows = ['24h', '7d', '30d'] as const;
type CapWindow = (typeof capWindows)[number];
type CapPoints = Partial<Record<CapWindow, string>>;

export interface AppCapDraft {
  appKey: string;
  limits: CapPoints;
}

const pageSize = 10;

function hasLimit(points: CapPoints) {
  return capWindows.some(window => {
    const value = Number.parseFloat(points[window] ?? '');
    return Number.isFinite(value) && value >= 0;
  });
}

function matches(app: Pick<RuntimeApp, 'name' | 'app_key'> | undefined, key: string, q: string) {
  if (!q) return true;
  return key.toLowerCase().includes(q) || Boolean(app?.name.toLowerCase().includes(q));
}

/**
 * @component AppCapsEditor
 * @category Feature
 * @status Beta
 * @description Edits the per-app caps of the end-user quota for any number of
 * apps: a searchable, paged table with inline limits, adding caps for many
 * apps at once, and setting or removing the caps of the selected apps.
 * @usage Rendered inside EndUserQuotaCard; changes stay a draft until the
 * card saves.
 * @example
 * <AppCapsEditor apps={apps} caps={caps} onChange={setCaps} />
 */
export function AppCapsEditor({
  apps,
  caps,
  onChange,
}: {
  apps: RuntimeApp[];
  caps: AppCapDraft[];
  onChange: (caps: AppCapDraft[]) => void;
}) {
  const t = useT('workspace');
  const [search, setSearch] = React.useState('');
  const [page, setPage] = React.useState(1);
  const [selected, setSelected] = React.useState<Set<string>>(new Set());
  const [dialog, setDialog] = React.useState<'add' | 'bulk' | null>(null);
  const appsByKey = React.useMemo(() => new Map(apps.map(app => [app.app_key, app])), [apps]);
  const capped = new Set(caps.map(cap => cap.appKey));
  const uncapped = apps.filter(app => !capped.has(app.app_key));

  const q = search.trim().toLowerCase();
  const rows = caps.filter(cap => matches(appsByKey.get(cap.appKey), cap.appKey, q));
  const totalPages = Math.max(1, Math.ceil(rows.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pageRows = rows.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  const pageSelected = pageRows.length > 0 && pageRows.every(cap => selected.has(cap.appKey));
  const selectedCount = caps.filter(cap => selected.has(cap.appKey)).length;

  const setLimits = (appKey: string, limits: CapPoints) =>
    onChange(caps.map(cap => (cap.appKey === appKey ? { ...cap, limits } : cap)));
  const remove = (keys: Set<string>) => {
    onChange(caps.filter(cap => !keys.has(cap.appKey)));
    setSelected(current => new Set([...current].filter(key => !keys.has(key))));
  };
  const togglePage = () =>
    setSelected(current => {
      const next = new Set(current);
      for (const cap of pageRows) {
        if (pageSelected) next.delete(cap.appKey);
        else next.add(cap.appKey);
      }
      return next;
    });

  return (
    <section className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <h4 className="text-sm font-medium">{t('runtime.quota.perApp')}</h4>
          {caps.length > 0 ? (
            <Badge variant="subtle" className="font-normal">
              {t('runtime.quota.capsCount', { count: caps.length })}
            </Badge>
          ) : null}
        </div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={uncapped.length === 0}
          title={uncapped.length === 0 ? t('runtime.quota.allCapped') : undefined}
          onClick={() => setDialog('add')}
        >
          <Plus className="size-4" />
          {t('runtime.quota.addApp')}
        </Button>
      </div>

      {caps.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('runtime.quota.noAppCaps')}</p>
      ) : (
        <div className="space-y-2">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <div className="relative min-w-0 flex-1">
              <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={event => {
                  setSearch(event.target.value);
                  setPage(1);
                }}
                placeholder={t('runtime.quota.searchCaps')}
                className="h-9 pl-9"
              />
            </div>
            {selectedCount > 0 ? (
              <div className="flex items-center gap-2">
                <Button type="button" size="sm" variant="outline" onClick={() => setDialog('bulk')}>
                  <SlidersHorizontal className="size-4" />
                  {t('runtime.quota.bulkSet', { count: selectedCount })}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className="text-destructive"
                  onClick={() => remove(selected)}
                >
                  <Trash2 className="size-4" />
                  {t('runtime.quota.removeSelected', { count: selectedCount })}
                </Button>
              </div>
            ) : null}
          </div>
          <div className="overflow-x-auto rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10">
                    <Checkbox
                      checked={pageSelected}
                      onCheckedChange={togglePage}
                      aria-label={t('runtime.appsTable.selectPage')}
                    />
                  </TableHead>
                  <TableHead>{t('runtime.appsTable.columns.app')}</TableHead>
                  {capWindows.map(window => (
                    <TableHead key={window} className="w-28">
                      {t(`runtime.quota.windows.${window}`)}
                    </TableHead>
                  ))}
                  <TableHead className="w-12" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {pageRows.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={capWindows.length + 3}
                      className="py-8 text-center text-sm text-muted-foreground"
                    >
                      {t('runtime.quota.noMatchingApps')}
                    </TableCell>
                  </TableRow>
                ) : (
                  pageRows.map(cap => {
                    const app = appsByKey.get(cap.appKey);
                    return (
                      <TableRow key={cap.appKey}>
                        <TableCell>
                          <Checkbox
                            checked={selected.has(cap.appKey)}
                            onCheckedChange={() =>
                              setSelected(current => {
                                const next = new Set(current);
                                if (next.has(cap.appKey)) next.delete(cap.appKey);
                                else next.add(cap.appKey);
                                return next;
                              })
                            }
                            aria-label={t('runtime.appsTable.selectRow')}
                          />
                        </TableCell>
                        <TableCell>
                          <div className="flex min-w-0 items-center gap-2">
                            {app ? <RuntimeAppIcon app={app} /> : null}
                            <div className="min-w-0">
                              <div className="truncate text-sm font-medium">
                                {app?.name ?? cap.appKey}
                              </div>
                              {app ? (
                                <code className="block truncate text-[11px] text-muted-foreground">
                                  {cap.appKey}
                                </code>
                              ) : (
                                <span className="text-xs text-warning">
                                  {t('runtime.quota.appGone')}
                                </span>
                              )}
                            </div>
                          </div>
                        </TableCell>
                        {capWindows.map(window => (
                          <TableCell key={window}>
                            <Input
                              type="number"
                              min={0}
                              step="any"
                              inputMode="decimal"
                              placeholder={t('runtime.quota.unlimited')}
                              aria-label={t(`runtime.quota.windows.${window}`)}
                              value={cap.limits[window] ?? ''}
                              onChange={event =>
                                setLimits(cap.appKey, {
                                  ...cap.limits,
                                  [window]: event.target.value,
                                })
                              }
                              className="h-8"
                            />
                          </TableCell>
                        ))}
                        <TableCell>
                          <Button
                            type="button"
                            size="sm"
                            variant="ghost"
                            className="size-8 p-0"
                            aria-label={t('runtime.quota.removeApp')}
                            onClick={() => remove(new Set([cap.appKey]))}
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        </TableCell>
                      </TableRow>
                    );
                  })
                )}
              </TableBody>
            </Table>
          </div>
          {rows.length > pageSize ? (
            <Pagination
              currentPage={currentPage}
              totalPages={totalPages}
              total={rows.length}
              pageSize={pageSize}
              onPageChange={setPage}
            />
          ) : null}
        </div>
      )}

      <AppCapsDialog
        mode={dialog}
        apps={uncapped}
        selectedCount={selectedCount}
        onOpenChange={open => (!open ? setDialog(null) : undefined)}
        onApply={(limits, appKeys) => {
          if (dialog === 'add') {
            onChange([...caps, ...appKeys.map(appKey => ({ appKey, limits }))]);
            setSearch('');
            setPage(Math.ceil((caps.length + appKeys.length) / pageSize));
          } else {
            onChange(caps.map(cap => (selected.has(cap.appKey) ? { ...cap, limits } : cap)));
          }
          setDialog(null);
        }}
      />
    </section>
  );
}

function AppCapsDialog({
  mode,
  apps,
  selectedCount,
  onOpenChange,
  onApply,
}: {
  mode: 'add' | 'bulk' | null;
  /** Apps that have no cap yet, offered when adding. */
  apps: RuntimeApp[];
  selectedCount: number;
  onOpenChange: (open: boolean) => void;
  onApply: (limits: CapPoints, appKeys: string[]) => void;
}) {
  const t = useT('workspace');
  const tCommon = useT('common');
  const [search, setSearch] = React.useState('');
  const [picked, setPicked] = React.useState<Set<string>>(new Set());
  const [limits, setLimits] = React.useState<CapPoints>({});
  // Keep the last mode while the dialog animates closed.
  const [shownMode, setShownMode] = React.useState<'add' | 'bulk'>('add');
  React.useEffect(() => {
    if (mode) {
      setShownMode(mode);
      setSearch('');
      setPicked(new Set());
      setLimits({});
    }
  }, [mode]);

  const view = mode ?? shownMode;
  const q = search.trim().toLowerCase();
  const visible = apps.filter(app => matches(app, app.app_key, q));
  const allVisiblePicked = visible.length > 0 && visible.every(app => picked.has(app.app_key));
  const count = view === 'add' ? picked.size : selectedCount;
  const limitSet = hasLimit(limits);

  return (
    <Dialog open={mode !== null} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {view === 'add'
              ? t('runtime.quota.addTitle')
              : t('runtime.quota.bulkTitle', { count: selectedCount })}
          </DialogTitle>
          <DialogDescription>
            {view === 'add'
              ? t('runtime.quota.addDescription')
              : t('runtime.quota.bulkDescription')}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {view === 'add' ? (
            <div className="space-y-2">
              <div className="relative">
                <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={search}
                  onChange={event => setSearch(event.target.value)}
                  placeholder={t('runtime.quota.searchCaps')}
                  className="h-9 pl-9"
                />
              </div>
              <div className="max-h-[40vh] divide-y overflow-y-auto rounded-lg border">
                {visible.length === 0 ? (
                  <p className="px-4 py-8 text-center text-sm text-muted-foreground">
                    {t('runtime.quota.noMatchingApps')}
                  </p>
                ) : (
                  <>
                    <label className="flex cursor-pointer items-center gap-3 bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
                      <Checkbox
                        checked={allVisiblePicked}
                        onCheckedChange={() =>
                          setPicked(current => {
                            const next = new Set(current);
                            for (const app of visible) {
                              if (allVisiblePicked) next.delete(app.app_key);
                              else next.add(app.app_key);
                            }
                            return next;
                          })
                        }
                      />
                      {t('runtime.quota.selectAllMatching', { count: visible.length })}
                    </label>
                    {visible.map(app => (
                      <label
                        key={app.app_key}
                        className="flex cursor-pointer items-center gap-3 px-3 py-2 hover:bg-muted/30"
                      >
                        <Checkbox
                          checked={picked.has(app.app_key)}
                          onCheckedChange={() =>
                            setPicked(current => {
                              const next = new Set(current);
                              if (next.has(app.app_key)) next.delete(app.app_key);
                              else next.add(app.app_key);
                              return next;
                            })
                          }
                        />
                        <RuntimeAppIcon app={app} />
                        <span className="min-w-0 flex-1 truncate text-sm">{app.name}</span>
                        <code className="shrink-0 text-[11px] text-muted-foreground">
                          {app.app_key}
                        </code>
                      </label>
                    ))}
                  </>
                )}
              </div>
            </div>
          ) : null}
          <div className="grid grid-cols-3 gap-2">
            {capWindows.map(window => (
              <label key={window} className="space-y-1">
                <span className="text-xs text-muted-foreground">
                  {t(`runtime.quota.windows.${window}`)}
                </span>
                <Input
                  type="number"
                  min={0}
                  step="any"
                  inputMode="decimal"
                  placeholder={t('runtime.quota.unlimited')}
                  value={limits[window] ?? ''}
                  onChange={event => setLimits({ ...limits, [window]: event.target.value })}
                  className="h-9"
                />
              </label>
            ))}
          </div>
          {!limitSet ? (
            <p className="text-xs text-muted-foreground">{t('runtime.quota.limitRequired')}</p>
          ) : null}
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {tCommon('cancel')}
          </Button>
          <Button
            type="button"
            disabled={count === 0 || !limitSet}
            onClick={() => onApply(limits, view === 'add' ? [...picked] : [])}
          >
            {view === 'add'
              ? t('runtime.quota.applyAdd', { count })
              : t('runtime.quota.applyBulk', { count })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

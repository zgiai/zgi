'use client';

import React, { useState, useMemo, useCallback } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Badge } from '@/components/ui/badge';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Separator } from '@/components/ui/separator';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Eye } from 'lucide-react';
import { useT } from '@/i18n';
import type {
  ModelMetaDiffResponse,
  ModelChange,
  ModelMetaSyncResult,
} from '@/services/types/provider';
import ModelDiffDetailsDialog from './model-diff-details-dialog';

interface ModelDiffDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  diffData: ModelMetaDiffResponse | null;
  onSyncSelected: (models: string[]) => void;
  isSyncing: boolean;
  onSyncProviderFull?: () => void;
  isSyncingProvider?: boolean;
  syncFeedback?: ModelMetaSyncResult | null;
  syncBlockedState?: 'readonly' | 'forbidden' | null;
}

export default function ModelDiffDialog({
  open,
  onOpenChange,
  diffData,
  onSyncSelected,
  isSyncing,
  onSyncProviderFull,
  isSyncingProvider = false,
  syncFeedback,
  syncBlockedState = null,
}: ModelDiffDialogProps): JSX.Element {
  const t = useT('aiProviders');
  const tCommon = useT('common');

  const [selectedNew, setSelectedNew] = useState<Set<string>>(new Set());
  const [selectedUpdated, setSelectedUpdated] = useState<Set<string>>(new Set());
  const [detailItem, setDetailItem] = useState<ModelChange | null>(null);
  const [detailsOpen, setDetailsOpen] = useState(false);

  // Reset selections when dialog opens or diffData changes
  React.useEffect(() => {
    if (open && diffData) {
      const allNew = new Set(diffData.changes.new.map(m => m.model));
      const allUpdated = new Set(diffData.changes.updated.map(m => m.model));
      setSelectedNew(allNew);
      setSelectedUpdated(allUpdated);
    }
  }, [open, diffData]);

  const toggleNew = useCallback((model: string) => {
    setSelectedNew(prev => {
      const next = new Set(prev);
      if (next.has(model)) {
        next.delete(model);
      } else {
        next.add(model);
      }
      return next;
    });
  }, []);

  const toggleUpdated = useCallback((model: string) => {
    setSelectedUpdated(prev => {
      const next = new Set(prev);
      if (next.has(model)) {
        next.delete(model);
      } else {
        next.add(model);
      }
      return next;
    });
  }, []);

  const toggleAllNew = useCallback(() => {
    if (!diffData) return;
    setSelectedNew(prev =>
      prev.size === diffData.changes.new.length
        ? new Set()
        : new Set(diffData.changes.new.map(m => m.model))
    );
  }, [diffData]);

  const toggleAllUpdated = useCallback(() => {
    if (!diffData) return;
    setSelectedUpdated(prev =>
      prev.size === diffData.changes.updated.length
        ? new Set()
        : new Set(diffData.changes.updated.map(m => m.model))
    );
  }, [diffData]);

  const selectedModels = useMemo(
    () => [...selectedNew, ...selectedUpdated],
    [selectedNew, selectedUpdated]
  );

  const handleSync = () => {
    if (selectedModels.length > 0) {
      onSyncSelected(selectedModels);
    }
  };

  const handleViewDetails = (model: ModelChange, e: React.MouseEvent) => {
    e.stopPropagation();
    setDetailItem(model);
    setDetailsOpen(true);
  };

  if (!diffData) return <></>;

  const { summary, changes } = diffData;

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="max-w-3xl h-[80vh] p-0 flex flex-col">
          <DialogHeader className="px-6 pt-6 pb-4">
            <DialogTitle>{t('diff.title') || 'Model Updates'}</DialogTitle>
            <DialogDescription>
              {t('diff.description') || 'Select models to synchronize from ModelMeta'}
            </DialogDescription>
          </DialogHeader>

          <DialogBody className="px-6 pb-6 flex-1 overflow-hidden flex flex-col gap-4">
            {syncBlockedState === 'readonly' ? (
              <Alert className="border-warning/40 bg-warning/5">
                <AlertTitle>{t('syncStatus.readonlyTitle')}</AlertTitle>
                <AlertDescription>{t('syncStatus.readonlyDescription')}</AlertDescription>
              </Alert>
            ) : null}

            {syncBlockedState === 'forbidden' ? (
              <Alert variant="destructive">
                <AlertTitle>{t('syncStatus.forbiddenTitle')}</AlertTitle>
                <AlertDescription>{t('syncStatus.forbiddenDescription')}</AlertDescription>
              </Alert>
            ) : null}

            {syncFeedback && syncFeedback.status !== 'success' ? (
              <Alert
                variant={syncFeedback.status === 'failed' ? 'destructive' : 'default'}
                className={
                  syncFeedback.status === 'partial'
                    ? 'border-warning/40 bg-warning/5'
                    : undefined
                }
              >
                <AlertTitle>
                  {syncFeedback.status === 'partial'
                    ? t('syncResult.partialTitle', {
                        success: syncFeedback.success_models,
                        failed: syncFeedback.failed_models,
                      })
                    : t('syncResult.failedTitle', {
                        failed: syncFeedback.failed_models,
                      })}
                </AlertTitle>
                <AlertDescription className="space-y-2">
                  {syncFeedback.errors && syncFeedback.errors.length > 0 ? (
                    <div className="space-y-1">
                      <p className="font-medium">{t('syncResult.errorsTitle')}</p>
                      {syncFeedback.errors.map(error => (
                        <div
                          key={error}
                          className="rounded-md border bg-background/60 px-3 py-2 text-xs"
                        >
                          {error}
                        </div>
                      ))}
                    </div>
                  ) : null}
                </AlertDescription>
              </Alert>
            ) : null}

            {/* Summary */}
            <div className="flex gap-3 items-center text-sm">
              <Badge variant="default" className="bg-green-600">
                {summary.new_models} {t('diff.new') || 'New'}
              </Badge>
              <Badge variant="default" className="bg-blue-600">
                {summary.updated_models} {t('diff.updated') || 'Updated'}
              </Badge>
              {summary.deprecated_models > 0 ? (
                <Badge variant="outline">
                  {summary.deprecated_models} {t('diff.deleted') || 'Pending deprecation'}
                </Badge>
              ) : null}
            </div>

            <Separator />

            {/* Model lists */}
            <ScrollArea className="flex-1">
              <div className="space-y-6 pr-4">
                {/* New models */}
                {changes.new.length > 0 && (
                  <div className="space-y-3">
                    <div className="flex items-center justify-between">
                      <h3 className="font-semibold text-sm flex items-center gap-2">
                        <Badge variant="default" className="bg-green-600">
                          {t('diff.new') || 'New'}
                        </Badge>
                        <span>
                          {selectedNew.size} / {changes.new.length}{' '}
                          {t('diff.selected') || 'selected'}
                        </span>
                      </h3>
                      <Button variant="ghost" size="sm" onClick={toggleAllNew}>
                        {selectedNew.size === changes.new.length
                          ? t('diff.deselectAll') || 'Deselect All'
                          : t('diff.selectAll') || 'Select All'}
                      </Button>
                    </div>
                    <div className="space-y-2">
                      {changes.new.map(model => (
                        <div
                          key={model.model}
                          className="flex items-center gap-3 p-3 rounded-lg border hover:bg-accent cursor-pointer group"
                          onClick={() => toggleNew(model.model)}
                        >
                          <Checkbox
                            checked={selectedNew.has(model.model)}
                            onCheckedChange={() => toggleNew(model.model)}
                            onClick={e => e.stopPropagation()}
                          />
                          <div className="flex-1 min-w-0">
                            <div className="font-medium text-sm">
                              {model.model_name || model.model}
                            </div>
                            <div className="text-xs text-muted-foreground">{model.model}</div>
                          </div>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="opacity-0 group-hover:opacity-100 transition-opacity"
                            onClick={e => handleViewDetails(model, e)}
                          >
                            <Eye className="h-4 w-4 mr-1" />
                            {t('diff.viewDetails') || 'Details'}
                          </Button>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Updated models */}
                {changes.updated.length > 0 && (
                  <div className="space-y-3">
                    <div className="flex items-center justify-between">
                      <h3 className="font-semibold text-sm flex items-center gap-2">
                        <Badge variant="default" className="bg-blue-600">
                          {t('diff.updated') || 'Updated'}
                        </Badge>
                        <span>
                          {selectedUpdated.size} / {changes.updated.length}{' '}
                          {t('diff.selected') || 'selected'}
                        </span>
                      </h3>
                      <Button variant="ghost" size="sm" onClick={toggleAllUpdated}>
                        {selectedUpdated.size === changes.updated.length
                          ? t('diff.deselectAll') || 'Deselect All'
                          : t('diff.selectAll') || 'Select All'}
                      </Button>
                    </div>
                    <div className="space-y-2">
                      {changes.updated.map(model => (
                        <div
                          key={model.model}
                          className="flex items-center gap-3 p-3 rounded-lg border hover:bg-accent cursor-pointer group"
                          onClick={() => toggleUpdated(model.model)}
                        >
                          <Checkbox
                            checked={selectedUpdated.has(model.model)}
                            onCheckedChange={() => toggleUpdated(model.model)}
                            onClick={e => e.stopPropagation()}
                          />
                          <div className="flex-1 min-w-0">
                            <div className="font-medium text-sm">
                              {model.model_name || model.model}
                            </div>
                            <div className="text-xs text-muted-foreground flex items-center gap-2">
                              <span>{model.model}</span>
                              {model.diff_fields && model.diff_fields.length > 0 && (
                                <Badge variant="outline" className="text-[10px] h-4">
                                  {model.diff_fields.length} changes
                                </Badge>
                              )}
                            </div>
                          </div>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="opacity-0 group-hover:opacity-100 transition-opacity"
                            onClick={e => handleViewDetails(model, e)}
                          >
                            <Eye className="h-4 w-4 mr-1" />
                            {t('diff.viewDetails') || 'Details'}
                          </Button>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {changes.deprecated.length > 0 && (
                  <div className="space-y-3">
                    <div className="flex items-center justify-between">
                      <h3 className="font-semibold text-sm flex items-center gap-2">
                        <Badge variant="outline">{t('diff.deleted') || 'Pending deprecation'}</Badge>
                        <span>
                          {changes.deprecated.length} {t('diff.deleted') || 'Pending deprecation'}
                        </span>
                      </h3>
                    </div>
                    <div className="rounded-lg border border-dashed bg-muted/20 p-3 text-xs text-muted-foreground">
                      {t('diff.deletedNote')}
                      <div className="mt-1">{t('diff.fullSyncForDeleted')}</div>
                    </div>
                    <div className="space-y-2">
                      {changes.deprecated.map(model => (
                        <div key={model.model} className="flex items-center gap-3 p-3 rounded-lg border">
                          <div className="flex-1 min-w-0">
                            <div className="font-medium text-sm">{model.model_name || model.model}</div>
                            <div className="text-xs text-muted-foreground">{model.model}</div>
                          </div>
                          <Badge variant="outline">{t('diff.deleted') || 'Pending deprecation'}</Badge>
                        </div>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            </ScrollArea>

            {/* Footer actions */}
            <div className="flex items-center justify-end gap-2 pt-4 border-t">
              <Button
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={isSyncing || isSyncingProvider}
              >
                {tCommon('cancel')}
              </Button>
              {onSyncProviderFull ? (
                <Button
                  variant="secondary"
                  onClick={onSyncProviderFull}
                  disabled={isSyncing || isSyncingProvider || syncBlockedState !== null}
                >
                  {isSyncingProvider
                    ? t('syncStatus.syncingProvider')
                    : t('syncStatus.syncProviderAndModels')}
                </Button>
              ) : null}
              <Button
                onClick={handleSync}
                disabled={
                  isSyncing ||
                  isSyncingProvider ||
                  syncBlockedState !== null ||
                  selectedModels.length === 0
                }
              >
                {isSyncing
                  ? t('sidebar.syncingModels') || 'Syncing...'
                  : `${t('diff.syncSelected') || 'Sync Selected'} (${selectedModels.length})`}
              </Button>
            </div>
          </DialogBody>
        </DialogContent>
      </Dialog>

      {/* Details Dialog */}
      <ModelDiffDetailsDialog open={detailsOpen} onOpenChange={setDetailsOpen} item={detailItem} />
    </>
  );
}

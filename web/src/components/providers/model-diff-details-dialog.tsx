'use client';

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Badge } from '@/components/ui/badge';
import { cn } from '@/lib/utils';
import { ArrowRight, Info, CheckCircle2 } from 'lucide-react';
import { useT } from '@/i18n';
import type { ModelChange, DiffField } from '@/services/types/provider';

interface ModelDiffDetailsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  item: ModelChange | null;
}

// Field display labels mapping
const FIELD_LABELS: Record<string, string> = {
  display_name: 'Display Name',
  model_name: 'Model Name',
  status: 'Status',
  tagline: 'Tagline',
  description: 'Description',
  context_window: 'Context Window',
  max_output_tokens: 'Max Output Tokens',
  input_price: 'Input Price',
  output_price: 'Output Price',
  currency: 'Currency',
  is_flagship: 'Flagship',
  is_recommended: 'Recommended',
  is_featured: 'Featured',
  is_new: 'New',
  endpoints: 'Endpoints',
  features: 'Features',
  tools: 'Tools',
  input_modalities: 'Input Modalities',
  output_modalities: 'Output Modalities',
};

export default function ModelDiffDetailsDialog({
  open,
  onOpenChange,
  item,
}: ModelDiffDetailsDialogProps): JSX.Element {
  const t = useT('aiProviders');
  const tCommon = useT('common');

  if (!item) return <></>;

  const isNew = item.change_type === 'new';
  const diffFields = item.diff_fields || [];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl h-[80vh] p-0 flex flex-col overflow-hidden">
        <DialogHeader className="px-6 pt-6 bg-muted/10">
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <DialogTitle className="text-xl">
                {isNew
                  ? t('diffDetails.newModelTitle') || 'New Model Definition'
                  : t('diffDetails.compareTitle') || 'Field Level Comparison'}
                : {item.model_name}
              </DialogTitle>
              <DialogDescription>
                {isNew
                  ? t('diffDetails.newModelDescription') ||
                    'Review the metadata that will be registered for this new model.'
                  : t('diffDetails.compareDescription') ||
                    'Review individual field changes detected between local and remote data.'}
              </DialogDescription>
            </div>
            <div className="flex items-center gap-2">
              <Badge
                variant={isNew ? 'default' : 'outline'}
                className={cn('h-6 font-mono', isNew && 'bg-green-600')}
              >
                {isNew ? t('diff.new') || 'NEW' : t('diff.updated') || 'UPDATE'}
              </Badge>
              <Badge variant="outline" className="h-6 font-mono">
                {item.model}
              </Badge>
            </div>
          </div>
        </DialogHeader>

        <DialogBody className="flex-1 px-6 overflow-hidden">
          <ScrollArea className="h-full">
            <div className="py-6 space-y-4">
              {diffFields.length === 0 && !isNew ? (
                <div className="flex flex-col items-center justify-center py-12 text-muted-foreground opacity-50">
                  <CheckCircle2 className="h-12 w-12 mb-2" />
                  <p>{t('diffDetails.noChanges') || 'No changes detected for this model.'}</p>
                </div>
              ) : isNew ? (
                // For new models, show remote_data fields
                <NewModelFields remoteData={item.remote_data} />
              ) : (
                // For updated models, show diff fields
                diffFields.map(diff => <FieldDiffItem key={diff.field} diff={diff} />)
              )}
            </div>
          </ScrollArea>
        </DialogBody>

        <div className="px-6 py-4 border-t bg-muted/10 flex items-center justify-between">
          <div className="text-sm text-muted-foreground flex items-center gap-2">
            <Info className="h-4 w-4" />
            <span>
              {isNew ? (
                <>
                  <span className="font-bold text-foreground">
                    {Object.keys(item.remote_data || {}).length}
                  </span>{' '}
                  {t('diffDetails.fieldsInitialized') || 'fields will be initialized'}
                </>
              ) : (
                <>
                  <span className="font-bold text-foreground">{diffFields.length}</span>{' '}
                  {t('diffDetails.differencesFound') || 'differences found in this model'}
                </>
              )}
            </span>
          </div>
          <Button variant="default" onClick={() => onOpenChange(false)}>
            {tCommon('close') || 'Close'}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

// Component for displaying new model fields
function NewModelFields({ remoteData }: { remoteData?: unknown }) {
  if (!remoteData || typeof remoteData !== 'object' || Array.isArray(remoteData)) return null;

  // Filter out internal fields and show relevant ones
  const displayFields = Object.entries(remoteData).filter(
    ([key]) => !['id', 'object', 'created_at', 'updated_at', 'last_updated'].includes(key)
  );

  return (
    <div className="space-y-3">
      {displayFields.map(([field, value]) => (
        <div key={field} className="rounded-xl border bg-card/50 overflow-hidden shadow-sm">
          <div className="px-4 py-2.5 border-b bg-muted/30 flex items-center justify-between">
            <span className="text-xs font-bold font-mono tracking-tight">
              {FIELD_LABELS[field] || formatFieldLabel(field)}
            </span>
            <span className="text-[10px] text-muted-foreground opacity-60 font-mono">{field}</span>
          </div>
          <div className="p-4">
            <div className="space-y-2">
              <span className="text-[10px] uppercase font-black text-muted-foreground/40 tracking-widest">
                Initial Value
              </span>
              <div className="text-sm font-mono p-3 rounded-lg bg-muted/30 border border-muted/50 text-foreground break-all overflow-auto max-h-[120px]">
                {formatValue(value)}
              </div>
            </div>
          </div>
        </div>
      ))}
    </div>
  );
}

// Component for displaying a single field diff
function FieldDiffItem({ diff }: { diff: DiffField }) {
  return (
    <div className="rounded-xl border bg-card/50 overflow-hidden shadow-sm transition-all hover:border-muted-foreground/20">
      <div className="px-4 py-2.5 border-b bg-muted/30 flex items-center justify-between">
        <span className="text-xs font-bold font-mono tracking-tight">
          {FIELD_LABELS[diff.field] || formatFieldLabel(diff.field)}
        </span>
        <span className="text-[10px] text-muted-foreground opacity-60 font-mono">{diff.field}</span>
      </div>
      <div className="p-4 grid grid-cols-2 gap-8 relative">
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 z-10 opacity-30">
          <ArrowRight className="h-5 w-5 text-muted-foreground" />
        </div>

        <div className="space-y-2 min-w-0">
          <span className="text-[10px] uppercase font-black text-muted-foreground/40 tracking-widest">
            Local
          </span>
          <div className="text-sm font-mono p-3 rounded-lg bg-red-50/20 border border-red-200/20 text-red-700/80 dark:bg-red-950/20 dark:border-red-800/20 dark:text-red-300/80 line-through break-all overflow-auto max-h-[120px]">
            {formatValue(diff.old_value)}
          </div>
        </div>

        <div className="space-y-2 min-w-0">
          <span className="text-[10px] uppercase font-black text-muted-foreground/40 tracking-widest">
            Remote
          </span>
          <div className="text-sm font-mono p-3 rounded-lg bg-green-50/40 border border-green-200/40 text-green-700 dark:bg-green-950/40 dark:border-green-800/40 dark:text-green-300 font-bold shadow-inner break-all overflow-auto max-h-[120px]">
            {formatValue(diff.new_value)}
          </div>
        </div>
      </div>
    </div>
  );
}

// Format field name for display
function formatFieldLabel(field: string): string {
  return field
    .split('_')
    .map(word => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ');
}

// Format value for display
function formatValue(value: unknown): string {
  if (value === null || value === undefined) return 'N/A';
  if (typeof value === 'boolean') return value ? 'TRUE' : 'FALSE';
  if (typeof value === 'object') {
    try {
      return JSON.stringify(value, null, 2);
    } catch {
      return 'Object';
    }
  }
  return String(value);
}

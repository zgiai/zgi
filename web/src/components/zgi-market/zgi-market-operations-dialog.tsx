'use client';

import { useEffect, useMemo, useState } from 'react';
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
import { Switch } from '@/components/ui/switch';
import { ZgiMarketCallPrice } from '@/components/zgi-market/zgi-market-call-price';
import { bindableOperations } from '@/hooks/use-zgi-market-subject-tools';
import { useT } from '@/i18n';
import type { MarketToolCandidate } from '@/services/types/zgi-market';

interface ZgiMarketOperationsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The API offering whose operations are chosen. */
  listing: MarketToolCandidate;
  /** The offering as it is named in the row that opened the dialog. */
  name: string;
  /** The operation keys bound now; undefined when every bindable operation is. */
  selected: string[] | undefined;
  saving?: boolean;
  /** The label of the confirming button: adding the offering, or saving a change. */
  confirmLabel: string;
  /** Called with the chosen keys, or undefined for every bindable operation. */
  onConfirm: (operations: string[] | undefined) => void;
}

/**
 * Chooses which operations of an API offering become tools of an agent or a digital human: all of
 * them, also the ones a later version adds, or some. An operation without a usable input schema
 * is listed with the reason and cannot be chosen.
 */
export function ZgiMarketOperationsDialog({
  open,
  onOpenChange,
  listing,
  name,
  selected,
  saving = false,
  confirmLabel,
  onConfirm,
}: ZgiMarketOperationsDialogProps) {
  const t = useT();
  const operations = listing.operations ?? [];
  const bindable = useMemo(() => bindableOperations(listing), [listing]);
  const [all, setAll] = useState(selected === undefined);
  const [chosen, setChosen] = useState<string[]>(selected ?? bindable.map(item => item.key));

  // Each opening starts from what is bound, not from what was last left in the dialog.
  useEffect(() => {
    if (open) {
      setAll(selected === undefined);
      setChosen(selected ?? bindable.map(item => item.key));
    }
  }, [open, selected, bindable]);

  const toggle = (key: string, checked: boolean) =>
    setChosen(current => (checked ? [...current, key] : current.filter(item => item !== key)));
  const count = all ? bindable.length : chosen.length;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('market.tools.operations.title', { name })}</DialogTitle>
          <DialogDescription>{t('market.tools.operations.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <div className="flex items-start justify-between gap-4 rounded-lg border bg-muted/30 p-3">
            <div className="min-w-0">
              <p id="zgi-market-operations-all" className="text-sm font-medium">
                {t('market.tools.operations.all', { count: bindable.length })}
              </p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t('market.tools.operations.allHint')}
              </p>
            </div>
            <Switch
              checked={all}
              aria-labelledby="zgi-market-operations-all"
              onCheckedChange={setAll}
            />
          </div>
          <ul className="max-h-[360px] divide-y overflow-y-auto rounded-lg border">
            {operations.map(operation => {
              const id = `zgi-market-operation-${operation.key}`;
              const checked = operation.bindable && (all || chosen.includes(operation.key));
              return (
                <li key={operation.key} className="flex items-start gap-3 px-3 py-2.5">
                  <Checkbox
                    id={id}
                    className="mt-0.5"
                    checked={checked}
                    disabled={all || !operation.bindable}
                    onCheckedChange={value => toggle(operation.key, value === true)}
                  />
                  <label htmlFor={id} className="min-w-0 flex-1">
                    <span className="flex flex-wrap items-center gap-2">
                      <Badge variant="outline" className="font-mono text-[10px]">
                        {operation.method}
                      </Badge>
                      <span className="break-all font-mono text-xs">{operation.path}</span>
                      {operation.bindable ? (
                        <Badge
                          variant={operation.risk_level === 'read' ? 'subtle' : 'warning'}
                          className="text-[10px]"
                        >
                          {t(`market.catalog.risk.${operation.risk_level}`)}
                        </Badge>
                      ) : null}
                    </span>
                    <span className="mt-1 block text-xs leading-5 text-muted-foreground">
                      {operation.bindable
                        ? operation.description
                        : t(
                            operation.reason === 'input_schema_not_object'
                              ? 'market.tools.operations.reasonNotObject'
                              : 'market.tools.operations.reasonNoSchema'
                          )}
                    </span>
                    {operation.bindable ? (
                      <span className="mt-0.5 block break-all font-mono text-[11px] text-muted-foreground">
                        {operation.name}
                      </span>
                    ) : null}
                  </label>
                </li>
              );
            })}
          </ul>
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <span>{t('market.tools.operations.price')}</span>
            <ZgiMarketCallPrice listing={listing} />
            <span>{t('market.tools.operations.confirmNote')}</span>
          </div>
        </DialogBody>
        <DialogFooter>
          <span className="mr-auto text-xs text-muted-foreground" aria-live="polite">
            {t('market.tools.operations.chosen', { count, total: bindable.length })}
          </span>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            {t('market.tools.operations.cancel')}
          </Button>
          <Button
            type="button"
            disabled={saving || count === 0}
            onClick={() => onConfirm(all ? undefined : chosen)}
          >
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

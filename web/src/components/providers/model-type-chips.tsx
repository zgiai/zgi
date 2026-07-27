'use client';

import React, { useMemo } from 'react';
import { cn } from '@/lib/utils';
import type { ModelUseCase } from '@/services/types/model';
import { USE_CASE_ORDER } from '@/config/model-colors';
import { useT } from '@/i18n';

interface ModelTypeChipsProps {
  availableTypes: Set<ModelUseCase>;
  selectedType: ModelUseCase | null;
  onSelect: (type: ModelUseCase | null) => void;
}

export default function ModelTypeChips({
  availableTypes,
  selectedType,
  onSelect,
}: ModelTypeChipsProps): JSX.Element {
  const types = useMemo(
    () => USE_CASE_ORDER.filter(type => availableTypes.has(type)),
    [availableTypes]
  );
  const t = useT();
  const isAllSelected = selectedType === null;

  return (
    <div className="flex items-center gap-1.5 flex-wrap">
      <button
        type="button"
        onClick={() => onSelect(null)}
        className={cn(
          'inline-flex items-center rounded-md border px-3 py-1.5 text-xs font-medium transition-colors',
          isAllSelected
            ? 'border-primary/30 bg-primary/10 text-primary'
            : 'border-border bg-background text-muted-foreground hover:bg-muted hover:text-foreground'
        )}
      >
        {t('aiProviders.models.filters.allTypes')}
      </button>
      {types.map(type => {
        const selected = selectedType === type;
        return (
          <button
            key={type}
            type="button"
            onClick={() => onSelect(selected ? null : type)}
            className={cn(
              'inline-flex items-center rounded-md border px-3 py-1.5 text-xs font-medium transition-colors',
              selected
                ? 'border-primary/30 bg-primary/10 text-primary'
                : 'border-border bg-background text-muted-foreground hover:bg-muted hover:text-foreground'
            )}
          >
            {t(`aiProviders.models.usecases.${type}`)}
          </button>
        );
      })}
    </div>
  );
}

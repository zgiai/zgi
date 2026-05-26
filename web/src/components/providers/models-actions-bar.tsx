'use client';

import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { useT } from '@/i18n';

interface ModelsActionsBarProps {
  totalCount: number;
  visibleCount: number;
  query: string;
  onQueryChange: (val: string) => void;
  onEnableVisible: () => void;
  onDisableVisible: () => void;
  extraActions?: React.ReactNode;
  onAdd?: () => void;
  addLabel?: string;
  disabled: boolean;
  hasActiveFilters?: boolean;
}

export default function ModelsActionsBar({
  totalCount,
  visibleCount,
  query,
  onQueryChange,
  onEnableVisible,
  onDisableVisible,
  extraActions,
  onAdd,
  addLabel,
  disabled,
  hasActiveFilters = false,
}: ModelsActionsBarProps): JSX.Element {
  const t = useT();
  const showScopedCount = hasActiveFilters || visibleCount !== totalCount;

  return (
    <div className="flex items-center justify-between">
      <div className="space-y-1">
        <div className="text-sm text-muted-foreground">
          {showScopedCount
            ? t('aiProviders.models.visibleCount', { visible: visibleCount, total: totalCount })
            : `${totalCount} ${t('aiProviders.models.count')}`}
        </div>
        {showScopedCount ? (
          <div className="text-xs text-muted-foreground/80">
            {t('aiProviders.models.actions.filteredScopeHint')}
          </div>
        ) : null}
      </div>
      <div className="flex items-center gap-2">
        {visibleCount > 0 && (
          <>
            <Button
              size="sm"
              variant="default"
              onClick={onEnableVisible}
              disabled={disabled}
            >
              {hasActiveFilters
                ? t('aiProviders.models.actions.enableVisible')
                : t('aiProviders.models.actions.enableAll')}
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={onDisableVisible}
              disabled={disabled}
            >
              {hasActiveFilters
                ? t('aiProviders.models.actions.disableVisible')
                : t('aiProviders.models.actions.disableAll')}
            </Button>
          </>
        )}
        {extraActions}
        {onAdd && (
          <Button
            size="sm"
            variant="default"
            onClick={onAdd}
            disabled={disabled}
          >
            {addLabel || t('aiProviders.models.actions.add') || 'Add Model'}
          </Button>
        )}
        <div className="w-80">
          <Input
            placeholder={t('aiProviders.models.searchPlaceholder')}
            value={query}
            onChange={e => onQueryChange(e.target.value)}
          />
        </div>
      </div>
    </div>
  );
}

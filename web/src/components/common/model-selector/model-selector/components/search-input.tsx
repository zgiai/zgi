'use client';

import { memo } from 'react';
import { Input } from '@/components/ui/input';
import { Search, RefreshCcw } from 'lucide-react';
import { Button } from '@/components/ui/button';

export interface SearchInputProps {
  inputRef: React.RefObject<HTMLInputElement>;
  value: string;
  placeholder: string;
  onChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onKeyDown: (e: React.KeyboardEvent<HTMLInputElement>) => void;
  onRefresh?: () => void;
  refreshLabel: string;
  disabled?: boolean;
  isFetching?: boolean;
}

// Search input component with icon
export const SearchInput = memo(function SearchInput({
  inputRef,
  value,
  placeholder,
  onChange,
  onKeyDown,
  onRefresh,
  refreshLabel,
  disabled = false,
  isFetching = false,
}: SearchInputProps) {
  return (
    <div className="px-2 pt-1 pb-2 border-b">
      <div className="flex items-center gap-1">
        <div className="relative grow">
          <Search className="absolute left-2 top-2 h-4 w-4 text-muted-foreground pointer-events-none" />
          <Input
            ref={inputRef}
            placeholder={placeholder}
            value={value}
            onChange={onChange}
            onKeyDown={onKeyDown}
            className="pl-8 h-8"
            autoComplete="off"
          />
        </div>
        <Button
          type="button"
          variant="ghost"
          isIcon
          className="h-8 w-8"
          onMouseDown={e => {
            e.preventDefault();
            e.stopPropagation();
            onRefresh?.();
          }}
          disabled={disabled || isFetching}
          aria-busy={isFetching}
          aria-label={refreshLabel}
        >
          <RefreshCcw className={`h-4 w-4 ${isFetching ? 'animate-spin' : ''}`} />
        </Button>
      </div>
    </div>
  );
});

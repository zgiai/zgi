import { memo, useCallback } from 'react';
import { Plus, RefreshCw, FolderPlus, Search, ArrowLeft } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';

export interface HeaderToolbarProps {
  titleText: string;
  isFetching: boolean;
  onReload: () => void;
  isRootView: boolean;
  searchKeyword: string;
  onSearchChange: (next: string) => void;
  // Use i18n-provided strings from parent to avoid i18n dependency inside this component
  searchPlaceholder: string;
  createFolderText?: string;
  onCreateFolder?: () => void;
  createText: string;
  onCreateDataset?: () => void;
  onBack?: () => void;
}

/**
 * Header toolbar with title, refresh, search, and create actions.
 * Memoized to avoid re-renders on scroll and list updates.
 */
function HeaderToolbarBase({
  titleText,
  isFetching,
  onReload,
  isRootView,
  searchKeyword,
  onSearchChange,
  searchPlaceholder,
  createFolderText,
  onCreateFolder,
  createText,
  onCreateDataset,
  onBack,
}: HeaderToolbarProps) {
  const handleInputChange = useCallback<React.ChangeEventHandler<HTMLInputElement>>(
    e => onSearchChange(e.target.value),
    [onSearchChange]
  );

  return (
    <div className="flex flex-col gap-4 @3xl/console:flex-row @3xl/console:items-center @3xl/console:justify-between">
      <div className="flex items-center gap-2">
        {!isRootView && onBack && (
          <Button
            isIcon
            variant="ghost"
            className="size-8 rounded-sm hover:bg-muted cursor-pointer"
            onClick={onBack}
          >
            <ArrowLeft className="h-5 w-5" />
          </Button>
        )}
        <h1 className="text-2xl font-bold">{titleText}</h1>
        <Button
          isIcon
          variant="ghost"
          className="size-7 rounded-sm hover:bg-muted cursor-pointer"
          onClick={onReload}
          disabled={isFetching}
        >
          <RefreshCw size={16} className={`${isFetching ? 'animate-spin' : ''} h-4 w-4`} />
        </Button>
      </div>

      <div className="flex w-full flex-col gap-3 @3xl/console:w-auto @3xl/console:flex-row">
        {/* Search Bar */}
        <div className="relative w-full @3xl/console:max-w-md">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder={searchPlaceholder}
            value={searchKeyword}
            onChange={handleInputChange}
            className="pl-9"
          />
        </div>
        {isRootView && onCreateFolder && (
          <Button variant="outline" onClick={onCreateFolder}>
            <FolderPlus size={16} />
            <span className="text-sm font-normal">{createFolderText}</span>
          </Button>
        )}
        {onCreateDataset && (
          <Button onClick={onCreateDataset}>
            <Plus size={16} />
            <span className="text-sm font-normal">{createText}</span>
          </Button>
        )}
      </div>
    </div>
  );
}

export const HeaderToolbar = memo(HeaderToolbarBase);

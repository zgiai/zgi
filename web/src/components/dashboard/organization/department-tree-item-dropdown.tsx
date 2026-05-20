'use client';

import { useState, memo } from 'react';
import { Button } from '@/components/ui/button';
import { ChevronRight, Check } from 'lucide-react';
import type { Department } from '@/services/types/organization';
import { cn } from '@/lib/utils';

interface DepartmentTreeItemDropdownProps {
  department: Department;
  level?: number;
  selectedId: string | null;
  onSelect: (id: string) => void;
  // For controlled expansion
  expandedIds?: Set<string>;
  onToggleExpand?: (id: string) => void;
  // Display options
  showCheckIcon?: boolean;
  isDepartmentDisabled?: (department: Department) => boolean;
}

export const DepartmentTreeItemDropdown = memo(function DepartmentTreeItemDropdown({
  department,
  level = 0,
  selectedId,
  onSelect,
  expandedIds,
  onToggleExpand,
  showCheckIcon = false,
  isDepartmentDisabled,
}: DepartmentTreeItemDropdownProps) {
  const hasChildren = department.children && department.children.length > 0;

  // Use controlled expansion if provided, otherwise use internal state
  const useControlledExpansion = expandedIds !== undefined && onToggleExpand !== undefined;
  const [internalExpanded, setInternalExpanded] = useState(false);
  const isExpanded = useControlledExpansion ? expandedIds.has(department.id) : internalExpanded;

  const isSelected = selectedId === department.id;
  const isDisabled = level >= 5 || !!isDepartmentDisabled?.(department);

  const handleToggle = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (hasChildren) {
      if (useControlledExpansion && onToggleExpand) {
        onToggleExpand(department.id);
      } else {
        setInternalExpanded(!internalExpanded);
      }
    }
  };

  const handleSelect = () => {
    if (isDisabled) return;
    onSelect(department.id);
  };

  return (
    <div className={cn(level > 0 && 'mb-1')}>
      <div
        className={cn(
          'flex items-center gap-1 hover:bg-accent/50 transition-colors rounded-md group pr-2',
          isSelected && 'bg-accent',
          isDisabled && 'cursor-not-allowed opacity-50 hover:bg-transparent'
        )}
      >
        {hasChildren ? (
          <Button
            type="button"
            variant="ghost"
            isIcon
            className={cn(
              'h-8 w-7 pl-2 hover:bg-transparent hover:text-primary text-muted-foreground shrink-0',
              'transition-transform duration-200'
            )}
            onClick={handleToggle}
          >
            <ChevronRight
              className={cn('h-5 w-5 transition-transform duration-200', isExpanded && 'rotate-90')}
            />
          </Button>
        ) : (
          <div className="w-7 h-8 shrink-0" />
        )}
        <button
          type="button"
          disabled={isDisabled}
          onClick={handleSelect}
          className={cn(
            'flex h-8 min-w-0 flex-1 items-center gap-2 text-left text-sm transition-colors disabled:cursor-not-allowed',
            department.id === 'ORG_ROOT' ? 'font-semibold' : 'font-normal',
            isSelected && 'text-primary'
          )}
        >
          <span className="min-w-0 flex-1 truncate">{department.name}</span>
          {showCheckIcon &&
            (isSelected ? (
              <Check className="h-4 w-4 text-primary shrink-0" />
            ) : (
              <span className="w-4 shrink-0" />
            ))}
        </button>
      </div>

      {hasChildren && isExpanded && department.children && (
        <div className="mt-1 ml-4 pl-2 border-l space-y-1">
          {department.children.map(child => (
            <DepartmentTreeItemDropdown
              key={child.id}
              department={child}
              level={level + 1}
              selectedId={selectedId}
              onSelect={onSelect}
              expandedIds={expandedIds}
              onToggleExpand={onToggleExpand}
              showCheckIcon={showCheckIcon}
              isDepartmentDisabled={isDepartmentDisabled}
            />
          ))}
        </div>
      )}
    </div>
  );
});

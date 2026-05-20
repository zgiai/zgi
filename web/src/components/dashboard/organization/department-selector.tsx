'use client';

import { useEffect, useState } from 'react';
import { Check, ChevronsUpDown } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { DepartmentTreeItemDropdown } from '@/components/dashboard/organization/department-tree-item-dropdown';
import { cn } from '@/lib/utils';
import type { Department } from '@/services/types/organization';

interface DepartmentSelectorProps {
  departments: Department[];
  value: string;
  onValueChange: (value: string) => void;
  placeholder: string;
  emptyLabel?: string;
  emptyValue?: string;
  allowEmpty?: boolean;
  disabled?: boolean;
  triggerId?: string;
  className?: string;
  contentClassName?: string;
  modal?: boolean;
  containerOpen?: boolean;
  isDepartmentDisabled?: (department: Department) => boolean;
}

/**
 * @component DepartmentSelector
 * @category Feature
 * @status Stable
 * @description Reusable organization department tree selector.
 * @usage Use in organization dialogs and filters that need a department picker.
 * @example
 * <DepartmentSelector departments={departments} value={departmentId} onValueChange={setDepartmentId} placeholder="Select department" />
 */
export function DepartmentSelector({
  departments,
  value,
  onValueChange,
  placeholder,
  emptyLabel,
  emptyValue = '',
  allowEmpty = false,
  disabled = false,
  triggerId,
  className,
  contentClassName,
  modal = false,
  containerOpen,
  isDepartmentDisabled,
}: DepartmentSelectorProps) {
  const [open, setOpen] = useState(false);
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());

  const isEmptyValue = value === emptyValue;
  const selectedLabel = !isEmptyValue ? getDepartmentName(departments, value) : null;

  useEffect(() => {
    if (containerOpen === false) {
      setOpen(false);
      setExpandedIds(new Set());
    }
  }, [containerOpen]);

  const handleOpenChange = (isOpen: boolean) => {
    if (isOpen && containerOpen === false) return;
    setOpen(isOpen);
    if (isOpen && expandedIds.size === 0) {
      setExpandedIds(new Set(departments.map(department => department.id)));
    }
  };

  const handleToggleExpand = (id: string) => {
    setExpandedIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const handleSelect = (id: string) => {
    onValueChange(id);
    setOpen(false);
  };

  const displayLabel =
    selectedLabel || (allowEmpty && isEmptyValue && emptyLabel ? emptyLabel : placeholder);

  return (
    <Popover open={open} onOpenChange={handleOpenChange} modal={modal}>
      <PopoverTrigger asChild>
        <Button
          id={triggerId}
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className={cn(
            'w-full justify-between px-3 font-normal',
            !selectedLabel && 'text-muted-foreground',
            className
          )}
        >
          <span className="truncate text-left">{displayLabel}</span>
          <ChevronsUpDown className="size-4 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        data-department-selector-content="true"
        align="start"
        className={cn(
          'w-[var(--radix-popover-trigger-width)] max-h-60 overflow-y-auto p-1 pointer-events-auto',
          contentClassName
        )}
      >
        {allowEmpty && emptyLabel ? (
          <button
            type="button"
            onClick={() => handleSelect(emptyValue)}
            className={cn(
              'flex w-full items-center rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent hover:text-accent-foreground',
              isEmptyValue && 'bg-accent text-accent-foreground'
            )}
          >
            <span className="flex-1 truncate">{emptyLabel}</span>
            {isEmptyValue ? (
              <Check className="size-4 text-primary" />
            ) : (
              <span className="size-4 shrink-0" />
            )}
          </button>
        ) : null}
        {departments.map(department => (
          <DepartmentTreeItemDropdown
            key={department.id}
            department={department}
            level={0}
            selectedId={isEmptyValue ? null : value}
            onSelect={handleSelect}
            expandedIds={expandedIds}
            onToggleExpand={handleToggleExpand}
            showCheckIcon
            isDepartmentDisabled={isDepartmentDisabled}
          />
        ))}
      </PopoverContent>
    </Popover>
  );
}

export function isDepartmentSelectorContent(target: EventTarget | null) {
  return (
    target instanceof HTMLElement && !!target.closest('[data-department-selector-content="true"]')
  );
}

function getDepartmentName(departments: Department[], targetId: string): string | null {
  for (const department of departments) {
    if (department.id === targetId) return department.name;
    if (department.children) {
      const found = getDepartmentName(department.children, targetId);
      if (found) return found;
    }
  }
  return null;
}

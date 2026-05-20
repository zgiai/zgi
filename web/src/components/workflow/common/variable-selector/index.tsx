'use client';

import React, { useState } from 'react';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  ValueSelectorMenu,
  type ValueSelectorMenuProps,
} from '../node-value-selector/value-selector-menu';
import { cn } from '@/lib/utils';

export interface VariableSelectorProps extends Omit<ValueSelectorMenuProps, 'onClose'> {
  /** The element that triggers the menu */
  children: React.ReactElement;
  /** Optional class for the content */
  contentClassName?: string;
  /** Alignment of the dropdown */
  align?: 'start' | 'center' | 'end';
}

/**
 * VariableSelector
 *
 * A generalized variable selector component that can wrap any trigger element (like a button).
 * It uses the hierarchical ValueSelectorMenu internally.
 */
export const VariableSelector: React.FC<VariableSelectorProps> = ({
  children,
  contentClassName,
  align = 'end',
  ...menuProps
}) => {
  const [open, setOpen] = useState(false);

  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>{children}</DropdownMenuTrigger>
      <DropdownMenuContent
        className={cn('w-64 max-h-[300px] overflow-auto p-0', contentClassName)}
        align={align}
      >
        <ValueSelectorMenu {...menuProps} onClose={() => setOpen(false)} />
      </DropdownMenuContent>
    </DropdownMenu>
  );
};

export default VariableSelector;

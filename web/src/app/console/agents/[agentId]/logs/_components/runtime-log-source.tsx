'use client';

import { Badge } from '@/components/ui/badge';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';

export interface RuntimeLogSourceOption<TValue extends string> {
  value: TValue;
  label: string;
}

interface RuntimeLogSourceTabsProps<TValue extends string> {
  value: TValue;
  options: ReadonlyArray<RuntimeLogSourceOption<TValue>>;
  onValueChange: (value: TValue) => void;
  ariaLabel: string;
}

export function RuntimeLogSourceTabs<TValue extends string>({
  value,
  options,
  onValueChange,
  ariaLabel,
}: RuntimeLogSourceTabsProps<TValue>) {
  return (
    <div className="max-w-full overflow-x-auto pb-0.5">
      <Tabs value={value} onValueChange={nextValue => onValueChange(nextValue as TValue)}>
        <TabsList className="h-8 min-w-max" aria-label={ariaLabel}>
          {options.map(option => (
            <TabsTrigger key={option.value} value={option.value} className="h-6 text-xs">
              {option.label}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
    </div>
  );
}

export function RuntimeLogSourceBadge({ label }: { label: string }) {
  return (
    <Badge variant="subtle" className="max-w-full font-normal">
      <span className="truncate">{label}</span>
    </Badge>
  );
}

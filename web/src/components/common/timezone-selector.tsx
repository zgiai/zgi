'use client';

import { useT } from '@/i18n';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { timezones, type TimezoneValue } from '@/lib/constants';

export interface TimezoneSelectorProps {
  value?: TimezoneValue | '' | null;
  onChange: (value: TimezoneValue) => void;
  disabled?: boolean;
  placeholder?: string;
  triggerClassName?: string;
  contentClassName?: string;
  name?: string;
}

/**
 * Reusable timezone selector with i18n-ready UI texts.
 * Uses IANA timezone values from constants and displays labels.
 */
export function TimezoneSelector({
  value,
  onChange,
  disabled,
  placeholder,
  triggerClassName,
  contentClassName,
  name,
}: TimezoneSelectorProps) {
  const t = useT('ui');
  const displayPlaceholder = placeholder ?? t('timezoneSelector.placeholder');

  return (
    <Select
      value={(value ?? '') as string}
      onValueChange={v => onChange(v as TimezoneValue)}
      disabled={disabled}
      name={name}
    >
      <SelectTrigger className={triggerClassName} aria-label={t('timezoneSelector.label')}>
        <SelectValue placeholder={displayPlaceholder} />
      </SelectTrigger>
      <SelectContent className={contentClassName}>
        {timezones.map(tz => (
          <SelectItem key={tz.value} value={tz.value}>
            {tz.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export default TimezoneSelector;

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
import { useLocale } from '@/hooks/use-locale';

export interface TimezoneSelectorProps {
  id?: string;
  value?: TimezoneValue | '' | null;
  onChange: (value: TimezoneValue) => void;
  disabled?: boolean;
  error?: boolean;
  placeholder?: string;
  triggerClassName?: string;
  contentClassName?: string;
  name?: string;
}

function getEtcGmtOffset(timezone: string): { full: string; compact: string } | null {
  const match = /^Etc\/GMT([+-])(\d{1,2})$/.exec(timezone);
  if (!match) {
    return null;
  }

  const [, sign, hour] = match;
  const offsetSign = sign === '-' ? '+' : '-';

  return {
    full: `UTC${offsetSign}${hour.padStart(2, '0')}:00`,
    compact: `UTC${offsetSign}${Number(hour)}`,
  };
}

/**
 * Reusable timezone selector with i18n-ready UI texts.
 * Uses IANA timezone values from constants and displays labels.
 */
export function TimezoneSelector({
  id,
  value,
  onChange,
  disabled,
  error,
  placeholder,
  triggerClassName,
  contentClassName,
  name,
}: TimezoneSelectorProps) {
  const t = useT('ui');
  const { locale } = useLocale();
  const displayPlaceholder = placeholder ?? t('timezoneSelector.placeholder');
  const normalizedValue = value?.trim() ?? '';
  const hasKnownValue = timezones.some(tz => tz.value === normalizedValue);
  const currentUnknownValue = normalizedValue && !hasKnownValue ? normalizedValue : '';

  const formatLabel = (timezone: (typeof timezones)[number]) =>
    `(${timezone.offset}) ${timezone.label[locale] ?? timezone.label['en-US']}`;
  const formatUnknownLabel = (timezone: string) => {
    const offset = getEtcGmtOffset(timezone);
    return offset
      ? `(${offset.full}) ${t('timezoneSelector.fixedOffset', { offset: offset.compact })}`
      : timezone;
  };

  return (
    <Select
      value={normalizedValue}
      onValueChange={v => onChange(v as TimezoneValue)}
      disabled={disabled}
      name={name}
    >
      <SelectTrigger
        id={id}
        className={triggerClassName}
        aria-label={t('timezoneSelector.label')}
        aria-invalid={error || undefined}
      >
        <SelectValue placeholder={displayPlaceholder} />
      </SelectTrigger>
      <SelectContent className={contentClassName}>
        {currentUnknownValue ? (
          <SelectItem key={currentUnknownValue} value={currentUnknownValue}>
            {formatUnknownLabel(currentUnknownValue)}
          </SelectItem>
        ) : null}
        {timezones.map(tz => (
          <SelectItem key={tz.value} value={tz.value}>
            {formatLabel(tz)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export default TimezoneSelector;

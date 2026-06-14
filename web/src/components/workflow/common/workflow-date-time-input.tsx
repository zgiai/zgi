import React from 'react';
import { Input } from '@/components/ui/input';

function pad(value: number): string {
  return String(value).padStart(2, '0');
}

export function toLocalDateTimeInputValue(value: string): string {
  if (!value) {
    return '';
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '';
  }

  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

export function toOffsetDateTime(value: string): string {
  if (!value) {
    return '';
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '';
  }

  const offsetMinutes = -date.getTimezoneOffset();
  const sign = offsetMinutes >= 0 ? '+' : '-';
  const absolute = Math.abs(offsetMinutes);

  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:00${sign}${pad(Math.floor(absolute / 60))}:${pad(absolute % 60)}`;
}

export function currentOffsetDateTime(): string {
  const date = new Date();
  return toOffsetDateTime(
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
  );
}

export interface WorkflowDateTimeInputProps
  extends Omit<React.ComponentProps<typeof Input>, 'type' | 'value' | 'onChange'> {
  value?: string;
  onChange: (value: string) => void;
}

export function WorkflowDateTimeInput({ value, onChange, ...props }: WorkflowDateTimeInputProps) {
  return (
    <Input
      {...props}
      type="datetime-local"
      value={toLocalDateTimeInputValue(value ?? '')}
      onChange={event => onChange(toOffsetDateTime(event.target.value))}
    />
  );
}

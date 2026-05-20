'use client';

import React from 'react';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';

interface NumberInputRowProps {
  label: string;
  value: number | '';
  onChange: (value: number) => void;
  min: number;
  max: number;
  step?: number;
  disabled?: boolean;
  placeholder?: string;
}

/**
 * Reusable number input row with label for timeout/retry settings
 */
const NumberInputRow: React.FC<NumberInputRowProps> = ({
  label,
  value,
  onChange,
  min,
  max,
  step = 1,
  disabled = false,
  placeholder = '-',
}) => {
  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const strVal = e.target.value;
    if (strVal === '') {
      onChange(0);
      return;
    }
    const parsed = parseInt(strVal, 10) || min;
    const val = Math.min(max, Math.max(min, Math.floor(parsed)));
    onChange(val);
  };

  return (
    <div className="flex items-center justify-between">
      <Label className="text-xs text-muted-foreground w-32 shrink-0">{label}</Label>
      <div className="flex items-center gap-2">
        <Input
          type="number"
          min={min}
          max={max}
          step={step}
          className="w-24 h-8 text-center"
          placeholder={placeholder}
          value={value || ''}
          onChange={handleChange}
          disabled={disabled}
        />
      </div>
    </div>
  );
};

export default NumberInputRow;

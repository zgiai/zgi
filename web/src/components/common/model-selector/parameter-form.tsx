'use client';

import { useCallback } from 'react';
import { Info } from 'lucide-react';
import { useT } from '@/i18n';
import { Switch } from '@/components/ui/switch';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Slider } from '@/components/ui/slider';
import type { ParameterRuleItem } from '@/services/types/model';
import { computeSliderStep, computeInitialNumber } from '@/utils/number';
import { resolveModelConfigParameterCopy } from '@/utils/model-config-parameter-i18n';

export interface ParameterFormProps {
  rules: ParameterRuleItem[];
  enabledMap: Record<string, boolean>;
  setEnabledMap: React.Dispatch<React.SetStateAction<Record<string, boolean>>>;
  localValues: Record<string, number | string | boolean>;
  setLocalValues: React.Dispatch<React.SetStateAction<Record<string, number | string | boolean>>>;
}

export default function ParameterForm({
  rules,
  enabledMap,
  setEnabledMap,
  localValues,
  setLocalValues,
}: ParameterFormProps) {
  const t = useT();
  const rowClassName = 'grid grid-cols-12 items-start gap-3 py-2';
  const leftLabelClassName = 'col-span-3 flex items-center gap-2 h-9';

  const renderField = useCallback(
    (r: ParameterRuleItem) => {
      const enabled = enabledMap[r.name];
      const { help: helpText, label: labelText } = resolveModelConfigParameterCopy({
        parameter: r,
        translate: key => t(key as never),
      });

      const setValue = (val: string | number | boolean) =>
        setLocalValues(prev => ({ ...prev, [r.name]: val }));

      // boolean -> switch
      if (r.type === 'boolean') {
        const v = Boolean(localValues[r.name] ?? r.default ?? false);
        return (
          <div key={r.name} className={rowClassName}>
            <div className={leftLabelClassName}>
              <Switch
                checked={enabled}
                onCheckedChange={c =>
                  setEnabledMap(m => ({ ...m, [r.name]: r.required ? true : c }))
                }
                disabled={r.required}
              />
              <Label>{labelText}</Label>
              {helpText ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Info className="h-4 w-4 text-muted-foreground" />
                  </TooltipTrigger>
                  <TooltipContent>{helpText}</TooltipContent>
                </Tooltip>
              ) : null}
            </div>
            <div className="col-span-9 flex items-center gap-2 h-9">
              <Switch checked={v} onCheckedChange={c => setValue(Boolean(c))} disabled={!enabled} />
              <span className="text-base text-primary">{v ? t('common.yes') : t('common.no')}</span>
            </div>
          </div>
        );
      }

      // options -> select
      if (Array.isArray(r.options) && r.options.length > 0) {
        const v = (localValues[r.name] ?? '') as string;
        return (
          <div key={r.name} className={rowClassName}>
            <div className={leftLabelClassName}>
              <Switch
                checked={enabled}
                onCheckedChange={c => {
                  setEnabledMap(m => ({ ...m, [r.name]: r.required ? true : c }));
                  if (!enabled && c && localValues[r.name] === undefined) {
                    if (r.default != null) setValue(r.default as string);
                  }
                }}
                disabled={r.required}
              />
              <Label>{labelText}</Label>
              {helpText ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Info className="h-4 w-4 text-muted-foreground" />
                  </TooltipTrigger>
                  <TooltipContent>{helpText}</TooltipContent>
                </Tooltip>
              ) : null}
            </div>
            <div className="col-span-9">
              <Select
                value={String(v)}
                onValueChange={val => {
                  if (val !== v) setValue(val);
                }}
                disabled={!enabled}
              >
                <SelectTrigger>
                  <SelectValue placeholder="" />
                </SelectTrigger>
                <SelectContent>
                  {r.options.map(opt => (
                    <SelectItem key={opt} value={opt}>
                      {opt}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        );
      }

      // numeric -> number input (+ slider if min/max)
      if (r.type === 'int' || r.type === 'float') {
        const min = typeof r.min === 'number' ? r.min : undefined;
        const max = typeof r.max === 'number' ? r.max : undefined;
        const precision = r.precision ?? null;
        const vRaw = localValues[r.name];
        const vNum = typeof vRaw === 'number' ? vRaw : Number(vRaw ?? 0);
        const step = min != null && max != null ? computeSliderStep(min, max, precision) : 1;

        const handleInput = (val: string) => {
          if (val === '') return setValue('');
          let n = Number(val);
          if (!Number.isFinite(n)) return;
          if (min != null) n = Math.max(n, min);
          if (max != null) n = Math.min(n, max);
          if (r.type === 'int') n = Math.round(n);
          setValue(n);
        };

        return (
          <div key={r.name} className={rowClassName}>
            <div className={leftLabelClassName}>
              <Switch
                checked={enabled}
                onCheckedChange={c => {
                  setEnabledMap(m => ({ ...m, [r.name]: r.required ? true : c }));
                  if (
                    !enabled &&
                    c &&
                    (localValues[r.name] === undefined || localValues[r.name] === '')
                  ) {
                    setValue(
                      computeInitialNumber({
                        defaultValue: typeof r.default === 'number' ? r.default : null,
                        min: min ?? null,
                        max: max ?? null,
                        isInt: r.type === 'int',
                      })
                    );
                  }
                }}
                disabled={r.required}
              />
              <Label>{labelText}</Label>
              {helpText ? (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Info className="h-4 w-4 text-muted-foreground" />
                  </TooltipTrigger>
                  <TooltipContent>{helpText}</TooltipContent>
                </Tooltip>
              ) : null}
            </div>
            <div className="col-span-9 flex items-center gap-3">
              {min != null && max != null ? (
                <div className="flex-1">
                  <Slider
                    value={[Number.isFinite(vNum) ? vNum : (min ?? 0)]}
                    min={min}
                    max={max}
                    step={step}
                    onValueChange={arr => {
                      const next = String(arr[0]);
                      if (next !== String(vRaw ?? '')) handleInput(next);
                    }}
                    disabled={!enabled}
                  />
                </div>
              ) : null}
              <div className="w-40">
                <Input
                  type="number"
                  inputMode="decimal"
                  value={typeof vRaw === 'number' || typeof vRaw === 'string' ? vRaw : ''}
                  onChange={e => {
                    const next = e.target.value;
                    if (next !== String(vRaw ?? '')) handleInput(next);
                  }}
                  min={min}
                  max={max}
                  step={step}
                  disabled={!enabled}
                />
              </div>
            </div>
          </div>
        );
      }

      // string/text -> input/textarea
      const v = (localValues[r.name] ?? '') as string;
      const isText = r.type === 'text';
      return (
        <div key={r.name} className={rowClassName}>
          <div className={leftLabelClassName}>
            <Switch
              checked={enabled}
              onCheckedChange={c => setEnabledMap(m => ({ ...m, [r.name]: r.required ? true : c }))}
              disabled={r.required}
            />
            <Label>{labelText}</Label>
            {helpText ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Info className="h-4 w-4 text-muted-foreground" />
                </TooltipTrigger>
                <TooltipContent>{helpText}</TooltipContent>
              </Tooltip>
            ) : null}
          </div>
          <div className="col-span-9">
            {isText ? (
              <Textarea
                value={v}
                onChange={e => {
                  const next = e.target.value;
                  if (next !== v) setValue(next);
                }}
                className="max-h-40"
                disabled={!enabled}
                rows={5}
              />
            ) : (
              <Input
                value={v}
                onChange={e => {
                  const next = e.target.value;
                  if (next !== v) setValue(next);
                }}
                disabled={!enabled}
              />
            )}
          </div>
        </div>
      );
    },
    [enabledMap, localValues, setEnabledMap, setLocalValues, t]
  );

  return <div className="divide-y">{rules.map(r => renderField(r))}</div>;
}

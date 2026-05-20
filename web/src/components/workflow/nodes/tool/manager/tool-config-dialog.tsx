'use client';

import React, { useState, useMemo, useCallback } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import { Input, PasswordInput } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import {
  Select,
  SelectTrigger,
  SelectContent,
  SelectItem,
  SelectValue,
} from '@/components/ui/select';
import { Tooltip, TooltipTrigger, TooltipContent } from '@/components/ui/tooltip';
import { Info } from 'lucide-react';
import { Icons } from '@/components/ui/icons';
import { cn } from '@/lib/utils';
import type { ToolParameterBinding, ToolNodeData } from '../config';
import type { ToolConfigParameters, ToolFormField } from '@/services/types/tool';
import { mapParametersToFormFields, coerceValue } from '@/utils/tool-helpers';
import { useLocale } from '@/hooks/use-locale';

interface ToolConfigDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  configParameters: ToolConfigParameters;
  initialValues?: Record<string, any>;
  onSave: (values: Record<string, any>) => void;
  title?: string;
}

const ToolConfigDialog: React.FC<ToolConfigDialogProps> = ({
  open,
  onOpenChange,
  configParameters,
  initialValues = {},
  onSave,
  title = '工具配置',
}) => {
  const { locale } = useLocale();
  const [values, setValues] = useState<Record<string, any>>(initialValues || {});
  const [errors, setErrors] = useState<Record<string, string>>({});

  const formFields = useMemo(() => {
    return mapParametersToFormFields(configParameters.parameters, locale);
  }, [configParameters.parameters, locale]);

  const handleUpdateValue = useCallback(
    (name: string, val: unknown) => {
      setValues(prev => {
        const field = formFields.find(f => f.name === name);
        const coerced = field ? coerceValue(field, val) : val;
        return {
          ...prev,
          [name]: coerced,
        };
      });

      // Clear error when user types
      if (errors[name]) {
        setErrors(prev => {
          const newE = { ...prev };
          delete newE[name];
          return newE;
        });
      }
    },
    [formFields, errors]
  );

  const handleSave = () => {
    const newErrors: Record<string, string> = {};
    formFields.forEach(field => {
      if (field.required) {
        const val = values[field.name];
        if (val === undefined || val === null || (typeof val === 'string' && val.trim() === '')) {
          newErrors[field.name] = '该项为必填项';
        }
      }
    });

    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      return;
    }

    setErrors({});
    onSave(values);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[560px] p-0 overflow-hidden">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">{title}</DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-6 py-6 scrollbar-thin">
          <div className="space-y-5">
            {formFields.map(field => {
              const value = values[field.name];
              const reqMark = field.required ? (
                <span className="text-destructive ml-1">*</span>
              ) : null;

              return (
                <div key={field.name} className="space-y-2.5">
                  <div className="flex items-center gap-1.5 px-0.5">
                    <Label className="text-sm font-bold tracking-tight">
                      {field.label}
                      {reqMark}
                    </Label>
                    {field.description && (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Info
                            size={14}
                            className="text-muted-foreground hover:text-primary transition-colors cursor-help"
                          />
                        </TooltipTrigger>
                        <TooltipContent
                          side="top"
                          className="max-w-[320px] p-3 text-xs font-medium leading-relaxed shadow-premium"
                        >
                          {field.description}
                        </TooltipContent>
                      </Tooltip>
                    )}
                  </div>

                  <div className="relative group/field">
                    {field.type === 'checkbox' ? (
                      <div className="flex items-center gap-3 p-3 rounded-xl border border-neutral-100 bg-neutral-50/30 shadow-sm transition-all group-hover/field:border-neutral-200">
                        <Switch
                          checked={Boolean(value)}
                          onCheckedChange={v => handleUpdateValue(field.name, v)}
                          className="data-[state=checked]:bg-primary"
                        />
                        <span className="text-xs text-muted-foreground font-semibold">
                          {Boolean(value) ? '已启用' : '已禁用'}
                        </span>
                      </div>
                    ) : field.type === 'select' && field.options ? (
                      <Select
                        value={typeof value === 'string' ? value : ''}
                        onValueChange={v => handleUpdateValue(field.name, v)}
                      >
                        <SelectTrigger className="h-11 shadow-sm font-medium rounded-xl border-neutral-200 hover:border-neutral-300 transition-all">
                          <SelectValue placeholder="请选择..." />
                        </SelectTrigger>
                        <SelectContent className="rounded-xl shadow-premium border-neutral-100">
                          {field.options.map(opt => (
                            <SelectItem key={opt.value} value={opt.value} className="font-medium">
                              {opt.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    ) : field.type === 'secret' ? (
                      <PasswordInput
                        value={typeof value === 'string' ? value : ''}
                        onChange={e => handleUpdateValue(field.name, e.target.value)}
                        autoComplete="new-password"
                        className="h-11 shadow-sm font-medium rounded-xl border-neutral-200 focus-visible:ring-primary/20"
                        errorText={errors[field.name]}
                      />
                    ) : field.type === 'number' ? (
                      <Input
                        type="number"
                        value={value !== undefined ? String(value) : ''}
                        onChange={e => handleUpdateValue(field.name, e.target.value)}
                        className="h-11 shadow-sm font-medium rounded-xl border-neutral-200"
                        errorText={errors[field.name]}
                      />
                    ) : (
                      <Input
                        value={typeof value === 'string' ? value : ''}
                        onChange={e => handleUpdateValue(field.name, e.target.value)}
                        placeholder={field.description}
                        className={cn(
                          'h-11 shadow-sm font-medium rounded-xl border-neutral-200 transition-all focus-visible:ring-primary/20',
                          errors[field.name] &&
                            'border-destructive bg-destructive/5 focus-visible:ring-destructive/20'
                        )}
                      />
                    )}
                  </div>

                  {errors[field.name] && (
                    <p className="text-xs text-destructive font-bold px-1 animate-in fade-in slide-in-from-top-1 duration-200">
                      {errors[field.name]}
                    </p>
                  )}
                </div>
              );
            })}
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="font-semibold">
            取消
          </Button>
          <Button onClick={handleSave} size="lg" className="px-10 font-bold shadow-sm">
            保存配置
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default ToolConfigDialog;

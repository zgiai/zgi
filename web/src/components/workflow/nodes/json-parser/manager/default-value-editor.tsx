'use client';

import React, { useCallback, useMemo, useState } from 'react';
import type {
  JsonParserDefaultValueItem,
  JsonParserNodeData,
  JsonParserOutputType,
  JsonParserOutputSchema,
} from '../config';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';
import { CodeEditor } from '@/components/ui/code-editor';
import { useT } from '@/i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Wand2 } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';

interface DefaultValueEditorProps {
  outputs: JsonParserNodeData['outputs'];
  defaultValues: JsonParserDefaultValueItem[];
  onChange: (values: JsonParserDefaultValueItem[]) => void;
  onAutoGenerate?: () => void;
  readOnly?: boolean;
}

// Helper to validate value against schema structure
function validateValueAgainstSchema(
  value: JsonParserOutputType,
  schema: JsonParserOutputSchema,
  t: ReturnType<typeof useT<'nodes'>>
): string | null {
  // 1. Basic type check
  if (schema.type === 'string' && typeof value !== 'string') {
    return t('jsonParser.validation.typeMismatch', { type: 'string' });
  }
  if (schema.type === 'number' && typeof value !== 'number') {
    return t('jsonParser.validation.typeMismatch', { type: 'number' });
  }
  if (schema.type === 'boolean' && typeof value !== 'boolean') {
    return t('jsonParser.validation.typeMismatch', { type: 'boolean' });
  }

  // 2. Object check
  if (schema.type === 'object') {
    if (typeof value !== 'object' || value === null || Array.isArray(value)) {
      return t('jsonParser.validation.typeMismatch', { type: 'object' });
    }
    // If children defined, check keys recursively
    if (schema.children && Object.keys(schema.children).length > 0) {
      for (const [key, childSchema] of Object.entries(schema.children)) {
        if (!(key in value)) {
          return t('jsonParser.validation.missingField', { key });
        }
        const error = validateValueAgainstSchema(value[key], childSchema, t);
        if (error) return t('jsonParser.validation.fieldError', { key, error });
      }
    }
  }

  // 3. Array check
  if (schema.type === 'array[object]') {
    if (!Array.isArray(value)) return t('jsonParser.validation.typeMismatch', { type: 'array' });

    if (schema.children && Object.keys(schema.children).length > 0) {
      for (let i = 0; i < value.length; i++) {
        const item = value[i];
        if (typeof item !== 'object' || item === null) {
          return t('jsonParser.validation.arrayItemObject', { index: i + 1 });
        }

        for (const [key, childSchema] of Object.entries(schema.children)) {
          if (!(key in item)) {
            return t('jsonParser.validation.arrayItemMissingField', { index: i + 1, key });
          }
          const error = validateValueAgainstSchema(item[key], childSchema, t);
          if (error) {
            return t('jsonParser.validation.arrayItemFieldError', {
              index: i + 1,
              key,
              error,
            });
          }
        }
      }
    }
  }
  // Simplified checks for other array types
  if (schema.type.startsWith('array[') && !Array.isArray(value)) {
    return t('jsonParser.validation.typeMismatch', { type: 'array' });
  }

  return null;
}

function renderInput(
  schema: JsonParserOutputSchema,
  value: string,
  onChange: (val: string) => void,
  readOnly: boolean | undefined,
  t: ReturnType<typeof useT<'nodes'>>
) {
  const type = schema.type;

  if (type === 'boolean') {
    return (
      <div className="flex items-center gap-2 h-9">
        <Switch
          checked={value === 'true'}
          onCheckedChange={checked => onChange(String(checked))}
          disabled={readOnly}
        />
        <span className="text-sm text-muted-foreground">{value === 'true' ? 'true' : 'false'}</span>
      </div>
    );
  }

  if (type === 'number') {
    return (
      <Input
        type="number"
        value={value}
        onChange={e => onChange(e.target.value)}
        disabled={readOnly}
        placeholder="0"
        className="font-mono text-sm h-9"
      />
    );
  }

  if (type === 'string') {
    return (
      <Input
        value={value}
        onChange={e => onChange(e.target.value)}
        disabled={readOnly}
        placeholder={t('jsonParser.defaultValue.inputLabel')}
        className="text-sm h-9"
      />
    );
  }

  // Complex types (object, array)
  // Parse JSON to validate structure
  let errorMsg: string | null = null;
  try {
    if (value) {
      const parsed = JSON.parse(value);
      errorMsg = validateValueAgainstSchema(parsed, schema, t);
    }
  } catch (_e) {
    errorMsg = t('jsonParser.validation.invalidJson');
  }

  return (
    <div className={`border rounded-md overflow-hidden ${errorMsg ? 'border-destructive' : ''}`}>
      <CodeEditor
        value={value}
        onChange={onChange}
        language="json"
        height={200}
        readOnly={readOnly}
        showLanguageSelector={false}
        showCopyButton={false}
        disableSuggest
      />
      <div className="px-2 py-1 bg-muted/50 text-[10px] text-muted-foreground border-t flex justify-between min-h-[24px] items-center">
        <span>{t('jsonParser.defaultValue.jsonPlaceholder')}</span>
        {errorMsg && <span className="text-destructive font-medium truncate ml-2">{errorMsg}</span>}
      </div>
    </div>
  );
}

const DefaultValueEditor: React.FC<DefaultValueEditorProps> = ({
  outputs,
  defaultValues,
  onChange,
  onAutoGenerate,
  readOnly,
}) => {
  const t = useT('nodes');
  const outputKeys = useMemo(() => (outputs ? Object.keys(outputs) : []), [outputs]);
  const [generateConfirmOpen, setGenerateConfirmOpen] = useState(false);

  const handleValueChange = useCallback(
    (key: string, value: string, type: JsonParserOutputType) => {
      const newValues = [...defaultValues];
      const index = newValues.findIndex(item => item.key === key);

      if (index > -1) {
        newValues[index] = { ...newValues[index], value, type };
      } else {
        newValues.push({ key, value, type });
      }
      onChange(newValues);
    },
    [defaultValues, onChange]
  );

  const handleGenerateConfirm = () => {
    onAutoGenerate?.();
    setGenerateConfirmOpen(false);
  };

  if (outputKeys.length === 0) {
    return null;
  }

  return (
    <div className="space-y-3 pt-2">
      <div className="space-y-1 flex items-center justify-between">
        <div>
          <h4 className="text-sm font-medium">{t('jsonParser.defaultValue.title')}</h4>
          <p className="text-xs text-muted-foreground">
            {t('jsonParser.defaultValue.description')}
          </p>
        </div>
        {onAutoGenerate && !readOnly && (
          <Button
            variant="ghost"
            size="xs"
            onClick={() => setGenerateConfirmOpen(true)}
            className="h-7 gap-1.5 text-xs text-muted-foreground hover:text-foreground"
          >
            <Wand2 className="h-3.5 w-3.5" />
            {t('jsonParser.actions.generate')}
          </Button>
        )}
      </div>

      <div className="space-y-3">
        {outputKeys.map(key => {
          const schema = outputs[key];
          const defaultValueItem = defaultValues.find(item => item.key === key);
          const currentValue = defaultValueItem?.value ?? '';

          return (
            <div key={key} className="space-y-1.5">
              <div className="flex items-center justify-between">
                <Label className="font-mono text-xs flex items-center gap-2 text-foreground/80">
                  {key}
                  <Badge
                    variant="outline"
                    className="text-[10px] h-4 px-1 font-normal text-muted-foreground"
                  >
                    {schema.type}
                  </Badge>
                  {schema.children && Object.keys(schema.children).length > 0 && (
                    <Badge variant="secondary" className="text-[10px] h-4 px-1 font-normal">
                      {t('jsonParser.labels.nestedFields', {
                        count: Object.keys(schema.children).length,
                      })}
                    </Badge>
                  )}
                </Label>
              </div>

              {renderInput(
                schema,
                currentValue,
                val => handleValueChange(key, val, schema.type),
                readOnly,
                t
              )}
            </div>
          );
        })}
      </div>

      {/* Confirmation Dialog */}
      <Dialog open={generateConfirmOpen} onOpenChange={setGenerateConfirmOpen}>
        <DialogContent className="sm:max-w-[425px]">
          <DialogHeader>
            <DialogTitle>{t('jsonParser.modal.generateTitle')}</DialogTitle>
          </DialogHeader>
          <DialogBody>
            <p className="text-sm text-muted-foreground">
              {t('jsonParser.modal.generateDescription')}
            </p>
          </DialogBody>
          <DialogFooter>
            <Button variant="outline" onClick={() => setGenerateConfirmOpen(false)}>
              {t('jsonParser.modal.generateCancel')}
            </Button>
            <Button onClick={handleGenerateConfirm}>{t('jsonParser.modal.generateConfirm')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
};

export default DefaultValueEditor;

'use client';

import React from 'react';
import { Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { CodeEditor } from '@/components/ui/code-editor';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import { WORKFLOW_CONTROL_COMPACT_CLASS } from '../../common/form-density';
import {
  clampWorkflowSafeNumber,
  WORKFLOW_SAFE_NUMBER_MAX,
  WORKFLOW_SAFE_NUMBER_MIN,
} from '../../common/number-limits';
import type { ConversationVariable } from '../../store/type';

// Parse string input to typed value by type
function parseValueByType(t: ConversationVariable['type'], raw: string): unknown {
  try {
    switch (t) {
      case 'string':
        return raw;
      case 'number':
        return raw === ''
          ? ''
          : Number.isNaN(Number(raw))
            ? ''
            : clampWorkflowSafeNumber(Number(raw));
      case 'boolean':
        return raw === 'true' || raw === '1';
      case 'object':
        return raw.trim() ? JSON.parse(raw) : {};
      case 'array[string]':
      case 'array[number]':
      case 'array[boolean]':
      case 'array[object]':
        return raw.trim() ? JSON.parse(raw) : [];
      default:
        return raw;
    }
  } catch {
    if (t === 'object') return {};
    if ((t as string).startsWith('array')) return [];
    return raw;
  }
}

// Serialize typed value to string for editors
function serializeValueByType(t: ConversationVariable['type'], v: unknown): string {
  if (t === 'string') return typeof v === 'string' ? v : '';
  if (t === 'number') return typeof v === 'number' || typeof v === 'string' ? String(v ?? '') : '';
  if (t === 'boolean') return v === true || v === 'true' ? 'true' : 'false';
  try {
    return v != null ? JSON.stringify(v) : '';
  } catch {
    return '';
  }
}

function isListEditableArray(type: ConversationVariable['type']): boolean {
  return type === 'array[string]' || type === 'array[number]' || type === 'array[boolean]';
}

function coerceArrayValue(
  type: ConversationVariable['type'],
  value: unknown
): string[] | number[] | boolean[] | unknown[] {
  if (!Array.isArray(value)) return [] as unknown[];
  if (type === 'array[string]') return value.map(v => String(v ?? '')) as string[];
  if (type === 'array[number]') {
    return value
      .map(v => (typeof v === 'number' ? v : Number(v)))
      .filter(v => !Number.isNaN(v)) as number[];
  }
  if (type === 'array[boolean]') return value.map(v => v === true || v === 'true') as boolean[];
  return value as unknown[];
}

export interface DefaultValueEditorProps {
  type: ConversationVariable['type'];
  value: unknown;
  onChange: (v: unknown) => void;
  density?: 'default' | 'compact';
  readOnly?: boolean;
}

const DefaultValueEditor: React.FC<DefaultValueEditorProps> = ({
  type,
  value,
  onChange,
  density = 'default',
  readOnly = false,
}) => {
  const t = useT('agents');
  const tCommon = useT('common');
  const isCompact = density === 'compact';

  const [mode, setMode] = React.useState<'list' | 'json'>(
    isListEditableArray(type) ? 'list' : 'json'
  );

  React.useEffect(() => {
    setMode(isListEditableArray(type) ? 'list' : 'json');
  }, [type]);

  if (type === 'string') {
    return (
      <Textarea
        placeholder={t('workflow.conversationVariables.placeholders.defaultValue')}
        value={serializeValueByType(type, value)}
        className={isCompact ? 'min-h-[72px] max-h-40 rounded-md px-2.5 py-2 text-xs' : 'max-h-52'}
        onChange={e => onChange(parseValueByType(type, e.target.value))}
        disabled={readOnly}
      />
    );
  }

  if (type === 'number') {
    return (
      <Input
        type="number"
        min={WORKFLOW_SAFE_NUMBER_MIN}
        max={WORKFLOW_SAFE_NUMBER_MAX}
        placeholder={t('workflow.conversationVariables.placeholders.defaultValue')}
        value={serializeValueByType(type, value)}
        className={isCompact ? WORKFLOW_CONTROL_COMPACT_CLASS : undefined}
        onChange={e => onChange(parseValueByType(type, e.target.value))}
        disabled={readOnly}
      />
    );
  }

  if (type === 'boolean') {
    const checked = value === true || value === 'true';
    return (
      <div className="flex items-center gap-2">
        <Switch checked={checked} onCheckedChange={v => onChange(v)} disabled={readOnly} />
        <span className="text-xs text-muted-foreground">
          {checked ? tCommon('yes') : tCommon('no')}
        </span>
      </div>
    );
  }

  if (type.startsWith('array')) {
    if (type === 'array[object]') {
      return (
        <CodeEditor
          value={serializeValueByType(type, value)}
          onChange={val => onChange(parseValueByType(type, String(val)))}
          language="json"
          allowLanguages={['json']}
          showLanguageSelector={false}
          showCopyButton={false}
          disableSuggest
          height={200}
          minHeight={200}
          maxHeight={200}
          readOnly={readOnly}
        />
      );
    }

    const list = coerceArrayValue(type, value) as string[] | number[] | boolean[];

    return (
      <div className="flex flex-col gap-2">
        <Tabs value={mode} onValueChange={v => setMode(v as 'list' | 'json')}>
          <TabsList>
            <TabsTrigger value="list" disabled={readOnly}>
              {t('workflow.conversationVariables.editMode.list')}
            </TabsTrigger>
            <TabsTrigger value="json" disabled={readOnly}>
              {t('workflow.conversationVariables.editMode.json')}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="list" className="flex flex-col gap-2">
            <div className="space-y-2">
              {(list as unknown[]).map((item, idx) => (
                <div key={idx} className="flex items-center gap-2">
                  {type === 'array[string]' ? (
                    <Input
                      value={String(item ?? '')}
                      placeholder={t('workflow.conversationVariables.placeholders.defaultValue')}
                      onChange={e => {
                        const next = [...(list as string[])];
                        next[idx] = e.target.value;
                        onChange(next);
                      }}
                      className="flex-1"
                      disabled={readOnly}
                    />
                  ) : null}
                  {type === 'array[number]' ? (
                    <Input
                      type="number"
                      min={WORKFLOW_SAFE_NUMBER_MIN}
                      max={WORKFLOW_SAFE_NUMBER_MAX}
                      value={String(item ?? '')}
                      placeholder={t('workflow.conversationVariables.placeholders.defaultValue')}
                      onChange={e => {
                        const next = [...(list as number[])];
                        const n = Number(e.target.value);
                        next[idx] = Number.isNaN(n)
                          ? ('' as unknown as number)
                          : clampWorkflowSafeNumber(n);
                        onChange(next);
                      }}
                      className="flex-1"
                      disabled={readOnly}
                    />
                  ) : null}
                  {type === 'array[boolean]' ? (
                    <div className="flex flex-1 items-center gap-2">
                      <Switch
                        checked={item === true}
                        onCheckedChange={v => {
                          const next = [...(list as boolean[])];
                          next[idx] = v;
                          onChange(next);
                        }}
                        disabled={readOnly}
                      />
                      <span className="text-xs text-muted-foreground">
                        {(item === true && tCommon('yes')) || tCommon('no')}
                      </span>
                    </div>
                  ) : null}
                  <Button
                    variant="outline"
                    isIcon
                    aria-label={t('workflow.conversationVariables.actions.removeItem')}
                    onClick={() => {
                      const next = (list as unknown[]).filter((_, i) => i !== idx);
                      onChange(next);
                    }}
                    disabled={readOnly}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </div>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  if (type === 'array[string]') onChange([...(list as string[]), '']);
                  else if (type === 'array[number]') onChange([...(list as number[]), 0]);
                  else if (type === 'array[boolean]') onChange([...(list as boolean[]), false]);
                }}
                disabled={readOnly}
              >
                {t('workflow.conversationVariables.actions.addItem')}
              </Button>
            </div>
          </TabsContent>
          <TabsContent value="json">
            <CodeEditor
              value={serializeValueByType(type, value)}
              onChange={val => onChange(parseValueByType(type, String(val)))}
              language="json"
              allowLanguages={['json']}
              showLanguageSelector={false}
              showCopyButton={false}
              disableSuggest
              height={200}
              minHeight={200}
              maxHeight={200}
              readOnly={readOnly}
            />
          </TabsContent>
        </Tabs>
      </div>
    );
  }

  return (
    <CodeEditor
      value={serializeValueByType(type, value)}
      onChange={val => onChange(parseValueByType(type, String(val)))}
      language="json"
      allowLanguages={['json']}
      showLanguageSelector={false}
      showCopyButton={false}
      disableSuggest
      height={200}
      minHeight={200}
      maxHeight={200}
      readOnly={readOnly}
    />
  );
};

export default DefaultValueEditor;

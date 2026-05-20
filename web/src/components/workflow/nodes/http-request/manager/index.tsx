'use client';

import React, { useCallback, useMemo, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select';
import { cn } from '@/lib/utils';
import type {
  HttpHeaderKV,
  HttpRequestNodeData,
  TimeoutConfig,
  RetryConfig,
  ErrorStrategy,
} from '../config';
import { DEFAULT_ERROR_VALUES, DEFAULT_TIMEOUT_CONFIG, DEFAULT_RETRY_CONFIG } from '../config';
import WorkflowValueEditor, {
  type WorkflowValueEditorHandle,
} from '../../../common/workflow-value-editor';
import { Braces, ChevronDown } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { Label } from '@/components/ui/label';
import { Slider } from '@/components/ui/slider';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import KeyValueEditor from './key-value-editor';
import FormDataEditor from './form-data-editor';
import NumberInputRow from './number-input-row';
import CurlImportDialog from './curl-import-dialog';
import type { ConvertCurlResult } from '@/utils/curl';
import OutputVariablesView from '../../../common/output-variables-view';
import { useT } from '@/i18n';
import { useNodeData, useNodeDataUpdate, useNodeOutputVariables } from '../../../hooks';

interface HttpRequestManagerProps {
  id: string;
  className?: string;
  // When true, render in read-only mode and disable interactive controls
  readOnly?: boolean;
}

// Allowed HTTP methods for the node
const METHODS: Array<HttpRequestNodeData['method']> = [
  'GET',
  'POST',
  'PUT',
  'DELETE',
  'PATCH',
  'HEAD',
];

// Validate a URL is http/https and roughly correct
const isHttpUrl = (url: string): boolean => {
  try {
    const normalized = url.replace(/\{\{#([^.#}]+)(?:\.([^#}]+))?#\}\}/g, '__VAR__');
    const u = new URL(normalized);
    return u.protocol === 'http:' || u.protocol === 'https:';
  } catch {
    return false;
  }
};

const HttpRequestManager: React.FC<HttpRequestManagerProps> = ({
  id: nodeId,
  className,
  readOnly = false,
}) => {
  const t = useT('nodes');
  const updateData = useNodeDataUpdate<HttpRequestNodeData>(nodeId);
  const selfNodeData = useNodeData<HttpRequestNodeData>(nodeId);

  const urlEditorRef = useRef<WorkflowValueEditorHandle | null>(null);
  const bodyEditorRef = useRef<WorkflowValueEditorHandle | null>(null);
  const [curlDialogOpen, setCurlDialogOpen] = useState<boolean>(false);
  // Control suggestion enablement to suppress dropdown during cURL autofill
  const [suppressSuggestOnce, setSuppressSuggestOnce] = useState<boolean>(false);
  const [settingsExpanded, setSettingsExpanded] = useState(true);

  // Safe data with defaults
  const nodeData = useMemo(
    () => ({
      ...selfNodeData,
      method: selfNodeData?.method || 'GET',
      url: selfNodeData?.url || '',
      params: selfNodeData?.params || [],
      headers: selfNodeData?.headers || [],
      body: selfNodeData?.body || { type: 'none', data: [] },
      timeout: selfNodeData?.timeout || DEFAULT_TIMEOUT_CONFIG,
      retry_config: selfNodeData?.retry_config || DEFAULT_RETRY_CONFIG,
    }),
    [selfNodeData]
  );

  // Body current type and first item value (new schema)
  const bodyType = (nodeData.body?.type ?? 'none') as HttpRequestNodeData['body']['type'];
  const firstItem = nodeData.body?.data?.[0];
  const bodyValue = firstItem?.value ?? '';

  // Apply imported cURL result to node data and manage suggestion suppression
  const onCurlImportSuccess = useCallback(
    (data: ConvertCurlResult) => {
      setSuppressSuggestOnce(true);

      // Extract query params from the imported URL and store them independently
      let urlNoQuery = data.url;
      const parsedParams: HttpHeaderKV[] = [];
      try {
        const u = new URL(data.url);
        // Collect all query params (keep duplicates and order)
        u.searchParams.forEach((value, key) => {
          parsedParams.push({ key, value });
        });
        // Remove query string from URL to ensure full independence
        u.search = '';
        urlNoQuery = u.toString();
      } catch {
        // If URL parsing fails, fall back to the original URL and no params
      }

      // Determine body type based on content-type header and body shape
      const contentType = (data.headers || []).find(
        (h: HttpHeaderKV) => h.key.toLowerCase() === 'content-type'
      )?.value;
      const rawBody = (data.body ?? '').toString();
      const trimmed = rawBody.trim();
      const looksJson = (() => {
        if (contentType && /json/i.test(contentType)) return true;
        return trimmed.startsWith('{') || trimmed.startsWith('[');
      })();

      const nextBody: HttpRequestNodeData['body'] =
        trimmed.length === 0
          ? { type: 'none', data: [] }
          : looksJson
            ? {
                type: 'json',
                data: [{ id: `key-value-${Date.now()}`, type: 'text', key: '', value: rawBody }],
              }
            : {
                type: 'raw-text',
                data: [{ id: `key-value-${Date.now()}`, type: 'text', key: '', value: rawBody }],
              };

      // Apply autofill to node data atomically
      updateData({
        method: data.method,
        url: urlNoQuery,
        headers: data.headers,
        body: nextBody,
        params: parsedParams,
      });

      setCurlDialogOpen(false);
      // Re-enable suggestion on next tick to avoid being triggered by autofill content
      setTimeout(() => setSuppressSuggestOnce(false), 0);
    },
    [updateData]
  );

  const outputs = useNodeOutputVariables(nodeId);

  // Whether the request method supports body input (allow GET per requirement; keep HEAD hidden)
  const showBody = useMemo(() => nodeData.method !== 'HEAD', [nodeData.method]);

  const handleMethodChange = useCallback(
    (value: HttpRequestNodeData['method']) => {
      updateData({ method: value });
    },
    [updateData]
  );

  const handleBodyChange = useCallback(
    (val: string) => {
      updateData((prev: HttpRequestNodeData) => {
        if (prev.body?.type === 'none') return {};
        const existing = prev.body?.data?.[0];
        const item = existing
          ? { ...existing, value: val }
          : { id: `key-value-${Date.now()}`, type: 'text' as const, key: '', value: val };
        return {
          body: { type: prev.body?.type, data: [item] } as HttpRequestNodeData['body'],
        };
      });
    },
    [updateData]
  );

  const handleBodyModeChange = useCallback(
    (m: 'none' | 'raw-text' | 'json' | 'form-data') => {
      updateData((prev: HttpRequestNodeData) => {
        if (m === 'none') {
          return { body: { type: 'none', data: [] } };
        }
        if (m === 'form-data') {
          const existingData = prev.body?.data || [];
          return { body: { type: 'form-data', data: existingData } };
        }
        const existingVal = prev.body?.data?.[0]?.value ?? '';
        const existingId = prev.body?.data?.[0]?.id ?? `key-value-${Date.now()}`;
        const item = { id: existingId, type: 'text', key: '', value: existingVal } as const;
        return { body: { type: m, data: [item] } };
      });
    },
    [updateData]
  );

  const urlValid = useMemo(() => (nodeData.url ? isHttpUrl(nodeData.url) : true), [nodeData.url]);

  return (
    <div className={cn('space-y-4', className)}>
      {/* Request line */}
      <div className="space-y-3">
        <h3 className="text-base font-semibold flex items-center justify-between">
          {t('httpRequest.section.request')}
          {!readOnly && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setCurlDialogOpen(true)}
            >
              {t('httpRequest.actions.importFromCurl')}
            </Button>
          )}
        </h3>
        <div className="grid grid-cols-1 sm:grid-cols-12 gap-3 items-end">
          <div className="col-span-3 h-full">
            <Select
              value={nodeData.method}
              onValueChange={v => handleMethodChange(v as HttpRequestNodeData['method'])}
              disabled={readOnly}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {METHODS.map(m => (
                  <SelectItem key={m} value={m}>
                    {m}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="sm:col-span-9">
            <div className="flex items-stretch gap-2">
              <WorkflowValueEditor
                ref={urlEditorRef}
                placeholder={t('httpRequest.placeholders.url')}
                className="min-w-0 grow"
                editorClassName="min-h-9"
                value={nodeData.url}
                onChange={val => updateData({ url: val })}
                nodeId={nodeId}
                readOnly={readOnly}
                suggestEnabled={!suppressSuggestOnce}
                slashTriggerEnabled={false}
              />
              {!readOnly && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      isIcon
                      className="h-9 w-9 shrink-0"
                      aria-label={t('common.insertVariable')}
                      onClick={() => urlEditorRef.current?.openVariableSelector()}
                    >
                      <Braces className="h-4 w-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t('common.insertVariable')}</TooltipContent>
                </Tooltip>
              )}
            </div>
            {!urlValid && (
              <p className="mt-1 text-xs text-red-500">
                {t('httpRequest.fields.urlInvalid')}
              </p>
            )}
          </div>
        </div>
      </div>

      {/* Query Params */}
      <KeyValueEditor
        title={t('httpRequest.section.queryParams')}
        nodeId={nodeId}
        readOnly={readOnly}
        items={nodeData.params}
        onChange={params => updateData({ params })}
        keyPlaceholder={t('httpRequest.placeholders.paramKey')}
        valuePlaceholder={t('httpRequest.placeholders.paramValue')}
      />

      {/* Headers */}
      <KeyValueEditor
        title={t('httpRequest.section.headers')}
        nodeId={nodeId}
        readOnly={readOnly}
        items={nodeData.headers}
        onChange={headers => updateData({ headers })}
        keyPlaceholder={t('httpRequest.placeholders.headerKey')}
        valuePlaceholder={t('httpRequest.placeholders.headerValue')}
      />

      {/* Body */}
      {showBody && (
        <div className="space-y-2">
          <Label className="flex items-center justify-between gap-4 py-1 w-full">
            <h3 className="text-base font-semibold shrink-0">
              {t('httpRequest.section.body')}
            </h3>
            <Select
              value={bodyType}
              onValueChange={v =>
                handleBodyModeChange(v as 'none' | 'raw-text' | 'json' | 'form-data')
              }
              disabled={readOnly}
            >
              <SelectTrigger className="w-40 h-9">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">{t('httpRequest.fields.none')}</SelectItem>
                <SelectItem value="raw-text">{t('httpRequest.fields.raw')}</SelectItem>
                <SelectItem value="json">{t('httpRequest.fields.json')}</SelectItem>
                <SelectItem value="form-data">{t('httpRequest.fields.formData')}</SelectItem>
              </SelectContent>
            </Select>
          </Label>
          {bodyType === 'form-data' && (
            <FormDataEditor
              nodeId={nodeId}
              readOnly={readOnly}
              items={nodeData.body.data || []}
              onChange={items =>
                updateData({
                  body: {
                    type: 'form-data',
                    data: items,
                  },
                })
              }
            />
          )}

          {bodyType !== 'none' && bodyType !== 'form-data' && (
            <WorkflowValueEditor
              ref={bodyEditorRef}
              placeholder={
                bodyType === 'json'
                  ? t('httpRequest.placeholders.bodyJson')
                  : t('httpRequest.placeholders.bodyRaw')
              }
              className="w-full"
              editorClassName="min-h-[180px]"
              value={bodyValue}
              onChange={handleBodyChange}
              nodeId={nodeId}
              readOnly={readOnly}
              // Keep suggestion enabled for body editor for better UX
              suggestEnabled={!suppressSuggestOnce}
            />
          )}
        </div>
      )}

      {/* Settings - Timeout & Retry */}
      <div className="space-y-4">
        <div
          className="flex items-center justify-between gap-2 cursor-pointer select-none group w-full"
          onClick={() => setSettingsExpanded(!settingsExpanded)}
        >
          <h3 className="text-base font-semibold">{t('httpRequest.section.settings')}</h3>
          <ChevronDown
            className={cn(
              'w-5 h-5 transition-transform duration-300 text-muted-foreground group-hover:text-foreground',
              !settingsExpanded && 'rotate-90'
            )}
          />
        </div>
        <div
          className={cn(
            'grid transition-all duration-300 ease-in-out',
            settingsExpanded ? 'grid-rows-[1fr] opacity-100' : 'grid-rows-[0fr] opacity-0 !mt-0'
          )}
        >
          <div className="overflow-hidden space-y-4">
            {/* Timeout Settings */}
            <div className="space-y-2">
              <h4 className="text-sm font-medium">{t('httpRequest.fields.timeout')}</h4>
              <div className="space-y-2">
                <NumberInputRow
                  label={t('httpRequest.fields.connectTimeout')}
                  value={nodeData.timeout?.connect || 0}
                  onChange={val =>
                    updateData({
                      timeout: { ...nodeData.timeout, connect: val } as TimeoutConfig,
                    })
                  }
                  min={1}
                  max={10}
                  disabled={readOnly}
                />
                <NumberInputRow
                  label={t('httpRequest.fields.readTimeout')}
                  value={nodeData.timeout?.read || 0}
                  onChange={val =>
                    updateData({
                      timeout: { ...nodeData.timeout, read: val } as TimeoutConfig,
                    })
                  }
                  min={1}
                  max={600}
                  disabled={readOnly}
                />
                <NumberInputRow
                  label={t('httpRequest.fields.writeTimeout')}
                  value={nodeData.timeout?.write || 0}
                  onChange={val =>
                    updateData({
                      timeout: { ...nodeData.timeout, write: val } as TimeoutConfig,
                    })
                  }
                  min={1}
                  max={600}
                  disabled={readOnly}
                />
              </div>
            </div>

            {/* Retry Settings */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label className="text-sm font-medium cursor-pointer" htmlFor="retry-switch">
                  {t('httpRequest.fields.retryEnabled')}
                </Label>
                <Switch
                  id="retry-switch"
                  checked={nodeData.retry_config?.retry_enabled ?? false}
                  onCheckedChange={checked => {
                    updateData({
                      retry_config: {
                        ...nodeData.retry_config,
                        retry_enabled: checked,
                      } as RetryConfig,
                    });
                  }}
                  disabled={readOnly}
                />
              </div>

              {nodeData.retry_config?.retry_enabled && (
                <div className="space-y-2">
                  {/* Max Retries: 1-10 */}
                  <div className="flex items-center justify-between">
                    <Label className="text-xs text-muted-foreground w-32 shrink-0">
                      {t('httpRequest.fields.maxRetries')}
                    </Label>
                    <div className="flex items-center gap-2">
                      <Input
                        type="number"
                        min={1}
                        max={10}
                        step={1}
                        className="w-24 h-8 text-center"
                        placeholder="-"
                        value={nodeData.retry_config?.max_retries || ''}
                        onChange={e => {
                          const strVal = e.target.value;
                          if (strVal === '') {
                            updateData({
                              retry_config: {
                                ...nodeData.retry_config,
                                max_retries: 0,
                              } as RetryConfig,
                            });
                            return;
                          }
                          const val = Math.min(
                            10,
                            Math.max(1, Math.floor(parseInt(strVal, 10) || 1))
                          );
                          updateData({
                            retry_config: {
                              ...nodeData.retry_config,
                              max_retries: val,
                            } as RetryConfig,
                          });
                        }}
                        disabled={readOnly}
                      />
                    </div>
                  </div>

                  {/* Retry Interval: 100-5000 */}
                  <div className="flex items-center w-full">
                    <Label className="text-xs text-muted-foreground w-32 shrink-0">
                      {t('httpRequest.fields.retryInterval')}
                    </Label>
                    <div className="flex items-center gap-2 grow">
                      <Slider
                        min={100}
                        max={5000}
                        step={100}
                        value={[nodeData.retry_config?.retry_interval ?? 1000]}
                        onValueChange={([val]) => {
                          updateData({
                            retry_config: {
                              ...nodeData.retry_config,
                              retry_interval: val,
                            } as RetryConfig,
                          });
                        }}
                        disabled={readOnly}
                      />
                      <Input
                        type="number"
                        min={100}
                        max={5000}
                        step={100}
                        className="w-24 h-8 text-center shrink-0"
                        placeholder="-"
                        value={nodeData.retry_config?.retry_interval || ''}
                        onChange={e => {
                          const strVal = e.target.value;
                          if (strVal === '') {
                            updateData({
                              retry_config: {
                                ...nodeData.retry_config,
                                retry_interval: 0,
                              } as RetryConfig,
                            });
                            return;
                          }
                          const val = Math.min(
                            5000,
                            Math.max(100, Math.floor(parseInt(strVal, 10) || 100))
                          );
                          updateData({
                            retry_config: {
                              ...nodeData.retry_config,
                              retry_interval: val,
                            } as RetryConfig,
                          });
                        }}
                        disabled={readOnly}
                      />
                    </div>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Error Handling Section */}
      <div className="space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="text-base font-semibold">
            {t('httpRequest.section.errorHandling')}
          </h3>
          <Select
            value={nodeData.error_strategy || 'none'}
            onValueChange={(v: ErrorStrategy) => {
              if (v === 'default-value') {
                updateData({
                  error_strategy: v,
                  default_value: nodeData.default_value || DEFAULT_ERROR_VALUES,
                });
              } else {
                updateData({ error_strategy: v });
              }
            }}
            disabled={readOnly}
          >
            <SelectTrigger className="w-40 h-9">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">
                {t('httpRequest.fields.errorStrategyNone')}
              </SelectItem>
              <SelectItem value="default-value">
                {t('httpRequest.fields.errorStrategyDefaultValue')}
              </SelectItem>
              <SelectItem value="fail-branch">
                {t('httpRequest.fields.errorStrategyFailBranch')}
              </SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-3">
          {/* Default Value Form - shown when strategy is 'default-value' */}
          {nodeData.error_strategy === 'default-value' && (
            <div className="space-y-3 border rounded-lg p-3 bg-muted/30">
              {/* Status Code */}
              <div className="flex items-center justify-between">
                <Label className="text-xs text-muted-foreground w-24 shrink-0">
                  {t('httpRequest.fields.defaultStatusCode')}
                </Label>
                <Input
                  type="number"
                  min={0}
                  max={999}
                  step={1}
                  className="w-24 h-8 text-center"
                  value={nodeData.default_value?.find(i => i.key === 'status_code')?.value || '200'}
                  onChange={e => {
                    const val = Math.min(999, Math.max(0, parseInt(e.target.value, 10) || 0));
                    const current = nodeData.default_value || DEFAULT_ERROR_VALUES;
                    const updated = current.map(item =>
                      item.key === 'status_code' ? { ...item, value: String(val) } : item
                    );
                    updateData({ default_value: updated });
                  }}
                  disabled={readOnly}
                />
              </div>

              {/* Headers */}
              <div className="space-y-2">
                <Label className="text-xs text-muted-foreground">
                  {t('httpRequest.fields.defaultHeaders')}
                </Label>
                <KeyValueEditor
                  nodeId={nodeId}
                  readOnly={readOnly}
                  items={(() => {
                    const headersStr =
                      nodeData.default_value?.find(i => i.key === 'headers')?.value || '[]';
                    try {
                      const parsed = JSON.parse(headersStr);
                      // Support both array format (new) and object format (legacy)
                      if (Array.isArray(parsed)) {
                        return parsed as Array<{ key: string; value: string }>;
                      }
                      // Convert legacy object format to array
                      return Object.entries(parsed).map(([key, value]) => ({
                        key,
                        value: String(value),
                      }));
                    } catch {
                      return [];
                    }
                  })()}
                  onChange={items => {
                    // Store as array to preserve items with empty keys
                    const current = nodeData.default_value || DEFAULT_ERROR_VALUES;
                    const updated = current.map(item =>
                      item.key === 'headers' ? { ...item, value: JSON.stringify(items) } : item
                    );
                    updateData({ default_value: updated });
                  }}
                  keyPlaceholder={t('httpRequest.placeholders.headerKey')}
                  valuePlaceholder={t('httpRequest.placeholders.headerValue')}
                />
              </div>

              {/* Body */}
              <div className="space-y-2">
                <Label className="text-xs text-muted-foreground">
                  {t('httpRequest.fields.defaultBody')}
                </Label>
                <textarea
                  className="w-full min-h-[80px] rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
                  value={nodeData.default_value?.find(i => i.key === 'body')?.value || ''}
                  onChange={e => {
                    const current = nodeData.default_value || DEFAULT_ERROR_VALUES;
                    const updated = current.map(item =>
                      item.key === 'body' ? { ...item, value: e.target.value } : item
                    );
                    updateData({ default_value: updated });
                  }}
                  disabled={readOnly}
                  placeholder={t('httpRequest.placeholders.bodyRaw')}
                />
              </div>
            </div>
          )}

          {/* Fail Branch indicator */}
          {nodeData.error_strategy === 'fail-branch' && (
            <div className="border rounded-lg p-3 bg-destructive/10 text-sm text-destructive">
              {t('httpRequest.fields.failBranchHint')}
            </div>
          )}
        </div>
      </div>

      {/* cURL Import Dialog */}
      <CurlImportDialog
        open={curlDialogOpen}
        onOpenChange={setCurlDialogOpen}
        onImportSuccess={onCurlImportSuccess}
      />
      <OutputVariablesView variables={outputs} />
    </div>
  );
};

export default React.memo(HttpRequestManager);

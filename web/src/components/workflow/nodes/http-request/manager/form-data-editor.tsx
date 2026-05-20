'use client';

import React from 'react';
import { Button } from '@/components/ui/button';
import { Plus, Trash } from 'lucide-react';
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select';
import WorkflowValueEditor from '../../../common/workflow-value-editor';
import NodeValueSelector from '../../../common/node-value-selector';
import { useT } from '@/i18n';
import type { HttpRequestBodyDataItem } from '../config';
import type { WorkflowVariable } from '../../../store/type';

interface FormDataEditorProps {
  items: HttpRequestBodyDataItem[];
  onChange: (items: HttpRequestBodyDataItem[]) => void;
  readOnly?: boolean;
  nodeId: string;
}

const FormDataEditor: React.FC<FormDataEditorProps> = ({
  items,
  onChange,
  readOnly = false,
  nodeId,
}) => {
  const t = useT();

  const addItem = () => {
    const newItem: HttpRequestBodyDataItem = {
      id: `key-value-${Date.now()}`,
      type: 'text',
      key: '',
      value: '',
    };
    onChange([...items, newItem]);
  };

  const updateItem = (idx: number, patch: Partial<HttpRequestBodyDataItem>) => {
    const next = items.map((item, i) => {
      if (i !== idx) return item;
      // When switching type, we need to handle the structure change
      if (patch.type && patch.type !== item.type) {
        if (patch.type === 'file') {
          return {
            id: item.id,
            type: 'file' as const,
            key: item.key,
            value: '',
            file: [],
          };
        } else {
          return {
            id: item.id,
            type: 'text' as const,
            key: item.key,
            value: item.type === 'file' ? '' : item.value,
          };
        }
      }
      return { ...item, ...patch } as HttpRequestBodyDataItem;
    });
    onChange(next);
  };

  const removeItem = (idx: number) => {
    const next = items.filter((_, i) => i !== idx);
    onChange(next);
  };

  const handleFileChange = (
    idx: number,
    payload: { sourceId: string; key: string; valuePath: string[]; type: WorkflowVariable['type'] }
  ) => {
    updateItem(idx, { file: payload.valuePath } as any);
  };

  return (
    <div className="space-y-3">
      <div className="border rounded-lg overflow-hidden bg-background">
        <div className="grid grid-cols-12 items-start bg-muted/50 border-b">
          <div className="col-span-3 h-9 leading-9 px-2.5 border-r font-medium text-sm uppercase tracking-wider">
            {t('nodes.httpRequest.fields.key')}
          </div>
          <div className="col-span-2 h-9 leading-9 px-2.5 border-r font-medium text-sm uppercase tracking-wider">
            {t('nodes.httpRequest.fields.formDataType')}
          </div>
          <div className="col-span-6 h-9 leading-9 px-2.5 font-medium text-sm uppercase tracking-wider">
            {t('nodes.httpRequest.fields.value')}
          </div>
          <div className="col-span-1" />
        </div>

        <div className="divide-y divide-border">
          {items.map((item, idx) => (
            <div key={item.id || idx} className="grid grid-cols-12 items-stretch group/row">
              {/* Key */}
              <div className="col-span-3 flex items-center border-r">
                <WorkflowValueEditor
                  placeholder={t('nodes.httpRequest.placeholders.paramKey')}
                  className="w-full h-full"
                  editorClassName="h-full rounded-none border-none py-2 px-2.5 focus:bg-accent/30 transition-colors"
                  value={item.key}
                  onChange={val => !readOnly && updateItem(idx, { key: val })}
                  nodeId={nodeId}
                  readOnly={readOnly}
                />
              </div>

              {/* Type selector */}
              <div className="col-span-2 flex items-center border-r px-1">
                <Select
                  value={item.type}
                  onValueChange={(v: 'text' | 'file') => updateItem(idx, { type: v })}
                  disabled={readOnly}
                >
                  <SelectTrigger
                    className={`h-8 w-full text-xs border-none shadow-none [&>svg]:hidden ${
                      item.type === 'file' ? 'text-amber-600' : 'text-blue-600'
                    }`}
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="text" className="text-blue-600">
                      {t('nodes.httpRequest.fields.formDataTypeText')}
                    </SelectItem>
                    <SelectItem value="file" className="text-amber-600">
                      {t('nodes.httpRequest.fields.formDataTypeFile')}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {/* Value - different based on type */}
              <div className="col-span-6 flex items-center bg-background/50">
                {item.type === 'text' ? (
                  <WorkflowValueEditor
                    placeholder={t('nodes.httpRequest.placeholders.bodyFormData')}
                    className="w-full h-full"
                    editorClassName="h-full rounded-none border-none py-2 px-2.5 focus:bg-accent/30 transition-colors"
                    value={item.value}
                    onChange={val => !readOnly && updateItem(idx, { value: val })}
                    nodeId={nodeId}
                    readOnly={readOnly}
                  />
                ) : (
                  <div className="w-full px-1.5">
                    <NodeValueSelector
                      nodeId={nodeId}
                      value={
                        Array.isArray(item.file) && item.file.length >= 2 ? item.file : undefined
                      }
                      onChange={payload => handleFileChange(idx, payload)}
                      typeFilter={type => type === 'file' || type === 'array[file]'}
                      placeholder={t('nodes.httpRequest.placeholders.selectFileVar')}
                      disabled={readOnly}
                    />
                  </div>
                )}
              </div>

              {/* Delete button */}
              <div className="col-span-1 flex items-center justify-center border-l bg-background/50">
                <Button
                  variant="ghost"
                  className="size-7 hover:text-destructive hover:bg-destructive/10 opacity-0 group-hover/row:opacity-100 transition-opacity"
                  isIcon
                  onClick={() => removeItem(idx)}
                  disabled={readOnly}
                >
                  <Trash size={14} />
                </Button>
              </div>
            </div>
          ))}

          {!readOnly && (
            <div
              className="flex items-center justify-center h-9 cursor-pointer hover:bg-muted/30 transition-all group active:bg-muted/50"
              onClick={addItem}
            >
              <Plus
                size={14}
                className="mr-1.5 group-hover:scale-110 group-hover:rotate-90 transition-transform duration-300"
              />
              <span className="text-sm font-medium">{t('nodes.httpRequest.fields.add')}</span>
            </div>
          )}
        </div>
      </div>
    </div>
  );
};

export default FormDataEditor;

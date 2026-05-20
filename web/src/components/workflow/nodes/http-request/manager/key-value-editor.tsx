'use client';

import React from 'react';
import { Button } from '@/components/ui/button';
import { Plus, Trash } from 'lucide-react';
import WorkflowValueEditor from '../../../common/workflow-value-editor';
import { useT } from '@/i18n';

interface KeyValueItem {
  key: string;
  value: string;
}

interface KeyValueEditorProps {
  items: KeyValueItem[];
  onChange: (items: KeyValueItem[]) => void;
  readOnly?: boolean;
  nodeId: string;
  keyPlaceholder?: string;
  valuePlaceholder?: string;
  title?: string;
}

const KeyValueEditor: React.FC<KeyValueEditorProps> = ({
  items,
  onChange,
  readOnly = false,
  nodeId,
  keyPlaceholder,
  valuePlaceholder,
  title,
}) => {
  const t = useT();

  const addItem = () => {
    onChange([...items, { key: '', value: '' }]);
  };

  const updateItem = (idx: number, patch: Partial<KeyValueItem>) => {
    const next = items.map((item, i) => (i === idx ? { ...item, ...patch } : item));
    onChange(next);
  };

  const removeItem = (idx: number) => {
    const next = items.filter((_, i) => i !== idx);
    onChange(next);
  };

  return (
    <div className="space-y-3">
      {title && (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 className="text-base font-semibold">{title}</h3>
        </div>
      )}
      <div className="border rounded-lg overflow-hidden bg-background">
        <div className="grid grid-cols-12 items-start bg-muted/50 border-b">
          <div className="col-span-4 h-9 leading-9 px-2.5 border-r font-medium text-sm uppercase tracking-wider">
            {t('nodes.httpRequest.fields.key')}
          </div>
          <div className="col-span-8 h-9 leading-9 px-2.5 font-medium text-sm uppercase tracking-wider">
            {t('nodes.httpRequest.fields.value')}
          </div>
        </div>

        <div className="divide-y divide-border">
          {items.map((item, idx) => (
            <div key={idx} className="grid grid-cols-12 items-stretch group/row">
              <div className="col-span-4 flex items-center border-r">
                <WorkflowValueEditor
                  placeholder={keyPlaceholder || t('nodes.httpRequest.placeholders.paramKey')}
                  className="w-full h-full"
                  editorClassName="h-full rounded-none border-none py-2 px-2.5 focus:bg-accent/30 transition-colors"
                  value={item.key}
                  onChange={val => !readOnly && updateItem(idx, { key: val })}
                  nodeId={nodeId}
                  readOnly={readOnly}
                />
              </div>
              <div className="col-span-7 flex items-center bg-background/50">
                <WorkflowValueEditor
                  placeholder={valuePlaceholder || t('nodes.httpRequest.placeholders.paramValue')}
                  className="w-full h-full"
                  editorClassName="h-full rounded-none border-none py-2 px-2.5 focus:bg-accent/30 transition-colors"
                  value={item.value}
                  onChange={val => !readOnly && updateItem(idx, { value: val })}
                  nodeId={nodeId}
                  readOnly={readOnly}
                />
              </div>
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

export default KeyValueEditor;

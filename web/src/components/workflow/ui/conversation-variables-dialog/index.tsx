'use client';

import React from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectTrigger,
  SelectContent,
  SelectItem,
  SelectValue,
} from '@/components/ui/select';
import { Trash2, Plus } from 'lucide-react';
import { useWorkflowStore } from '../../store';
import type { ConversationVariable } from '../../store/type';
import { CodeEditor } from '@/components/ui/code-editor';
import { generateClientId } from '@/utils/client-id';

interface Props {
  open: boolean;
  onClose: () => void;
}

// Supported primitive types for conversation variables (no file types by requirement)
const TYPE_OPTIONS: Array<ConversationVariable['type']> = [
  'string',
  'number',
  'boolean',
  'object',
  'array[string]',
  'array[number]',
  'array[boolean]',
  'array[object]',
];

function createEmptyVar(): ConversationVariable {
  return {
    id: generateClientId('conversation-variable'),
    name: '',
    type: 'string',
    value: '',
    description: '',
  };
}

const ConversationVariablesDialog: React.FC<Props> = ({ open, onClose }) => {
  const current = useWorkflowStore.use.workflowData().conversation_variables;
  const updateConversationVariables = useWorkflowStore.use.updateConversationVariables();
  const [items, setItems] = React.useState<ConversationVariable[]>([]);

  React.useEffect(() => {
    if (open) {
      setItems(Array.isArray(current) ? current : []);
    }
  }, [open, current]);

  const handleAdd = () => {
    setItems(prev => [...prev, createEmptyVar()]);
  };
  const handleRemove = (id: string) => setItems(prev => prev.filter(v => v.id !== id));
  const updateAt = (id: string, patch: Partial<ConversationVariable>) =>
    setItems(prev => prev.map(v => (v.id === id ? { ...v, ...patch } : v)));

  const parseValueByType = (t: ConversationVariable['type'], raw: string): unknown => {
    try {
      switch (t) {
        case 'string': {
          return raw;
        }
        case 'number': {
          return raw === '' ? '' : Number.isNaN(Number(raw)) ? '' : Number(raw);
        }
        case 'boolean': {
          return raw === 'true' || raw === '1';
        }
        case 'object': {
          return raw.trim() ? JSON.parse(raw) : {};
        }
        case 'array[string]':
        case 'array[number]':
        case 'array[boolean]':
        case 'array[object]': {
          return raw.trim() ? JSON.parse(raw) : [];
        }
        default: {
          return raw;
        }
      }
    } catch {
      // Fallback to empty container when JSON parse fails
      if (t === 'object') {
        return {};
      }
      if (t.startsWith('array')) {
        return [];
      }
      return raw;
    }
  };

  const serializeValueByType = (t: ConversationVariable['type'], v: unknown): string => {
    if (t === 'string') {
      return typeof v === 'string' ? v : '';
    }
    if (t === 'number') {
      return typeof v === 'number' || typeof v === 'string' ? String(v ?? '') : '';
    }
    if (t === 'boolean') {
      return v === true || v === 'true' ? 'true' : 'false';
    }
    try {
      return v != null ? JSON.stringify(v) : '';
    } catch {
      return '';
    }
  };

  const handleSave = () => {
    // Sanitize names (trim, unique, non-empty)
    const seen = new Set<string>();
    const sanitized: ConversationVariable[] = [];
    for (const v of items) {
      const name = (v.name || '').trim();
      if (name.length === 0) continue;
      if (seen.has(name)) continue;
      seen.add(name);
      sanitized.push({
        id: v.id,
        name,
        type: v.type,
        value: v.value,
        description: v.description,
      });
    }
    updateConversationVariables(sanitized);
    onClose();
  };

  return (
    <Dialog open={open} onOpenChange={openState => (!openState ? onClose() : undefined)}>
      <DialogContent className="max-w-3xl p-0 overflow-hidden">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            Conversation Variables
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-6 py-6">
          <div className="flex items-center justify-between bg-neutral-50 p-4 rounded-xl border border-neutral-200">
            <div className="text-sm text-muted-foreground font-medium max-w-[400px]">
              Variables are available to all nodes. Writable attribute is applied in editor only.
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={handleAdd}
              className="font-semibold shadow-sm"
            >
              <Plus className="w-4 h-4 mr-2" /> Add Variable
            </Button>
          </div>

          <div className="space-y-4">
            {items.map(item => (
              <div
                key={item.id}
                className="group relative border rounded-xl p-5 space-y-4 bg-white shadow-premium transition-all hover:ring-1 hover:ring-primary/20"
              >
                <div className="grid grid-cols-12 gap-4">
                  <div className="col-span-4 space-y-1.5">
                    <Label className="text-[10px] uppercase tracking-wider text-muted-foreground font-bold px-1">
                      Name
                    </Label>
                    <Input
                      placeholder="variable_name"
                      className="h-10 shadow-sm font-medium"
                      value={item.name}
                      onChange={e => updateAt(item.id, { name: e.target.value })}
                    />
                  </div>
                  <div className="col-span-4 space-y-1.5">
                    <Label className="text-[10px] uppercase tracking-wider text-muted-foreground font-bold px-1">
                      Type
                    </Label>
                    <Select
                      value={item.type}
                      onValueChange={v => {
                        const nextType = v as ConversationVariable['type'];
                        const nextVal =
                          nextType === 'string'
                            ? ''
                            : nextType === 'boolean'
                              ? 'false'
                              : nextType === 'number'
                                ? ''
                                : nextType.startsWith('array')
                                  ? '[]'
                                  : '{}';
                        updateAt(item.id, {
                          type: nextType,
                          value: parseValueByType(nextType, nextVal),
                        });
                      }}
                    >
                      <SelectTrigger className="h-10 shadow-sm font-medium">
                        <SelectValue placeholder="Select type" />
                      </SelectTrigger>
                      <SelectContent>
                        {TYPE_OPTIONS.map(t => (
                          <SelectItem key={t} value={t} className="font-medium">
                            {t}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="col-span-3 space-y-1.5">
                    <Label className="text-[10px] uppercase tracking-wider text-muted-foreground font-bold px-1">
                      Description
                    </Label>
                    <Input
                      placeholder="Optional info..."
                      className="h-10 shadow-sm"
                      value={item.description ?? ''}
                      onChange={e => updateAt(item.id, { description: e.target.value })}
                    />
                  </div>
                  <div className="col-span-1 flex items-end justify-end pb-1.5">
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-10 w-10 p-0 rounded-lg text-neutral-400 hover:text-destructive hover:bg-destructive/5"
                      onClick={() => handleRemove(item.id)}
                      aria-label="Delete"
                    >
                      <Trash2 className="w-5 h-5" />
                    </Button>
                  </div>
                </div>

                <div className="space-y-1.5">
                  <Label className="text-[10px] uppercase tracking-wider text-muted-foreground font-bold px-1">
                    Initial Value
                  </Label>
                  <div className="rounded-lg overflow-hidden border border-neutral-100 shadow-inner">
                    {item.type === 'string' || item.type === 'number' ? (
                      <Input
                        placeholder={item.type === 'string' ? 'Enter string value' : 'Enter number'}
                        className="h-11 border-none focus-visible:ring-0"
                        value={serializeValueByType(item.type, item.value)}
                        onChange={e =>
                          updateAt(item.id, { value: parseValueByType(item.type, e.target.value) })
                        }
                      />
                    ) : item.type === 'boolean' ? (
                      <Select
                        value={serializeValueByType(item.type, item.value)}
                        onValueChange={v => updateAt(item.id, { value: v === 'true' })}
                      >
                        <SelectTrigger className="h-11 border-none focus-visible:ring-0">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="true" className="font-semibold text-primary">
                            true
                          </SelectItem>
                          <SelectItem value="false" className="font-semibold">
                            false
                          </SelectItem>
                        </SelectContent>
                      </Select>
                    ) : (
                      <CodeEditor
                        value={serializeValueByType(item.type, item.value)}
                        onChange={val =>
                          updateAt(item.id, { value: parseValueByType(item.type, String(val)) })
                        }
                        language="json"
                        allowLanguages={['json']}
                        showLanguageSelector={false}
                        showCopyButton={false}
                        disableSuggest
                        height={140}
                        minHeight={140}
                        maxHeight={140}
                        className="border-none"
                      />
                    )}
                  </div>
                </div>
              </div>
            ))}

            {items.length === 0 && (
              <div className="flex flex-col items-center justify-center py-20 bg-neutral-50 rounded-2xl border-2 border-dashed border-neutral-200">
                <div className="size-12 rounded-full bg-neutral-100 flex items-center justify-center mb-4">
                  <Plus className="size-6 text-neutral-400" />
                </div>
                <div className="text-sm text-neutral-500 font-semibold mb-1">
                  No variables defined
                </div>
                <div className="text-xs text-neutral-400 mb-6 font-medium">
                  Add variables to share data across your workflow nodes.
                </div>
                <Button onClick={handleAdd} size="sm" className="font-bold">
                  Add Your First Variable
                </Button>
              </div>
            )}
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t">
          <Button variant="ghost" onClick={onClose} className="font-semibold">
            Cancel
          </Button>
          <Button onClick={handleSave} size="lg" className="px-10 font-bold">
            Save Variables
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default ConversationVariablesDialog;

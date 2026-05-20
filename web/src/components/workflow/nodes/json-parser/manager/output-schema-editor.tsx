'use client';

import React, { useCallback, useState, useEffect } from 'react';
import type { JsonParserOutputSchema, JsonParserOutputType } from '../config';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { CodeEditor } from '@/components/ui/code-editor';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { Plus, Trash2, ChevronRight, Code, AlertCircle } from 'lucide-react';

interface OutputSchemaEditorDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  editing: boolean;
  initialKey: string;
  initialSchema: JsonParserOutputSchema;
  existingKeys: string[];
  typeOptions: JsonParserOutputType[];
  onSubmit: (key: string, schema: JsonParserOutputSchema) => void;
}

/**
 * Validate output key name (must be valid identifier)
 */
function isValidKey(key: string): boolean {
  if (typeof key !== 'string' || key.length === 0) return false;
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(key);
}

/**
 * Check if type supports children
 */
function supportsChildren(type: JsonParserOutputType): boolean {
  return type === 'object' || type === 'array[object]';
}

/**
 * Infer JsonParserOutputType from a JavaScript value
 */
function inferType(value: unknown): JsonParserOutputType {
  if (value === null || value === undefined) return 'string';
  if (typeof value === 'string') return 'string';
  if (typeof value === 'number') return 'number';
  if (typeof value === 'boolean') return 'boolean';
  if (Array.isArray(value)) {
    if (value.length === 0) return 'array[string]';
    const first = value[0];
    if (typeof first === 'string') return 'array[string]';
    if (typeof first === 'number') return 'array[number]';
    if (typeof first === 'boolean') return 'array[boolean]';
    if (typeof first === 'object' && first !== null) return 'array[object]';
    return 'array[string]';
  }
  if (typeof value === 'object') return 'object';
  return 'string';
}

/**
 * Convert a JSON value to JsonParserOutputSchema recursively
 */
function jsonToSchema(value: unknown): JsonParserOutputSchema {
  const type = inferType(value);
  const schema: JsonParserOutputSchema = { type };

  if (type === 'object' && typeof value === 'object' && value !== null && !Array.isArray(value)) {
    const children: Record<string, JsonParserOutputSchema> = {};
    for (const [k, v] of Object.entries(value)) {
      if (isValidKey(k)) {
        children[k] = jsonToSchema(v);
      }
    }
    if (Object.keys(children).length > 0) {
      schema.children = children;
    }
  } else if (type === 'array[object]' && Array.isArray(value) && value.length > 0) {
    const first = value[0];
    if (typeof first === 'object' && first !== null) {
      const children: Record<string, JsonParserOutputSchema> = {};
      for (const [k, v] of Object.entries(first)) {
        if (isValidKey(k)) {
          children[k] = jsonToSchema(v);
        }
      }
      if (Object.keys(children).length > 0) {
        schema.children = children;
      }
    }
  }

  return schema;
}

type NodesTranslation = ReturnType<typeof useT<'nodes'>>;

interface SchemaTreeNodeProps {
  path: string;
  name: string;
  schema: JsonParserOutputSchema;
  typeOptions: JsonParserOutputType[];
  onUpdate: (path: string, schema: JsonParserOutputSchema) => void;
  onRemove: (path: string) => void;
  onAddChild: (path: string) => void;
  depth: number;
  t: NodesTranslation;
}

/**
 * Recursive tree node for schema editing (department tree style)
 */
const SchemaTreeNode: React.FC<SchemaTreeNodeProps> = ({
  path,
  name,
  schema,
  typeOptions,
  onUpdate,
  onRemove,
  onAddChild,
  depth,
  t,
}) => {
  const [expanded, setExpanded] = useState(true);
  const hasChildren = supportsChildren(schema.type);
  const childKeys = schema.children ? Object.keys(schema.children) : [];

  const handleTypeChange = useCallback(
    (newType: string) => {
      const newSchema: JsonParserOutputSchema = {
        type: newType as JsonParserOutputType,
      };
      // Preserve children if new type supports them
      if (supportsChildren(newType as JsonParserOutputType) && schema.children) {
        newSchema.children = schema.children;
      }
      onUpdate(path, newSchema);
    },
    [path, schema.children, onUpdate]
  );

  const handleToggle = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      if (hasChildren) {
        setExpanded(!expanded);
      }
    },
    [hasChildren, expanded]
  );

  return (
    <div className={cn(depth > 0 && 'mb-0.5')}>
      {/* Node row */}
      <div
        className={cn(
          'flex items-center gap-1 hover:bg-accent/50 transition-colors rounded-md group pr-1',
          depth === 0 && 'bg-accent/30'
        )}
      >
        {/* Expand/Collapse button */}
        {hasChildren ? (
          <Button
            type="button"
            variant="ghost"
            isIcon
            className={cn(
              'h-7 w-6 hover:bg-transparent hover:text-primary text-muted-foreground shrink-0',
              'transition-transform duration-200'
            )}
            onClick={handleToggle}
          >
            <ChevronRight
              className={cn('h-4 w-4 transition-transform duration-200', expanded && 'rotate-90')}
            />
          </Button>
        ) : (
          <div className="w-6 h-7 shrink-0" />
        )}

        {/* Field name with level-based styling */}
        <span
          className={cn(
            'font-mono text-sm transition-colors truncate min-w-[60px]',
            depth === 0 && 'font-semibold',
            depth === 1 && 'font-medium',
            depth >= 2 && 'font-normal text-muted-foreground'
          )}
        >
          {name}
        </span>

        {/* Type selector */}
        <Select value={schema.type} onValueChange={handleTypeChange}>
          <SelectTrigger className="h-6 w-[120px] text-xs ml-auto">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {typeOptions.map(type => (
              <SelectItem key={type} value={type} className="text-xs">
                {type}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {/* Action buttons - show on hover */}
        <div className="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity">
          {/* Add child button for object/array[object] */}
          {hasChildren && (
            <Button
              type="button"
              variant="ghost"
              isIcon
              onClick={() => onAddChild(path)}
              className="h-6 w-6 p-0 hover:bg-accent"
            >
              <Plus className="h-3.5 w-3.5" />
            </Button>
          )}

          {/* Remove button (not for root) */}
          {depth > 0 && (
            <Button
              type="button"
              variant="ghost"
              isIcon
              onClick={() => onRemove(path)}
              className="h-6 w-6 p-0 text-destructive hover:text-destructive hover:bg-destructive/10"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </Button>
          )}
        </div>
      </div>

      {/* Children with vertical line */}
      {hasChildren && expanded && (
        <div className="mt-0.5 ml-3 pl-3 border-l border-border space-y-0.5">
          {childKeys.length === 0 ? (
            <div className="text-xs text-muted-foreground py-1 pl-1 italic">
              {t('jsonParser.empty.noChildren')}
            </div>
          ) : (
            childKeys.map(childKey => {
              const childSchema = schema.children?.[childKey];
              if (!childSchema) return null;
              return (
                <SchemaTreeNode
                  key={childKey}
                  path={`${path}.${childKey}`}
                  name={childKey}
                  schema={childSchema}
                  typeOptions={typeOptions}
                  onUpdate={onUpdate}
                  onRemove={onRemove}
                  onAddChild={onAddChild}
                  depth={depth + 1}
                  t={t}
                />
              );
            })
          )}
        </div>
      )}
    </div>
  );
};

/**
 * Dialog for editing output schema with nested children support
 * Supports visual tree editor with JSON import button
 */
const OutputSchemaEditorDialog: React.FC<OutputSchemaEditorDialogProps> = ({
  open,
  onOpenChange,
  editing,
  initialKey,
  initialSchema,
  existingKeys,
  typeOptions,
  onSubmit,
}) => {
  const t = useT('nodes');
  const tc = useT('common');

  const [key, setKey] = useState(initialKey);
  const [schema, setSchema] = useState<JsonParserOutputSchema>(initialSchema);
  const [newChildKey, setNewChildKey] = useState('');
  const [addingChildPath, setAddingChildPath] = useState<string | null>(null);

  // JSON import dialog state
  const [jsonImportOpen, setJsonImportOpen] = useState(false);
  const [jsonInput, setJsonInput] = useState('');
  const [jsonError, setJsonError] = useState<string | null>(null);

  // Reset state when dialog opens
  useEffect(() => {
    if (open) {
      setKey(initialKey);
      setSchema(JSON.parse(JSON.stringify(initialSchema)));
      setNewChildKey('');
      setAddingChildPath(null);
      setJsonImportOpen(false);
      setJsonInput('');
      setJsonError(null);
    }
  }, [open, initialKey, initialSchema]);

  // Open JSON import dialog
  const handleOpenJsonImport = useCallback(() => {
    setJsonInput('');
    setJsonError(null);
    setJsonImportOpen(true);
  }, []);

  // Handle JSON input change (validate but don't apply yet)
  const handleJsonInputChange = useCallback((value: string) => {
    setJsonInput(value);
    if (!value.trim()) {
      setJsonError(null);
      return;
    }
    try {
      JSON.parse(value);
      setJsonError(null);
    } catch (e) {
      setJsonError(e instanceof Error ? e.message : 'Invalid JSON');
    }
  }, []);

  // Apply JSON import and close dialog
  const handleApplyJsonImport = useCallback(() => {
    if (!jsonInput.trim()) return;
    try {
      const parsed = JSON.parse(jsonInput);
      const newSchema = jsonToSchema(parsed);
      setSchema(newSchema);
      setJsonImportOpen(false);
      setJsonInput('');
      setJsonError(null);
    } catch (e) {
      setJsonError(e instanceof Error ? e.message : 'Invalid JSON');
    }
  }, [jsonInput]);

  // Validation
  const keyError = !isValidKey(key)
    ? t('jsonParser.validation.invalidKey')
    : existingKeys.includes(key)
      ? t('jsonParser.validation.duplicateKey')
      : null;

  const canSubmit = isValidKey(key) && !existingKeys.includes(key);

  // Update schema at path
  const handleUpdateSchema = useCallback((path: string, newSchema: JsonParserOutputSchema) => {
    setSchema(prev => {
      const updated = JSON.parse(JSON.stringify(prev)) as JsonParserOutputSchema;
      const parts = path.split('.').filter(Boolean);

      if (parts.length === 1) {
        // Root level update
        return newSchema;
      }

      // Navigate to parent and update
      let current = updated;
      for (let i = 1; i < parts.length - 1; i++) {
        const part = parts[i];
        if (current.children && part) {
          const child = current.children[part];
          if (child) {
            current = child;
          }
        }
      }

      const lastPart = parts[parts.length - 1];
      if (current.children && lastPart) {
        current.children[lastPart] = newSchema;
      }

      return updated;
    });
  }, []);

  // Remove schema at path
  const handleRemoveSchema = useCallback((path: string) => {
    setSchema(prev => {
      const updated = JSON.parse(JSON.stringify(prev)) as JsonParserOutputSchema;
      const parts = path.split('.').filter(Boolean);

      if (parts.length <= 1) return prev; // Can't remove root

      // Navigate to parent
      let current = updated;
      for (let i = 1; i < parts.length - 1; i++) {
        const part = parts[i];
        if (current.children && part) {
          const child = current.children[part];
          if (child) {
            current = child;
          }
        }
      }

      const lastPart = parts[parts.length - 1];
      if (current.children && lastPart) {
        delete current.children[lastPart];
      }

      return updated;
    });
  }, []);

  // Start adding child at path
  const handleStartAddChild = useCallback((path: string) => {
    setAddingChildPath(path);
    setNewChildKey('');
  }, []);

  // Confirm add child
  const handleConfirmAddChild = useCallback(() => {
    if (!addingChildPath || !isValidKey(newChildKey)) return;

    setSchema(prev => {
      const updated = JSON.parse(JSON.stringify(prev)) as JsonParserOutputSchema;
      const parts = addingChildPath.split('.').filter(Boolean);

      // Navigate to target
      let current = updated;
      for (let i = 1; i < parts.length; i++) {
        const part = parts[i];
        if (current.children && part) {
          const child = current.children[part];
          if (child) {
            current = child;
          }
        }
      }

      // Add child
      if (!current.children) {
        current.children = {};
      }
      current.children[newChildKey] = { type: 'string' };

      return updated;
    });

    setAddingChildPath(null);
    setNewChildKey('');
  }, [addingChildPath, newChildKey]);

  const handleSubmit = useCallback(() => {
    if (!canSubmit) return;
    onSubmit(key, schema);
  }, [canSubmit, key, schema, onSubmit]);

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>
              {editing ? t('jsonParser.modal.editTitle') : t('jsonParser.modal.addTitle')}
            </DialogTitle>
          </DialogHeader>

          <DialogBody className="space-y-4">
            {/* Output key name */}
            <div className="space-y-2">
              <Label htmlFor="output-key">{t('jsonParser.fields.outputKey')}</Label>
              <Input
                id="output-key"
                value={key}
                onChange={e => setKey(e.target.value)}
                placeholder={t('jsonParser.placeholders.outputKey')}
                className={cn(keyError && 'border-destructive')}
              />
              {keyError && <p className="text-xs text-destructive">{keyError}</p>}
            </div>

            {/* Schema editor */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>{t('jsonParser.fields.schema')}</Label>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={handleOpenJsonImport}
                  className="h-7 text-xs gap-1"
                >
                  <Code className="h-3 w-3" />
                  {t('jsonParser.editor.importJson')}
                </Button>
              </div>

              {/* Visual tree editor */}
              <div className="border rounded-md p-3 bg-muted/30 min-h-[200px] max-h-[400px] overflow-y-auto">
                <SchemaTreeNode
                  path={key || 'root'}
                  name={key || 'root'}
                  schema={schema}
                  typeOptions={typeOptions}
                  onUpdate={handleUpdateSchema}
                  onRemove={handleRemoveSchema}
                  onAddChild={handleStartAddChild}
                  depth={0}
                  t={t}
                />
              </div>

              {/* Add child input */}
              {addingChildPath && (
                <div className="flex items-center gap-2 p-3 border rounded-md bg-muted/50">
                  <Input
                    value={newChildKey}
                    onChange={e => setNewChildKey(e.target.value)}
                    placeholder={t('jsonParser.placeholders.childKey')}
                    className="h-8 text-sm"
                    autoFocus
                    onKeyDown={e => {
                      if (e.key === 'Enter') handleConfirmAddChild();
                      if (e.key === 'Escape') setAddingChildPath(null);
                    }}
                  />
                  <Button
                    type="button"
                    size="sm"
                    onClick={handleConfirmAddChild}
                    disabled={!isValidKey(newChildKey)}
                  >
                    {tc('add')}
                  </Button>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => setAddingChildPath(null)}
                  >
                    {tc('cancel')}
                  </Button>
                </div>
              )}
            </div>
          </DialogBody>

          <DialogFooter>
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              {tc('cancel')}
            </Button>
            <Button onClick={handleSubmit} disabled={!canSubmit}>
              {tc('save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* JSON Import Dialog */}
      <Dialog open={jsonImportOpen} onOpenChange={setJsonImportOpen}>
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>{t('jsonParser.editor.importJsonTitle')}</DialogTitle>
          </DialogHeader>

          <DialogBody className="space-y-3">
            <p className="text-sm text-muted-foreground">{t('jsonParser.editor.jsonHint')}</p>
            <CodeEditor
              value={jsonInput}
              onChange={handleJsonInputChange}
              language="json"
              height={280}
              showLanguageSelector={false}
              showCopyButton={false}
              disableSuggest
            />
            {jsonError && (
              <div className="flex items-center gap-2 text-xs text-destructive">
                <AlertCircle className="h-3 w-3" />
                {jsonError}
              </div>
            )}
          </DialogBody>

          <DialogFooter>
            <Button variant="outline" onClick={() => setJsonImportOpen(false)}>
              {tc('cancel')}
            </Button>
            <Button onClick={handleApplyJsonImport} disabled={!jsonInput.trim() || !!jsonError}>
              {t('jsonParser.editor.applyImport')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
};

export default OutputSchemaEditorDialog;

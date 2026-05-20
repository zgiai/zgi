'use client';

import React from 'react';
import { Panel } from '@xyflow/react';
import { usePanelStackItem } from '../../hooks';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { useT } from '@/i18n';
import { useWorkflowStore } from '../../store';
import type { EnvironmentVariable } from '../../store/type';
import { X, Plus, Pencil, Trash2, EyeOff, KeySquare } from 'lucide-react';
import EnvironmentVariableEditorDialog from './variable-editor-dialog';
import { getRightPanelMotionClassName, getRightPanelMotionStyle } from '../right-panel-motion';
import { generateClientId } from '@/utils/client-id';

const TYPE_OPTIONS: Array<EnvironmentVariable['type']> = ['string', 'number', 'secret'];

function createEmptyEnv(): EnvironmentVariable {
  const id = generateClientId('environment-variable');
  return { id, name: '', type: 'string', value: '', description: '', is_secret_set: false };
}

const EnvironmentVariablesPanel: React.FC<{
  open: boolean;
  temporarilyHidden?: boolean;
  onClose: () => void;
}> = ({ open, temporarilyHidden = false, onClose }) => {
  const t = useT('agents');
  const tCommon = useT('common');
  const { panelStyle } = usePanelStackItem({
    id: 'environment-variables',
    position: 'top-right',
    order: 0,
    visible: open,
    width: 400,
    gap: 8,
  });

  const vars = useWorkflowStore.use.workflowData().environment_variables as EnvironmentVariable[];
  const updateEnvironmentVariables = useWorkflowStore.use.updateEnvironmentVariables();
  const variables = React.useMemo(() => (Array.isArray(vars) ? vars : []), [vars]);

  const [editingOpen, setEditingOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<EnvironmentVariable>(createEmptyEnv());
  const [editingIndex, setEditingIndex] = React.useState<number | null>(null);

  const [shake, setShake] = React.useState(false);
  React.useEffect(() => {
    const win = window as Window & {
      __workflowEnvironmentPanelOpen?: boolean;
      __workflowEnvironmentPanelShake?: () => void;
    };
    win.__workflowEnvironmentPanelOpen = open;
    win.__workflowEnvironmentPanelShake = () => {
      setShake(true);
      window.setTimeout(() => setShake(false), 600);
    };
    return () => {
      win.__workflowEnvironmentPanelOpen = false;
      win.__workflowEnvironmentPanelShake = undefined as unknown as () => void;
    };
  }, [open]);

  const handleAdd = React.useCallback(() => {
    setEditing(createEmptyEnv());
    setEditingIndex(null);
    setEditingOpen(true);
  }, []);

  const handleEdit = React.useCallback(
    (index: number) => {
      const item = variables[index];
      if (!item) return;
      setEditing({ ...item });
      setEditingIndex(index);
      setEditingOpen(true);
    },
    [variables]
  );

  const handleRemove = React.useCallback(
    (index: number) => {
      const next = variables.filter((_, i) => i !== index);
      updateEnvironmentVariables(next);
    },
    [variables, updateEnvironmentVariables]
  );

  const handleSubmitDialog = React.useCallback(
    (item: EnvironmentVariable) => {
      // Get fresh variables from store to avoid stale closure data after auto-save
      const currentVars = useWorkflowStore.getState().workflowData
        .environment_variables as EnvironmentVariable[];
      const freshVars = Array.isArray(currentVars) ? currentVars : [];
      const next: EnvironmentVariable[] =
        typeof editingIndex === 'number'
          ? freshVars.map((v, i) => (i === editingIndex ? item : v))
          : [...freshVars, item];
      updateEnvironmentVariables(next);
      setEditingOpen(false);
      setEditingIndex(null);
    },
    [editingIndex, updateEnvironmentVariables]
  );

  if (!open) return null;

  return (
    <Panel
      position="top-right"
      aria-hidden={temporarilyHidden}
      className={getRightPanelMotionClassName(
        `p-0 bg-primary-foreground border border-muted rounded-lg shadow-lg w-[400px] h-[calc(100%-120px)] overflow-hidden ${shake ? 'workflow-panel-attention' : ''}`,
        temporarilyHidden
      )}
      style={getRightPanelMotionStyle(panelStyle, temporarilyHidden)}
    >
      <div className="flex flex-col h-full" onContextMenu={e => e.stopPropagation()}>
        <div className="flex items-center justify-between border-b px-3 py-2">
          <div className="font-medium flex items-center gap-1">
            <KeySquare className="h-5 w-5" /> {t('workflow.environmentVariables.title')}
          </div>
          <div className="flex items-center gap-2">
            <Button variant="ghost" isIcon onClick={onClose} aria-label={tCommon('close')}>
              <X size={16} className="text-primary" />
            </Button>
          </div>
        </div>

        <div className="flex-1 min-h-0 overflow-auto p-3">
          <div className="mb-2">
            <Alert className="bg-highlight/10">
              <AlertDescription>{t('workflow.environmentVariables.hint')}</AlertDescription>
            </Alert>
          </div>
          <div className="flex items-center justify-end mb-2">
            <Button onClick={handleAdd}>
              <Plus className="h-4 w-4 mr-1" /> {t('workflow.environmentVariables.actions.add')}
            </Button>
          </div>

          {variables.length === 0 ? null : (
            <div className="space-y-2">
              {variables.map((v, i) => (
                <div
                  key={v.id}
                  className="border rounded-md p-2 group relative overflow-hidden shadow-md"
                >
                  <div className="flex-1 space-y-1">
                    <div className="font-medium leading-none flex justify-between items-center min-h-6">
                      <div className="flex items-center gap-2 truncate">
                        <span className="truncate">{v.name}</span>
                        <Badge className="py-0 px-1 shrink-0">{v.type}</Badge>
                      </div>
                      <div className="hidden group-hover:flex items-center gap-1 shrink-0 ml-2">
                        <Button
                          variant="ghost"
                          isIcon
                          onClick={() => handleEdit(i)}
                          aria-label="Edit"
                          className="w-6 h-6"
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          isIcon
                          onClick={() => handleRemove(i)}
                          aria-label="Remove"
                          className="w-6 h-6 text-destructive hover:bg-red-100 hover:text-destructive"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </div>
                    {v.description ? (
                      <div className="text-xs text-muted-foreground/60 italic line-clamp-2">
                        {v.description}
                      </div>
                    ) : null}
                    {/* Display value: masked for secret, actual value for others */}
                    {v.type === 'secret' ? (
                      v.is_secret_set ? (
                        <div className="text-xs text-foreground/80 inline-flex items-center gap-1 bg-muted-foreground/10 px-1.5 py-0.5 rounded">
                          <EyeOff className="h-3 w-3" /> **********
                        </div>
                      ) : null
                    ) : v.value ? (
                      <div className="text-xs text-foreground/80 font-mono truncate bg-muted-foreground/10 px-1.5 py-0.5 rounded">
                        {v.value}
                      </div>
                    ) : null}
                  </div>
                </div>
              ))}
            </div>
          )}

          <EnvironmentVariableEditorDialog
            open={editingOpen}
            editing={typeof editingIndex === 'number'}
            value={editing}
            onChange={setEditing}
            onOpenChange={setEditingOpen}
            onSubmit={handleSubmitDialog}
            existingNames={variables
              .filter((_, i) => i !== editingIndex)
              .map(v => (v.name || '').trim())}
            typeOptions={TYPE_OPTIONS}
          />
        </div>
      </div>
    </Panel>
  );
};

export default EnvironmentVariablesPanel;

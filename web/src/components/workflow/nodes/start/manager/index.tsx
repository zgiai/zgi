import React, { useMemo, useState } from 'react';
import type { ValidationError } from '../../common/validation';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Plus, Trash2, GripVertical, AlertCircle, Edit } from 'lucide-react';
import { cn } from '@/lib/utils';
import type { InputVar, SystemVariable } from '../../../types/input-var';
import { validateInputVar, SYSTEM_VARIABLES } from '../../../types/input-var';
import type { StartNodeData } from '../../../store/type';
import VariableEditModal from './variable-edit-modal';
import { AgentType } from '@/services/types/agent';
import { useT } from '@/i18n';
import { useLocalNodeData } from '../../../hooks/use-local-node-data';
import { sanitizeIdentifier, ensureUniqueIdentifier } from '@/utils/validation';

interface VariableManagerProps {
  id: string; // Changed from nodeData/onUpdateNodeData
  className?: string;
  // Current agent type to determine system variable visibility
  agentType: AgentType;
  readOnly?: boolean;
}

interface VariableItemProps {
  variable: InputVar;
  index: number;
  onEdit: (index: number) => void;
  onRemove: (index: number) => void;
  errors?: ValidationError[];
  readOnly?: boolean;
}

/**
 * Individual variable item component
 */
const VariableItem: React.FC<VariableItemProps> = ({
  variable,
  index,
  onEdit,
  onRemove,
  errors = [],
  readOnly = false,
}) => {
  const hasErrors = errors.length > 0;
  const t = useT('nodes');

  return (
    <Card className={cn('relative', hasErrors && 'border-red-200 bg-red-50')}>
      <CardContent className="px-3 py-1">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <GripVertical className="w-4 h-4 text-gray-400 cursor-move" />
            <CardTitle className="text-sm font-medium">
              {variable.variable || t('start.item.variableN', { index: index + 1 })}
            </CardTitle>
            <Badge variant="outline" className="text-xs">
              {t(`start.types.${variable.type}` as Parameters<typeof t>[0])}
            </Badge>
            {variable.required && (
              <Badge variant="destructive" className="text-xs">
                {t('start.badge.required')}
              </Badge>
            )}
            {hasErrors && <AlertCircle className="w-4 h-4 text-red-500" />}
          </div>
          <div className="flex items-center gap-2">
            <Button variant="ghost" size="sm" onClick={() => onEdit(index)} disabled={readOnly}>
              <Edit className="w-4 h-4" />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onRemove(index)}
              disabled={readOnly}
              className="text-red-600 hover:text-red-700"
            >
              <Trash2 className="w-4 h-4" />
            </Button>
          </div>
        </div>

        {hasErrors && (
          <div className="text-sm text-red-600">
            {errors.map((error, i) => (
              <div key={i}>• {t(`${error.code}` as any, error.params as any)}</div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
};

interface SystemVariableItemProps {
  variable: SystemVariable;
}

const SystemVariableItem: React.FC<SystemVariableItemProps> = ({ variable }) => {
  const t = useT('nodes');
  // Get translated label and description
  const label = t(variable.label as any);
  const description = t(variable.description as any);

  return (
    <Card title={description}>
      <CardContent className="px-3 py-2">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2 justify-between w-full">
            <div className="flex flex-col">
              <CardTitle className="text-sm font-medium">{label}</CardTitle>
              <span className="text-xs text-muted-foreground">{variable.key}</span>
            </div>
            <Badge variant="outline" className="text-xs">
              {variable.type}
            </Badge>
          </div>
        </div>
      </CardContent>
    </Card>
  );
};

/**
 * Main variable manager component
 */
const VariableManager: React.FC<VariableManagerProps> = ({
  id,
  className,
  agentType,
  readOnly = false,
}) => {
  const t = useT('nodes');

  // Use store-aware useLocalNodeData to manage 'variables' field
  const { localData, setLocalData, flush } = useLocalNodeData<InputVar[]>(id, {
    path: 'variables',
    delay: 300,
  });

  const variables = localData || [];
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [editingVariable, setEditingVariable] = useState<InputVar | null>(null);
  const [editingVariableIndex, setEditingVariableIndex] = useState<number | null>(null);

  // Determine if the current agent is conversational to decide system variables visibility
  const isConversational = agentType === AgentType.CONVERSATIONAL_AGENT;

  const handleAddVariable = () => {
    if (readOnly) return;
    setEditingVariable(null);
    setEditingVariableIndex(null);
    setIsModalOpen(true);
  };

  const handleEditVariable = (index: number) => {
    if (readOnly) return;
    setEditingVariable(variables[index]);
    setEditingVariableIndex(index);
    setIsModalOpen(true);
  };

  const handleSaveVariable = (savedVariable: InputVar) => {
    const updatedVariables = [...variables];

    if (editingVariableIndex !== null) {
      const cleaned = sanitizeIdentifier(savedVariable.variable || '');
      const names = updatedVariables.map(v => v.variable).filter(Boolean) as string[];
      const exclude = variables[editingVariableIndex]?.variable || undefined;
      const unique = ensureUniqueIdentifier(cleaned, names, exclude);
      updatedVariables[editingVariableIndex] = { ...savedVariable, variable: unique } as InputVar;
    } else {
      const newVariable = { ...savedVariable };
      if (!newVariable.variable) {
        newVariable.variable = `variable_${variables.length + 1}`;
      }
      newVariable.variable = ensureUniqueIdentifier(
        sanitizeIdentifier(newVariable.variable || ''),
        updatedVariables.map(v => v.variable).filter(Boolean) as string[]
      );
      updatedVariables.push(newVariable);
    }

    setLocalData(updatedVariables);
    flush();
  };

  const handleRemoveVariable = (index: number) => {
    if (readOnly) return;
    const updatedVariables = [...variables];
    updatedVariables.splice(index, 1);

    setLocalData(updatedVariables);
    flush();
  };

  const getVariableErrors = (variable: InputVar): ValidationError[] => {
    return validateInputVar(variable);
  };

  const visibleSystemVariables = useMemo(() => {
    return SYSTEM_VARIABLES.filter(v => (isConversational ? true : !v.chatModeOnly));
  }, [isConversational]);

  return (
    <div className={cn('space-y-4', className)}>
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-medium">{t('start.section.inputs')}</h3>
        <Button onClick={handleAddVariable} disabled={readOnly}>
          <Plus className="w-4 h-4 mr-2" />
          {t('start.actions.addVariable')}
        </Button>
      </div>

      {variables.length === 0 ? (
        <Card>
          <CardContent className="py-8">
            <div className="text-center text-gray-500">
              <p>{t('start.empty.noInputs')}</p>
              <p className="text-sm mt-1">{t('start.empty.hint')}</p>
            </div>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {Array.isArray(variables) &&
            variables.map((variable, index) => (
              <VariableItem
                key={
                  (variable.variable && `var-${variable.variable}`) ||
                  (variable.label && `label-${variable.label}`) ||
                  `idx-${index}`
                }
                variable={variable}
                index={index}
                onEdit={handleEditVariable}
                onRemove={handleRemoveVariable}
                errors={getVariableErrors(variable)}
                readOnly={readOnly}
              />
            ))}
        </div>
      )}

      {/* System Variables Info */}
      <div className="space-y-2">
        <h3 className="text-lg font-medium">{t('start.section.system')}</h3>
        <div className="space-y-1">
          {visibleSystemVariables.map(variable => (
            <SystemVariableItem key={`sys-${variable.key}`} variable={variable} />
          ))}
        </div>
      </div>

      <VariableEditModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onSave={handleSaveVariable}
        variable={editingVariable}
      />
    </div>
  );
};

export default VariableManager;

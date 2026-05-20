import React, { useMemo } from 'react';
import type { StartNodeData } from './config';
import { AlertCircle } from 'lucide-react';
import { getInputVarTypeLabel } from '../../types/input-var';
import { checkValid } from './config';
import { useT } from '@/i18n';
import OutputVariablesView from '../../common/output-variables-view';

export interface StartContentProps {
  nodeId: string;
  data: StartNodeData;
}

// Content-only renderer for Start node; layout and handles are provided by CustomNode
const StartContent: React.FC<StartContentProps> = ({ nodeId: _nodeId, data }) => {
  const t = useT();
  const tNodes = useT('nodes');
  const translateNodeLabel = React.useCallback(
    (key: string, values?: Record<string, string | number | Date>) =>
      tNodes(key as never, values as never),
    [tNodes]
  );
  const validation = useMemo(() => checkValid(data), [data]);
  const hasErrors = !validation.isValid;
  const hasWarnings = validation.warnings.length > 0;
  const inputVariables = useMemo(
    () =>
      (data.variables || [])
        .filter(variable => !variable.hide && (variable.variable || variable.label))
        .map(variable => ({
          name: variable.label || variable.variable,
          type: getInputVarTypeLabel(variable.type, translateNodeLabel),
          description: variable.variable,
        })),
    [data.variables, translateNodeLabel]
  );

  return (
    <>
      <OutputVariablesView
        variant="compact"
        title={t('nodes.start.section.inputs')}
        variables={inputVariables}
        maxItems={3}
      />

      {(hasErrors || hasWarnings) && (
        <div className="mt-2 space-y-1">
          {validation.errors.slice(0, 2).map((error, index) => (
            <div
              key={`err-${error.code}-${index}`}
              className="text-xs text-red-600 flex items-center gap-1"
            >
              <AlertCircle className="w-3 h-3" />
              {t(`nodes.${error.code}`, error.params)}
            </div>
          ))}
          {validation.warnings.slice(0, 1).map((warning, index) => (
            <div
              key={`warn-${warning.code}-${index}`}
              className="text-xs text-yellow-600 flex items-center gap-1"
            >
              <AlertCircle className="w-3 h-3" />
              {t(`nodes.${warning.code}`, warning.params)}
            </div>
          ))}
          {(validation.errors.length > 2 || validation.warnings.length > 1) && (
            <div className="text-xs text-gray-500">
              {Math.max(0, validation.errors.length - 2) +
                Math.max(0, validation.warnings.length - 1)}{' '}
              {t('nodes.start.content.moreIssues')}
            </div>
          )}
        </div>
      )}
    </>
  );
};

export default StartContent;

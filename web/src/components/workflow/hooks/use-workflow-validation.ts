import { useMemo } from 'react';
import { useT } from '@/i18n';
import { useWorkflowStore } from '../store';
import type { StoreValidationError } from '../store/type';

/**
 * Lightweight hook for accessing workflow validation results from store.
 * The core O(V+E) validation logic now runs inside the store to avoid render waterfalls.
 */
const useWorkflowValidation = () => {
  const validationResults = useWorkflowStore.use.validationResults();
  const t = useT();

  // Map result errors/warnings to include translated messages
  const mappedResults = useMemo(() => {
    const translate = (issue: StoreValidationError) => ({
      type: issue.type,
      message: t(`nodes.${issue.code}` as any, issue.params),
      nodeId: issue.nodeId,
      nodeTitle: issue.nodeTitle,
    });

    const errors = validationResults.errors.map(translate);
    const warnings = validationResults.warnings.map(translate);

    return { errors, warnings };
  }, [validationResults.errors, validationResults.warnings, t]);

  const isValid = mappedResults.errors.length === 0;
  const hasWarnings = mappedResults.warnings.length > 0;

  const getNodeValidationStatus = (nodeId: string) => {
    const nodeErrors = validationResults.errorMap.get(nodeId) || [];
    const nodeWarnings = validationResults.warningMap.get(nodeId) || [];

    return {
      hasErrors: nodeErrors.length > 0,
      hasWarnings: nodeWarnings.length > 0,
      errors: nodeErrors.map(e => ({
        type: e.type,
        message: t(`nodes.${e.code}` as any, e.params),
        nodeId: e.nodeId,
        nodeTitle: e.nodeTitle,
      })),
      warnings: nodeWarnings.map(w => ({
        type: w.type,
        message: t(`nodes.${w.code}` as any, w.params),
        nodeId: w.nodeId,
        nodeTitle: w.nodeTitle,
      })),
    };
  };

  return {
    isValid,
    hasWarnings,
    errors: mappedResults.errors,
    warnings: mappedResults.warnings,
    getNodeValidationStatus,
  };
};

export default useWorkflowValidation;

/**
 * Start node configuration and validation methods
 */

import type { WorkflowNodeData } from '../../store/type';
import type { InputVar } from '../../types/input-var';
import type { InputVarType } from '../../types/input-var';
import { validateInputVar, createDefaultInputVar } from '../../types/input-var';
import type { ValidationResult, ValidationError } from '../common/validation';

export interface StartNodeData {
  type: 'start';
  title: string;
  desc: string;
  variables: InputVar[];
  unique: true;
  isInLoop?: boolean;
  isInIteration?: boolean;
}

// Default start node configuration
export const DEFAULT_START_NODE_CONFIG: Partial<StartNodeData> = {
  type: 'start',
  title: 'Start',
  desc: 'Agent start node',
  variables: [],
  unique: true,
  isInLoop: false,
  isInIteration: false,
};

// Available variable interface
export interface AvailableVariable {
  variable: string;
  type: string;
  description?: string;
  isSystem?: boolean;
}

// Available node with branch interface
export interface AvailableNodeWithBranch {
  nodeId: string;
  nodeType: string;
  title: string;
  branches?: string[];
}

/**
 * Validate start node configuration
 * @param nodeData - Start node data
 * @returns Validation result
 */
export const checkValid = (nodeData: StartNodeData): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  // Check if title is provided
  if (!nodeData.title || nodeData.title.trim() === '') {
    errors.push({ code: 'start.validation.nodeTitleRequired' });
  }

  // Validate variables
  if (nodeData.variables && nodeData.variables.length > 0) {
    const variableNames = new Set<string>();

    nodeData.variables.forEach((variable, index) => {
      // Validate individual variable
      const variableErrors = validateInputVar(variable);
      variableErrors.forEach(err => {
        errors.push({
          code: 'start.validation.variableIndexError',
          params: { index: index + 1, error: err.code }, // err.code is the key
        });
      });

      // Check for duplicate variable names
      if (variable.variable) {
        if (variableNames.has(variable.variable)) {
          errors.push({
            code: 'start.validation.duplicateVariableName',
            params: { variable: variable.variable },
          });
        } else {
          variableNames.add(variable.variable);
        }
      }

      // Check for system variable name conflicts
      if (variable.variable && variable.variable.startsWith('sys.')) {
        warnings.push({
          code: 'start.validation.systemVariableConflict',
          params: { variable: variable.variable },
        });
      }
    });
  }

  return {
    isValid: errors.length === 0,
    errors,
    warnings,
  };
};

/**
 * Get available variables from start node
 * @param nodeData - Start node data
 * @param includeSystem - Whether to include system variables
 * @returns Array of available variables
 */
export const getAvailableVars = (
  nodeData: StartNodeData,
  includeSystem: boolean = true
): AvailableVariable[] => {
  const variables: AvailableVariable[] = [];

  // Add custom variables
  if (nodeData.variables) {
    nodeData.variables.forEach(variable => {
      if (variable.variable && !variable.hide) {
        variables.push({
          variable: variable.variable,
          type: variable.type,
          isSystem: false,
        });
      }
    });
  }

  // Add system variables if requested
  if (includeSystem) {
    // Common system variables (all workflow types)
    variables.push(
      {
        variable: 'sys.tenant_id',
        type: 'string',
        description: 'Tenant ID',
        isSystem: true,
      },
      {
        variable: 'sys.user_id',
        type: 'string',
        description: 'User ID',
        isSystem: true,
      },
      {
        variable: 'sys.agent_id',
        type: 'string',
        description: 'Agent ID',
        isSystem: true,
      },
      {
        variable: 'sys.workflow_id',
        type: 'string',
        description: 'Workflow ID',
        isSystem: true,
      },
      {
        variable: 'sys.workflow_run_id',
        type: 'string',
        description: 'Workflow Run ID',
        isSystem: true,
      },
      {
        variable: 'sys.workflow_type',
        type: 'string',
        description: 'Workflow type (workflow or chat)',
        isSystem: true,
      },
      {
        variable: 'sys.query',
        type: 'string',
        description: 'User input',
        isSystem: true,
      },
      {
        variable: 'sys.files',
        type: 'array',
        description: 'Uploaded files list',
        isSystem: true,
      },
      // Chat workflow exclusive variables
      {
        variable: 'sys.conversation_id',
        type: 'string',
        description: 'Conversation ID (chat mode only)',
        isSystem: true,
      },
      {
        variable: 'sys.dialogue_count',
        type: 'number',
        description: 'Dialogue count (chat mode only)',
        isSystem: true,
      }
    );
  }

  return variables;
};

/**
 * Get available nodes with branch information
 * @param allNodes - All nodes in the workflow
 * @param currentNodeId - Current node ID to exclude
 * @returns Array of available nodes with branch info
 */
export const getAvailableNodesWithBranch = (
  allNodes: Array<{ id: string; type: string; data: WorkflowNodeData }>,
  currentNodeId: string
): AvailableNodeWithBranch[] => {
  return allNodes
    .filter(node => node.id !== currentNodeId)
    .map(node => ({
      nodeId: node.id,
      nodeType: node.data.type,
      title: node.data.title || node.data.type,
      branches: [], // Start node doesn't have branches
    }));
};

/**
 * Add a new variable to start node
 * @param nodeData - Current start node data
 * @param variableType - Type of variable to add
 * @returns Updated node data
 */
export const addVariable = (
  nodeData: StartNodeData,
  variableType: InputVarType = 'text-input' as InputVarType
): StartNodeData => {
  const newVariable = createDefaultInputVar(variableType);
  newVariable.variable = `variable_${(nodeData.variables?.length || 0) + 1}`;

  return {
    ...nodeData,
    variables: [...(nodeData.variables || []), newVariable],
  };
};

/**
 * Remove a variable from start node
 * @param nodeData - Current start node data
 * @param variableIndex - Index of the variable to remove
 * @returns Updated node data
 */
export const removeVariable = (nodeData: StartNodeData, variableIndex: number): StartNodeData => {
  const newVariables = [...(nodeData.variables || [])];
  newVariables.splice(variableIndex, 1);

  return {
    ...nodeData,
    variables: newVariables,
  };
};

/**
 * Update a variable in start node
 * @param nodeData - Current start node data
 * @param variableIndex - Index of the variable to update
 * @param updatedVariable - Partial variable info to update
 * @returns Updated node data
 */
export const updateVariable = (
  nodeData: StartNodeData,
  variableIndex: number,
  updatedVariable: Partial<InputVar>
): StartNodeData => {
  const newVariables = [...(nodeData.variables || [])];
  newVariables[variableIndex] = {
    ...newVariables[variableIndex],
    ...updatedVariable,
  } as InputVar;

  return {
    ...nodeData,
    variables: newVariables,
  };
};

/**
 * Get dependencies for a given variable across the workflow
 * @param variableName - The variable name to search for
 * @param allNodes - All nodes in the workflow
 * @param currentNodeId - The current node's ID to exclude
 * @returns Array of node IDs that depend on the given variable
 */
export const getVariableDependencies = (
  variableName: string,
  allNodes: Array<{ id: string; type: string; data: unknown }>,
  currentNodeId: string
): string[] => {
  const dependentNodes: string[] = [];

  allNodes.forEach(node => {
    if (node.id === currentNodeId) return;
    // In a real implementation, analyze node data to detect usage of the variable
    // Here we just simulate the dependency extraction
    const data = node.data;
    if (data && typeof data === 'object') {
      const values = Object.values(data);
      if (values.some(v => typeof v === 'string' && v.includes(`{{${variableName}}}`))) {
        dependentNodes.push(node.id);
      }
    }
  });

  return dependentNodes;
};

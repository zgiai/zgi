import { mailConnectionName } from '@/components/connectors/mail-connection-name';
import type { ConnectorPickerBindingRef } from '@/components/connectors/picker';
import type { PersonalConnection } from '@/services/types/connector';

interface SelectionInput {
  workflowID: string;
  nodeID: string;
  connectorID: string;
  currentConnectionID: string;
  bindingID?: string;
  revision?: number;
  personalConnection?: Pick<
    PersonalConnection,
    'id' | 'connector_id' | 'name' | 'external_display_name' | 'state' | 'external_account_id'
  >;
  locale?: string;
  icon?: string;
  unavailableLabel: string;
}
export function workflowConnectorSelection(input: SelectionInput): {
  pickerBindings: ConnectorPickerBindingRef[];
  selectedConnectionLabel: string;
} {
  const { personalConnection: connection, currentConnectionID, connectorID } = input;
  if (!currentConnectionID || !connectorID) {
    return { pickerBindings: [], selectedConnectionLabel: '' };
  }
  const owned =
    connection?.id === currentConnectionID && connection.connector_id === connectorID
      ? connection
      : undefined;
  return {
    pickerBindings: [
      {
        id: input.bindingID || `workflow-draft:${input.workflowID}:${input.nodeID}`,
        connection_id: currentConnectionID,
        connector_id: connectorID,
        revision: input.revision,
        status: owned?.state === 'ready' ? 'active' : 'needs_replacement',
      },
    ],
    selectedConnectionLabel: owned
      ? mailConnectionName(owned, input.locale) ||
        owned.name ||
        owned.external_display_name ||
        input.unavailableLabel
      : input.unavailableLabel,
  };
}

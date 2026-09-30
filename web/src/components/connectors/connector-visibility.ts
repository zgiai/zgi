import type { ConnectorPickerRow } from './picker/connector-picker-model';
import type { ConnectorPickerMenuGroup } from './picker/connector-picker-menu-model';

/** List every identified connector returned by the catalog; authorization remains independent. */
export function isConnectorListed(connectorId: string): boolean {
  return connectorId.trim().length > 0;
}

export function listedConnectorRows(rows: ConnectorPickerRow[]): ConnectorPickerRow[] {
  return rows.filter(row => isConnectorListed(row.connector.id) || Boolean(row.binding));
}

export function listedConnectorGroups(
  groups: ConnectorPickerMenuGroup[]
): ConnectorPickerMenuGroup[] {
  return groups
    .filter(group => isConnectorListed(group.id) || Boolean(group.currentBinding))
    .map(group => (isConnectorListed(group.id) ? group : { ...group, createRow: undefined }));
}

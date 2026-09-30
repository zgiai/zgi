import { isMailProvider, MAIL_PROVIDER_IDS } from '../mail-display-model';
import type { ConnectorPickerMenuGroup } from './connector-picker-menu-model';

export interface MailPickerGroup {
  kind: 'mail';
  id: 'mail';
  name: string;
  count: number;
  providers: ConnectorPickerMenuGroup[];
}
export type PickerDisplayGroup = ConnectorPickerMenuGroup | MailPickerGroup;
export function isMailPickerGroup(group: PickerDisplayGroup): group is MailPickerGroup {
  return 'providers' in group;
}
export function groupMailPickerProviders(
  groups: ConnectorPickerMenuGroup[],
  name: string,
  allGroups: ConnectorPickerMenuGroup[] = groups
): PickerDisplayGroup[] {
  if (!groups.some(group => isMailProvider(group.id))) return groups;
  const providers = MAIL_PROVIDER_IDS.flatMap(id => allGroups.filter(group => group.id === id));
  if (!providers.length) return groups;
  const mail: MailPickerGroup = {
    kind: 'mail',
    id: 'mail',
    name,
    providers,
    count: providers.reduce((count, group) => count + group.count, 0),
  };
  const result: PickerDisplayGroup[] = [];
  for (const group of groups) {
    if (!isMailProvider(group.id)) result.push(group);
    else if (!result.includes(mail)) result.push(mail);
  }
  return result;
}

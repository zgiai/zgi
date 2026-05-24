import type { ApprovalSMSRecipient, ApprovalSubmitMethods } from '../config';

export interface ApprovalSMSMemberOption {
  account_id: string;
  has_mobile?: boolean;
  phone_status?: ApprovalSMSMemberPhoneStatus;
}

export type ApprovalSMSMemberPhoneStatus = 'has_mobile' | 'no_mobile' | 'checking' | 'unconfirmed';

export function createSMSMemberOptionMap(
  memberOptions: ApprovalSMSMemberOption[]
): Map<string, ApprovalSMSMemberOption> {
  return new Map(memberOptions.map(member => [member.account_id, member]));
}

export function getDefaultSMSMemberAccountId(
  memberOptionsByAccountId: ReadonlyMap<string, ApprovalSMSMemberOption>,
  defaultRecipientAccountId: string
): string {
  const accountId = defaultRecipientAccountId.trim();
  if (!accountId) return '';

  const member = memberOptionsByAccountId.get(accountId);
  return getSMSMemberPhoneStatus(memberOptionsByAccountId, accountId) === 'has_mobile'
    ? (member?.account_id ?? '')
    : '';
}

export function getSMSMemberPhoneStatus(
  memberOptionsByAccountId: ReadonlyMap<string, ApprovalSMSMemberOption>,
  accountId: string
): ApprovalSMSMemberPhoneStatus {
  const trimmedAccountId = accountId.trim();
  if (!trimmedAccountId) return 'unconfirmed';

  const member = memberOptionsByAccountId.get(trimmedAccountId);
  if (!member) return 'unconfirmed';
  if (member.phone_status) return member.phone_status;
  if (member.has_mobile === true) return 'has_mobile';
  if (member.has_mobile === false) return 'no_mobile';
  return 'unconfirmed';
}

export function isSMSMemberUnavailable(
  memberOptionsByAccountId: ReadonlyMap<string, ApprovalSMSMemberOption>,
  accountId: string
): boolean {
  return getSMSMemberPhoneStatus(memberOptionsByAccountId, accountId) !== 'has_mobile';
}

export function isSMSRecipientIncomplete(
  recipient: ApprovalSMSRecipient,
  memberOptionsByAccountId: ReadonlyMap<string, ApprovalSMSMemberOption>
): boolean {
  if (recipient.type === 'external') {
    return !recipient.phone.trim();
  }

  return isSMSMemberUnavailable(memberOptionsByAccountId, recipient.account_id);
}

export function isApprovalSMSConfigIncomplete(
  smsConfig: ApprovalSubmitMethods['sms'],
  memberOptionsByAccountId: ReadonlyMap<string, ApprovalSMSMemberOption>
): boolean {
  return (
    smsConfig.enabled &&
    (!smsConfig.notification_title.trim() ||
      smsConfig.recipients.length === 0 ||
      smsConfig.recipients.some(recipient =>
        isSMSRecipientIncomplete(recipient, memberOptionsByAccountId)
      ))
  );
}

export function resolveSMSMemberAccountIdForTypeSwitch(
  recipient: ApprovalSMSRecipient,
  memberOptionsByAccountId: ReadonlyMap<string, ApprovalSMSMemberOption>,
  defaultSMSMemberAccountId: string
): string {
  if (
    recipient.type === 'member' &&
    getSMSMemberPhoneStatus(memberOptionsByAccountId, recipient.account_id) === 'has_mobile'
  ) {
    return recipient.account_id;
  }

  return defaultSMSMemberAccountId;
}

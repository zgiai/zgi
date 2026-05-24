import type { ApprovalSMSRecipient, ApprovalSubmitMethods } from '../config';

export interface ApprovalSMSMemberOption {
  account_id: string;
  has_mobile?: boolean;
}

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
  return member?.has_mobile ? member.account_id : '';
}

export function isSMSMemberUnavailable(
  memberOptionsByAccountId: ReadonlyMap<string, ApprovalSMSMemberOption>,
  accountId: string
): boolean {
  const trimmedAccountId = accountId.trim();
  if (!trimmedAccountId) return true;

  return memberOptionsByAccountId.get(trimmedAccountId)?.has_mobile !== true;
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
    memberOptionsByAccountId.get(recipient.account_id)?.has_mobile
  ) {
    return recipient.account_id;
  }

  return defaultSMSMemberAccountId;
}

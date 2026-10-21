import type {
  ConnectorCatalogItem,
  ConnectorOnboardingFlow,
} from '../../../services/types/connector.ts';

import { supportsMessagingContract } from './app-setup-state';
export { supportsMessagingContract, messagingMethod } from './app-setup-state';
// This is rendered locally as a QR image, never fetched as an image URL. The
// backend additionally enforces the deployment's exact admitted host list.
export function messagingQR(
  connector: ConnectorCatalogItem,
  flow?: ConnectorOnboardingFlow,
  now = Date.now()
): string | undefined {
  if (
    !supportsMessagingContract(connector) ||
    flow?.connector_id !== connector.id ||
    flow.protocol_version !== 1 ||
    flow.state !== 'awaiting_authorization' ||
    !(Date.parse(flow.expires_at) > now) ||
    !flow.qr_payload ||
    flow.qr_payload.length > 8192
  ) {
    return;
  }
  try {
    const url = new URL(flow.qr_payload);
    const suffix = connector.id === 'dingtalk' ? '.dingtalk.com' : '.weixin.qq.com';
    if (
      url.protocol === 'https:' &&
      !url.username &&
      !url.password &&
      !url.port &&
      !url.hash &&
      url.hostname.endsWith(suffix)
    ) {
      return flow.qr_payload;
    }
  } catch {
    /* Invalid provider payload is never rendered. */
  }
}

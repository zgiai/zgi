export type ConnectorBrand = 'exa' | 'github' | 'feishu' | 'dingtalk' | 'weixin' | 'wecom';

export function connectorBrand(connectorId?: string, icon?: string): ConnectorBrand | undefined {
  const id = connectorId?.trim().toLowerCase();
  if (id === 'web-search' || id === 'exa') return 'exa';
  if (id === 'github' || id === 'github-mcp') return 'github';
  if (id === 'feishu' || id === 'lark') return 'feishu';
  if (id === 'dingtalk') return 'dingtalk';
  if (id === 'weixin' || id === 'wechat') return 'weixin';
  if (id === 'wecom' || id === 'wechat-work') return 'wecom';
  // Tokens are metadata, not relative URLs. Do not override a different stable ID.
  if (id) return undefined;
  const token = icon?.trim().toLowerCase();
  if (token === 'exa' || token === 'globe-2') return 'exa';
  if (token === 'github') return 'github';
  if (token === 'feishu' || token === 'lark') return 'feishu';
  if (token === 'dingtalk') return 'dingtalk';
  if (token === 'weixin' || token === 'wechat') return 'weixin';
  if (token === 'wecom' || token === 'wechat-work') return 'wecom';
}

export function safeConnectorIconSource(icon?: string): string | undefined {
  if (
    !icon ||
    icon.length > 1024 ||
    icon !== icon.trim() ||
    icon.includes('..') ||
    /[\\?#%\s]/.test(icon)
  ) {
    return undefined;
  }
  return /^\/(?:icons|images|assets)\/[a-zA-Z0-9/_-]+\.(?:svg|png|webp|jpg)$/.test(icon)
    ? icon
    : undefined;
}

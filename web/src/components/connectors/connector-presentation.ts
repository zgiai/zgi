/** Product copy only. Catalog definitions and runtime tool instructions remain unchanged. */
const PURPOSES: Record<string, [string, string]> = {
  'web-search': ['搜索网页，读取网页内容', 'Search the web and read webpages'],
  github: [
    '查找仓库、代码、Issue 和 Pull Request',
    'Find repositories, code, issues and pull requests',
  ],
  feishu: [
    '使用飞书消息、文档、表格和协作资源',
    'Use Feishu messages, documents, tables and collaboration resources',
  ],
  dingtalk: [
    '连接钉钉机器人，发送消息和回复',
    'Connect a DingTalk bot to send messages and replies',
  ],
  wecom: [
    '连接企业微信机器人，使用消息、文档和表格',
    'Connect a WeCom bot to use messages, documents and tables',
  ],
  weixin: ['连接微信，接收消息和回复', 'Connect WeChat to receive messages and reply'],
  gmail: ['读取和发送 Gmail 邮件', 'Read and send Gmail messages'],
  qqmail: ['读取和发送 QQ 邮件', 'Read and send QQ Mail messages'],
  netease163: ['读取和发送 163 邮件', 'Read and send 163 Mail messages'],
};
export function connectorPurpose(id: string, locale: string): string | undefined {
  return PURPOSES[id]?.[locale.toLowerCase().startsWith('en') ? 1 : 0];
}

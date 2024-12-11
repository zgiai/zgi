/** 主进程发出的通知事件 */
export const INVOKE_CHANNLE = {
  /** 获取聊天记录 */
  loadChats: 'load-chats',
  /** 保存聊天记录 */
  saveChats: 'save-chats',
}

/** 渲染进程发出的通知，监听回调事件 */
export const RECEIVE_CHANNLE = {
  /** 更新配置文件 */
  demo: 'demo',
}

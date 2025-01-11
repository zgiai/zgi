/** Notification events emitted by the main process */
/** Notification events emitted by the main process */
export const INVOKE_CHANNLE = {
  /** Get chat history */
  loadChats: 'load-chats',
  /** Save chat history */
  saveChats: 'save-chats',
  saveAppSettings: 'saveAppSettings',
  loadAppSettings: 'loadAppSettings',
  saveUserInfo: 'saveUserInfo',
  loadUserInfo: 'loadUserInfo',
}

/** Notifications from the renderer process, listening for callback events */
/** Notifications from the renderer process, listening for callback events */
export const RECEIVE_CHANNLE = {
  demo: 'demo',
}

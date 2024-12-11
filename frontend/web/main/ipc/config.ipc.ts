import fs from 'node:fs'
import path from 'node:path'
import { INVOKE_CHANNLE } from '@shared/constants/channleName'
import { ipcMain } from 'electron'
import { app } from 'electron'

const CHAT_FILE_PATH = path.join(app.getPath('userData'), 'chat-data.json')

// 注册 IPC 处理程序
export function registerIpcHandlers() {
  // 读取聊天记录事件
  ipcMain.handle(INVOKE_CHANNLE.loadChats, async () => {
    try {
      const data = await fs.promises.readFile(CHAT_FILE_PATH, 'utf-8')
      return JSON.parse(data)
    } catch (e) {
      console.log('No chat history found or error reading file:', e)
      return { chatHistories: [], currentChatId: null }
    }
  })

  // 保存聊天记录事件
  ipcMain.handle(INVOKE_CHANNLE.saveChats, async (_, data) => {
    try {
      await fs.promises.writeFile(CHAT_FILE_PATH, JSON.stringify(data, null, 2), 'utf-8')
      return { success: true }
    } catch (e) {
      console.error('sava chats error:', e)
      return { success: false, error: e.message }
    }
  })
}

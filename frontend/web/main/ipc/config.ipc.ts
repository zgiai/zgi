import fs from 'node:fs'
import path from 'node:path'
import { INVOKE_CHANNLE } from '@shared/constants/channleName'
import { ipcMain } from 'electron'
import { app } from 'electron'

const CHAT_HISTORY_FILE_PATH = path.join(app.getPath('userData'), 'userChatHistoryData.json')
const APP_SETTINGS_FILE_PATH = path.join(app.getPath('userData'), 'appSettingsData.json')

// Register IPC handlers
export function registerIpcHandlers() {
  // Handle loading chats
  ipcMain.handle(INVOKE_CHANNLE.loadChats, async () => {
    try {
      const data = await fs.promises.readFile(CHAT_HISTORY_FILE_PATH, 'utf-8')
      return JSON.parse(data)
    } catch (e) {
      console.log('No chat history found or error reading file:', e)
      return { chatHistories: [], currentChatId: null }
    }
  })

  // Handle saving chats
  ipcMain.handle(INVOKE_CHANNLE.saveChats, async (_, data) => {
    try {
      await fs.promises.writeFile(CHAT_HISTORY_FILE_PATH, JSON.stringify(data, null, 2), 'utf-8')
      return { success: true }
    } catch (e) {
      console.error('sava chats error:', e)
      return { success: false, error: e.message }
    }
  })

  // Handle loading app settings
  ipcMain.handle(INVOKE_CHANNLE.loadAppSettings, async () => {
    try {
      const data = await fs.promises.readFile(APP_SETTINGS_FILE_PATH, 'utf-8')
      return JSON.parse(data)
    } catch (e) {
      console.log('No chat history found or error reading file:', e)
      return {}
    }
  })

  // Handle saving app settings
  ipcMain.handle(INVOKE_CHANNLE.saveAppSettings, async (_, data) => {
    try {
      await fs.promises.writeFile(APP_SETTINGS_FILE_PATH, JSON.stringify(data, null, 2), 'utf-8')
      return { success: true }
    } catch (e) {
      console.error('sava chats error:', e)
      return { success: false, error: e.message }
    }
  })
}

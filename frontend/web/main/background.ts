import fs from 'node:fs/promises'
import path from 'node:path'
import { app } from 'electron'
import serve from 'electron-serve'
import { createWindow } from './helpers/create-window'
import { registerIpcHandlers } from './ipc/config.ipc'

const isProd = process.env.NODE_ENV === 'production'

if (isProd) {
  serve({ directory: 'app' })
} else {
  app.setPath('userData', `${app.getPath('userData')} (development)`)
}

const WINDOW_STATE_FILE = path.join(app.getPath('userData'), 'window-state.json')

// 读取窗口状态
async function loadWindowState() {
  try {
    const data = await fs.readFile(WINDOW_STATE_FILE, 'utf-8')
    return JSON.parse(data)
  } catch (_e) {
    return null
  }
}

// 保存窗口状态
async function saveWindowState(window) {
  const state = {
    bounds: window.getBounds(),
    isMaximized: window.isMaximized(),
  }
  await fs.writeFile(WINDOW_STATE_FILE, JSON.stringify(state), 'utf-8')
}
// 主程序启动
;(async () => {
  await app.whenReady()
  registerIpcHandlers()
  const windowState = await loadWindowState()

  const mainWindow = createWindow('main', {
    width: windowState?.bounds?.width || 1000,
    height: windowState?.bounds?.height || 600,
    x: windowState?.bounds?.x,
    y: windowState?.bounds?.y,
    title: 'ZGIChat',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
    },
  })

  if (!windowState || windowState.isMaximized) {
    mainWindow.maximize()
  }
  ;['resize', 'move'].forEach((event) => {
    mainWindow.on(event, () => {
      saveWindowState(mainWindow)
    })
  })

  mainWindow.setMenu(null)

  if (isProd) {
    await mainWindow.loadURL('app://./chat')
  } else {
    const port = process.argv[2]
    await mainWindow.loadURL(`http://localhost:${port}/chat`)
    mainWindow.webContents.openDevTools()
  }
})()

app.on('window-all-closed', () => {
  app.quit()
})

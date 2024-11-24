import { contextBridge, ipcRenderer } from 'electron'

const handler = {
  // 主动发送通知，并获取返回数据，electron层使用handle处理
  invoke: (channel: string, ...args: any[]) => {
    return ipcRenderer.invoke(channel, ...args)
  },
  // 渲染进程监听到主进程发来的通知，执行相关的操作
  receive: (channel: string, func: (...args: any[]) => void) => {
    ipcRenderer.on(channel, (event, ...args) => func(...args))
  },
}

contextBridge.exposeInMainWorld('ipc', handler)

export type IpcHandler = typeof handler

import { INVOKE_CHANNLE } from "@shared/constants/channleName";
import { isDesktop } from "./utils";

/**
 * 存储适配器接口
 */
interface StorageAdapter {
  save: (data: any) => Promise<void>;
  load: () => Promise<any>;
}

/**
 * 桌面端存储适配器
 */
class DesktopStorageAdapter implements StorageAdapter {
  async save(data: any) {
    return window.ipc?.invoke(INVOKE_CHANNLE.saveChats, data);
  }

  async load() {
    return window.ipc?.invoke(INVOKE_CHANNLE.loadChats);
  }
}

/**
 * Web端存储适配器
 */
class WebStorageAdapter implements StorageAdapter {
  private readonly STORAGE_KEY = 'chat_store_data';

  async save(data: any) {
    try {
      localStorage.setItem(this.STORAGE_KEY, JSON.stringify(data));
    } catch (error) {
      console.error('保存到localStorage失败:', error);
    }
  }

  async load() {
    try {
      const data = localStorage.getItem(this.STORAGE_KEY);
      return data ? JSON.parse(data) : null;
    } catch (error) {
      console.error('从localStorage加载失败:', error);
      return null;
    }
  }
}

/**
 * 获取当前环境的存储适配器
 */
export const getStorageAdapter = (): StorageAdapter => {
  return isDesktop() ? new DesktopStorageAdapter() : new WebStorageAdapter();
};

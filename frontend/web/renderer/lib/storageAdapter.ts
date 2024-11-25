import { INVOKE_CHANNLE } from "@shared/constants/channleName";
import { isDesktop } from "./utils";

/**
 * Storage adapter interface
 */
interface StorageAdapter {
	save: (data: any) => Promise<void>;
	load: () => Promise<any>;
}

/**
 * Desktop storage adapter
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
 * Web storage adapter
 */
class WebStorageAdapter implements StorageAdapter {
	private readonly STORAGE_KEY = "chat_store_data";

	async save(data: any) {
		try {
			localStorage.setItem(this.STORAGE_KEY, JSON.stringify(data));
		} catch (error) {
			console.error("Failed to save to localStorage:", error);
		}
	}

	async load() {
		try {
			const data = localStorage.getItem(this.STORAGE_KEY);
			return data ? JSON.parse(data) : null;
		} catch (error) {
			console.error("Failed to load from localStorage:", error);
			return null;
		}
	}
}

/**
 * Get storage adapter for current environment
 */
export const getStorageAdapter = (): StorageAdapter => {
	return isDesktop() ? new DesktopStorageAdapter() : new WebStorageAdapter();
};

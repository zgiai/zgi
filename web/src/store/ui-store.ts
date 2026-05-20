/**
 * UI state store
 * Manages UI-specific state like sidebar visibility, modals, toasts, etc.
 */
import { create } from 'zustand';
import { createSelectors } from './utils/selectors';

interface UIState {
  // Sidebar state
  sidebarOpen: boolean;
  sidebarMobile: boolean;

  // Modal management
  activeModals: Record<string, boolean>;

  // Toast/notification queue
  notifications: Notification[];

  // Actions
  toggleSidebar: () => void;
  setSidebarOpen: (open: boolean) => void;
  setSidebarMobile: (mobile: boolean) => void;
  openModal: (modalId: string) => void;
  closeModal: (modalId: string) => void;
  addNotification: (notification: Notification) => void;
  removeNotification: (id: string) => void;
  clearNotifications: () => void;
}

interface Notification {
  id: string;
  type: 'info' | 'success' | 'warning' | 'error';
  title: string;
  message: string;
  duration?: number; // in milliseconds
  createdAt: number;
}

/**
 * UI store implementation
 * Non-persistent by default as UI state is typically session-specific
 */
const useUIStoreBase = create<UIState>()(set => ({
  // Initial state
  sidebarOpen: true,
  sidebarMobile: false,
  activeModals: {},
  notifications: [],

  // Actions
  toggleSidebar: () => set(state => ({ sidebarOpen: !state.sidebarOpen })),
  setSidebarOpen: open => set({ sidebarOpen: open }),
  setSidebarMobile: mobile => set({ sidebarMobile: mobile }),

  openModal: modalId =>
    set(state => ({
      activeModals: { ...state.activeModals, [modalId]: true },
    })),

  closeModal: modalId =>
    set(state => {
      const newModals = { ...state.activeModals };
      delete newModals[modalId];
      return { activeModals: newModals };
    }),

  addNotification: notification =>
    set(state => ({
      notifications: [
        ...state.notifications,
        {
          ...notification,
          id: notification.id || Date.now().toString(),
          createdAt: Date.now(),
        },
      ],
    })),

  removeNotification: id =>
    set(state => ({
      notifications: state.notifications.filter(n => n.id !== id),
    })),

  clearNotifications: () => set({ notifications: [] }),
}));

/**
 * UI store with selectors for optimized component updates
 *
 * @example
 * // Using individual selectors (preferred for performance)
 * const sidebarOpen = useUIStore.use.sidebarOpen();
 * const toggleSidebar = useUIStore.use.toggleSidebar();
 *
 * // Or using the entire store
 * const { sidebarOpen, toggleSidebar } = useUIStore();
 */
export const useUIStore = createSelectors(useUIStoreBase);

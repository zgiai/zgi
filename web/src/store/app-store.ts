/**
 * Application-level state store
 * Manages global application state like authentication, preferences, etc.
 */
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { createSelectors } from './utils/selectors';

interface AppState {
  // User & authentication
  user: User | null;
  isAuthenticated: boolean;

  // App preferences
  theme: 'light' | 'dark' | 'system';
  language: string;

  // App status
  isLoading: boolean;

  // Actions
  setUser: (user: User | null) => void;
  setAuthenticated: (status: boolean) => void;
  setTheme: (theme: 'light' | 'dark' | 'system') => void;
  setLanguage: (language: string) => void;
  setLoading: (isLoading: boolean) => void;
  logout: () => void;
}

interface User {
  id: string;
  name: string;
  email: string;
  avatar?: string;
  role?: string;
}

/**
 * App store implementation with persistence
 */
const useAppStoreBase = create<AppState>()(
  persist(
    set => ({
      // Initial state
      user: null,
      isAuthenticated: false,
      theme: 'system',
      language: 'en',
      isLoading: false,

      // Actions
      setUser: user => set({ user, isAuthenticated: !!user }),
      setAuthenticated: status => set({ isAuthenticated: status }),
      setTheme: theme => set({ theme }),
      setLanguage: language => set({ language }),
      setLoading: isLoading => set({ isLoading }),
      logout: () => set({ user: null, isAuthenticated: false }),
    }),
    {
      name: 'app-storage', // localStorage key
      partialize: state => ({
        user: state.user,
        theme: state.theme,
        language: state.language,
      }),
    }
  )
);

/**
 * App store with selectors for optimized component updates
 *
 * @example
 * // Using individual selectors (preferred for performance)
 * const theme = useAppStore.use.theme();
 * const setTheme = useAppStore.use.setTheme();
 *
 * // Or using the entire store
 * const { theme, setTheme } = useAppStore();
 */
export const useAppStore = createSelectors(useAppStoreBase);

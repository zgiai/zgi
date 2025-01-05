import { create } from 'zustand'
import type { AppSettingsStore } from './types'

export const useAppSettingsStore = create<AppSettingsStore>()((set, get) => {
  return {
    isOpenModal: false,
    activeSection: 'language-models', // Default active section
    expandedCards: [], // Track expanded card states

    setOpenModal: (flag: boolean) =>
      set({
        isOpenModal: flag,
      }),

    setActiveSection: (section: string) =>
      set({
        activeSection: section,
      }),

    toggleCard: (cardId: string) =>
      set((state) => ({
        expandedCards: state.expandedCards.includes(cardId)
          ? state.expandedCards.filter((id) => id !== cardId)
          : [...state.expandedCards, cardId],
      })),
  }
})

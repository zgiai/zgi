export interface ModelConfig {
  id: string
  name: string
  isEnabled: boolean
}

export interface AppSettingsStore {
  isOpenModal: boolean
  setOpenModal: (flag: boolean) => void
  activeSection: string
  setActiveSection: (section: string) => void
  expandedCards: string[]
  toggleCard: (cardId: string) => void
}

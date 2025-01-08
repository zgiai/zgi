import type { ChatHistory, ChatMessage, ModelConfig } from '@/types/chat'

/**
 * Chat state management interface
 * @interface ChatStore
 */
export interface ChatStore {
  currentChatId: string | null // Currently selected chat ID
  chatHistories: ChatHistory[] // All chat history records
  messageStreamingMap: Record<string, string> // Streaming message status for each chat
  isLoadingMap: Record<string, boolean> // Loading status for each chat
  isFirstOpen: boolean // Flag indicating if it's first open

  selectedModel: ModelConfig
  fileInputRef: React.RefObject<HTMLInputElement>
  attachments: File[] // New attachment state
  inputMessage: string // New message state
  refreshModelsLoading: boolean

  init: () => Promise<void>
  setCurrentChatId: (id: string | null) => void // Set current chat ID
  createChat: () => void // Create new chat
  deleteChat: (id: string) => void // Delete chat
  updateChatMessages: (chatId: string, messages: ChatMessage[]) => void // Update chat messages
  updateChatTitle: (chatId: string, title: string) => void // Update chat title
  clearAllChats: () => void // Clear all chats
  loadChatsFromDisk: () => void // Load chats from disk
  saveChatsToDisk: () => void // Save chats to disk
  sendMessage: (message: ChatMessage) => void // Send message
  updateChatTitleByContent: (chatId: string) => void // Add new method
  updateChatTitleByFirstMessage: (chatId: string) => void // Add new method
  setSelectedModel: (model: ModelConfig) => void
  setInputMessage: (msg: string) => void // Function to set message
  setAttachments: (files: File[]) => void // Function to set attachments
  handleSend: () => Promise<void> // Function to send message
}

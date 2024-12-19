import { getStorageAdapter } from '@/lib/storageAdapter'
import { localStreamChatCompletions } from '@/server/chat.server'
import { type ChatHistory, type ChatMessage, StreamChatMode } from '@/types/chat'
import { debounce } from 'lodash'
import React from 'react'
import { create } from 'zustand'
import { handleStreamResponse } from './handleStreamResponse'

/**
 * Chat state management interface
 * @interface ChatStore
 */
export interface ChatStore {
  currentChatId: string | null // Currently selected chat ID
  chatHistories: ChatHistory[] // All chat history records
  messageStreamingMap: Record<string, string> // Streaming message status for each chat
  isLoadingMap: Record<string, boolean> // Loading status for each chat
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
  isFirstOpen: boolean // Flag indicating if it's first open
  updateChatTitleByFirstMessage: (chatId: string) => void // Add new method
  models: string[] // Array of available models
  selectedModel: string
  setSelectedModel: (model: string) => void
  fileInputRef: React.RefObject<HTMLInputElement>
  attachments: File[] // New attachment state
  inputMessage: string // New message state
  setInputMessage: (msg: string) => void // Function to set message
  setAttachments: (files: File[]) => void // Function to set attachments
  handleSend: () => Promise<void> // Function to send message
}

// Define the configuration interface for stream response handling

/**
 * Create chat state management store
 */
export const useChatStore = create<ChatStore>()((set, get) => {
  const storageAdapter = getStorageAdapter()

  // Add helper function to update chat title based on content
  const updateChatTitleByContent = (chatId: string) => {
    const { chatHistories, isFirstOpen } = get()
    const chat = chatHistories.find((c) => c.id === chatId)

    // Only update title when the software is first opened
    if (!chat || !chat.messages.length || !isFirstOpen) return

    // Get the first text message
    const firstTextMessage = chat.messages.find(
      (msg) => msg.role === 'user' && !msg.fileType && msg.content.trim(),
    )

    if (firstTextMessage) {
      const newTitle =
        firstTextMessage.content.slice(0, 20) + (firstTextMessage.content.length > 20 ? '...' : '')

      set((state) => ({
        chatHistories: state.chatHistories.map((c) => {
          if (c.id === chatId) {
            return {
              ...c,
              title: newTitle,
            }
          }
          return c
        }),
      }))

      get().saveChatsToDisk()
    }
  }

  // Add helper function to update chat title based on first text message
  const updateChatTitleByFirstMessage = (chatId: string) => {
    const { chatHistories } = get()
    const chat = chatHistories.find((c) => c.id === chatId)

    if (!chat || !chat.messages.length) return

    // Find first text message (no fileType)
    const firstTextMessage = chat.messages.find(
      (msg) => msg.role === 'user' && !msg.fileType && msg.content.trim(),
    )

    if (firstTextMessage) {
      const newTitle =
        firstTextMessage.content.slice(0, 20) + (firstTextMessage.content.length > 20 ? '...' : '')

      set((state) => ({
        chatHistories: state.chatHistories.map((c) => {
          if (c.id === chatId) {
            return {
              ...c,
              title: newTitle,
            }
          }
          return c
        }),
      }))

      get().saveChatsToDisk()
    }
  }

  return {
    // Initial state
    currentChatId: null,
    chatHistories: [],
    messageStreamingMap: {},
    isLoadingMap: {},
    isFirstOpen: true, // Add first open flag
    models: ['gpt-4o', 'gpt-3.5'], // Add available models here
    selectedModel: 'gpt-4o', // Default model
    setSelectedModel: (model) => set({ selectedModel: model }), // Function to update selected model
    fileInputRef: React.createRef<HTMLInputElement>(),
    attachments: [], // Initialize attachment state
    inputMessage: '', // Initialize message state
    setInputMessage: (msg) => set({ inputMessage: msg }), // Set message
    setAttachments: (files) => set({ attachments: files }), // Set attachments
    handleSend: async () => {
      const { inputMessage, attachments, sendMessage, isLoadingMap, currentChatId } = get()
      const isLoading = currentChatId ? isLoadingMap[currentChatId] : false
      if (isLoading) return

      // Clear input and attachments immediately after sending
      set({ inputMessage: '', attachments: [] }) // Clear input and attachments

      try {
        const messages: ChatMessage[] = []

        // Handle attachments
        if (attachments.length > 0) {
          const filePromises = attachments.map((file) => {
            return new Promise<ChatMessage>((resolve) => {
              const reader = new FileReader()
              reader.onload = () => {
                const fileContent = reader.result as string
                const base64Content = fileContent.split(',')[1]

                resolve({
                  id: Date.now().toString(),
                  role: 'user',
                  content: base64Content,
                  fileType: file.type,
                  fileName: file.name,
                  timestamp: new Date().toISOString(),
                  skipAIResponse: true,
                })
              }
              reader.readAsDataURL(file)
            })
          })

          const fileMessages = await Promise.all(filePromises)
          messages.push(...fileMessages)
        }

        // Add text message
        if (inputMessage.trim()) {
          messages.push({
            id: Date.now().toString(),
            role: 'user',
            content: inputMessage.trim(),
            timestamp: new Date().toISOString(),
            skipAIResponse: false,
          })
        }

        // Send messages
        if (messages.length > 0) {
          for (const msg of messages) {
            await sendMessage(msg)
          }
        }
      } catch (error) {
        console.error('Failed to send message:', error)
      }
    },
    /**
     * Set current chat ID
     * @param id Chat ID
     */
    setCurrentChatId: (id) => {
      set({ currentChatId: id })
      get().saveChatsToDisk()
    },

    /**
     * Create new chat
     */
    createChat: () => {
      const newChat: ChatHistory = {
        id: Date.now().toString(),
        title: 'New Chat',
        messages: [],
        createdAt: new Date().toISOString(),
      }
      set((state) => ({
        chatHistories: [newChat, ...state.chatHistories],
        currentChatId: newChat.id,
      }))
      get().saveChatsToDisk()
    },

    /**
     * Delete specified chat
     * @param id Chat ID to delete
     */
    deleteChat: (id) => {
      set((state) => {
        const newHistories = state.chatHistories.filter((chat) => chat.id !== id)
        return {
          chatHistories: newHistories,
          currentChatId:
            state.currentChatId === id ? (newHistories[0]?.id ?? null) : state.currentChatId,
        }
      })
      get().saveChatsToDisk()
    },

    /**
     * Update message list for specified chat
     * @param chatId Chat ID
     * @param messages New message list
     */
    updateChatMessages: (chatId, messages) => {
      set((state) => ({
        chatHistories: state.chatHistories.map((chat) =>
          chat.id === chatId ? { ...chat, messages } : chat,
        ),
      }))
      get().saveChatsToDisk()
    },

    /**
     * Update title for specified chat
     * @param chatId Chat ID
     * @param title New title
     */
    updateChatTitle: (chatId, title) => {
      set((state) => ({
        chatHistories: state.chatHistories.map((chat) =>
          chat.id === chatId ? { ...chat, title } : chat,
        ),
      }))
      get().saveChatsToDisk()
    },

    /**
     * Clear all chats
     */
    clearAllChats: () => {
      set({ chatHistories: [], currentChatId: null })
      get().saveChatsToDisk()
    },

    /**
     * Load chat history from storage
     */
    loadChatsFromDisk: async () => {
      try {
        const data = await storageAdapter.load()
        if (data) {
          set({
            chatHistories: data.chatHistories || [],
            currentChatId: data.currentChatId || null,
            isFirstOpen: true, // Reset to true each time loading
          })

          // Update titles for all chats after loading
          if (data.chatHistories) {
            for (const chat of data.chatHistories) {
              get().updateChatTitleByFirstMessage(chat.id)
            }
          }
        }
      } catch (error) {
        console.error('Failed to load chat history:', error)
      }
    },

    /**
     * Save chat history to storage
     * Using debounce to avoid frequent saves
     */
    saveChatsToDisk: debounce(() => {
      const state = get()
      const data = {
        chatHistories: state.chatHistories,
        currentChatId: state.currentChatId,
      }
      storageAdapter.save(data)
    }, 1000),

    /**
     * Send message and handle AI response
     * @param message User message
     */
    sendMessage: async (message: ChatMessage) => {
      const { currentChatId } = get()
      let chatId = currentChatId

      // Check if already loading
      const isLoading = get().isLoadingMap[chatId || '']
      if (isLoading) return

      if (!chatId) {
        // Don't set title when creating new chat, wait for first message
        const newChat = {
          id: Date.now().toString(),
          title: 'New Chat',
          messages: [],
          createdAt: new Date().toISOString(),
        }

        set((state) => ({
          chatHistories: [newChat, ...state.chatHistories],
          currentChatId: newChat.id,
        }))

        chatId = newChat.id
      }

      const currentChat = get().chatHistories.find((chat) => chat.id === chatId)
      if (!currentChat) return

      // Add user message to history
      const newMessages = [...currentChat.messages, message]

      // Update status immediately
      set((state) => ({
        chatHistories: state.chatHistories.map((chat) => {
          if (chat.id === chatId) {
            return {
              ...chat,
              messages: newMessages,
            }
          }
          return chat
        }),
      }))

      // Update chat title if this is a text message
      if (!message.fileType && message.content.trim()) {
        get().updateChatTitleByFirstMessage(chatId)
      }

      // If it's a file message and marked to skip AI response, return
      if (message.skipAIResponse) {
        return
      }

      // Set loading status
      set((state) => ({
        isLoadingMap: { ...state.isLoadingMap, [chatId]: true },
        messageStreamingMap: { ...state.messageStreamingMap, [chatId]: '' },
      }))

      try {
        // Modify the format of the message sent to AI
        const messagesToSend = currentChat.messages.map((msg) => {
          if (msg.fileType?.includes('image/')) {
            // Handle image message
            let imageUrl = msg.content
            if (!msg.content.startsWith('http')) {
              // If not a URL, convert to base64
              imageUrl = `data:${msg.fileType};base64,${msg.content}`
            }

            return {
              role: msg.role,
              content: [
                {
                  type: 'image_url',
                  image_url: {
                    url: imageUrl,
                  },
                },
              ],
            }
          }
          // Handle normal text message
          return {
            role: msg.role,
            content: msg.content,
          }
        })

        // Handle the current message to send
        const currentMessageToSend = message.fileType?.includes('image/')
          ? {
              role: message.role,
              content: [
                {
                  type: 'image_url',
                  image_url: {
                    url: message.content.startsWith('http')
                      ? message.content
                      : `data:${message.fileType};base64,${message.content}`,
                  },
                },
              ],
            }
          : {
              role: message.role,
              content: message.content,
            }

        // If this is not a message to skip AI response, send request
        if (!message.skipAIResponse) {
          // const reader = await streamChatCompletions({
          // 	messages: [...messagesToSend, currentMessageToSend],
          // 	stream: true,
          // 	temperature: 1,
          // });
          const res = await localStreamChatCompletions({
            messages: [...messagesToSend, currentMessageToSend],
          })
          await handleStreamResponse({
            reader: res,
            chatId,
            messages: newMessages,
            set,
            onError: (error) => {
              console.error('Stream handling error:', error)
              set((state) => ({
                messageStreamingMap: {
                  ...state.messageStreamingMap,
                  [chatId]: 'Sorry, an error occurred while processing your message.',
                },
              }))
            },
            onComplete: (fullMessage) => {
              // Additional actions after completion if needed
              get().saveChatsToDisk()
            },
            streamMode: StreamChatMode.ollama,
          })
        }
      } catch (error) {
        console.error('Failed to send message:', error)
        set((state) => ({
          messageStreamingMap: {
            ...state.messageStreamingMap,
            [chatId]: 'Sorry, failed to send message. Please try again later.',
          },
        }))
      } finally {
        set((state) => ({
          isLoadingMap: { ...state.isLoadingMap, [chatId]: false },
        }))

        if (!get().messageStreamingMap[chatId]) {
          get().saveChatsToDisk()
        }
      }
    },

    updateChatTitleByContent, // Export method
    updateChatTitleByFirstMessage,
  }
})
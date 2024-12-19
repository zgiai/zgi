import { type ChatMessage, StreamChatMode } from '@/types/chat'
import type { ChatResponse } from 'ollama/dist/browser'
import type { ChatStore } from './types'

interface StreamResponseConfig {
  reader: ReadableStreamDefaultReader<Uint8Array> | AsyncIterable<ChatResponse>
  chatId: string
  messages: ChatMessage[]
  set: (
    partial: Partial<ChatStore> | ((state: ChatStore) => Partial<ChatStore>),
    replace?: boolean,
  ) => void
  onError?: (error: Error) => void
  onComplete?: (fullMessage: string) => void
  streamMode: StreamChatMode
}

// Refactored helper function with configuration object
export const handleStreamResponse = async ({
  reader,
  chatId,
  messages,
  set,
  onError,
  onComplete,
  streamMode,
}: StreamResponseConfig) => {
  const decoder = new TextDecoder()
  let fullMessage = ''

  try {
    if (streamMode === StreamChatMode.ollama) {
      for await (const part of reader as AsyncIterable<ChatResponse>) {
        try {
          const content = part.message.content
          if (content) {
            fullMessage += content
            // Update streaming message status
            set((state) => ({
              messageStreamingMap: {
                ...state.messageStreamingMap,
                [chatId]: fullMessage,
              },
            }))
          }
        } catch (error) {
          console.error('Error parsing JSON:', error)
          onError?.(error as Error)
        }
      }
    } else {
      while (true) {
        const { done, value } = await (reader as ReadableStreamDefaultReader<Uint8Array>).read()
        if (done) break

        const chunk = decoder.decode(value)
        const lines = chunk
          .split('\n')
          .filter((line) => line.trim() !== '' && line.trim() !== 'data: [DONE]')

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            try {
              const data = JSON.parse(line.slice(6))
              const content = data.choices[0]?.delta?.content
              if (content) {
                fullMessage += content
                // Update streaming message status
                set((state) => ({
                  messageStreamingMap: {
                    ...state.messageStreamingMap,
                    [chatId]: fullMessage,
                  },
                }))
              }
            } catch (error) {
              console.error('Error parsing JSON:', error)
              onError?.(error as Error)
            }
          }
        }
      }
    }

    // Add AI response to message list if we have a complete message
    if (fullMessage) {
      const assistantMessage: ChatMessage = {
        role: 'assistant',
        content: fullMessage,
        timestamp: new Date().toISOString(),
      }

      set((state) => ({
        chatHistories: state.chatHistories.map((chat) => {
          if (chat.id === chatId) {
            return {
              ...chat,
              messages: [...messages, assistantMessage],
            }
          }
          return chat
        }),
        messageStreamingMap: { ...state.messageStreamingMap, [chatId]: '' },
      }))

      onComplete?.(fullMessage)
    }

    return fullMessage
  } catch (error) {
    console.error('Error in stream handling:', error)
    onError?.(error as Error)
    throw error
  }
}

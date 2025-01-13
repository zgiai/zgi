import { useChatStore } from '@/store/chatStore'
import React, { useEffect, useRef, useState } from 'react'
import MessageItem from './MessageItem'

// Chat area component
const ChatArea = () => {
  const { currentChatId, chatHistories, messageStreamingMap, isLoadingMap } = useChatStore()
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const currentChat = chatHistories.find((chat) => chat.id === currentChatId)
  const streamingMessage = messageStreamingMap[currentChatId || '']
  const isLoading = isLoadingMap[currentChatId || '']
  const [isAtBottom, setIsAtBottom] = useState(true)

  const getAllMessages = () => {
    if (!currentChat) return []
    const baseMessages = currentChat.messages || []
    if (streamingMessage || isLoading) {
      return [...baseMessages, { role: 'assistant', content: streamingMessage || '' }]
    }
    return baseMessages
  }

  const messages = getAllMessages()

  const scrollToBottom = () => {
    if (isAtBottom) {
      messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
    }
  }

  const handleScroll = () => {
    const { scrollTop, clientHeight, scrollHeight } = messagesEndRef.current || ({} as any)
    setIsAtBottom(scrollTop + clientHeight >= scrollHeight)
  }

  useEffect(() => {
    if (currentChatId || messages.length > 0 || streamingMessage) {
      scrollToBottom()
    }
  }, [currentChatId, messages.length, streamingMessage])

  return (
    <div
      className="h-[calc(100vh-180px)] bg-white overflow-y-auto scrollbar-thin scrollbar-thumb-gray-300 scrollbar-track-gray-100 hover:scrollbar-thumb-gray-400"
      onScroll={handleScroll}
    >
      <div className="mx-auto pb-4">
        {messages.map((message, index) => (
          <MessageItem
            key={index}
            message={message}
            style={{
              marginBottom: index === messages.length - 1 ? '1rem' : undefined,
            }}
          />
        ))}
        <div ref={messagesEndRef} />
      </div>
    </div>
  )
}

export default ChatArea

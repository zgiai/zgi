'use client'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { useChatStore } from '@/store/chat.store'
import type { ChatCompletionResponse, ChatMessage } from '@/types/chat'
import {
  Bot,
  Image as ImageIcon,
  Paperclip,
  Plus,
  Search,
  SendHorizontal,
  Trash2,
} from 'lucide-react'
import * as React from 'react'
import { useEffect, useRef, useState } from 'react'

interface Message {
  role: 'user' | 'assistant'
  content: string
}

const SYSTEM_MESSAGE: ChatMessage = {
  role: 'system',
  content: 'You are a helpful assistant.',
}

// 添加类型定义
interface StreamChunk {
  id: string
  object: string
  created: number
  model: string
  system_fingerprint: string
  choices: {
    index: number
    delta: {
      content?: string
    }
    logprobs: null
    finish_reason: null | string
  }[]
}

// 添加类型定义
interface ChatHistory {
  id: string
  title: string
  messages: ChatMessage[]
  createdAt: string
}

const STORAGE_KEY = 'chat_histories'

export default function ChatPage() {
  // 添加 messagesEndRef 定义
  const messagesEndRef = useRef<HTMLDivElement>(null)

  // 1. 首先定义所有状态
  const {
    currentChatId,
    chatHistories,
    setCurrentChatId,
    createChat,
    deleteChat,
    updateChatMessages,
  } = useChatStore()

  const [input, setInput] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [streamingMessage, setStreamingMessage] = useState('')

  // 获取当前聊天的消息
  const currentChat = chatHistories.find((chat) => chat.id === currentChatId)
  const messages = currentChat?.messages || []

  const loadChatsFromDisk = useChatStore((state) => state.loadChatsFromDisk)

  useEffect(() => {
    loadChatsFromDisk()
  }, [])

  // 修改发送消息函数
  async function sendMessage(e: React.FormEvent) {
    e.preventDefault()
    if (!input.trim() || isLoading || !currentChatId) return

    const userMessage: ChatMessage = { role: 'user', content: input.trim() }
    const newMessages = [...messages, userMessage]
    updateChatMessages(currentChatId, newMessages)

    setInput('')
    setIsLoading(true)
    setStreamingMessage('')

    try {
      // ... 保持 API 调用部分不变 ...
      const response = await fetch('https://api.zgi.ai/v1/chat/completions', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          model: 'gpt-4o',
          messages: [...messages, userMessage],
          stream: true,
          temperature: 0.7,
          top_p: 1.0,
          n: 1,
          max_tokens: 4096,
        }),
      })

      if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`)

      const reader = response.body?.getReader()
      if (!reader) throw new Error('No reader available')

      const decoder = new TextDecoder()
      let fullMessage = ''
      while (true) {
        const { done, value } = await reader.read()
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
                setStreamingMessage(fullMessage)
              }
            } catch (error) {
              console.error('Error parsing JSON:', error)
            }
          }
        }
      }
      if (fullMessage) {
        const updatedMessages = [
          ...newMessages,
          {
            role: 'assistant',
            content: fullMessage,
          },
        ]
        updateChatMessages(currentChatId, updatedMessages as any)
      }
    } catch (error) {
      console.error('Error:', error)
    } finally {
      setIsLoading(false)
      setStreamingMessage('')
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    // Check if Enter is pressed without Shift key
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault() // Prevent default to avoid new line
      sendMessage(e)
    }
  }

  // 添加自动滚动到底部的效果
  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }

  // 当消息更新时自动滚动
  useEffect(() => {
    scrollToBottom()
  }, [messages, streamingMessage])

  return (
    <div className="flex h-[100dvh] bg-background">
      {/* 左侧聊天列表 */}
      <div className="hidden md:flex w-[300px] flex-col border-r">
        <div className="p-4">
          <Button
            variant="outline"
            className="w-full justify-start gap-2 mb-4"
            onClick={createChat}
            type="button"
          >
            <Plus className="h-4 w-4" />
            <span>新建对话</span>
          </Button>

          <div className="space-y-1">
            <h2 className="text-lg font-semibold tracking-tight mb-2">最近对话</h2>
            <ScrollArea className="h-[calc(100vh-180px)]">
              <div className="space-y-1">
                {chatHistories.map((chat) => (
                  <div key={chat.id} className="flex items-center gap-2 px-2">
                    <Button
                      variant={currentChatId === chat.id ? 'secondary' : 'ghost'}
                      className="w-full justify-start text-left"
                      onClick={() => setCurrentChatId(chat.id)}
                    >
                      <Bot className="mr-2 h-4 w-4" />
                      <span className="truncate">{chat.title}</span>
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={(e) => {
                        e.stopPropagation()
                        deleteChat(chat.id)
                      }}
                    >
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                ))}
              </div>
            </ScrollArea>
          </div>
        </div>
      </div>

      {/* 右侧聊天区域 */}
      <div className="flex-1 flex flex-col">
        <ScrollArea className="flex-1 p-4">
          <div className="max-w-3xl mx-auto">
            {messages.map((message, index) => (
              <div
                key={index}
                className={cn('mb-4', message.role === 'user' ? 'ml-auto' : 'mr-auto')}
              >
                <Card
                  className={cn(
                    'max-w-[80%]',
                    message.role === 'user' ? 'bg-primary text-primary-foreground' : 'bg-muted',
                  )}
                >
                  <CardContent className="p-3">
                    <p className="whitespace-pre-wrap">{message.content}</p>
                  </CardContent>
                </Card>
              </div>
            ))}
            {streamingMessage && (
              <div className="mr-auto mb-4">
                <Card className="bg-muted max-w-[80%]">
                  <CardContent className="p-3">
                    <p className="whitespace-pre-wrap">{streamingMessage}</p>
                  </CardContent>
                </Card>
              </div>
            )}
            <div ref={messagesEndRef} />
          </div>
        </ScrollArea>

        {/* 输入区域 */}
        <div className="border-t p-4">
          <form onSubmit={sendMessage} className="max-w-3xl mx-auto flex gap-4">
            <div className="relative flex-1">
              <Textarea
                placeholder="输入消息..."
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault()
                    sendMessage(e)
                  }
                }}
                rows={1}
                className="min-h-[44px] w-full resize-none py-3 pr-20"
              />
            </div>
            <Button type="submit" size="icon" disabled={isLoading}>
              <SendHorizontal className="h-4 w-4" />
              <span className="sr-only">发送消息</span>
            </Button>
          </form>
        </div>
      </div>
    </div>
  )
}

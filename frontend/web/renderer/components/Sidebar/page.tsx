import { useChatStore } from '@/store/chat.store'
import { debounce } from 'lodash'
import { MessageCircle, Plus, Search } from 'lucide-react'
import React, { useState, useCallback, useMemo } from 'react'

const Sidebar = () => {
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const { chatHistories, currentChatId, createChat, setCurrentChatId, deleteChat } = useChatStore()

  // 使用 lodash 的 debounce
  const debouncedSearch = useMemo(
    () =>
      debounce((value: string) => {
        setDebouncedQuery(value)
      }, 300),
    [],
  )

  const handleSearchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    setSearchQuery(value)
    debouncedSearch(value)
  }

  // 优化搜索过滤逻辑
  const filteredChats = chatHistories.filter((chat) => {
    const titleMatch = chat.title.toLowerCase().includes(debouncedQuery.toLowerCase())

    // 如果标题匹配或没有搜索词，直接返回
    if (titleMatch || !debouncedQuery) return true

    // 搜索最近的5条消息内容
    const recentMessages = chat.messages.slice(-5)
    return recentMessages.some((message) =>
      message.content.toLowerCase().includes(debouncedQuery.toLowerCase()),
    )
  })

  return (
    <div className="w-64 bg-white border-r border-gray-200 flex flex-col h-screen">
      <button
        onClick={createChat}
        className="m-4 p-2 bg-black text-white rounded-md flex items-center justify-center hover:bg-gray-800"
      >
        <Plus className="mr-2" size={18} />
        新建对话
      </button>

      <div className="px-4 mb-4">
        <div className="flex items-center bg-gray-100 rounded-md p-2">
          <Search size={18} className="text-gray-500 mr-2" />
          <input
            type="text"
            value={searchQuery}
            onChange={handleSearchChange}
            placeholder="搜索对话"
            className="bg-transparent outline-none w-full"
          />
          <span className="text-xs text-gray-500">⌘K</span>
        </div>
      </div>

      {/* 聊天历史记录 */}
      <div className="flex-1 overflow-y-auto">
        <div className="px-4 py-2">
          <h3 className="font-semibold mb-2">聊天记录</h3>
          <ul className="text-sm text-gray-600">
            {filteredChats.map((chat) => (
              <li
                key={chat.id}
                onClick={() => setCurrentChatId(chat.id)}
                className={`
                  p-2 rounded-md mb-1 cursor-pointer flex justify-between items-center
                  ${currentChatId === chat.id ? 'bg-gray-100' : 'hover:bg-gray-50'}
                `}
              >
                <span className="truncate">{chat.title}</span>
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    deleteChat(chat.id)
                  }}
                  className="text-gray-400 hover:text-red-500"
                >
                  ×
                </button>
              </li>
            ))}
          </ul>
        </div>
      </div>

      <div className="px-4 py-2">
        <div className="flex items-center text-purple-600">
          <MessageCircle size={18} className="mr-2" />
          <span>Explore Assistants</span>
        </div>
      </div>

      <div className="px-4 py-2">
        <h3 className="font-semibold mb-2">Examples</h3>
        <ul className="text-sm text-gray-600">
          <li className="mb-1">(Example) Top-Rated Restaurants...</li>
          <li className="mb-1">(Example) Top Performing...</li>
          <li className="mb-1">(Example) JavaScript Function to...</li>
        </ul>
      </div>

      <div className="px-4 py-2">
        <h3 className="font-semibold mb-2">Last 7 Days</h3>
        <ul className="text-sm text-gray-600">
          <li className="mb-1">Untitled</li>
        </ul>
      </div>

      <div className="mt-auto px-4 py-4">
        <button className="w-full p-2 border border-gray-300 rounded-md flex items-center justify-center">
          SignIn
        </button>
      </div>

      <div className="px-4 py-2 flex justify-between items-center text-sm text-gray-500">
        <div className="flex items-center">
          <button className="mr-2">Add API</button>
          <button>Settings</button>
        </div>
        <span>v 1.0.3</span>
      </div>
    </div>
  )
}

export default Sidebar

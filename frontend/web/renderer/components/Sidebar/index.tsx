import { useChatStore } from '@/store/chatStore'
import { ReloadIcon } from '@radix-ui/react-icons'
import { debounce } from 'lodash'
import { MessageCircle, Plus, Search } from 'lucide-react'
import type React from 'react'
import { useCallback, useMemo, useState } from 'react'
import SignInBtn from './SignInBtn'

const Sidebar = () => {
  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const {
    chatHistories,
    currentChatId,
    createChat,
    setCurrentChatId,
    deleteChat,
    createChatLoading,
  } = useChatStore()

  // Use lodash debounce
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

  // Optimize search filtering logic
  const filteredChats = chatHistories.filter((chat) => {
    const titleMatch = chat.title.toLowerCase().includes(debouncedQuery.toLowerCase())

    // Return if title matches or no search query
    if (titleMatch || !debouncedQuery) return true

    // Search in last 5 messages
    const recentMessages = chat.messages.slice(-5)
    return recentMessages.some((message) =>
      message.content.toLowerCase().includes(debouncedQuery.toLowerCase()),
    )
  })

  return (
    <div className="w-64 bg-white border-r border-gray-200 flex flex-col h-screen">
      <button
        type="button"
        onClick={createChat}
        className="m-4 p-2 bg-black text-white rounded-md flex items-center justify-center hover:bg-gray-800"
        disabled={createChatLoading}
      >
        {createChatLoading ? (
          <ReloadIcon className="mr-2 animate-spin" />
        ) : (
          <Plus className="mr-2" size={18} />
        )}
        New Chat
      </button>

      <div className="px-4 mb-4">
        <div className="flex items-center bg-gray-100 rounded-md p-2">
          <Search size={18} className="text-gray-500 mr-2" />
          <input
            type="text"
            value={searchQuery}
            onChange={handleSearchChange}
            placeholder="Search chats"
            className="bg-transparent outline-none w-full"
          />
          <span className="text-xs text-gray-500">⌘K</span>
        </div>
      </div>

      {/* Chat History */}
      <div className="flex-1 overflow-y-auto">
        <div className="px-4 py-2">
          <h3 className="font-semibold mb-2">Chat History</h3>
          <ul className="text-sm text-gray-600">
            {filteredChats.map((chat) => (
              <li
                key={chat.id}
                onClick={() => setCurrentChatId(chat.id)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    setCurrentChatId(chat.id)
                  }
                }}
                role="button"
                tabIndex={0}
                className={`
                  p-2 rounded-md mb-1 cursor-pointer flex justify-between items-center
                  ${currentChatId === chat.id ? 'bg-gray-100' : 'hover:bg-gray-50'}
                `}
              >
                <span className="truncate">{chat.title}</span>
                <button
                  type="button"
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

      <SignInBtn />

      <div className="px-4 py-2 flex justify-between items-center text-sm text-gray-500">
        <div className="flex items-center">
          <button type="button" className="mr-2">
            Add API
          </button>
          <button type="button">Settings</button>
        </div>
        <span>v 1.0.3</span>
      </div>
    </div>
  )
}

export default Sidebar

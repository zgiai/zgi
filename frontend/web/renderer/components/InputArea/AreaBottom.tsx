import { useChatStore } from '@/store/chatStore'
import type { ChatMessage } from '@/types/chat'
import { ChevronDown, FileText, LayoutGrid, Maximize, Send, Settings } from 'lucide-react'
import React from 'react'

const AreaBottom = () => {
  const {
    isLoadingMap,
    currentChatId,
    models,
    selectedModel,
    setSelectedModel,
    fileInputRef,
    inputMessage,
    attachments,
    handleSend,
  } = useChatStore()

  const isLoading = currentChatId ? isLoadingMap[currentChatId] : false

  const handleFileButtonClick = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    fileInputRef.current?.click()
  }

  const getModelDescription = (model: string) => {
    switch (model) {
      case 'Chat GPT 3.5':
        return '适合一般对话和问题解答'
      case 'Chat GPT 4':
        return '更强大的对话能力，适合复杂问题'
      default:
        return ''
    }
  }

  return (
    <div className="flex justify-between items-center mt-2 text-sm z-10">
      <div className="flex items-center gap-4">
        {/* File upload button */}
        <button
          type="button"
          onClick={handleFileButtonClick}
          className="flex items-center text-gray-500 hover:text-gray-600"
          title="Add attachment"
        >
          <FileText size={18} />
        </button>

        {/* Model selection dropdown */}
        <div className="mb-2 relative">
          <select
            value={selectedModel}
            onChange={(e) => setSelectedModel(e.target.value)}
            className="border border-gray-300 rounded p-2 w-full bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-blue-500 h-10"
          >
            {models.map((model) => (
              <option key={model} value={model}>
                {model} - {getModelDescription(model)}
              </option>
            ))}
          </select>
        </div>

        {/* Content safety protocol link */}
        <div className="text-gray-400 text-xs">
          <span>Please follow the </span>
          <a href="/safety-protocol" className="text-gray-500 hover:text-blue-500">
            content safety protocol
          </a>
          <span>. No inappropriate content allowed.</span>
        </div>
      </div>

      {/* Right side functionality */}
      <div className="flex items-center gap-3">
        {/* Settings button */}
        <button type="button" className="text-gray-500 hover:text-gray-600" title="Settings">
          <Settings size={18} />
        </button>

        {/* Format button */}
        <button type="button" className="text-gray-500 hover:text-gray-600" title="Format">
          <LayoutGrid size={18} />
        </button>

        {/* Fullscreen button */}
        <button type="button" className="text-gray-500 hover:text-gray-600" title="Fullscreen">
          <Maximize size={18} />
        </button>

        {/* Send button */}
        <button
          type="button"
          onClick={handleSend}
          className={`${
            isLoading ? 'bg-gray-400 cursor-not-allowed' : 'bg-[#3b82f6] hover:bg-blue-600'
          } text-white px-4 py-1.5 rounded-lg flex items-center gap-2`}
          disabled={isLoading || (!inputMessage.trim() && attachments.length === 0)}
        >
          {isLoading ? (
            <>
              <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
              <span>Send</span>
            </>
          ) : (
            <>
              <Send size={16} />
              <span>Send</span>
            </>
          )}
        </button>
      </div>
    </div>
  )
}

export default AreaBottom

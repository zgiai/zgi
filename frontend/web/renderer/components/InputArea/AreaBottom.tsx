import { useAppSettingsStore } from '@/store/appSettingsStore'
import { useChatStore } from '@/store/chatStore'
import { FileText, LayoutGrid, Maximize, Send, Settings } from 'lucide-react'
import React from 'react'
import ModelSelector from '../ModelSelector'

const AreaBottom = () => {
  const { isLoadingMap, currentChatId, fileInputRef, inputMessage, attachments, handleSend } =
    useChatStore()
  const { setOpenModal } = useAppSettingsStore()

  const isLoading = currentChatId ? isLoadingMap[currentChatId] : false

  const handleFileButtonClick = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    fileInputRef.current?.click()
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
        <ModelSelector />
      </div>

      {/* Right side functionality */}
      <div className="flex items-center gap-3">
        {/* Settings button */}
        <button
          type="button"
          className="text-gray-500 hover:text-gray-600"
          title="Settings"
          onClick={() => setOpenModal(true)}
        >
          <Settings size={18} />
        </button>

        {/* Format button */}
        {/* <button type="button" className="text-gray-500 hover:text-gray-600" title="Format">
          <LayoutGrid size={18} />
        </button> */}

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

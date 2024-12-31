import { useChatStore } from '@/store/chatStore'
import type React from 'react'
import { useEffect } from 'react'
import ModelSelector from '../ModelSelector'
import AreaBottom from './AreaBottom'

// Define allowed file types
const ALLOWED_FILE_TYPES = ['image/*', '.pdf', '.doc', '.docx', '.txt'].join(',')

const InputArea = () => {
  const {
    inputMessage,
    setInputMessage,
    attachments,
    setAttachments,
    handleSend,
    isLoadingMap,
    currentChatId,
    fileInputRef,
  } = useChatStore()
  const isLoading = currentChatId ? isLoadingMap[currentChatId] : false

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      if (!isLoading) {
        handleSend()
      }
    }
  }

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files || [])
    if (files.length > 0) {
      setAttachments([...attachments, ...files])
    }
    if (fileInputRef.current) {
      fileInputRef.current.value = ''
    }
  }

  // Clear input and attachments
  useEffect(() => {
    setInputMessage('')
    setAttachments([])
  }, [currentChatId])

  // Check if the file is an image
  const isImageFile = (file: File) => {
    return file.type.startsWith('image/')
  }

  return (
    <div
      className="absolute bottom-0 right-0 bg-white border-t border-gray-200 p-4"
      style={{ width: 'calc(100% - 256px)' }}
    >
      <div className="max-w-3xl mx-auto">
        <div className="flex flex-col">
          {/* Attachment preview area */}
          {attachments.length > 0 && (
            <div className="mb-2">
              {attachments.map((file, index) => (
                <div
                  key={`file-${file.name}-${index}`}
                  className="inline-flex items-center mr-2 mb-2"
                >
                  {isImageFile(file) ? (
                    <div className="relative group">
                      <img
                        src={URL.createObjectURL(file)}
                        alt={file.name}
                        className="h-16 w-16 object-cover rounded"
                      />
                      <button
                        type="button"
                        onClick={() => setAttachments(attachments.filter((_, i) => i !== index))}
                        className="absolute -top-2 -right-2 bg-white rounded-full p-0.5 shadow-sm 
                                 text-gray-500 hover:text-red-500"
                      >
                        ×
                      </button>
                    </div>
                  ) : (
                    <div className="flex items-center bg-gray-100 rounded-lg p-2">
                      <span className="truncate max-w-[200px]">{file.name}</span>
                      <button
                        type="button"
                        onClick={() => setAttachments(attachments.filter((_, i) => i !== index))}
                        className="ml-2 text-gray-500 hover:text-red-500"
                      >
                        ×
                      </button>
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}

          {/* Input area */}
          <div className="flex items-center bg-white border border-gray-300 rounded-lg">
            <textarea
              value={inputMessage}
              onChange={(e) => setInputMessage(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Enter your question. Press Enter to send, Shift+Enter for new line"
              className="flex-1 p-3 outline-none resize-none min-h-[40px] max-h-[200px] rounded-lg"
              rows={1}
            />
          </div>

          {/* Bottom functionality area */}
          <AreaBottom />

          {/* Hidden file upload input */}
          <input
            type="file"
            ref={fileInputRef}
            onChange={handleFileSelect}
            onClick={(e) => e.stopPropagation()}
            className="hidden"
            multiple
            accept={ALLOWED_FILE_TYPES}
          />
        </div>
      </div>
    </div>
  )
}

export default InputArea

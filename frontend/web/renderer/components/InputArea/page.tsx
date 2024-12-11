import { useChatStore } from '@/store/chat.store'
import { ChevronDown, FileText, LayoutGrid, Maximize, Send, Settings } from 'lucide-react'
import React, { useState, useRef } from 'react'

// 添加允许的文件类型常量
const ALLOWED_FILE_TYPES = ['image/*', '.pdf', '.doc', '.docx', '.txt'].join(',')

const InputArea = () => {
  const [message, setMessage] = useState('')
  const [attachments, setAttachments] = useState<File[]>([])
  const fileInputRef = useRef<HTMLInputElement>(null)
  const { sendMessage } = useChatStore()

  const handleSend = () => {
    // 首先发送文本消息（如果有）
    if (message.trim()) {
      sendMessage({
        id: Date.now().toString(),
        role: 'user',
        content: message,
        timestamp: new Date().toISOString(),
      })
    }

    // 然后单独发送每个附件
    attachments.forEach((file) => {
      if (isImageFile(file)) {
        // 将图片转换为 base64
        const reader = new FileReader()
        reader.onload = () => {
          sendMessage({
            id: Date.now().toString(),
            role: 'user',
            content: reader.result as string,
            fileType: 'image',
            fileName: file.name,
            timestamp: new Date().toISOString(),
          })
        }
        reader.readAsDataURL(file)
      } else {
        // 处理其他类型文件...
        sendMessage({
          id: Date.now().toString(),
          role: 'user',
          content: `[文件: ${file.name}]`,
          fileType: file.type,
          fileName: file.name,
          timestamp: new Date().toISOString(),
        })
      }
    })

    setMessage('')
    setAttachments([])
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files || [])
    if (files.length > 0) {
      console.log('Selected files:', files) // 添加日志以便调试
      setAttachments((prev) => [...prev, ...files])
    }
    // 重置 input 值，这样同一个文件可以重复选择
    if (fileInputRef.current) {
      fileInputRef.current.value = ''
    }
  }

  const handleFileButtonClick = (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    fileInputRef.current?.click()
  }

  // 判断文件是否为图片
  const isImageFile = (file: File) => {
    return file.type.startsWith('image/')
  }

  return (
    <div className="bg-white border-t border-gray-200 p-4">
      <div className="max-w-3xl mx-auto">
        <div className="flex flex-col">
          {/* 附件预览区域 */}
          {attachments.length > 0 && (
            <div className="mb-2">
              {attachments.map((file, index) => (
                <div key={index} className="inline-flex items-center mr-2 mb-2">
                  {isImageFile(file) ? (
                    <div className="relative group">
                      <img
                        src={URL.createObjectURL(file)}
                        alt={file.name}
                        className="h-16 w-16 object-cover rounded"
                      />
                      <button
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

          {/* 输入框区域 */}
          <div className="flex items-center bg-white border border-gray-300 rounded-lg">
            <textarea
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="输入您的问题，可通过enter发送，或使用shift+enter换行"
              className="flex-1 p-3 outline-none resize-none min-h-[40px]"
              rows={1}
            />
          </div>

          {/* 底部功能区 */}
          <div className="flex justify-between items-center mt-2 text-sm">
            <div className="flex items-center gap-4">
              {/* 文件上传按钮 - 使用 FileText 替换 Paperclip */}
              <button
                onClick={handleFileButtonClick}
                className="flex items-center text-gray-500 hover:text-gray-600"
                title="添加附件"
              >
                <FileText size={18} />
              </button>

              {/* 模型选择下拉框 */}
              <div className="flex items-center gap-1 text-gray-600">
                <img src="/gpt-icon.png" alt="GPT" className="w-4 h-4" />
                <span>GPT 4-Turbo</span>
                <ChevronDown size={14} className="text-gray-400" />
              </div>

              {/* 内容安全协议链接 */}
              <div className="text-gray-400 text-xs">
                <span>请遵守</span>
                <a href="#" className="text-gray-500 hover:text-blue-500">
                  内容安全协议
                </a>
                <span>，禁止交谈违规内容</span>
              </div>
            </div>

            {/* 右侧功能区 */}
            <div className="flex items-center gap-3">
              {/* 设置按钮 */}
              <button className="text-gray-500 hover:text-gray-600" title="设置">
                <Settings size={18} />
              </button>

              {/* 格式化按钮 - 使用 LayoutGrid */}
              <button className="text-gray-500 hover:text-gray-600" title="格式化">
                <LayoutGrid size={18} />
              </button>

              {/* 全屏按钮 - 使用 Maximize */}
              <button className="text-gray-500 hover:text-gray-600" title="全屏">
                <Maximize size={18} />
              </button>

              {/* 发送按钮 */}
              <button
                onClick={handleSend}
                className="bg-[#3b82f6] text-white px-4 py-1.5 rounded-lg hover:bg-blue-600 flex items-center gap-2"
                disabled={!message.trim() && attachments.length === 0}
              >
                <Send size={16} />
                <span>发送</span>
              </button>
            </div>
          </div>

          {/* 隐藏的文件上传输入框 */}
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

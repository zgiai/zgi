import { useChatStore } from '@/store/chatStore'
import { Bot, FileText, User } from 'lucide-react'
import { ImageIcon } from 'lucide-react'
import React, { useEffect, useRef, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import { PrismLight as SyntaxHighlighter } from 'react-syntax-highlighter'
import { vscDarkPlus } from 'react-syntax-highlighter/dist/cjs/styles/prism'

// Message item component
const MessageItem = ({ message, style }) => {
  const { currentChatId, isLoadingMap } = useChatStore()
  const isUser = message.role === 'user'
  const isLoading = currentChatId ? isLoadingMap[currentChatId] : false
  // Add copy state
  const [copiedMap, setCopiedMap] = React.useState<Record<string, boolean>>({})
  // Function to generate a unique code ID
  const generateCodeId = (code: string) => {
    let hash = 0
    for (let i = 0; i < code.length; i++) {
      const char = code.charCodeAt(i)
      hash = (hash << 5) - hash + char
      hash = hash & hash // Convert to 32bit integer
    }
    return `code-${Math.abs(hash)}`
  }

  // Function to handle code copying
  const handleCopyCode = (code: string, codeId: string) => {
    if (copiedMap[codeId]) return

    navigator.clipboard
      .writeText(code)
      .then(() => {
        setCopiedMap((prev) => ({ ...prev, [codeId]: true }))
        setTimeout(() => {
          setCopiedMap((prev) => ({ ...prev, [codeId]: false }))
        }, 1000)
      })
      .catch((err) => {
        console.error('Copy failed:', err)
      })
  }

  const renderContent = () => {
    // If it's a streaming message and has no content (loading)
    if (isLoading && !message.content) {
      return (
        <div className="flex items-center space-x-2">
          <div className="flex space-x-1">
            <div
              className="w-2 h-2 bg-gray-400 rounded-full animate-bounce"
              style={{ animationDelay: '0ms' }}
            />
            <div
              className="w-2 h-2 bg-gray-400 rounded-full animate-bounce"
              style={{ animationDelay: '150ms' }}
            />
            <div
              className="w-2 h-2 bg-gray-400 rounded-full animate-bounce"
              style={{ animationDelay: '300ms' }}
            />
          </div>
          <span className="text-sm text-gray-500">AI is thinking...</span>
        </div>
      )
    }

    const contentClass = isLoading ? 'typing-effect' : ''

    // Handle array form of message content
    if (Array.isArray(message.content)) {
      return (
        <div className={`${contentClass} break-words whitespace-pre-wrap`}>
          {message.content.map((item, index) => {
            if (item.type === 'file') {
              const isImage =
                item.fileType?.includes('image/') ||
                /\.(jpg|jpeg|png|gif|webp)$/i.test(item.fileName || '')

              if (isImage) {
                // Process image URL logic
                let imageUrl = ''
                if (item.content.startsWith('data:') || item.content.startsWith('http')) {
                  imageUrl = item.content
                } else {
                  // Ensure correct MIME type
                  const mimeType =
                    item.fileType ||
                    (item.fileName?.toLowerCase().endsWith('.webp') ? 'image/webp' : 'image/jpeg')
                  imageUrl = `data:${mimeType};base64,${item.content}`
                }

                return (
                  <div key={index} className="mb-2">
                    <img
                      src={imageUrl}
                      alt={item.fileName || 'Image'}
                      className="rounded-lg max-w-full h-auto"
                      onError={(e) => {
                        console.error('Image loading failed:', e)
                        const target = e.target as HTMLImageElement
                        target.style.display = 'none'
                        const errorDiv = document.createElement('div')
                        errorDiv.className = 'text-red-500 text-sm mt-2'
                        errorDiv.textContent = 'Image loading failed'
                        target.parentElement?.appendChild(errorDiv)
                      }}
                    />
                    {item.fileName && (
                      <div className="flex items-center gap-2 mt-2 text-sm text-gray-500">
                        <ImageIcon size={16} />
                        <span>{item.fileName}</span>
                      </div>
                    )}
                  </div>
                )
              }
              // Handle base64 image data
              const imageUrl = item.content.startsWith('data:')
                ? item.content
                : `data:image/jpeg;base64,${item.content}`

              return (
                <div key={index} className="mb-2 break-words whitespace-pre-wrap">
                  <img
                    src={imageUrl}
                    alt={item.fileName || 'Image'}
                    className="rounded-lg max-w-full h-auto"
                  />
                  {item.fileName && (
                    <div className="text-sm mt-1 text-gray-500">{item.fileName}</div>
                  )}
                </div>
              )
            }
            if (item.type === 'text') {
              return (
                <div key={index} className="mb-2 break-words whitespace-pre-wrap">
                  <ReactMarkdown
                    components={{
                      code({ node, className, children, ...props }) {
                        const match = /language-(\w+)/.exec(className || '')

                        if (!match) {
                          return (
                            <code className={className} {...props}>
                              {children}
                            </code>
                          )
                        }

                        const code = String(children).replace(/\n$/, '')
                        // Use new ID generation function
                        const codeId = generateCodeId(code)
                        const isCopied = copiedMap[codeId]

                        return (
                          <div className="relative group">
                            <div className="absolute right-2 top-2 opacity-0 group-hover:opacity-100">
                              <button
                                onClick={() => handleCopyCode(code, codeId)}
                                disabled={isCopied}
                                className={`px-2 py-1 rounded text-xs ${
                                  isCopied
                                    ? 'bg-gray-500 text-white cursor-not-allowed'
                                    : 'bg-gray-700 hover:bg-gray-600 text-white'
                                }`}
                              >
                                {isCopied ? 'Copied' : 'Copy Code'}
                              </button>
                            </div>
                            <SyntaxHighlighter
                              style={vscDarkPlus}
                              language={match[1]}
                              PreTag="div"
                              {...props}
                            >
                              {code}
                            </SyntaxHighlighter>
                          </div>
                        )
                      },
                    }}
                  >
                    {item.content}
                  </ReactMarkdown>
                </div>
              )
            }
            return null
          })}
        </div>
      )
    }

    // Handle file type messages
    if (message.fileType || message.fileName) {
      const isImage =
        message.fileType?.includes('image/') ||
        /\.(jpg|jpeg|png|gif|webp)$/i.test(message.fileName || '')

      if (isImage) {
        // Handle image display
        let imageUrl = ''
        if (message.content.startsWith('data:') || message.content.startsWith('http')) {
          imageUrl = message.content
        } else {
          // Ensure correct MIME type
          const mimeType =
            message.fileType ||
            (message.fileName?.toLowerCase().endsWith('.webp') ? 'image/webp' : 'image/jpeg')
          imageUrl = `data:${mimeType};base64,${message.content}`
        }

        return (
          <div className="max-w-sm">
            <img
              src={imageUrl}
              alt={message.fileName || 'Image'}
              className="rounded-lg max-w-full h-auto"
              onError={(e) => {
                console.error('Image loading failed:', e)
                const target = e.target as HTMLImageElement
                target.style.display = 'none'
                const errorDiv = document.createElement('div')
                errorDiv.className = 'text-red-500 text-sm mt-2'
                errorDiv.textContent = 'Image loading failed'
                target.parentElement?.appendChild(errorDiv)
              }}
            />
            {message.fileName && (
              <div className="flex items-center gap-2 mt-2 text-sm text-gray-500">
                <ImageIcon size={16} />
                <span>{message.fileName}</span>
              </div>
            )}
          </div>
        )
      }

      // Handle other file types
      return (
        <div className="flex items-center gap-2 p-3 bg-gray-50 rounded-lg">
          <FileText size={20} className="text-gray-500" />
          <span className="text-sm text-gray-700">{message.fileName || 'Unknown file'}</span>
          {message.fileType && (
            <span className="text-xs text-gray-500">
              ({message.fileType.split('/').pop()?.toUpperCase() || 'Unknown format'})
            </span>
          )}
        </div>
      )
    }

    // Handle regular text messages
    return (
      <div className={`${contentClass} break-words whitespace-pre-wrap`}>
        <ReactMarkdown
          components={{
            code({ node, className, children, ...props }) {
              const match = /language-(\w+)/.exec(className || '')

              if (!match) {
                return (
                  <code className={className} {...props}>
                    {children}
                  </code>
                )
              }

              const code = String(children).replace(/\n$/, '')
              // Use new ID generation function
              const codeId = generateCodeId(code)
              const isCopied = copiedMap[codeId]

              return (
                <div className="relative group">
                  <div className="absolute right-2 top-2 opacity-0 group-hover:opacity-100">
                    <button
                      onClick={() => handleCopyCode(code, codeId)}
                      disabled={isCopied}
                      className={`px-2 py-1 rounded text-xs ${
                        isCopied
                          ? 'bg-gray-500 text-white cursor-not-allowed'
                          : 'bg-gray-700 hover:bg-gray-600 text-white'
                      }`}
                    >
                      {isCopied ? 'Copied' : 'Copy Code'}
                    </button>
                  </div>
                  <SyntaxHighlighter
                    style={vscDarkPlus}
                    language={match[1]}
                    PreTag="div"
                    {...props}
                  >
                    {code}
                  </SyntaxHighlighter>
                </div>
              )
            },
            p: ({ children }) => <p className="break-words whitespace-pre-wrap">{children}</p>,
          }}
        >
          {message.content}
        </ReactMarkdown>
      </div>
    )
  }

  return (
    <div style={style} className="py-2">
      <div className={`flex items-start gap-4 px-4 ${isUser ? 'flex-row-reverse' : 'flex-row'}`}>
        <div className="flex-shrink-0 mt-1.5">
          <div
            className={`w-8 h-8 rounded-full flex items-center justify-center ${isUser ? 'bg-purple-600' : 'bg-gray-200'}`}
          >
            {isUser ? (
              <User size={20} className="text-white" />
            ) : (
              <Bot size={20} className="text-gray-700" />
            )}
          </div>
        </div>
        <div
          className={`shrink-0 max-w-[80%] rounded-lg p-3 overflow-hidden ${isUser ? 'bg-[#F5FAFF] text-gray-800' : 'bg-white text-gray-800'}`}
        >
          {renderContent()}
        </div>
      </div>
    </div>
  )
}

export default MessageItem

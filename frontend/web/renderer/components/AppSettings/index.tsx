import { useAppSettingsStore } from '@/store/appSettingsStore'
import { useChatStore } from '@/store/chatStore'
import { Dialog } from '@headlessui/react'
import { Switch } from '@headlessui/react'
import React, { useState } from 'react'

const AppSettings = () => {
  const { isOpenModal, setOpenModal, activeSection, setActiveSection, expandedCards, toggleCard } =
    useAppSettingsStore()

  // Sidebar items configuration
  const sidebarItems = [
    { id: 'general', icon: '⚙️', label: '通用设置' },
    { id: 'language-models', icon: '🤖', label: '语言模型' },
    { id: 'voice-services', icon: '🎤', label: '语音服务' },
    { id: 'default-assistant', icon: '💬', label: '默认助手' },
    { id: 'about', icon: 'ℹ️', label: '关于' },
  ]

  return (
    <Dialog open={isOpenModal} onClose={() => setOpenModal(false)} className="relative z-10">
      <div className="fixed inset-0 bg-black/30" aria-hidden="true" />
      <div className="fixed inset-0 flex items-center justify-center p-4">
        <Dialog.Panel className="mx-auto w-[1000px] h-[600px] rounded-lg bg-white shadow-xl flex">
          {/* Sidebar */}
          <div className="w-48 border-r border-gray-200 p-4">
            <h2 className="text-lg font-medium mb-4">设置</h2>
            <div className="space-y-2">
              {sidebarItems.map((item) => (
                <button
                  key={item.id}
                  onClick={() => setActiveSection(item.id)}
                  className={`w-full text-left px-3 py-2 rounded-lg flex items-center space-x-2
                    ${activeSection === item.id ? 'bg-blue-50 text-blue-600' : 'hover:bg-gray-50'}`}
                >
                  <span>{item.icon}</span>
                  <span>{item.label}</span>
                </button>
              ))}
            </div>
          </div>

          {/* Main Content */}
          <div className="flex-1 p-6 overflow-y-auto">
            <div className="flex justify-between items-center mb-6">
              <Dialog.Title className="text-xl font-medium">
                {sidebarItems.find((item) => item.id === activeSection)?.label}
              </Dialog.Title>
              <button
                onClick={() => setOpenModal(false)}
                className="p-2 hover:bg-gray-100 rounded-full"
              >
                <svg
                  className="w-5 h-5"
                  fill="none"
                  strokeWidth="1.5"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            {/* Content based on active section */}
            {activeSection === 'language-models' && (
              <div className="space-y-4">
                {/* OpenAI Card */}
                <div className="border rounded-lg overflow-hidden">
                  <div className="flex items-center justify-between p-4 bg-gray-50">
                    <div className="flex items-center space-x-3">
                      <button
                        onClick={() => toggleCard('openai')}
                        className="p-1 hover:bg-gray-200 rounded"
                      >
                        <svg
                          className={`w-4 h-4 transform transition-transform ${
                            expandedCards.includes('openai') ? 'rotate-90' : ''
                          }`}
                          fill="none"
                          stroke="currentColor"
                          viewBox="0 0 24 24"
                        >
                          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
                        </svg>
                      </button>
                      <img src="/openai-logo.svg" alt="OpenAI" className="w-6 h-6" />
                      <span className="font-medium">OpenAI</span>
                    </div>
                    <div className="flex items-center space-x-2">
                      <button
                        className="p-1 hover:bg-gray-200 rounded-full"
                        title="OpenAI Configuration Help"
                      >
                        <svg
                          className="w-5 h-5 text-gray-500"
                          fill="none"
                          stroke="currentColor"
                          viewBox="0 0 24 24"
                        >
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth="2"
                            d="M8.228 9c.549-1.165 2.03-2 3.772-2 2.21 0 4 1.343 4 3 0 1.4-1.278 2.575-3.006 2.907-.542.104-.994.54-.994 1.093m0 3h.01M12 21a9 9 0 100-18 9 9 0 000 18z"
                          />
                        </svg>
                      </button>
                      <Switch
                        checked={true}
                        onChange={() => {}}
                        className={`${
                          true ? 'bg-blue-600' : 'bg-gray-200'
                        } relative inline-flex h-6 w-11 items-center rounded-full`}
                      >
                        <span className="sr-only">Enable OpenAI</span>
                        <span
                          className={`${
                            true ? 'translate-x-6' : 'translate-x-1'
                          } inline-block h-4 w-4 transform rounded-full bg-white transition`}
                        />
                      </Switch>
                    </div>
                  </div>

                  {expandedCards.includes('openai') && (
                    <div className="p-4 space-y-4 border-t">
                      {/* API Key Input */}
                      <div className="space-y-2">
                        <label className="block text-sm font-medium text-gray-700">
                          API Key
                          <span className="text-gray-400 font-normal ml-2 text-xs">
                            请填写你的 OpenAI API Key
                          </span>
                        </label>
                        <div className="relative">
                          <input
                            type="password"
                            className="w-full px-3 py-2 border rounded-md"
                            placeholder="OpenAI API Key"
                          />
                          <button className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600">
                            <svg
                              className="w-5 h-5"
                              fill="none"
                              stroke="currentColor"
                              viewBox="0 0 24 24"
                            >
                              <path
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                              />
                              <path
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"
                              />
                            </svg>
                          </button>
                        </div>
                      </div>

                      {/* API Proxy Input */}
                      <div className="space-y-2">
                        <label className="block text-sm font-medium text-gray-700">
                          API 代理地址
                          <span className="text-gray-400 font-normal ml-2 text-xs">
                            验证认证地址，必须包含 http(s)://
                          </span>
                        </label>
                        <input
                          type="text"
                          className="w-full px-3 py-2 border rounded-md"
                          placeholder="https://api.openai.com/v1"
                        />
                      </div>

                      {/* Model Selection */}
                      <div className="space-y-2">
                        <label className="block text-sm font-medium text-gray-700">
                          模型列表
                          <span className="text-gray-400 font-normal ml-2 text-xs">
                            选择在全局中显示的模型，选择的模型会在模型列表中显示
                          </span>
                        </label>
                        <div className="flex flex-wrap gap-2 p-2 border rounded-md">
                          <span className="inline-flex items-center px-2 py-1 bg-gray-100 rounded-md text-sm">
                            GPT-4
                            <button className="ml-1 text-gray-500 hover:text-gray-700">×</button>
                          </span>
                          <span className="inline-flex items-center px-2 py-1 bg-gray-100 rounded-md text-sm">
                            GPT-3.5-Turbo
                            <button className="ml-1 text-gray-500 hover:text-gray-700">×</button>
                          </span>
                        </div>
                        <div className="flex justify-between items-center mt-1">
                          <span className="text-xs text-gray-500">共 22 个模型可用</span>
                          <button className="text-sm text-blue-600 hover:text-blue-700">
                            获取模型列表
                          </button>
                        </div>
                      </div>

                      {/* Connection Test Section */}
                      <div className="flex items-center justify-between">
                        <div className="space-y-1">
                          <div className="text-s font-medium text-gray-700">连通性检查</div>
                          <div className="text-xs text-gray-500">
                            测试 Api Key 与代理地址是否正确填写
                          </div>
                        </div>
                        <button className="px-4 py-1.5 bg-blue-600 text-white text-m rounded-md hover:bg-blue-700 transition-colors">
                          检查
                        </button>
                      </div>
                    </div>
                  )}
                </div>

                {/* Ollama Card - Similar structure */}
                <div className="border rounded-lg overflow-hidden">
                  {/* ... Similar structure as OpenAI card ... */}
                </div>
              </div>
            )}
          </div>
        </Dialog.Panel>
      </div>
    </Dialog>
  )
}

export default AppSettings

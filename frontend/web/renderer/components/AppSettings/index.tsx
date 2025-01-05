import { useAppSettingsStore } from '@/store/appSettingsStore'
import { useChatStore } from '@/store/chatStore'
import { Dialog } from '@headlessui/react'
import { Switch } from '@headlessui/react'
import { Combobox } from '@headlessui/react'
import React, { useState, useEffect } from 'react'

const AppSettings = () => {
  const {
    isOpenModal,
    setOpenModal,
    activeSection,
    setActiveSection,
    expandedCards,
    toggleCard,
    providers,
    toggleProvider,
    updateProvider,
    loadSettings,
    saveSettings,
    addCustomModel,
    toggleModel,
  } = useAppSettingsStore()

  // Load settings on mount
  useEffect(() => {
    loadSettings()
  }, [])

  // Save settings when providers change
  useEffect(() => {
    saveSettings()
  }, [providers])

  // Sidebar items configuration
  const sidebarItems = [
    { id: 'general', icon: '⚙️', label: '通用设置' },
    { id: 'language-models', icon: '🤖', label: '语言模型' },
    { id: 'voice-services', icon: '🎤', label: '语音服务' },
    { id: 'default-assistant', icon: '💬', label: '默认助手' },
    { id: 'about', icon: 'ℹ️', label: '关于' },
  ]

  const renderProviderCard = (providerId: string) => {
    const provider = providers[providerId]

    return (
      <div className="border rounded-lg overflow-hidden">
        <div className="flex items-center justify-between p-4 bg-gray-50">
          <div className="flex items-center space-x-3">
            <button
              onClick={() => toggleCard(providerId)}
              className="p-1 hover:bg-gray-200 rounded"
            >
              <svg
                className={`w-4 h-4 transform transition-transform ${
                  expandedCards.includes(providerId) ? 'rotate-90' : ''
                }`}
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
              </svg>
            </button>
            {/* <img src={`/${providerId}-logo.svg`} alt={provider.name} className="w-6 h-6" /> */}
            <span className="font-medium">{provider.name}</span>
          </div>
          <div className="flex items-center space-x-2">
            <button className="p-1 hover:bg-gray-200 rounded-full">
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
              checked={provider.enabled}
              onChange={() => toggleProvider(providerId)}
              className={`${provider.enabled ? 'bg-blue-600' : 'bg-gray-200'} relative inline-flex h-6 w-11 items-center rounded-full`}
            >
              <span className="sr-only">Enable {provider.name}</span>
              <span
                className={`${provider.enabled ? 'translate-x-6' : 'translate-x-1'} inline-block h-4 w-4 transform rounded-full bg-white transition`}
              />
            </Switch>
          </div>
        </div>

        {expandedCards.includes(providerId) && (
          <div className="p-4 space-y-4 border-t">
            {/* Provider specific fields */}
            {providerId === 'openai' && (
              <>
                <div className="space-y-2">
                  <label className="block text-sm font-medium text-gray-700">
                    API Key
                    <span className="text-gray-400 font-normal ml-2">
                      请填写你的 OpenAI API Key
                    </span>
                  </label>
                  <input
                    type="password"
                    value={provider.apiKey}
                    onChange={(e) => updateProvider(providerId, { apiKey: e.target.value })}
                    className="w-full px-3 py-2 border rounded-md"
                    placeholder="OpenAI API Key"
                  />
                </div>
                <div className="space-y-2">
                  <label className="block text-sm font-medium text-gray-700">
                    API 代理地址
                    <span className="text-gray-400 font-normal ml-2">
                      验证认证地址，必须包含 http(s)://
                    </span>
                  </label>
                  <input
                    type="text"
                    value={provider.apiEndpoint}
                    onChange={(e) => updateProvider(providerId, { apiEndpoint: e.target.value })}
                    className="w-full px-3 py-2 border rounded-md"
                    placeholder="https://api.openai.com/v1"
                  />
                </div>
              </>
            )}

            {providerId === 'ollama' && (
              <>
                <div className="space-y-2">
                  <label className="block text-sm font-medium text-gray-700">
                    Ollama 服务地址
                    <span className="text-gray-400 font-normal ml-2">本地未额外指定可留空</span>
                  </label>
                  <input
                    type="text"
                    value={provider.apiEndpoint}
                    onChange={(e) => updateProvider(providerId, { apiEndpoint: e.target.value })}
                    className="w-full px-3 py-2 border rounded-md"
                    placeholder="http://127.0.0.1:11434"
                  />
                </div>
                <div className="flex items-center justify-between">
                  <div>
                    <label className="text-sm font-medium text-gray-700">使用客户端请求模式</label>
                    <p className="text-sm text-gray-500">
                      客户端请求模式将从浏览器直接发起会话请求，可提升响应速度
                    </p>
                  </div>
                  <Switch
                    checked={provider.useStreamMode || false}
                    onChange={() =>
                      updateProvider(providerId, { useStreamMode: !provider.useStreamMode })
                    }
                    className={`${provider.useStreamMode ? 'bg-blue-600' : 'bg-gray-200'} relative inline-flex h-6 w-11 items-center rounded-full`}
                  >
                    <span className="sr-only">Enable stream mode</span>
                    <span
                      className={`${provider.useStreamMode ? 'translate-x-6' : 'translate-x-1'} inline-block h-4 w-4 transform rounded-full bg-white transition`}
                    />
                  </Switch>
                </div>
              </>
            )}

            {/* Connection Test */}
            <div className="flex items-center justify-between">
              <div className="space-y-1">
                <div className="text-sm font-medium text-gray-700">连通性检查</div>
                <div className="text-sm text-gray-500">测试 Api Key 与代理地址是否正确填写</div>
              </div>
              <button className="px-4 py-1.5 bg-blue-600 text-white text-sm rounded-md hover:bg-blue-700 transition-colors">
                检查
              </button>
            </div>

            {/* Model Selector */}
            <ModelSelector providerId={providerId} />
          </div>
        )}
      </div>
    )
  }

  return (
    <Dialog open={isOpenModal} onClose={() => setOpenModal(false)} className="relative z-10">
      <div className="fixed inset-0 bg-black/30" aria-hidden="true" />
      <div className="fixed inset-0 flex items-center justify-center p-4">
        <Dialog.Panel className="relative mx-auto w-[1000px] h-[600px] rounded-lg bg-white shadow-xl flex">
          {/* Close button - Absolutely positioned */}
          <button
            onClick={() => setOpenModal(false)}
            className="absolute right-4 top-4 p-1.5 hover:bg-gray-100 rounded transition-colors"
          >
            <svg
              className="w-4 h-4 text-gray-500"
              fill="none"
              strokeWidth="1.5"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>

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

          {/* Main Content Container */}
          <div className="flex-1 flex flex-col">
            {/* Scrollable Content */}
            <div className="flex-1 overflow-y-auto custom-thin-scrollbar px-6 py-6 pr-10">
              {activeSection === 'language-models' && (
                <div className="space-y-4">
                  {renderProviderCard('openai')}
                  {renderProviderCard('ollama')}
                </div>
              )}
            </div>
          </div>
        </Dialog.Panel>
      </div>
    </Dialog>
  )
}

const ModelSelector = ({ providerId }: { providerId: string }) => {
  const [query, setQuery] = useState('')
  const { providers, toggleModel } = useAppSettingsStore()
  const provider = providers[providerId]
  const [selectedModels, setSelectedModels] = useState<string[]>([])

  // Filter models based on search query
  const filteredModels =
    query === ''
      ? provider.models
      : provider.models.filter((model) => model.name.toLowerCase().includes(query.toLowerCase()))

  return (
    <div className="space-y-2">
      <label className="block text-sm font-medium text-gray-700">
        模型列表
        <span className="text-gray-400 font-normal ml-2">
          选择在全局中显示的模型，选择的模型会在模型列表中显示
        </span>
      </label>

      <div className="relative">
        <Combobox value={selectedModels} onChange={setSelectedModels} multiple>
          <div className="relative">
            <div className="flex flex-wrap gap-2 p-2 border rounded-md min-h-[42px]">
              {selectedModels.map((modelId) => (
                <span
                  key={modelId}
                  className="inline-flex items-center px-2 py-1 bg-gray-100 rounded-md text-sm"
                >
                  {provider.models.find((m) => m.id === modelId)?.name}
                  <button
                    className="ml-1 text-gray-500 hover:text-gray-700"
                    onClick={(e) => {
                      e.preventDefault()
                      setSelectedModels(selectedModels.filter((id) => id !== modelId))
                    }}
                  >
                    ×
                  </button>
                </span>
              ))}
              <Combobox.Input
                className="flex-1 outline-none min-w-[120px]"
                placeholder="搜索或选择模型"
                onChange={(event) => setQuery(event.target.value)}
              />
            </div>
            <div className="absolute right-2 top-1/2 -translate-y-1/2 flex items-center space-x-1">
              <button className="p-1 hover:bg-gray-100 rounded-full" onClick={() => setQuery('')}>
                <svg className="w-4 h-4 text-gray-400" viewBox="0 0 24 24">
                  <path
                    fill="currentColor"
                    d="M12 4c4.41 0 8 3.59 8 8s-3.59 8-8 8-8-3.59-8-8 3.59-8 8-8m0-2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 13l-4-4h8z"
                  />
                </svg>
              </button>
              <button className="p-1 hover:bg-gray-100 rounded-full">
                <svg className="w-4 h-4 text-gray-400" viewBox="0 0 24 24">
                  <path fill="currentColor" d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z" />
                </svg>
              </button>
            </div>
          </div>

          <Combobox.Options className="absolute z-10 mt-1 w-full bg-white rounded-md shadow-lg max-h-60 overflow-auto custom-scrollbar">
            {filteredModels.length === 0 && query !== '' ? (
              <div className="px-4 py-2 text-sm text-gray-500">未找到匹配的模型</div>
            ) : (
              filteredModels.map((model) => (
                <Combobox.Option
                  key={model.id}
                  value={model.id}
                  className={({ active }) =>
                    `${active ? 'bg-blue-50 text-blue-600' : 'text-gray-900'}
                    cursor-pointer select-none relative py-2 pl-3 pr-9`
                  }
                >
                  {({ active, selected }) => (
                    <div className="flex items-center justify-between">
                      <div className="flex items-center">
                        <span className="block truncate">{model.name}</span>
                        {model.contextSize && (
                          <span className="ml-2 text-xs text-gray-500">{model.contextSize}</span>
                        )}
                      </div>
                      {selected && (
                        <span className="absolute inset-y-0 right-0 flex items-center pr-4">
                          <svg className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
                            <path
                              fillRule="evenodd"
                              d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z"
                              clipRule="evenodd"
                            />
                          </svg>
                        </span>
                      )}
                    </div>
                  )}
                </Combobox.Option>
              ))
            )}
          </Combobox.Options>
        </Combobox>

        <div className="mt-1 flex justify-between text-sm text-gray-500">
          <span>共 {provider.models.length} 个模型可用</span>
          <button className="text-blue-600 hover:text-blue-700">获取模型列表</button>
        </div>
      </div>
    </div>
  )
}

export default AppSettings

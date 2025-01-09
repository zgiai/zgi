import { useAppSettingsStore } from '@/store/appSettingsStore'
import { Dialog } from '@headlessui/react'
import React from 'react'
import ProviderCard from './ProviderCard'

const AppSettings = () => {
  const { isOpenModal, setOpenModal, activeSection, setActiveSection, providers } =
    useAppSettingsStore()

  // Sidebar items configuration
  const sidebarItems = [
    { id: 'general', icon: '⚙️', label: 'Common Settings' },
    { id: 'language-models', icon: '🤖', label: 'Language Model' },
    { id: 'voice-services', icon: '🎤', label: 'Text-to-Speech' },
    { id: 'default-assistant', icon: '💬', label: 'Default Assistant' },
    { id: 'about', icon: 'ℹ️', label: 'About' },
  ]

  return (
    <Dialog open={isOpenModal} onClose={() => {}} className="relative z-10">
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
          <div className="w-60 border-r border-gray-200 p-4">
            <h2 className="text-lg font-medium mb-4">Settings</h2>
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
                  {Object.keys(providers)?.map((item) => {
                    return <ProviderCard key={item} providerId={item} />
                  })}
                </div>
              )}
            </div>
          </div>
        </Dialog.Panel>
      </div>
    </Dialog>
  )
}

export default AppSettings

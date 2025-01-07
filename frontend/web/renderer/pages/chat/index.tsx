import ChatArea from '@/components/ChatArea'
import Footer from '@/components/Footer'
import Header from '@/components/Header'
import InputArea from '@/components/InputArea'
import Sidebar from '@/components/Sidebar'
import { useAppSettingsStore } from '@/store/appSettingsStore'
import { useChatStore } from '@/store/chatStore'
import { useEffect } from 'react'

export default function Chat() {
  const { init } = useChatStore()
  const appSettingsStore = useAppSettingsStore()
  useEffect(() => {
    init()
    appSettingsStore.init()
  }, [])

  return (
    <div className="flex h-screen bg-gray-100">
      <Sidebar />
      <div className="flex-1 flex flex-col overflow-hidden">
        <Header />
        <div className="flex-1 relative">
          <ChatArea />
        </div>
        <InputArea />
        {/* <Footer /> */}
      </div>
    </div>
  )
}

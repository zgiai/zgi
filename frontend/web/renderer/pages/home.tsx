import ChatArea from '../components/ChatArea/page'
import Footer from '../components/Footer/page'
import Header from '../components/Header/page'
import InputArea from '../components/InputArea/page'
import Sidebar from '../components/Sidebar/page'

export default function Home() {
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

'use client'

import { createContext, Dispatch, SetStateAction, useContext, useState } from 'react'

interface ContextProps {
  sidebarOpen: boolean
  setSidebarOpen: Dispatch<SetStateAction<boolean>>
  userInfo: any
  setUserInfo: Dispatch<SetStateAction<any>>
  language: string
  setLanguage: Dispatch<SetStateAction<string>>
}

const AppContext = createContext<ContextProps>({
  sidebarOpen: false,
  setSidebarOpen: (): boolean => false,
  userInfo: {},
  setUserInfo: (): any => {},
  language: "en",
  setLanguage: (): any => {}
})

export default function AppProvider({
  children,
}: {
  children: React.ReactNode
}) {  
  const [sidebarOpen, setSidebarOpen] = useState<boolean>(false)
  const [userInfo, setUserInfo] = useState<any>({})
  const [language, setLanguage] = useState<string>("en")
  return (
    <AppContext.Provider value={{ sidebarOpen, setSidebarOpen, userInfo, setUserInfo, language, setLanguage }}>
      {children}
    </AppContext.Provider>
  )
}

export const useAppProvider = () => useContext(AppContext)
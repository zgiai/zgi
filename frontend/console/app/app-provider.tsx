'use client'

import { createContext, Dispatch, SetStateAction, useContext, useState } from 'react'

interface ContextProps {
  sidebarOpen: boolean
  setSidebarOpen: Dispatch<SetStateAction<boolean>>
  userInfo: any
  setUserInfo: Dispatch<SetStateAction<any>>
}

const AppContext = createContext<ContextProps>({
  sidebarOpen: false,
  setSidebarOpen: (): boolean => false,
  userInfo: {},
  setUserInfo: (): any => {}
})

export default function AppProvider({
  children,
}: {
  children: React.ReactNode
}) {  
  const [sidebarOpen, setSidebarOpen] = useState<boolean>(false)
  const [userInfo, setUserInfo] = useState<any>({})
  return (
    <AppContext.Provider value={{ sidebarOpen, setSidebarOpen, userInfo, setUserInfo }}>
      {children}
    </AppContext.Provider>
  )
}

export const useAppProvider = () => useContext(AppContext)
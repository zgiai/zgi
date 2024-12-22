'use client'

import { createContext, useContext, useState } from 'react'

interface FlyoutContextProps {
  flyoutOpen: boolean
  setFlyoutOpen: (flyoutOpen: boolean) => void
}

const FlyoutContext = createContext<FlyoutContextProps | undefined>(undefined)

export const FlyoutProvider = ({
  children,
  initialState = false
}: {
  children: React.ReactNode,
  initialState?: boolean
}) => {
  const [flyoutOpen, setFlyoutOpen] = useState<boolean>(initialState)
  return (
    <FlyoutContext.Provider value={{ flyoutOpen, setFlyoutOpen }}>
      {children}
    </FlyoutContext.Provider>
  )
}

export const useFlyoutContext = () => {
  const context = useContext(FlyoutContext)
  if (!context) {
    throw new Error('useFlyout must be used within a FlyoutProvider')
  }
  return context
}
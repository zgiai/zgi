/**
 * Check if current environment is desktop
 */
export const isDesktop = () => {
  if (typeof window === 'undefined') return false
  return typeof window.ipc !== 'undefined'
}

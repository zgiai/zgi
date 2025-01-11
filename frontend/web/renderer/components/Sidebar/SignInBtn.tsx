import { useUserStore } from '@/store/userStore'
import { Cross2Icon, ReloadIcon } from '@radix-ui/react-icons'
import * as Popover from '@radix-ui/react-popover'
import React, { useEffect, useState } from 'react'

const SignInBtn = () => {
  const {
    user,
    userFormData,
    setUserFormData,
    isUserOpen,
    setUserOpen,
    isRegistering,
    toggleRegistering,
    handleSignIn,
    handleRegister,
    loading,
    handleLogout,
    isUserInfoPopoverOpen,
    onClickSigninBaseBtn,
    setUserInfoPopoverOpen,
  } = useUserStore()
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    if (!isUserOpen) {
      setError('')
      setConfirmPassword('')
    }
  }, [isUserOpen])

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (userFormData?.password && userFormData.password.length < 8) {
      setError('Password must be at least 8 characters long.')
      return
    }
    if (isRegistering) {
      if (userFormData.password !== confirmPassword) {
        setError('Passwords do not match.')
        return
      }
      handleRegister()
    } else {
      handleSignIn()
    }
  }

  return (
    <div className="mt-auto px-4 py-4">
      <Popover.Root open={isUserInfoPopoverOpen}>
        <Popover.Trigger asChild>
          <button
            type="button"
            className="w-full p-2 border border-gray-300 rounded-md flex items-center justify-center hover:border-blue-500"
            onClick={onClickSigninBaseBtn}
          >
            {user ? user.username : 'Sign In'}
          </button>
        </Popover.Trigger>

        {user && (
          <Popover.Content className="bg-gray-800 text-white p-4 rounded-md shadow-lg">
            <h2 className="text-lg font-semibold mb-2">用户信息</h2>
            <p>用户名: {user.username}</p>
            <p>邮箱: {user.email}</p>
            <button
              type="button"
              onClick={handleLogout}
              className="mt-2 text-red-500 hover:text-red-400"
            >
              退出登录
            </button>
            <Popover.Close
              className="absolute top-2 right-2 text-gray-400 hover:text-gray-300"
              onClick={() => setUserInfoPopoverOpen(false)}
            >
              <Cross2Icon />
            </Popover.Close>
          </Popover.Content>
        )}
      </Popover.Root>

      {isUserOpen && (
        <div className="fixed inset-0 flex items-center justify-center bg-black bg-opacity-50 z-50">
          <div className="bg-white p-6 rounded-md w-96 transition-transform transform duration-300 ease-in-out">
            <button
              type="button"
              onClick={() => setUserOpen(false)}
              className="absolute top-4 right-4 text-gray-500 hover:text-gray-700"
            >
              <Cross2Icon />
            </button>
            <h2 className="text-lg font-semibold mb-4">{isRegistering ? 'Register' : 'Sign In'}</h2>
            <form onSubmit={handleSubmit}>
              {isRegistering && (
                <div className="mb-4">
                  <label htmlFor="username" className="block text-sm font-medium text-gray-700">
                    Username
                  </label>
                  <input
                    id="username"
                    name="username"
                    type="text"
                    placeholder="Username"
                    value={userFormData?.username || ''}
                    onChange={(e) => setUserFormData({ username: e.target.value })}
                    className="border p-2 w-full"
                    required
                  />
                </div>
              )}
              <div className="mb-4">
                <label htmlFor="email" className="block text-sm font-medium text-gray-700">
                  Email
                </label>
                <input
                  id="email"
                  name="email"
                  type="email"
                  placeholder="Email"
                  value={userFormData.email}
                  onChange={(e) => setUserFormData({ email: e.target.value })}
                  className="border p-2 w-full"
                  required
                />
              </div>
              <div className="mb-4">
                <label htmlFor="password" className="block text-sm font-medium text-gray-700">
                  Password
                </label>
                <input
                  id="password"
                  name="password"
                  type="password"
                  placeholder="Password"
                  value={userFormData.password}
                  onChange={(e) => setUserFormData({ password: e.target.value })}
                  className="border p-2 w-full"
                  required
                />
              </div>
              {isRegistering && (
                <div className="mb-4">
                  <label
                    htmlFor="confirmPassword"
                    className="block text-sm font-medium text-gray-700"
                  >
                    Confirm Password
                  </label>
                  <input
                    id="confirmPassword"
                    name="confirmPassword"
                    type="password"
                    placeholder="Confirm Password"
                    value={confirmPassword}
                    onChange={(e) => setConfirmPassword(e.target.value)}
                    className="border p-2 w-full"
                    required
                  />
                </div>
              )}
              {error && <p className="text-red-500 text-sm">{error}</p>}
              <button
                type="submit"
                className={`w-full p-2 bg-blue-500 text-white rounded-md ${loading ? 'bg-gray-200 text-gray-400 cursor-not-allowed' : 'bg-blue-600 text-white hover:bg-blue-700 transition-colors'}`}
                disabled={loading}
              >
                {loading ? (
                  <div className="flex items-center justify-center">
                    <ReloadIcon className="mr-2 animate-spin" />
                    <span className="ml-2">{isRegistering ? 'Register' : 'Sign In'}</span>
                  </div>
                ) : isRegistering ? (
                  'Register'
                ) : (
                  'Sign In'
                )}
              </button>
            </form>
            <button type="button" onClick={toggleRegistering} className="mt-2 text-blue-500">
              {isRegistering ? 'Already have an account?' : 'Need an account?'}
              <span style={{ marginLeft: '20px' }}>{isRegistering ? 'Sign In' : 'Register'}</span>
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

export default SignInBtn

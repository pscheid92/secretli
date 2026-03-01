import { createContext, useContext, useCallback } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as api from '../lib/api'

interface AuthContextValue {
  user: api.User | null
  isLoading: boolean
  login: (email: string, password: string) => Promise<api.User>
  register: (email: string, password: string, displayName: string) => Promise<api.User>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const queryClient = useQueryClient()

  const { data: user, isLoading } = useQuery({
    queryKey: ['auth', 'me'],
    queryFn: api.getCurrentUser,
    retry: false,
    staleTime: 5 * 60 * 1000,
  })

  const loginMutation = useMutation({
    mutationFn: ({ email, password }: { email: string; password: string }) =>
      api.login(email, password),
    onSuccess: (data) => {
      queryClient.setQueryData(['auth', 'me'], data)
    },
  })

  const registerMutation = useMutation({
    mutationFn: ({
      email,
      password,
      displayName,
    }: {
      email: string
      password: string
      displayName: string
    }) => api.register(email, password, displayName),
    onSuccess: (data) => {
      queryClient.setQueryData(['auth', 'me'], data)
    },
  })

  const logoutMutation = useMutation({
    mutationFn: api.logout,
    onSuccess: () => {
      queryClient.setQueryData(['auth', 'me'], null)
      queryClient.removeQueries({ queryKey: ['auth'] })
    },
  })

  const login = useCallback(
    async (email: string, password: string) => {
      return loginMutation.mutateAsync({ email, password })
    },
    [loginMutation],
  )

  const register = useCallback(
    async (email: string, password: string, displayName: string) => {
      return registerMutation.mutateAsync({ email, password, displayName })
    },
    [registerMutation],
  )

  const logoutFn = useCallback(async () => {
    return logoutMutation.mutateAsync()
  }, [logoutMutation])

  return (
    <AuthContext value={{
      user: user ?? null,
      isLoading,
      login,
      register,
      logout: logoutFn,
    }}>
      {children}
    </AuthContext>
  )
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}

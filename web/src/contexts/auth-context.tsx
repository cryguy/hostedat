import { createContext, useCallback, useEffect, useState, type ReactNode } from "react"
import { auth as authApi } from "@/lib/api"
import type { User } from "@/types/api"

interface AuthContextValue {
  user: User | null
  isLoading: boolean
  isAdmin: boolean
  login: (email: string, password: string) => Promise<void>
  register: (email: string, password: string, inviteCode?: string) => Promise<void>
  logout: () => void
}

export const AuthContext = createContext<AuthContextValue | null>(null)

function decodeJwtPayload(token: string): Record<string, unknown> | null {
  try {
    const base64 = token.split(".")[1]
    const json = atob(base64.replace(/-/g, "+").replace(/_/g, "/"))
    return JSON.parse(json)
  } catch {
    return null
  }
}

function userFromToken(token: string): User | null {
  const payload = decodeJwtPayload(token)
  if (!payload) return null

  const exp = payload.exp as number | undefined
  if (exp && exp * 1000 < Date.now()) return null

  return {
    id: payload.user_id as string,
    email: payload.email as string,
    role: payload.role as User["role"],
    created_at: "",
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    const token = authApi.getToken()
    if (token) {
      const u = userFromToken(token)
      if (u) {
        setUser(u)
      } else {
        authApi.clearToken()
      }
    }
    setIsLoading(false)
  }, [])

  const login = useCallback(async (email: string, password: string) => {
    const res = await authApi.login(email, password)
    authApi.setToken(res.token)
    setUser(res.user)
  }, [])

  const register = useCallback(
    async (email: string, password: string, inviteCode?: string) => {
      const res = await authApi.register(email, password, inviteCode)
      authApi.setToken(res.token)
      setUser(res.user)
    },
    [],
  )

  const logout = useCallback(() => {
    authApi.logout().catch(() => {})
    authApi.clearToken()
    setUser(null)
  }, [])

  const isAdmin = user?.role === "admin" || user?.role === "superadmin"

  return (
    <AuthContext.Provider value={{ user, isLoading, isAdmin, login, register, logout }}>
      {children}
    </AuthContext.Provider>
  )
}

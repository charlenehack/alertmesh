import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import type { UserInfo } from '../types'

interface AuthState {
  token: string | null
  userInfo: UserInfo | null
  setToken: (token: string) => void
  setUserInfo: (info: UserInfo) => void
  logout: () => void
  hasPermission: (perm: string) => boolean
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      token: null,
      userInfo: null,

      setToken: (token) => set({ token }),

      setUserInfo: (userInfo) => set({ userInfo }),

      logout: () => set({ token: null, userInfo: null }),

      hasPermission: (perm) => {
        const perms = get().userInfo?.permissions ?? []
        return perms.includes('*') || perms.includes(perm)
      },
    }),
    {
      name: 'alertmesh-auth',
      partialize: (s) => ({ token: s.token }),
    },
  ),
)

// Standalone helpers — useful outside React components (e.g. inside an
// effect, an api wrapper, or a non-hook utility).  Admins / superadmins
// with wildcard permission "*" always pass.

export function hasPermission(perm: string): boolean {
  const perms = useAuthStore.getState().userInfo?.permissions ?? []
  return perms.includes('*') || perms.includes(perm)
}

export function hasRole(...roles: string[]): boolean {
  const userRoles = useAuthStore.getState().userInfo?.roles ?? []
  return roles.some((r) => userRoles.includes(r))
}

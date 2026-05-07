import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useAuthStore } from '../store/auth'
import { getUserInfo } from '../api/system'

// Auth store only persists the token (roles/permissions can change
// server-side), so on a hard refresh `userInfo` is null and role-gated
// UI like the admin sidebar disappears until the user re-logs in.
// Re-fetch /user/info once whenever we have a token but no cached
// identity — and gate rendering of protected pages until it lands so
// the menu doesn't flicker.
//
// Implemented via React Query so loading / error / cache lifecycle is
// owned by RQ instead of bespoke state flipping inside an effect.
export function useUserInfoHydration() {
  const token = useAuthStore((s) => s.token)
  const userInfo = useAuthStore((s) => s.userInfo)
  const setUserInfo = useAuthStore((s) => s.setUserInfo)
  const logout = useAuthStore((s) => s.logout)

  const enabled = !!token && !userInfo
  const query = useQuery({
    queryKey: ['user-info', token],
    queryFn: getUserInfo,
    enabled,
    retry: 0,
    staleTime: Infinity,
  })

  useEffect(() => {
    if (query.data) setUserInfo(query.data)
  }, [query.data, setUserInfo])

  useEffect(() => {
    // 401 already triggers logout in the request layer; for any other
    // failure clear the token so we route the user back to /login
    // instead of rendering an empty shell.
    if (query.isError) logout()
  }, [query.isError, logout])

  const hydrating = enabled && (query.isLoading || query.isFetching)
  return { token, hydrating }
}

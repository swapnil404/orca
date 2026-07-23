const tokenKey = 'orca.jwt'

export function getAuthToken(): string | null {
  if (typeof window === 'undefined') {
    return null
  }
  return window.localStorage.getItem(tokenKey)
}

export function storeAuthToken(token: string): void {
  window.localStorage.setItem(tokenKey, token)
}

export function clearAuthToken(): void {
  if (typeof window !== 'undefined') {
    window.localStorage.removeItem(tokenKey)
  }
}

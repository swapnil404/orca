import { createServerFn } from '@tanstack/react-start'
import {
  deleteCookie,
  getCookie,
  getRequest,
  setCookie,
  setResponseHeader,
} from '@tanstack/react-start/server'
import { ApiError } from './client'

const sessionCookie = 'orca.session'
const sessionLifetimeSeconds = 24 * 60 * 60

export interface Session {
  user_id: string
}

export interface AuthResult {
  session: Session
}

export type OAuthProvider = 'github' | 'google'

interface Credentials {
  email: string
  password: string
}

interface AuthFailure {
  ok: false
  status: number
  message: string
}

interface AuthSuccess {
  ok: true
  session: Session
}

type ServerAuthResult = AuthFailure | AuthSuccess

interface TokenResponse {
  token?: unknown
}

interface ErrorResponse {
  error?: unknown
}

function validateCredentials(value: unknown): Credentials {
  if (
    typeof value !== 'object' ||
    value === null ||
    !('email' in value) ||
    typeof value.email !== 'string' ||
    !('password' in value) ||
    typeof value.password !== 'string'
  ) {
    throw new Error('Invalid credentials')
  }
  return { email: value.email, password: value.password }
}

function apiURL(path: string): URL {
  const baseURL = process.env.ORCA_API_URL ?? 'http://127.0.0.1:8080'
  const parsed = new URL(baseURL)
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new Error('ORCA_API_URL must be an HTTP URL')
  }
  return new URL(path, parsed)
}

function secureRequest(): boolean {
  const publicServerURL = process.env.ORCA_SERVER_URL
  if (publicServerURL) {
    return new URL(publicServerURL).protocol === 'wss:'
  }
  return new URL(getRequest().url).protocol === 'https:'
}

async function readError(response: Response): Promise<string> {
  const payload = (await response.json().catch(() => ({}))) as ErrorResponse
  return typeof payload.error === 'string' ? payload.error : `Request failed with status ${response.status}`
}

async function fetchSession(token: string): Promise<Session | null> {
  const response = await fetch(apiURL('/auth/session'), {
    headers: { Accept: 'application/json', Authorization: `Bearer ${token}` },
    cache: 'no-store',
  })
  if (response.status === 401) {
    return null
  }
  if (!response.ok) {
    throw new Error(await readError(response))
  }
  const value = (await response.json()) as unknown
  if (typeof value !== 'object' || value === null || !('user_id' in value) || typeof value.user_id !== 'string') {
    throw new Error('Invalid session response')
  }
  return { user_id: value.user_id }
}

function writeSessionCookie(token: string): void {
  setCookie(sessionCookie, token, {
    httpOnly: true,
    secure: secureRequest(),
    sameSite: 'lax',
    path: '/',
    maxAge: sessionLifetimeSeconds,
  })
}

function clearSessionCookie(): void {
  deleteCookie(sessionCookie, { path: '/' })
}

async function authenticate(data: Credentials, path: '/auth/login' | '/auth/register'): Promise<ServerAuthResult> {
  try {
    const response = await fetch(apiURL(path), {
      method: 'POST',
      headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
      cache: 'no-store',
    })
    if (!response.ok) {
      return { ok: false, status: response.status, message: await readError(response) }
    }
    const payload = (await response.json()) as TokenResponse
    if (typeof payload.token !== 'string' || payload.token === '') {
      return { ok: false, status: 502, message: 'Invalid authentication response' }
    }
    const session = await fetchSession(payload.token)
    if (!session) {
      return { ok: false, status: 401, message: 'Authentication failed' }
    }
    writeSessionCookie(payload.token)
    setResponseHeader('Cache-Control', 'no-store')
    return { ok: true, session }
  } catch {
    return { ok: false, status: 0, message: 'Authentication service is unavailable' }
  }
}

const loginServer = createServerFn({ method: 'POST' })
  .validator(validateCredentials)
  .handler(({ data }) => authenticate(data, '/auth/login'))

const signupServer = createServerFn({ method: 'POST' })
  .validator(validateCredentials)
  .handler(({ data }) => authenticate(data, '/auth/register'))

const sessionServer = createServerFn({ method: 'GET' }).handler(async (): Promise<Session | null> => {
  setResponseHeader('Cache-Control', 'no-store')
  const token = getCookie(sessionCookie)
  if (!token) {
    return null
  }
  const session = await fetchSession(token)
  if (!session) {
    clearSessionCookie()
  }
  return session
})

const logoutServer = createServerFn({ method: 'POST' }).handler(() => {
  clearSessionCookie()
})

async function unwrapAuth(result: ServerAuthResult): Promise<AuthResult> {
  if (!result.ok) {
    throw new ApiError(result.status, result.message)
  }
  return { session: result.session }
}

export async function login(email: string, password: string): Promise<AuthResult> {
  return unwrapAuth(await loginServer({ data: { email, password } }))
}

export async function signup(email: string, password: string): Promise<AuthResult> {
  return unwrapAuth(await signupServer({ data: { email, password } }))
}

export function startOAuth(provider: OAuthProvider): void {
  window.location.assign(`/auth/${provider}`)
}

export function getSession(): Promise<Session | null> {
  return sessionServer()
}

export async function logout(): Promise<void> {
  await logoutServer()
}

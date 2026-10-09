// Sign-in: shown whenever the API answers 401; on success the meta query is refetched and the app renders
import { useEffect, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, api } from '../api'
import { Logo } from '../components/Layout'
import { Card } from '../components/ui'
import { t } from '../prefs'

const input = 'mt-1 h-10 w-full rounded-sm border border-transparent bg-fill px-3 text-body text-text outline-none focus:border-primary disabled:opacity-60'

function errorText(e: unknown): string {
  if (e instanceof ApiError && e.status === 401) return t('用户名或密码错误', 'Wrong username or password')
  if (e instanceof ApiError && e.status === 429) return t('失败次数过多，请稍后再试', 'Too many failed attempts. Try again later.')
  return e instanceof Error ? e.message : t('登录失败', 'Sign-in failed')
}

export function SignInPage() {
  const qc = useQueryClient()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  // /v1/auth/me tells whether any dashboard user exists at all
  const me = useQuery({ queryKey: ['me'], queryFn: api.me, retry: false })
  const noUsers = me.error instanceof ApiError && me.error.body.users_configured === false

  useEffect(() => {
    document.title = `${t('登录', 'Sign in')} · HiwiKInsight`
  }, [])

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api.login(username.trim(), password)
      await qc.invalidateQueries()
    } catch (err) {
      setError(errorText(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-bg px-4 py-12">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex justify-center"><Logo /></div>
        <Card>
          <form onSubmit={submit} className="flex flex-col gap-4">
            <div>
              <h1 className="text-title text-text">{t('登录', 'Sign in')}</h1>
              <p className="mt-1 text-small text-text-2">{t('登录后查看看板', 'Sign in to view the dashboard')}</p>
            </div>
            {noUsers && (
              <div className="rounded-sm bg-fill px-3 py-2.5 text-small text-text-2">
                <p className="font-medium text-text">{t('还没有配置看板用户', 'No dashboard users are configured')}</p>
                <p className="mt-1">{t('用下面的命令生成密码哈希，再添加到 config.yaml 的 dashboard.users：', 'Generate a password hash with the command below, then add it to dashboard.users in config.yaml:')}</p>
                <code className="mt-2 block rounded-[8px] bg-surface px-2.5 py-1.5 font-mono text-caption text-text">hiwikinsight hash-password</code>
              </div>
            )}
            <label className="text-caption text-text-3">
              {t('用户名', 'Username')}
              <input
                autoFocus
                autoComplete="username"
                autoCapitalize="none"
                spellCheck={false}
                value={username}
                disabled={busy}
                onChange={(e) => setUsername(e.target.value)}
                className={input}
              />
            </label>
            <label className="text-caption text-text-3">
              {t('密码', 'Password')}
              <input
                type="password"
                autoComplete="current-password"
                value={password}
                disabled={busy}
                onChange={(e) => setPassword(e.target.value)}
                className={input}
              />
            </label>
            {error && <p role="alert" className="text-small text-danger">{error}</p>}
            <button
              type="submit"
              disabled={busy || !username.trim() || !password}
              className="press h-10 w-full rounded-full bg-primary px-4 text-body font-semibold text-on-primary hover:brightness-110 disabled:opacity-40"
            >
              {busy ? t('正在登录…', 'Signing in…') : t('登录', 'Sign in')}
            </button>
          </form>
        </Card>
      </div>
    </div>
  )
}

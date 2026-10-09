import { QueryCache, QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { BrowserRouter, Navigate, Outlet, Route, Routes, useLocation } from 'react-router'
import { api, isUnauthorized } from './api'
import { MetaProvider } from './data'
import { ThemeProvider } from './theme'
import { PrefsProvider, setMoneyBasis } from './prefs'
import { AppShell, Logo } from './components/Layout'
import { ErrorState } from './components/ui'
import { ErrorBoundary } from './components/ErrorBoundary'
import { Overview } from './pages/Overview'
import { PortfolioPage } from './pages/Portfolio'
import { ActivityPage } from './pages/Activity'
import { RetentionPage } from './pages/Retention'
import { ModulesPage } from './pages/Modules'
import { UsersPage } from './pages/Users'
import { UserDetailPage } from './pages/UserDetail'
import { RevenuePage } from './pages/Revenue'
import { AcquisitionPage } from './pages/Acquisition'
import { DevicesPage } from './pages/Devices'
import { EventsPage } from './pages/Events'
import { ErrorsPage } from './pages/Errors'
import { SettingsPage } from './pages/Settings'
import { SignInPage } from './pages/SignIn'

const queryClient: QueryClient = new QueryClient({
  // A 401 from any query means the session ended: refetch meta, which then shows the sign-in page
  queryCache: new QueryCache({
    onError: (error, query) => {
      if (isUnauthorized(error) && query.queryKey[0] !== 'meta' && query.queryKey[0] !== 'me') queryClient.invalidateQueries({ queryKey: ['meta'] })
    },
  }),
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      refetchOnWindowFocus: false,
      retry: (count, error) => !isUnauthorized(error) && count < 1,
    },
  },
})

function Protected() {
  const loc = useLocation()
  const meta = useQuery({ queryKey: ['meta'], queryFn: api.meta, staleTime: 5 * 60_000 })
  if (meta.isPending) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-bg">
        <div className="animate-pulse"><Logo /></div>
      </div>
    )
  }
  if (isUnauthorized(meta.error)) return <SignInPage />
  setMoneyBasis(meta.data?.currency, meta.data?.fx)
  if (meta.error) return <div className="min-h-screen bg-bg pt-24"><ErrorState error={meta.error} onRetry={() => meta.refetch()} /></div>
  return (
    <MetaProvider meta={meta.data}>
      <AppShell>
        <ErrorBoundary key={loc.pathname}>
          <Outlet />
        </ErrorBoundary>
      </AppShell>
    </MetaProvider>
  )
}

export function App() {
  return (
    <ThemeProvider>
      <PrefsProvider>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter basename="/dashboard">
          <Routes>
            <Route element={<Protected />}>
              <Route index element={<PortfolioPage />} />
              <Route path="overview" element={<Overview />} />
              <Route path="activity" element={<ActivityPage />} />
              <Route path="retention" element={<RetentionPage />} />
              <Route path="modules" element={<ModulesPage />} />
              <Route path="users" element={<UsersPage />} />
              <Route path="users/:id" element={<UserDetailPage />} />
              <Route path="acquisition" element={<AcquisitionPage />} />
              <Route path="revenue" element={<RevenuePage />} />
              <Route path="devices" element={<DevicesPage />} />
              <Route path="events" element={<EventsPage />} />
              <Route path="errors" element={<ErrorsPage />} />
              <Route path="settings" element={<SettingsPage />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Route>
          </Routes>
        </BrowserRouter>
      </QueryClientProvider>
      </PrefsProvider>
    </ThemeProvider>
  )
}

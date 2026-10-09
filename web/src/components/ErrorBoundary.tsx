// Page-level error boundary: a failing chart only breaks the current page, never the whole dashboard
import { Component, type ReactNode } from 'react'
import { ErrorState } from './ui'
import { t } from '../prefs'

export class ErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state = { error: null as Error | null }

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  render() {
    if (this.state.error) {
      return (
        <div className="pt-24">
          <ErrorState error={new Error(t(`页面渲染出错：${this.state.error.message}`, `Page failed to render: ${this.state.error.message}`))} onRetry={() => this.setState({ error: null })} />
        </div>
      )
    }
    return this.props.children
  }
}

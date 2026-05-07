import { Component, type ErrorInfo, type ReactNode } from 'react'
import { Button, Result } from 'antd'

interface ErrorBoundaryProps {
  children: ReactNode
  resetKey?: string
  fallback?: (args: { error: Error; reset: () => void }) => ReactNode
}

interface ErrorBoundaryState {
  error: Error | null
}

// Simple class boundary — needed because there is no hooks-based equivalent.
// We intentionally avoid pulling in `react-error-boundary` to keep the bundle lean.
export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[ErrorBoundary]', error, info.componentStack)
  }

  componentDidUpdate(prev: ErrorBoundaryProps) {
    if (this.state.error && prev.resetKey !== this.props.resetKey) {
      this.setState({ error: null })
    }
  }

  reset = () => this.setState({ error: null })

  render() {
    const { error } = this.state
    if (!error) return this.props.children

    if (this.props.fallback) return this.props.fallback({ error, reset: this.reset })

    return (
      <Result
        status="error"
        title="页面渲染出错"
        subTitle={error.message || '未知错误'}
        extra={
          <Button type="primary" onClick={this.reset}>
            重试
          </Button>
        }
      />
    )
  }
}

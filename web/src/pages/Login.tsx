import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Form, Input, Button, App } from 'antd'
import JSEncrypt from 'jsencrypt'
import { getPublicKey, login } from '../api/system'
import { getUserInfo } from '../api/system'
import { useAuthStore } from '../store/auth'
import { ApiError } from '../api/request'

export default function Login() {
  const navigate = useNavigate()
  const { message } = App.useApp()
  const { setToken, setUserInfo } = useAuthStore()
  const [loading, setLoading] = useState(false)

  const onFinish = async (values: { username: string; password: string }) => {
    setLoading(true)
    try {
      const { public_key: publicKey } = await getPublicKey()

      const encryptor = new JSEncrypt()
      encryptor.setPublicKey(publicKey)
      const encryptedPassword = encryptor.encrypt(values.password)
      if (!encryptedPassword) {
        message.error('密码加密失败，请刷新重试')
        return
      }

      const { token } = await login(values.username, encryptedPassword)
      setToken(token)

      const info = await getUserInfo()
      setUserInfo(info)

      message.success('登录成功')
      navigate('/')
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        message.error('用户名或密码错误')
      } else {
        message.error('登录失败，请稍后重试')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div
      style={{
        minHeight: '100vh',
        background: '#f5f5f5',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      <div style={{ width: 360 }}>
        {/* Brand */}
        <div style={{ marginBottom: 48 }}>
          <div style={{ display: 'inline-flex', alignItems: 'center', gap: 10, marginBottom: 6 }}>
            <svg width="28" height="28" viewBox="0 0 28 28" fill="none">
              <rect width="28" height="28" rx="6" fill="#1677ff" />
              <path
                d="M7 14h4l3-7 4 14 3-9 2 2h2"
                stroke="#ffffff"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
            <div style={{ display: 'flex', flexDirection: 'column', lineHeight: 1.25 }}>
              <span style={{ color: '#1a1a1a', fontSize: 18, fontWeight: 700, letterSpacing: '0.5px' }}>
                运维管理平台
              </span>
              <span style={{ color: '#888888', fontSize: 11, letterSpacing: '0.5px' }}>Cloud-Hub DevOps</span>
            </div>
          </div>
        </div>

        {/* Form */}
        <Form onFinish={onFinish} layout="vertical" size="large">
          <Form.Item
            name="username"
            label={<span style={{ color: '#333333', fontSize: 13 }}>用户名</span>}
            rules={[{ required: true, message: '' }]}
          >
            <Input
              placeholder="请输入用户名"
              autoComplete="username"
              style={{
                background: '#ffffff',
                border: '1px solid #d9d9d9',
                borderRadius: 6,
                color: '#1a1a1a',
                height: 44,
              }}
            />
          </Form.Item>

          <Form.Item
            name="password"
            label={<span style={{ color: '#333333', fontSize: 13 }}>密码</span>}
            rules={[{ required: true, message: '' }]}
          >
            <Input.Password
              placeholder="请输入密码"
              autoComplete="current-password"
              style={{
                background: '#ffffff',
                border: '1px solid #d9d9d9',
                borderRadius: 6,
                color: '#1a1a1a',
                height: 44,
              }}
            />
          </Form.Item>

          <Form.Item style={{ marginTop: 24 }}>
            <Button
              type="primary"
              htmlType="submit"
              loading={loading}
              block
              style={{
                height: 44,
                background: '#1677ff',
                color: '#ffffff',
                border: 'none',
                borderRadius: 6,
                fontWeight: 600,
                fontSize: 14,
                letterSpacing: '1px',
              }}
            >
              {loading ? '登录中…' : '登 录'}
            </Button>
          </Form.Item>
        </Form>

        <p style={{ color: '#999999', fontSize: 12, textAlign: 'center', marginTop: 32 }}>
          {/* 默认账户信息已移除 */}
        </p>
      </div>
    </div>
  )
}

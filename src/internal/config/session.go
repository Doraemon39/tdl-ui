package config

import "errors"

// Session leases prevent a client using one identity while login replaces the
// underlying credentials. A transition is exclusive for its entire lifetime.
func (m *Manager) UseSession() (func(), error) {
	if m == nil {
		return func() {}, nil
	}
	if !m.sessionMu.TryRLock() {
		return nil, errors.New("账号会话正在变更，请稍后重试")
	}
	return m.sessionMu.RUnlock, nil
}
func (m *Manager) ChangeSession() (func(), error) {
	if m == nil {
		return func() {}, nil
	}
	if !m.sessionMu.TryLock() {
		return nil, errors.New("当前会话仍在使用，请停止下载或等待媒体请求结束后重试")
	}
	return m.sessionMu.Unlock, nil
}

// Package secrets 提供不落入普通数据库的敏感配置存取。
package secrets

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Backend 是操作系统安全存储的最小接口。
type Backend interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

// Store 优先使用安全存储，并将环境变量作为只读降级。
type Store struct {
	backend   Backend
	envPrefix string
}

// NewStore 创建 Secret Store。
func NewStore(backend Backend, envPrefix string) *Store {
	return &Store{backend: backend, envPrefix: envPrefix}
}

// NewSystemStore 创建使用操作系统 Keychain 的 Secret Store。
func NewSystemStore() *Store {
	return NewStore(keyringBackend{}, "INKHUB_")
}

// Get 读取 Secret，Keychain 不可用时读取对应环境变量。
func (s *Store) Get(ctx context.Context, key string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if value, err := s.backend.Get(key); err == nil {
		return value, nil
	}
	envKey := s.envPrefix + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
	if value, ok := os.LookupEnv(envKey); ok {
		return value, nil
	}
	return "", fmt.Errorf("Secret %q 不可用", key)
}

// Set 将 Secret 写入操作系统 Keychain。
func (s *Store) Set(ctx context.Context, key, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.backend.Set(key, value); err != nil {
		return fmt.Errorf("保存 Secret %q: %w", key, err)
	}
	return nil
}

// Delete 从操作系统 Keychain 删除 Secret。
func (s *Store) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.backend.Delete(key); err != nil {
		return fmt.Errorf("删除 Secret %q: %w", key, err)
	}
	return nil
}

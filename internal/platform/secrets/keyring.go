package secrets

import "github.com/zalando/go-keyring"

const keyringService = "InkHub"

type keyringBackend struct{}

// Get 从操作系统 Keychain 读取指定键。
func (keyringBackend) Get(key string) (string, error) {
	return keyring.Get(keyringService, key)
}

// Set 将指定键值写入操作系统 Keychain。
func (keyringBackend) Set(key, value string) error {
	return keyring.Set(keyringService, key, value)
}

// Delete 从操作系统 Keychain 删除指定键。
func (keyringBackend) Delete(key string) error {
	return keyring.Delete(keyringService, key)
}

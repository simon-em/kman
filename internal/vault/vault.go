package vault

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const keyFileName = "vault.key"

const secretsFileName = "secrets/vault.enc"

var ErrNotFound = errors.New("secret not found")

type Vault struct {
	keyPath     string
	secretsPath string
	key         []byte
}

func Open(home string) (*Vault, error) {
	v := &Vault{
		keyPath:     filepath.Join(home, keyFileName),
		secretsPath: filepath.Join(home, secretsFileName),
	}
	key, err := loadOrCreateKey(v.keyPath)
	if err != nil {
		return nil, err
	}
	v.key = key
	return v, nil
}

func loadOrCreateKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		if len(data) != 32 {
			return nil, fmt.Errorf("%s: expected a 32-byte key, got %d bytes", path, len(data))
		}
		return data, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func (v *Vault) Set(name, value string) error {
	secrets, err := v.load()
	if err != nil {
		return err
	}
	secrets[name] = value
	return v.save(secrets)
}

func (v *Vault) Get(name string) (string, error) {
	secrets, err := v.load()
	if err != nil {
		return "", err
	}
	value, ok := secrets[name]
	if !ok {
		return "", fmt.Errorf("%s: %w", name, ErrNotFound)
	}
	return value, nil
}

func (v *Vault) Remove(name string) error {
	secrets, err := v.load()
	if err != nil {
		return err
	}
	if _, ok := secrets[name]; !ok {
		return fmt.Errorf("%s: %w", name, ErrNotFound)
	}
	delete(secrets, name)
	return v.save(secrets)
}

func (v *Vault) List() ([]string, error) {
	secrets, err := v.load()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(secrets))
	for name := range secrets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (v *Vault) load() (map[string]string, error) {
	data, err := os.ReadFile(v.secretsPath)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	plain, err := decrypt(v.key, data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", v.secretsPath, err)
	}
	var secrets map[string]string
	if err := json.Unmarshal(plain, &secrets); err != nil {
		return nil, fmt.Errorf("%s: %w", v.secretsPath, err)
	}
	return secrets, nil
}

func (v *Vault) save(secrets map[string]string) error {
	plain, err := json.Marshal(secrets)
	if err != nil {
		return err
	}
	sealed, err := encrypt(v.key, plain)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(v.secretsPath), 0o700); err != nil {
		return err
	}
	tmp := v.secretsPath + ".tmp"
	if err := os.WriteFile(tmp, sealed, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, v.secretsPath)
}

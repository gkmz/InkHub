package secrets

import (
	"context"
	"errors"
	"testing"
)

func TestStoreFallsBackToEnvironmentForReads(t *testing.T) {
	t.Setenv("INKHUB_AI_API_KEY", "from-env")
	store := NewStore(failingBackend{}, "INKHUB_")

	value, err := store.Get(context.Background(), "ai_api_key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if value != "from-env" {
		t.Fatalf("Get() = %q, want from-env", value)
	}
}

func TestStoreDoesNotFallBackToEnvironmentForWrites(t *testing.T) {
	store := NewStore(failingBackend{}, "INKHUB_")
	if err := store.Set(context.Background(), "ai_api_key", "secret"); err == nil {
		t.Fatal("Set() must fail when the Keychain is unavailable")
	}
}

type failingBackend struct{}

func (failingBackend) Get(string) (string, error) { return "", errors.New("unavailable") }
func (failingBackend) Set(string, string) error   { return errors.New("unavailable") }
func (failingBackend) Delete(string) error        { return errors.New("unavailable") }

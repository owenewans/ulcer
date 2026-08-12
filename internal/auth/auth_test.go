package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/owenewans/ulcer/internal/store"
)

func TestConfiguredOperatorDoesNotRegenerateSetupToken(t *testing.T) {
	database, err := store.Open("", true)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	hash := sha256.Sum256([]byte("recovery"))
	if err := database.CompleteOperator("totp-secret", 1, []string{hex.EncodeToString(hash[:])}, "session", SessionTTL); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	service, generated, err := New(database, directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if generated || service.CheckSetupToken("") {
		t.Fatal("configured operator must not have a setup token")
	}
	if _, err := os.Stat(filepath.Join(directory, "setup.token")); !os.IsNotExist(err) {
		t.Fatalf("setup token was regenerated: %v", err)
	}
}

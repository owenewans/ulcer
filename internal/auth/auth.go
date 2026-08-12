package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/owenewans/ulcer/internal/store"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const SessionTTL = 12 * time.Hour

var ErrInvalidCode = errors.New("invalid authentication code")

type Service struct {
	store          *store.Store
	setupTokenPath string
	mu             sync.RWMutex
	setupToken     string
}

func New(store *store.Store, dataDir, configuredToken string) (*Service, bool, error) {
	ready, err := store.OperatorReady()
	if err != nil {
		return nil, false, err
	}
	if ready {
		return &Service{store: store}, false, nil
	}
	token, tokenPath, generated, err := setupToken(dataDir, configuredToken)
	if err != nil {
		return nil, false, err
	}
	return &Service{store: store, setupToken: token, setupTokenPath: tokenPath}, generated, nil
}

func (s *Service) CheckSetupToken(candidate string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.setupToken != "" && subtle.ConstantTimeCompare([]byte(candidate), []byte(s.setupToken)) == 1
}

func (s *Service) BeginSetup() (secret, uri string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "ulcer",
		AccountName: "operator",
		SecretSize:  32,
	})
	if err != nil {
		return "", "", err
	}
	if err := s.store.SetPendingTOTP(key.Secret()); err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

func (s *Service) CompleteSetup(code string) ([]string, string, error) {
	secret, err := s.store.PendingTOTP()
	if err != nil {
		return nil, "", err
	}
	step, valid := MatchTOTPStep(secret, code, time.Now().UTC())
	if !valid {
		return nil, "", ErrInvalidCode
	}
	codes := make([]string, 10)
	hashes := make([]string, 10)
	for index := range codes {
		partA, err := randomHex(8)
		if err != nil {
			return nil, "", err
		}
		partB, err := randomHex(8)
		if err != nil {
			return nil, "", err
		}
		codes[index] = strings.ToUpper(partA + "-" + partB)
		sum := sha256.Sum256([]byte(codes[index]))
		hashes[index] = hex.EncodeToString(sum[:])
	}
	session, err := randomHex(32)
	if err != nil {
		return nil, "", err
	}
	if err := s.store.CompleteOperator(secret, step, hashes, session, SessionTTL); err != nil {
		return nil, "", err
	}
	s.mu.Lock()
	s.setupToken = ""
	tokenPath := s.setupTokenPath
	s.setupTokenPath = ""
	s.mu.Unlock()
	if tokenPath != "" {
		_ = os.Remove(tokenPath)
	}
	return codes, session, nil
}

func (s *Service) Login(code string) (string, error) {
	secret, err := s.store.TOTPSecret()
	if err != nil {
		return "", err
	}
	step, valid := MatchTOTPStep(secret, code, time.Now().UTC())
	if valid {
		valid, err = s.store.ConsumeTOTPStep(step)
		if err != nil {
			return "", err
		}
	}
	if !valid {
		valid, err = s.store.ConsumeRecoveryCode(code)
		if err != nil {
			return "", err
		}
	}
	if !valid {
		return "", ErrInvalidCode
	}
	return s.NewSession()
}

func (s *Service) NewSession() (string, error) {
	token, err := randomHex(32)
	if err != nil {
		return "", err
	}
	if err := s.store.CreateSession(token, SessionTTL); err != nil {
		return "", err
	}
	return token, nil
}

func ValidateTOTP(secret, code string) bool {
	_, valid := MatchTOTPStep(secret, code, time.Now().UTC())
	return valid
}

func MatchTOTPStep(secret, code string, now time.Time) (uint64, bool) {
	opts := totp.ValidateOpts{
		Period:    30,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	}
	current := now.Unix() / int64(opts.Period)
	for _, offset := range []int64{0, 1, -1} {
		step := current + offset
		if step < 0 {
			continue
		}
		candidate, err := totp.GenerateCodeCustom(secret, time.Unix(step*int64(opts.Period), 0).UTC(), opts)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(strings.TrimSpace(code))) == 1 {
			return uint64(step), true
		}
	}
	return 0, false
}

func setupToken(dataDir, configured string) (string, string, bool, error) {
	if configured != "" {
		return configured, "", false, nil
	}
	path := filepath.Join(dataDir, "setup.token")
	data, err := os.ReadFile(path)
	if err == nil {
		return strings.TrimSpace(string(data)), path, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", "", false, fmt.Errorf("read setup token: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", "", false, err
	}
	token, err := randomHex(24)
	if err != nil {
		return "", "", false, err
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", "", false, fmt.Errorf("write setup token: %w", err)
	}
	return token, path, true, nil
}

func randomHex(bytes int) (string, error) {
	data := make([]byte, bytes)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

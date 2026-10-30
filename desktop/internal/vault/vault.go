// Package vault encrypts desktop credentials with a user passphrase.
//
// The passphrase is never stored. PBKDF2-HMAC-SHA256 derives an AES-256-GCM
// key from it; the ciphertext and KDF parameters live in the SQLite config
// database. Losing the passphrase means re-entering AK/SK, by design.
package vault

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
)

const (
	Version           = 1
	KDF               = "pbkdf2-sha256"
	DefaultIterations = 600_000
	MinIterations     = 100_000
	MaxIterations     = 2_000_000
	SaltSize          = 16
	NonceSize         = 12
	KeySize           = 32
	MinPassphrase     = 8
	MaxPassphrase     = 1024
	MaxCiphertext     = 8192
)

var (
	// ErrWrongPassphrase deliberately hides whether the failure came from the
	// passphrase, the ciphertext or the authenticated metadata.
	ErrWrongPassphrase = errors.New("vault passphrase rejected")
	ErrMissing         = configstore.ErrNoVault
	ErrInvalid         = errors.New("invalid vault record")
)

// Payload is the plaintext inside the encrypted envelope.
type Payload struct {
	Provider      string `json:"provider,omitempty"`
	AccessKey     string `json:"accessKey"`
	SecretKey     string `json:"secretKey"`
	SecurityToken string `json:"securityToken,omitempty"`
	ExpiresAt     string `json:"expiresAt,omitempty"`
}

// Status is safe to project to the UI; it contains no secrets.
type Status struct {
	Exists    bool
	AccountID string
	Region    string
	UpdatedAt int64
}

type Service struct {
	store      *configstore.Store
	iterations int
}

func New(store *configstore.Store) *Service {
	return &Service{store: store, iterations: DefaultIterations}
}

// NewWithIterations is for tests and future KDF migration; production uses
// New. Values below MinIterations are rejected at seal time.
func NewWithIterations(store *configstore.Store, iterations int) *Service {
	return &Service{store: store, iterations: iterations}
}

func (s *Service) Status(ctx context.Context) (Status, error) {
	record, err := s.store.LoadVault(ctx)
	if errors.Is(err, configstore.ErrNoVault) {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	return Status{Exists: true, AccountID: record.AccountID, Region: record.Region, UpdatedAt: record.UpdatedAt}, nil
}

func (s *Service) Save(ctx context.Context, passphrase []byte, payload Payload, accountID, region string) error {
	record, err := seal(passphrase, payload, accountID, region, s.iterations, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	return s.store.SaveVault(ctx, record)
}

func (s *Service) Unlock(ctx context.Context, passphrase []byte) (Payload, Status, error) {
	record, err := s.store.LoadVault(ctx)
	if err != nil {
		return Payload{}, Status{}, err
	}
	payload, err := open(passphrase, record)
	if err != nil {
		return Payload{}, Status{}, err
	}
	return payload, Status{Exists: true, AccountID: record.AccountID, Region: record.Region, UpdatedAt: record.UpdatedAt}, nil
}

func (s *Service) Forget(ctx context.Context) error {
	return s.store.DeleteVault(ctx)
}

func validPassphrase(passphrase []byte) bool {
	return len(passphrase) >= MinPassphrase &&
		len(passphrase) <= MaxPassphrase &&
		strings.TrimSpace(string(passphrase)) != ""
}

func validPayload(p Payload) bool {
	return (p.Provider == "" || p.Provider == "volcengine" || p.Provider == "byteplus") && p.AccessKey != "" && p.SecretKey != "" &&
		len(p.AccessKey) <= 2048 && len(p.SecretKey) <= 2048 &&
		len(p.SecurityToken) <= 16384 && len(p.ExpiresAt) <= 64 &&
		!strings.ContainsAny(p.AccessKey+p.SecretKey+p.SecurityToken+p.ExpiresAt, "\x00\r\n")
}

func validAccount(accountID, region string) bool {
	return accountID != "" && len(accountID) <= 128 && len(region) <= 64 &&
		!strings.ContainsAny(accountID+region, "\x00\r\n/?#")
}

func aad(accountID, region string) []byte {
	return []byte("openmpa-vault-v1\x00" + accountID + "\x00" + region)
}

func seal(passphrase []byte, payload Payload, accountID, region string, iterations int, now int64) (configstore.VaultRecord, error) {
	if !validPassphrase(passphrase) || !validPayload(payload) || !validAccount(accountID, region) {
		return configstore.VaultRecord{}, ErrInvalid
	}
	if iterations < MinIterations || iterations > MaxIterations {
		return configstore.VaultRecord{}, ErrInvalid
	}
	salt := make([]byte, SaltSize)
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(salt); err != nil {
		return configstore.VaultRecord{}, err
	}
	if _, err := rand.Read(nonce); err != nil {
		return configstore.VaultRecord{}, err
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return configstore.VaultRecord{}, err
	}
	defer clear(plaintext)
	// crypto/pbkdf2 accepts a string; this transient copy is the one place the
	// passphrase exists as an immutable Go string. Everything else uses []byte.
	key, err := pbkdf2.Key(sha256.New, string(passphrase), salt, iterations, KeySize)
	if err != nil {
		return configstore.VaultRecord{}, err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return configstore.VaultRecord{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return configstore.VaultRecord{}, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad(accountID, region))
	if len(ciphertext) > MaxCiphertext {
		return configstore.VaultRecord{}, ErrInvalid
	}
	return configstore.VaultRecord{
		Version:    Version,
		KDF:        KDF,
		Iterations: iterations,
		Salt:       salt,
		Nonce:      nonce,
		Ciphertext: ciphertext,
		AccountID:  accountID,
		Region:     region,
		UpdatedAt:  now,
	}, nil
}

func open(passphrase []byte, record configstore.VaultRecord) (Payload, error) {
	if !validPassphrase(passphrase) || !validRecord(record) {
		return Payload{}, ErrWrongPassphrase
	}
	key, err := pbkdf2.Key(sha256.New, string(passphrase), record.Salt, record.Iterations, KeySize)
	if err != nil {
		return Payload{}, ErrWrongPassphrase
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return Payload{}, ErrWrongPassphrase
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Payload{}, ErrWrongPassphrase
	}
	plaintext, err := gcm.Open(nil, record.Nonce, record.Ciphertext, aad(record.AccountID, record.Region))
	if err != nil {
		return Payload{}, ErrWrongPassphrase
	}
	defer clear(plaintext)
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	var payload Payload
	if decoder.Decode(&payload) != nil || decoder.Decode(new(any)) != io.EOF || !validPayload(payload) {
		return Payload{}, ErrWrongPassphrase
	}
	return payload, nil
}

func validRecord(record configstore.VaultRecord) bool {
	return record.Version == Version &&
		record.KDF == KDF &&
		record.Iterations >= MinIterations && record.Iterations <= MaxIterations &&
		len(record.Salt) == SaltSize && len(record.Nonce) == NonceSize &&
		len(record.Ciphertext) > 0 && len(record.Ciphertext) <= MaxCiphertext &&
		validAccount(record.AccountID, record.Region)
}

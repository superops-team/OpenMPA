// Package vault encrypts desktop credentials and Runtime user tokens with a
// user passphrase.
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
	Version                = 1
	KDF                    = "pbkdf2-sha256"
	DefaultIterations      = 600_000
	MinIterations          = 100_000
	MaxIterations          = 2_000_000
	SaltSize               = 16
	NonceSize              = 12
	KeySize                = 32
	MinPassphrase          = 8
	MaxPassphrase          = 1024
	MaxCiphertext          = 8192
	MaxUserTokenCiphertext = 32768
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

// UserToken is the decrypted Runtime user identity material. It should stay in
// memory only long enough to call Runtime identity APIs.
type UserToken struct {
	RuntimeID    string `json:"runtimeId"`
	UserPoolID   string `json:"userPoolId"`
	Issuer       string `json:"issuer"`
	ClientID     string `json:"clientId"`
	Subject      string `json:"subject"`
	MPAID        string `json:"mpaId"`
	SpaceID      string `json:"spaceId"`
	IDToken      string `json:"idToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    string `json:"expiresAt"`
}

// UserTokenStatus is safe to project to the UI; it contains no tokens.
type UserTokenStatus struct {
	Exists     bool
	RuntimeID  string
	UserPoolID string
	Issuer     string
	ClientID   string
	Subject    string
	MPAID      string
	SpaceID    string
	ExpiresAt  string
	UpdatedAt  int64
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

func (s *Service) UserTokenStatus(ctx context.Context, runtimeID string) (UserTokenStatus, error) {
	record, err := s.store.LoadUserTokenVault(ctx, runtimeID)
	if errors.Is(err, configstore.ErrNoVault) {
		return UserTokenStatus{}, nil
	}
	if err != nil {
		return UserTokenStatus{}, err
	}
	return userTokenStatus(record), nil
}

func (s *Service) SaveUserToken(ctx context.Context, passphrase []byte, token UserToken) error {
	record, err := sealUserToken(passphrase, token, s.iterations, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	return s.store.SaveUserTokenVault(ctx, record)
}

func (s *Service) UnlockUserToken(ctx context.Context, passphrase []byte, runtimeID string) (UserToken, error) {
	record, err := s.store.LoadUserTokenVault(ctx, runtimeID)
	if err != nil {
		return UserToken{}, err
	}
	return openUserToken(passphrase, record)
}

func (s *Service) ForgetUserToken(ctx context.Context, runtimeID string) error {
	return s.store.DeleteUserTokenVault(ctx, runtimeID)
}

func validPassphrase(passphrase []byte) bool {
	return len(passphrase) >= MinPassphrase &&
		len(passphrase) <= MaxPassphrase &&
		strings.TrimSpace(string(passphrase)) != ""
}

func validPayload(p Payload) bool {
	return p.AccessKey != "" && p.SecretKey != "" &&
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

func validRuntimeID(value string) bool {
	return value != "" && len(value) <= 256 &&
		!strings.ContainsAny(value, "\x00\r\n/?#") &&
		value != "." && value != ".."
}

func validUserTokenText(value string, limit int) bool {
	return value != "" && len(value) <= limit && !strings.ContainsAny(value, "\x00\r\n")
}

func validUserToken(token UserToken) bool {
	return validRuntimeID(token.RuntimeID) &&
		validUserTokenText(token.UserPoolID, 128) &&
		validUserTokenText(token.Issuer, 1024) &&
		validUserTokenText(token.ClientID, 512) &&
		validUserTokenText(token.Subject, 512) &&
		validUserTokenText(token.MPAID, 256) &&
		validUserTokenText(token.SpaceID, 256) &&
		validUserTokenText(token.IDToken, 16384) &&
		validUserTokenText(token.RefreshToken, 16384) &&
		validUserTokenText(token.ExpiresAt, 64)
}

func userTokenAAD(runtimeID, userPoolID, issuer, clientID, subject, mpaID, spaceID, expiresAt string) []byte {
	return []byte("openmpa-user-token-vault-v1\x00" + runtimeID + "\x00" + userPoolID + "\x00" + issuer + "\x00" + clientID + "\x00" + subject + "\x00" + mpaID + "\x00" + spaceID + "\x00" + expiresAt)
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

func sealUserToken(passphrase []byte, token UserToken, iterations int, now int64) (configstore.UserTokenVaultRecord, error) {
	if !validPassphrase(passphrase) || !validUserToken(token) {
		return configstore.UserTokenVaultRecord{}, ErrInvalid
	}
	if iterations < MinIterations || iterations > MaxIterations {
		return configstore.UserTokenVaultRecord{}, ErrInvalid
	}
	salt := make([]byte, SaltSize)
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(salt); err != nil {
		return configstore.UserTokenVaultRecord{}, err
	}
	if _, err := rand.Read(nonce); err != nil {
		return configstore.UserTokenVaultRecord{}, err
	}
	plaintext, err := json.Marshal(struct {
		IDToken      string `json:"idToken"`
		RefreshToken string `json:"refreshToken"`
	}{
		IDToken:      token.IDToken,
		RefreshToken: token.RefreshToken,
	})
	if err != nil {
		return configstore.UserTokenVaultRecord{}, err
	}
	defer clear(plaintext)
	key, err := pbkdf2.Key(sha256.New, string(passphrase), salt, iterations, KeySize)
	if err != nil {
		return configstore.UserTokenVaultRecord{}, err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return configstore.UserTokenVaultRecord{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return configstore.UserTokenVaultRecord{}, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, userTokenAAD(token.RuntimeID, token.UserPoolID, token.Issuer, token.ClientID, token.Subject, token.MPAID, token.SpaceID, token.ExpiresAt))
	if len(ciphertext) > MaxUserTokenCiphertext {
		return configstore.UserTokenVaultRecord{}, ErrInvalid
	}
	return configstore.UserTokenVaultRecord{
		RuntimeID:  token.RuntimeID,
		Version:    Version,
		KDF:        KDF,
		Iterations: iterations,
		Salt:       salt,
		Nonce:      nonce,
		Ciphertext: ciphertext,
		UserPoolID: token.UserPoolID,
		Issuer:     token.Issuer,
		ClientID:   token.ClientID,
		Subject:    token.Subject,
		MPAID:      token.MPAID,
		SpaceID:    token.SpaceID,
		ExpiresAt:  token.ExpiresAt,
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

func openUserToken(passphrase []byte, record configstore.UserTokenVaultRecord) (UserToken, error) {
	if !validPassphrase(passphrase) || !validUserTokenRecord(record) {
		return UserToken{}, ErrWrongPassphrase
	}
	key, err := pbkdf2.Key(sha256.New, string(passphrase), record.Salt, record.Iterations, KeySize)
	if err != nil {
		return UserToken{}, ErrWrongPassphrase
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return UserToken{}, ErrWrongPassphrase
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return UserToken{}, ErrWrongPassphrase
	}
	plaintext, err := gcm.Open(nil, record.Nonce, record.Ciphertext, userTokenAAD(record.RuntimeID, record.UserPoolID, record.Issuer, record.ClientID, record.Subject, record.MPAID, record.SpaceID, record.ExpiresAt))
	if err != nil {
		return UserToken{}, ErrWrongPassphrase
	}
	defer clear(plaintext)
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	var payload struct {
		IDToken      string `json:"idToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if decoder.Decode(&payload) != nil || decoder.Decode(new(any)) != io.EOF {
		return UserToken{}, ErrWrongPassphrase
	}
	token := UserToken{
		RuntimeID:    record.RuntimeID,
		UserPoolID:   record.UserPoolID,
		Issuer:       record.Issuer,
		ClientID:     record.ClientID,
		Subject:      record.Subject,
		MPAID:        record.MPAID,
		SpaceID:      record.SpaceID,
		IDToken:      payload.IDToken,
		RefreshToken: payload.RefreshToken,
		ExpiresAt:    record.ExpiresAt,
	}
	if !validUserToken(token) {
		return UserToken{}, ErrWrongPassphrase
	}
	return token, nil
}

func validRecord(record configstore.VaultRecord) bool {
	return record.Version == Version &&
		record.KDF == KDF &&
		record.Iterations >= MinIterations && record.Iterations <= MaxIterations &&
		len(record.Salt) == SaltSize && len(record.Nonce) == NonceSize &&
		len(record.Ciphertext) > 0 && len(record.Ciphertext) <= MaxCiphertext &&
		validAccount(record.AccountID, record.Region)
}

func validUserTokenRecord(record configstore.UserTokenVaultRecord) bool {
	return record.Version == Version &&
		record.KDF == KDF &&
		record.Iterations >= MinIterations && record.Iterations <= MaxIterations &&
		len(record.Salt) == SaltSize && len(record.Nonce) == NonceSize &&
		len(record.Ciphertext) > 0 && len(record.Ciphertext) <= MaxUserTokenCiphertext &&
		validRuntimeID(record.RuntimeID) &&
		validUserTokenText(record.UserPoolID, 128) &&
		validUserTokenText(record.Issuer, 1024) &&
		validUserTokenText(record.ClientID, 512) &&
		validUserTokenText(record.Subject, 512) &&
		validUserTokenText(record.MPAID, 256) &&
		validUserTokenText(record.SpaceID, 256) &&
		validUserTokenText(record.ExpiresAt, 64)
}

func userTokenStatus(record configstore.UserTokenVaultRecord) UserTokenStatus {
	return UserTokenStatus{
		Exists:     true,
		RuntimeID:  record.RuntimeID,
		UserPoolID: record.UserPoolID,
		Issuer:     record.Issuer,
		ClientID:   record.ClientID,
		Subject:    record.Subject,
		MPAID:      record.MPAID,
		SpaceID:    record.SpaceID,
		ExpiresAt:  record.ExpiresAt,
		UpdatedAt:  record.UpdatedAt,
	}
}

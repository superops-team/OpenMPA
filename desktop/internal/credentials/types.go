// Package credentials stores desktop credentials in the system secret store.
// It is an internal Go dependency, never a MyGo-bound IPC service.
package credentials

import (
	"errors"
	"time"
)

const MaxSecretSize = 2560
const MaxMemorySecretSize = 64 << 10

type Slot string

const (
	CloudAK              Slot = "cloudAK"
	CloudSK              Slot = "cloudSK"
	SecurityToken        Slot = "securityToken"
	CloudRefresh         Slot = "cloudRefresh"
	RuntimeGateway       Slot = "runtimeGateway"
	ApplicationJWT       Slot = "applicationJWT"
	TaskJWT              Slot = "taskJWT"
	ChannelManagement    Slot = "channelManagement"
	ChannelManagementJWT Slot = "channelManagementJWT"
	// TrustedOwner stores caller-verified owner material and its provenance.
	// Storing material grants no trust and never creates or infers an identity.
	TrustedOwner Slot = "trustedOwner"
)

var allSlots = []Slot{CloudAK, CloudSK, SecurityToken, CloudRefresh, RuntimeGateway, ApplicationJWT, TaskJWT, ChannelManagement, ChannelManagementJWT, TrustedOwner}

type Profile struct {
	ID string `json:"profileId"`
	// Scope is the verified, non-secret namespace supplied by the Go caller.
	// The store hashes it for native keys, and never derives an identity from it.
	Scope string `json:"scope"`
}

type Mode string

const (
	SystemStore   Mode = "systemStore"
	SessionMemory Mode = "sessionMemory"
)

type StorageState string

const (
	Persisted  StorageState = "persisted"
	MemoryOnly StorageState = "memoryOnly"
	Missing    StorageState = "missing"
	Failed     StorageState = "failed"
)

// Entry is write-only across JSON. An empty Value explicitly clears a slot.
type Entry struct {
	Slot      Slot      `json:"slot"`
	Value     []byte    `json:"-"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

// Metadata is the only persistent metadata representation. No value, token,
// native key, fingerprint, or caller-controlled owner claim belongs here.
type Metadata struct {
	ProfileID string    `json:"profileId"`
	Scope     string    `json:"scope"`
	Slot      Slot      `json:"slot"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

type SlotState struct {
	Metadata
	HasCredential bool `json:"hasCredential"`
}

type Snapshot struct {
	ProfileID     string       `json:"profileId"`
	Scope         string       `json:"scope"`
	HasCredential bool         `json:"hasCredential"`
	StorageState  StorageState `json:"storageState"`
	Slots         []SlotState  `json:"slots"`
}

// Bundle is for Go-only authentication adapters. Accidental JSON serialization
// still excludes every secret; callers must project Snapshot into IPC DTOs.
type Bundle struct {
	Snapshot Snapshot `json:"status"`
	Entries  []Entry  `json:"-"`
}

// SecretsBackend matches MyGo Secrets, without binding its methods to IPC.
// Get must return ErrNotFound for an absent key (or MyGo ErrSecretNotFound).
// Set replaces one whole value atomically; Delete is idempotent.
type SecretsBackend interface {
	Get(string) ([]byte, error)
	Set(string, []byte) error
	Delete(string) error
}

// All exported errors have fixed text. Native errors are never wrapped, copied
// into DTOs, logged, or retained as causes that could reveal secret material.
var (
	ErrNotFound         = errors.New("credentialMissing")
	ErrInvalidInput     = errors.New("credentialInvalidInput")
	ErrTooLarge         = errors.New("credentialTooLarge")
	ErrUnavailable      = errors.New("credentialStoreUnavailable")
	ErrStoreFailed      = errors.New("credentialStoreFailed")
	ErrRecoveryRequired = errors.New("credentialRecoveryRequired")
	ErrCanceled         = errors.New("credentialOperationCanceled")
)

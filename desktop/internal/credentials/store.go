package credentials

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

type Store struct {
	mu      sync.Mutex
	backend SecretsBackend
	memory  map[string]Bundle
}

// Multiple Store objects in one process must not interleave native transactions
// for the same profile. The desktop host owns one Store and one app instance.
var nativeProfiles sync.Map

func lockNativeProfile(key string) func() {
	value, _ := nativeProfiles.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// Profile/scope occur once, so all bounded slots still fit in one small native
// metadata entry even at the maximum accepted namespace lengths.
type manifest struct {
	ProfileID string         `json:"profileId"`
	Scope     string         `json:"scope"`
	Slots     []slotMetadata `json:"slots"`
}

type slotMetadata struct {
	Slot      Slot      `json:"slot"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

func NewWithBackend(backend SecretsBackend) *Store {
	return &Store{backend: backend, memory: make(map[string]Bundle)}
}

// Save updates the supplied slots. Cloud AK/SK updates must include both slots,
// and include SecurityToken (possibly empty) to replace an earlier STS token.
// SessionMemory is explicit and performs no native I/O. SystemStore failures
// leave no new in-memory login and never fall back to a plaintext file.
func (s *Store) Save(ctx context.Context, p Profile, entries []Entry, mode Mode) (Snapshot, error) {
	failed := snapshot(p, nil, Failed)
	if !validProfile(p) || !validEntries(entries, mode) || ctx == nil {
		return failed, ErrInvalidInput
	}
	if mode == SystemStore {
		for _, e := range entries {
			if len(e.Value) > MaxSecretSize {
				return failed, ErrTooLarge
			}
		}
	}
	if err := contextError(ctx); err != nil {
		return failed, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return failed, err
	}
	key := profileKey(p)
	if mode == SessionMemory {
		current := s.memory[key]
		next := applyUpdates(current.Entries, entries)
		b := Bundle{Snapshot: snapshot(p, next, MemoryOnly), Entries: next}
		s.memory[key] = clone(b)
		return b.Snapshot, nil
	}
	if s.backend == nil {
		return failed, ErrUnavailable
	}
	unlock := lockNativeProfile(key)
	defer unlock()
	old, err := s.read(ctx, p)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return failed, err
	}
	base := old.Entries
	if current, ok := s.memory[key]; ok {
		base = current.Entries
	}
	next := applyUpdates(base, entries)
	metadata, err := encodeMetadata(p, next)
	if err != nil {
		return failed, err
	}
	keys := keysForProfile(p)
	previous := make(map[string][]byte, len(keys))
	for _, k := range keys {
		v, err := s.get(ctx, k)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return failed, err
		}
		if err == nil {
			previous[k] = v
		}
	}
	if err := contextError(ctx); err != nil {
		return failed, err
	}
	// Persist the marker before touching any slot. A crash or failed rollback
	// is visible after restart, so Load can never return a torn AK/SK pair.
	if err := s.set(ctx, key+".pending", []byte("pending")); err != nil {
		// A native operation can mutate and then report failure. With no slot
		// changed yet, removing its possible marker restores the old profile.
		if cleanup := s.backend.Delete(key + ".pending"); cleanup != nil {
			delete(s.memory, key)
			return failed, ErrRecoveryRequired
		}
		return failed, err
	}
	changed := []string{}
	write := func(k string, v []byte, remove bool) error {
		if err := contextError(ctx); err != nil {
			return err
		}
		changed = append(changed, k) // also roll back a failing, possibly applied call
		if remove {
			return s.remove(ctx, k)
		}
		return s.set(ctx, k, v)
	}
	for _, slot := range allSlots {
		v, ok := entryValue(next, slot)
		if err = write(key+"."+string(slot), v, !ok); err != nil {
			break
		}
	}
	if err == nil {
		err = write(key+".metadata", metadata, len(next) == 0)
	}
	if err == nil {
		err = s.remove(ctx, key+".pending")
	}
	if err != nil {
		// Rollback deliberately ignores the obsolete epoch context. It repairs
		// persistent data, and never publishes an obsolete login to the caller.
		if !s.rollback(key, changed, previous) {
			delete(s.memory, key)
			return failed, ErrRecoveryRequired
		}
		return failed, err
	}
	b := Bundle{Snapshot: snapshot(p, next, Persisted), Entries: next}
	s.memory[key] = clone(b)
	return b.Snapshot, nil
}

// Load lazily reads an exact profile and scope. The caller validates expiry and
// authenticates/validates all identities again before activating a connection.
func (s *Store) Load(ctx context.Context, p Profile) (Bundle, error) {
	if !validProfile(p) || ctx == nil {
		return Bundle{}, ErrInvalidInput
	}
	if err := contextError(ctx); err != nil {
		return Bundle{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return Bundle{}, err
	}
	key := profileKey(p)
	if b, ok := s.memory[key]; ok {
		return clone(b), nil
	}
	if s.backend == nil {
		return Bundle{}, ErrUnavailable
	}
	unlock := lockNativeProfile(key)
	defer unlock()
	b, err := s.read(ctx, p)
	if err != nil {
		return Bundle{}, err
	}
	s.memory[key] = clone(b)
	return clone(b), nil
}

func (s *Store) Status(ctx context.Context, p Profile) (Snapshot, error) {
	b, err := s.Load(ctx, p)
	if errors.Is(err, ErrNotFound) {
		return snapshot(p, nil, Missing), nil
	}
	if err != nil {
		return snapshot(p, nil, Failed), err
	}
	return b.Snapshot, nil
}

// ForgetSession clears only this process's copy and performs no native I/O.
func (s *Store) ForgetSession(p Profile) error {
	if !validProfile(p) {
		return ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.memory, profileKey(p))
	return nil
}

// Delete removes only this exact profile+scope, including an interrupted write.
// Failed/canceled deletion leaves its marker; restoring the profile then fails
// closed until the user retries deletion. Other profiles are never enumerated.
func (s *Store) Delete(ctx context.Context, p Profile) error {
	if !validProfile(p) || ctx == nil {
		return ErrInvalidInput
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := profileKey(p)
	delete(s.memory, key)
	if s.backend == nil {
		return ErrUnavailable
	}
	unlock := lockNativeProfile(key)
	defer unlock()
	if err := s.set(ctx, key+".pending", []byte("pending")); err != nil {
		return err
	}
	for _, k := range keysForProfile(p) {
		if err := s.remove(ctx, k); err != nil {
			return err
		}
	}
	return s.remove(ctx, key+".pending")
}

func (s *Store) read(ctx context.Context, p Profile) (Bundle, error) {
	key := profileKey(p)
	if _, err := s.get(ctx, key+".pending"); err == nil {
		return Bundle{}, ErrRecoveryRequired
	} else if !errors.Is(err, ErrNotFound) {
		return Bundle{}, err
	}
	data, err := s.get(ctx, key+".metadata")
	if err != nil {
		return Bundle{}, err
	}
	var metadata manifest
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if len(data) > MaxSecretSize || d.Decode(&metadata) != nil || metadata.ProfileID != p.ID || metadata.Scope != p.Scope || len(metadata.Slots) > len(allSlots) || d.Decode(new(any)) != io.EOF {
		return Bundle{}, ErrRecoveryRequired
	}
	seen := map[Slot]bool{}
	entries := make([]Entry, 0, len(metadata.Slots))
	for _, m := range metadata.Slots {
		if !validSlot(m.Slot) || seen[m.Slot] {
			return Bundle{}, ErrRecoveryRequired
		}
		seen[m.Slot] = true
		v, err := s.get(ctx, key+"."+string(m.Slot))
		if errors.Is(err, ErrNotFound) {
			return Bundle{}, ErrRecoveryRequired
		}
		if err != nil {
			return Bundle{}, err
		}
		if len(v) == 0 || len(v) > MaxSecretSize {
			return Bundle{}, ErrRecoveryRequired
		}
		entries = append(entries, Entry{Slot: m.Slot, Value: v, ExpiresAt: m.ExpiresAt})
	}
	if seen[CloudAK] != seen[CloudSK] || (seen[SecurityToken] && !seen[CloudAK]) {
		return Bundle{}, ErrRecoveryRequired
	}
	if len(entries) == 0 {
		return Bundle{}, ErrNotFound
	}
	return Bundle{Snapshot: snapshot(p, entries, Persisted), Entries: entries}, nil
}

func (s *Store) rollback(key string, changed []string, previous map[string][]byte) bool {
	// Re-establish the marker in case the final Delete mutated then failed.
	if s.backend.Set(key+".pending", []byte("pending")) != nil {
		return false
	}
	ok := true
	for i := len(changed) - 1; i >= 0; i-- {
		k := changed[i]
		v, present := previous[k]
		var err error
		if present {
			err = s.backend.Set(k, append([]byte(nil), v...))
		} else {
			err = s.backend.Delete(k)
		}
		if err != nil {
			ok = false
		}
	}
	if !ok {
		return false
	}
	return s.backend.Delete(key+".pending") == nil
}

func (s *Store) get(ctx context.Context, key string) ([]byte, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	v, err := s.backend.Get(key)
	if e := contextError(ctx); e != nil {
		return nil, e
	}
	if err != nil {
		return nil, safeBackendError(err)
	}
	return append([]byte(nil), v...), nil
}
func (s *Store) set(ctx context.Context, key string, value []byte) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	err := s.backend.Set(key, append([]byte(nil), value...))
	if e := contextError(ctx); e != nil {
		return e
	}
	return safeBackendError(err)
}
func (s *Store) remove(ctx context.Context, key string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	err := s.backend.Delete(key)
	if e := contextError(ctx); e != nil {
		return e
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, mygo.ErrSecretNotFound) {
		return nil
	}
	return safeBackendError(err)
}
func safeBackendError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, mygo.ErrSecretNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, errors.ErrUnsupported) || errors.Is(err, ErrUnavailable) {
		return ErrUnavailable
	}
	return ErrStoreFailed
}
func contextError(ctx context.Context) error {
	if ctx.Err() != nil {
		return ErrCanceled
	}
	return nil
}
func validProfile(p Profile) bool {
	if len(p.ID) == 0 || len(p.ID) > 96 || len(p.Scope) == 0 || len(p.Scope) > 512 {
		return false
	}
	for _, r := range p.ID {
		if !asciiIdentifier(r) {
			return false
		}
	}
	for _, r := range p.Scope {
		if !asciiIdentifier(r) && !strings.ContainsRune("./:@+", r) {
			return false
		}
	}
	return !strings.Contains(p.Scope, "..")
}
func asciiIdentifier(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-'
}
func validSlot(slot Slot) bool {
	for _, s := range allSlots {
		if slot == s {
			return true
		}
	}
	return false
}
func validEntries(entries []Entry, mode Mode) bool {
	if mode != SystemStore && mode != SessionMemory || len(entries) == 0 || len(entries) > len(allSlots) {
		return false
	}
	seen := map[Slot]bool{}
	for _, e := range entries {
		if !validSlot(e.Slot) || seen[e.Slot] || len(e.Value) > MaxMemorySecretSize {
			return false
		}
		seen[e.Slot] = true
	}
	if seen[CloudAK] != seen[CloudSK] || seen[SecurityToken] && !seen[CloudAK] {
		return false
	}
	if seen[CloudAK] {
		ak, _ := entryValue(entries, CloudAK)
		sk, _ := entryValue(entries, CloudSK)
		if (len(ak) == 0) != (len(sk) == 0) {
			return false
		}
		sts, _ := entryValue(entries, SecurityToken)
		if len(ak) == 0 && len(sts) != 0 {
			return false
		}
	}
	return true
}
func profileKey(p Profile) string {
	h := sha256.Sum256([]byte(p.ID + "\x00" + p.Scope))
	return "openmpa.credentials.v1." + hex.EncodeToString(h[:])
}
func keysForProfile(p Profile) []string {
	key := profileKey(p)
	keys := make([]string, 0, len(allSlots)+1)
	for _, slot := range allSlots {
		keys = append(keys, key+"."+string(slot))
	}
	return append(keys, key+".metadata")
}
func containsSlot(entries []Entry, slot Slot) bool {
	for _, e := range entries {
		if e.Slot == slot {
			return true
		}
	}
	return false
}
func entryValue(entries []Entry, slot Slot) ([]byte, bool) {
	for _, e := range entries {
		if e.Slot == slot {
			return e.Value, true
		}
	}
	return nil, false
}
func merge(base, updates []Entry) []Entry {
	m := map[Slot]Entry{}
	for _, e := range base {
		m[e.Slot] = e
	}
	for _, e := range updates {
		if len(e.Value) == 0 {
			delete(m, e.Slot)
		} else {
			m[e.Slot] = e
		}
	}
	out := make([]Entry, 0, len(m))
	for _, slot := range allSlots {
		if e, ok := m[slot]; ok {
			e.Value = append([]byte(nil), e.Value...)
			out = append(out, e)
		}
	}
	return out
}

func applyUpdates(base, updates []Entry) []Entry {
	next := merge(base, updates)
	if containsSlot(updates, CloudAK) && !containsSlot(updates, SecurityToken) {
		next = merge(next, []Entry{{Slot: SecurityToken}})
	}
	return next
}
func snapshot(p Profile, entries []Entry, state StorageState) Snapshot {
	out := Snapshot{ProfileID: p.ID, Scope: p.Scope, HasCredential: len(entries) > 0, StorageState: state, Slots: []SlotState{}}
	for _, e := range entries {
		out.Slots = append(out.Slots, SlotState{Metadata: Metadata{ProfileID: p.ID, Scope: p.Scope, Slot: e.Slot, ExpiresAt: e.ExpiresAt}, HasCredential: true})
	}
	return out
}
func encodeMetadata(p Profile, entries []Entry) ([]byte, error) {
	metadata := manifest{ProfileID: p.ID, Scope: p.Scope, Slots: make([]slotMetadata, 0, len(entries))}
	for _, e := range entries {
		if len(e.Value) > MaxSecretSize {
			return nil, ErrTooLarge
		}
		metadata.Slots = append(metadata.Slots, slotMetadata{Slot: e.Slot, ExpiresAt: e.ExpiresAt})
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if len(data) > MaxSecretSize {
		return nil, ErrTooLarge
	}
	return data, nil
}
func clone(b Bundle) Bundle {
	b.Entries = merge(nil, b.Entries)
	b.Snapshot.Slots = append([]SlotState(nil), b.Snapshot.Slots...)
	return b
}

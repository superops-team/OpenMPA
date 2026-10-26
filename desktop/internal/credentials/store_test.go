package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeBackend struct {
	values     map[string][]byte
	calls      int
	getHook    func(string) error
	setHook    func(string, []byte) error
	deleteHook func(string) error
}

func fakeSecrets() *fakeBackend { return &fakeBackend{values: map[string][]byte{}} }
func (f *fakeBackend) Get(key string) ([]byte, error) {
	f.calls++
	if f.getHook != nil {
		if err := f.getHook(key); err != nil {
			return nil, err
		}
	}
	value, ok := f.values[key]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), value...), nil
}
func (f *fakeBackend) Set(key string, value []byte) error {
	f.calls++
	if f.setHook != nil {
		if err := f.setHook(key, value); err != nil {
			return err
		}
	}
	f.values[key] = append([]byte(nil), value...)
	return nil
}
func (f *fakeBackend) Delete(key string) error {
	f.calls++
	if f.deleteHook != nil {
		if err := f.deleteHook(key); err != nil {
			return err
		}
	}
	delete(f.values, key)
	return nil
}

var profileA = Profile{ID: "credential-profile-a", Scope: "account/123/profile/a"}
var profileB = Profile{ID: "credential-profile-b", Scope: "account/456/profile/b"}

func cloudPair(ak, sk string) []Entry {
	return []Entry{{Slot: CloudAK, Value: []byte(ak)}, {Slot: CloudSK, Value: []byte(sk)}}
}
func valueOf(b Bundle, slot Slot) string {
	for _, e := range b.Entries {
		if e.Slot == slot {
			return string(e.Value)
		}
	}
	return ""
}
func mustSave(t *testing.T, s *Store, p Profile, entries []Entry) {
	t.Helper()
	if _, err := s.Save(context.Background(), p, entries, SystemStore); err != nil {
		t.Fatal(err)
	}
}

func TestConstructionIsLazyAndRestartRestoresSeparateSlots(t *testing.T) {
	f := fakeSecrets()
	s := NewWithBackend(f)
	_ = New()
	if f.calls != 0 {
		t.Fatal("constructors performed secret I/O")
	}
	mustSave(t, s, profileA, cloudPair("fixture-access-material", "fixture-signing-material"))
	b, err := NewWithBackend(f).Load(context.Background(), profileA)
	if err != nil || valueOf(b, CloudAK) != "fixture-access-material" || valueOf(b, CloudSK) != "fixture-signing-material" {
		t.Fatalf("restart failed: %v", err)
	}
	if b.Snapshot.StorageState != Persisted {
		t.Fatal("restart did not report persisted")
	}
	for k, v := range f.values {
		if strings.Contains(string(v), "fixture-access-material") && strings.Contains(string(v), "fixture-signing-material") {
			t.Fatalf("secrets combined at %s", k)
		}
	}
}

func TestLimitIsBytesAndExplicitMemoryDoesNotTouchBackend(t *testing.T) {
	f := fakeSecrets()
	s := NewWithBackend(f)
	oversized := []Entry{{Slot: ApplicationJWT, Value: []byte(strings.Repeat("界", 854))}}
	if _, err := s.Save(context.Background(), profileA, oversized, SystemStore); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want size failure: %v", err)
	}
	if f.calls != 0 {
		t.Fatal("invalid value reached backend")
	}
	status, err := s.Save(context.Background(), profileA, oversized, SessionMemory)
	if err != nil || status.StorageState != MemoryOnly || f.calls != 0 {
		t.Fatalf("explicit memory failed: %v", err)
	}
	b, err := s.Load(context.Background(), profileA)
	if err != nil || valueOf(b, ApplicationJWT) != string(oversized[0].Value) || f.calls != 0 {
		t.Fatal("memory value was not retained")
	}
	if _, err := NewWithBackend(f).Load(context.Background(), profileA); !errors.Is(err, ErrNotFound) {
		t.Fatalf("memory persisted: %v", err)
	}
	mustSave(t, NewWithBackend(f), profileB, []Entry{{Slot: RuntimeGateway, Value: []byte(strings.Repeat("x", 2560))}})
}

func TestNativeErrorsAreSafeAndUnavailableDoesNotFallback(t *testing.T) {
	f := fakeSecrets()
	s := NewWithBackend(f)
	f.getHook = func(string) error { return errors.New("fixture-private-value in /secret/path") }
	status, err := s.Save(context.Background(), profileA, cloudPair("new-a", "new-s"), SystemStore)
	if err == nil || strings.Contains(err.Error(), "fixture-private-value") || status.StorageState == Persisted || status.StorageState == MemoryOnly {
		t.Fatal("unsafe or misleading storage failure")
	}
	f.getHook = func(string) error { return errors.ErrUnsupported }
	if _, err := s.Load(context.Background(), profileA); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unsupported classification: %v", err)
	}
}

func TestPartialWriteRollsBackOldPairAndMetadata(t *testing.T) {
	f := fakeSecrets()
	s := NewWithBackend(f)
	mustSave(t, s, profileA, cloudPair("old-a", "old-s"))
	f.setHook = func(key string, v []byte) error {
		if strings.HasSuffix(key, ".cloudSK") && string(v) == "new-s" {
			return errors.New("fixture-private-value")
		}
		return nil
	}
	status, err := s.Save(context.Background(), profileA, cloudPair("new-a", "new-s"), SystemStore)
	if err == nil || status.StorageState == Persisted {
		t.Fatal("partial write reported success")
	}
	b, err := NewWithBackend(f).Load(context.Background(), profileA)
	if err != nil || valueOf(b, CloudAK) != "old-a" || valueOf(b, CloudSK) != "old-s" {
		t.Fatalf("old pair not restored: %v", err)
	}
}

func TestFailedRollbackLeavesDurablePoisonMarker(t *testing.T) {
	f := fakeSecrets()
	s := NewWithBackend(f)
	mustSave(t, s, profileA, cloudPair("old-a", "old-s"))
	f.setHook = func(key string, v []byte) error {
		if strings.HasSuffix(key, ".cloudSK") && string(v) == "new-s" {
			return errors.New("failed write")
		}
		if strings.HasSuffix(key, ".cloudAK") && string(v) == "old-a" {
			return errors.New("failed rollback")
		}
		return nil
	}
	if _, err := s.Save(context.Background(), profileA, cloudPair("new-a", "new-s"), SystemStore); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("rollback failure not classified: %v", err)
	}
	if _, err := NewWithBackend(f).Load(context.Background(), profileA); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("torn pair loaded after restart: %v", err)
	}
	f.setHook = nil
	if err := NewWithBackend(f).Delete(context.Background(), profileA); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWithBackend(f).Load(context.Background(), profileA); !errors.Is(err, ErrNotFound) {
		t.Fatal("recovery delete incomplete")
	}
}

func TestEpochCancellationDuringWriteRestoresOldPair(t *testing.T) {
	f := fakeSecrets()
	s := NewWithBackend(f)
	mustSave(t, s, profileA, cloudPair("old-a", "old-s"))
	ctx, cancel := context.WithCancel(context.Background())
	f.setHook = func(key string, v []byte) error {
		if strings.HasSuffix(key, ".cloudAK") && string(v) == "new-a" {
			cancel()
		}
		return nil
	}
	if _, err := s.Save(ctx, profileA, cloudPair("new-a", "new-s"), SystemStore); !errors.Is(err, ErrCanceled) {
		t.Fatalf("canceled epoch accepted: %v", err)
	}
	b, err := NewWithBackend(f).Load(context.Background(), profileA)
	if err != nil || valueOf(b, CloudAK) != "old-a" || valueOf(b, CloudSK) != "old-s" {
		t.Fatalf("late store committed: %v", err)
	}
}

func TestMetadataAndJSONCannotExposeValues(t *testing.T) {
	f := fakeSecrets()
	s := NewWithBackend(f)
	mustSave(t, s, profileA, []Entry{{Slot: RuntimeGateway, Value: []byte("fixture-private-value"), ExpiresAt: time.Now().Add(time.Hour)}})
	status, err := s.Status(context.Background(), profileA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Load(context.Background(), profileA)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []any{status, b, b.Entries} {
		encoded, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "fixture-private-value") {
			t.Fatal("secret in marshaled return type")
		}
	}
	for key, v := range f.values {
		if strings.HasSuffix(key, ".metadata") && strings.Contains(string(v), "fixture-private-value") {
			t.Fatal("secret in metadata")
		}
	}
}

func TestDeleteAndScopeIsolation(t *testing.T) {
	f := fakeSecrets()
	s := NewWithBackend(f)
	mustSave(t, s, profileA, cloudPair("one-a", "one-s"))
	mustSave(t, s, profileB, cloudPair("two-a", "two-s"))
	sameIDOtherScope := Profile{ID: profileA.ID, Scope: "account/123/runtime/other"}
	mustSave(t, s, sameIDOtherScope, []Entry{{Slot: RuntimeGateway, Value: []byte("other-runtime")}})
	if err := s.Delete(context.Background(), profileA); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWithBackend(f).Load(context.Background(), profileA); !errors.Is(err, ErrNotFound) {
		t.Fatal("deleted profile loaded")
	}
	b, err := NewWithBackend(f).Load(context.Background(), profileB)
	if err != nil || valueOf(b, CloudSK) != "two-s" {
		t.Fatal("another profile deleted")
	}
	b, err = NewWithBackend(f).Load(context.Background(), sameIDOtherScope)
	if err != nil || valueOf(b, RuntimeGateway) != "other-runtime" {
		t.Fatal("another scope deleted")
	}
}

func TestInvalidProfileSlotPairAndCanceledLoadNeverTouchSecrets(t *testing.T) {
	f := fakeSecrets()
	s := NewWithBackend(f)
	for _, p := range []Profile{{ID: "../injection", Scope: "account/123"}, {ID: "ok", Scope: "https://host?token=private"}, {ID: "ok", Scope: "a\x00b"}, {ID: "", Scope: "a"}} {
		if _, err := s.Save(context.Background(), p, cloudPair("a", "s"), SystemStore); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid profile accepted")
		}
	}
	for _, entries := range [][]Entry{{{Slot: Slot("../other"), Value: []byte("v")}}, {{Slot: CloudAK, Value: []byte("a")}}, {{Slot: ApplicationJWT, Value: []byte("a")}, {Slot: ApplicationJWT, Value: []byte("b")}}} {
		if _, err := s.Save(context.Background(), profileA, entries, SystemStore); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid slot/partial pair accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Load(ctx, profileA); !errors.Is(err, ErrCanceled) {
		t.Fatal("canceled load accepted")
	}
	if f.calls != 0 {
		t.Fatal("invalid operation accessed secrets")
	}
}

func TestMaximumScopeAndAllSlotsHaveBoundedMetadata(t *testing.T) {
	f := fakeSecrets()
	s := NewWithBackend(f)
	p := Profile{ID: strings.Repeat("p", 96), Scope: strings.Repeat("s", 512)}
	entries := make([]Entry, 0, len(allSlots))
	for _, slot := range allSlots {
		entries = append(entries, Entry{Slot: slot, Value: []byte("fixture-" + string(slot))})
	}
	mustSave(t, s, p, entries)
	b, err := NewWithBackend(f).Load(context.Background(), p)
	if err != nil || len(b.Entries) != len(allSlots) {
		t.Fatalf("maximum profile failed: %v", err)
	}
	for _, value := range f.values {
		if len(value) > MaxSecretSize {
			t.Fatal("a native value exceeded the shared maximum")
		}
	}
}

func TestReplacingCloudPairInMemoryClearsPriorSTS(t *testing.T) {
	f := fakeSecrets()
	s := NewWithBackend(f)
	entries := append(cloudPair("first-a", "first-s"), Entry{Slot: SecurityToken, Value: []byte("previous-sts")})
	if _, err := s.Save(context.Background(), profileA, entries, SessionMemory); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(context.Background(), profileA, cloudPair("second-a", "second-s"), SessionMemory); err != nil {
		t.Fatal(err)
	}
	b, err := s.Load(context.Background(), profileA)
	if err != nil || valueOf(b, SecurityToken) != "" {
		t.Fatal("replacement reused an old STS token")
	}
	if f.calls != 0 {
		t.Fatal("session-memory performed native I/O")
	}
}

func TestLateNativeReadDoesNotActivateCanceledEpoch(t *testing.T) {
	f := fakeSecrets()
	mustSave(t, NewWithBackend(f), profileA, cloudPair("saved-a", "saved-s"))
	s := NewWithBackend(f)
	ctx, cancel := context.WithCancel(context.Background())
	f.getHook = func(key string) error {
		if strings.HasSuffix(key, ".cloudAK") {
			cancel()
		}
		return nil
	}
	if b, err := s.Load(ctx, profileA); !errors.Is(err, ErrCanceled) || len(b.Entries) != 0 {
		t.Fatal("canceled epoch exposed a bundle")
	}
	f.getHook = nil
	if b, err := s.Load(context.Background(), profileA); err != nil || valueOf(b, CloudAK) != "saved-a" {
		t.Fatal("subsequent current read failed")
	}
}

func TestCannotSaveSTSWithoutUsableCloudPair(t *testing.T) {
	f := fakeSecrets()
	s := NewWithBackend(f)
	entries := append(cloudPair("", ""), Entry{Slot: SecurityToken, Value: []byte("orphan-sts")})
	for _, mode := range []Mode{SystemStore, SessionMemory} {
		if _, err := s.Save(context.Background(), profileA, entries, mode); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("orphan STS accepted")
		}
	}
	if f.calls != 0 {
		t.Fatal("invalid STS accessed the native store")
	}
}

func TestEveryTransactionFailureRestoresOldValues(t *testing.T) {
	for _, target := range []string{".pending", ".cloudAK", ".cloudSK", ".securityToken", ".runtimeGateway", ".metadata", ".pending-delete"} {
		t.Run(target, func(t *testing.T) {
			f := fakeSecrets()
			s := NewWithBackend(f)
			old := append(cloudPair("old-a", "old-s"), Entry{Slot: SecurityToken, Value: []byte("old-sts")})
			mustSave(t, s, profileA, old)
			failed := false
			f.setHook = func(key string, value []byte) error {
				if !failed && target != ".pending-delete" && strings.HasSuffix(key, target) {
					failed = true
					f.values[key] = append([]byte(nil), value...) // applied despite error
					return errors.New("fixture-private-native-failure")
				}
				return nil
			}
			f.deleteHook = func(key string) error {
				wanted := (target == ".runtimeGateway" || target == ".securityToken") && strings.HasSuffix(key, target) || target == ".pending-delete" && strings.HasSuffix(key, ".pending")
				if !failed && wanted {
					failed = true
					delete(f.values, key)
					return errors.New("fixture-private-native-failure")
				}
				return nil
			}
			status, err := s.Save(context.Background(), profileA, cloudPair("new-a", "new-s"), SystemStore)
			if !failed || err == nil || status.StorageState == Persisted {
				t.Fatal("failed native transaction accepted")
			}
			b, err := NewWithBackend(f).Load(context.Background(), profileA)
			if err != nil || valueOf(b, CloudAK) != "old-a" || valueOf(b, CloudSK) != "old-s" || valueOf(b, SecurityToken) != "old-sts" {
				t.Fatalf("old credentials not restored: %v", err)
			}
		})
	}
}

func TestCorruptMetadataAndDeletionFailuresFailClosed(t *testing.T) {
	f := fakeSecrets()
	mustSave(t, NewWithBackend(f), profileA, cloudPair("old-a", "old-s"))
	for key := range f.values {
		if strings.HasSuffix(key, ".metadata") {
			f.values[key] = []byte(`{"profileId":"credential-profile-a","scope":"account/123/profile/a","slots":[],"secret":"fixture-private-value"}`)
		}
	}
	if _, err := NewWithBackend(f).Load(context.Background(), profileA); !errors.Is(err, ErrRecoveryRequired) || strings.Contains(err.Error(), "fixture-private-value") {
		t.Fatal("malicious metadata accepted or leaked")
	}
	f.deleteHook = func(key string) error {
		if strings.HasSuffix(key, ".cloudSK") {
			return errors.New("fixture-private-value")
		}
		return nil
	}
	if err := NewWithBackend(f).Delete(context.Background(), profileA); err == nil || strings.Contains(err.Error(), "fixture-private-value") {
		t.Fatal("delete failed unsafely")
	}
	if _, err := NewWithBackend(f).Load(context.Background(), profileA); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("partial deletion restored login")
	}
	f.deleteHook = nil
	if err := NewWithBackend(f).Delete(context.Background(), profileA); err != nil {
		t.Fatal(err)
	}
}

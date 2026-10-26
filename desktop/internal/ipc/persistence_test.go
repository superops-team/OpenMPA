package ipc

import (
	"context"
	"encoding/json"
	"github.com/superops-team/OpenMPA/desktop/internal/cloud"
	"github.com/superops-team/OpenMPA/desktop/internal/connection"
	"github.com/superops-team/OpenMPA/desktop/internal/credentials"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type secretsFixture struct{ values map[string][]byte }

func (f *secretsFixture) Get(k string) ([]byte, error) {
	v, ok := f.values[k]
	if !ok {
		return nil, credentials.ErrNotFound
	}
	return v, nil
}
func (f *secretsFixture) Set(k string, v []byte) error {
	f.values[k] = append([]byte(nil), v...)
	return nil
}
func (f *secretsFixture) Delete(k string) error { delete(f.values, k); return nil }
func TestSavedLoginReferencesExcludeSecretsAndSignOutKeepsOtherProfiles(t *testing.T) {
	dir := t.TempDir()
	filename := dir + "/saved-logins.json"
	backend := &secretsFixture{map[string][]byte{}}
	store := credentials.NewWithBackend(backend)
	p := newLoginPersistence(store, func() (string, error) { return filename, nil })
	ctx := context.Background()
	state := dto.PlatformState{SignedIn: true, Principal: &dto.Principal{AccountID: "1", Scope: "1/User/one"}}
	r := dto.SignInRequest{AccessKey: "SECRET_AK_ONE", SecretKey: "SECRET_SK_ONE", Region: "cn-beijing", Remember: true}
	if e := p.signIn(ctx, state, r); e != nil {
		t.Fatal(e)
	}
	first := p.activeID
	state.Principal = &dto.Principal{AccountID: "2", Scope: "2/User/two"}
	r.AccessKey = "SECRET_AK_TWO"
	r.SecretKey = "SECRET_SK_TWO"
	if e := p.signIn(ctx, state, r); e != nil {
		t.Fatal(e)
	}
	data, readError := os.ReadFile(filename)
	if readError != nil || strings.Contains(string(data), "SECRET") {
		t.Fatal("secret entered refs")
	}
	if e := p.signOut(ctx); e != nil {
		t.Fatal(e)
	}
	list, e := p.list()
	if e != nil || len(list.Profiles) != 1 || list.Profiles[0].ProfileID != first {
		t.Fatalf("wrong profiles %+v %v", list, e)
	}
	again := newLoginPersistence(credentials.NewWithBackend(backend), func() (string, error) { return filename, nil })
	record, e := again.lookup(first)
	if e != nil {
		t.Fatal(e)
	}
	bundle, err := again.store.Load(ctx, record.Profile)
	if err != nil || slotValue(bundle, credentials.CloudAK) != "SECRET_AK_ONE" {
		t.Fatal("other credential lost")
	}
}
func TestRuntimeCredentialScopesNeverMixAfterRestart(t *testing.T) {
	dir := t.TempDir()
	backend := &secretsFixture{map[string][]byte{}}
	p := newLoginPersistence(credentials.NewWithBackend(backend), func() (string, error) { return dir + "/logins.json", nil })
	state := dto.PlatformState{Principal: &dto.Principal{AccountID: "1", Scope: "1/User/one"}}
	if e := p.signIn(context.Background(), state, dto.SignInRequest{Remember: true, AccessKey: "AK", SecretKey: "SK", Region: "cn-beijing"}); e != nil {
		t.Fatal(e)
	}
	id := p.activeID
	one := dto.RuntimeRequest{Region: "cn-beijing", ProjectName: "default", RuntimeID: "one"}
	two := dto.RuntimeRequest{Region: "cn-beijing", ProjectName: "default", RuntimeID: "two"}
	p.bind(one)
	_, e := p.saveAuth(context.Background(), one, &dto.IdentityRequest{AuthMode: "legacyJwt", LegacyJWT: "SECRET_ONE"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	p.bind(two)
	_, e = p.saveAuth(context.Background(), two, &dto.IdentityRequest{AuthMode: "legacyJwt", LegacyJWT: "SECRET_TWO"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	again := newLoginPersistence(credentials.NewWithBackend(backend), func() (string, error) { return dir + "/logins.json", nil })
	record, e := again.lookup(id)
	if e != nil {
		t.Fatal(e)
	}
	for _, b := range record.Bindings {
		bundle, err := again.store.Load(context.Background(), b.Profile)
		if err != nil {
			t.Fatal(err)
		}
		expected := "SECRET_ONE"
		if b.Runtime.RuntimeID == "two" {
			expected = "SECRET_TWO"
		}
		if slotValue(bundle, credentials.ApplicationJWT) != expected {
			t.Fatal("Runtime slots crossed")
		}
	}
}
func TestConstructionDoesNotResolveSavedProfilePathOrReadSecrets(t *testing.T) {
	backend := &secretsFixture{map[string][]byte{}}
	resolved := false
	_ = newLoginPersistence(credentials.NewWithBackend(backend), func() (string, error) { resolved = true; return "ignored", nil })
	if resolved || len(backend.values) > 0 {
		t.Fatal("constructor did I/O")
	}
}

type cloudFixtureTransport struct{ requests int }

func (f *cloudFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.requests++
	body := `{"Result":{"AccountId":"1","IdentityType":"User","UserId":"one"}}`
	if r.URL.Query().Get("Action") == "GetRuntime" {
		body = `{"Result":{"RuntimeId":"r1","ProjectName":"default","Endpoint":"https://runtime.invalid/root","AuthorizerConfiguration":{"KeyAuth":{"ApiKey":"SECRET_GATEWAY"}},"CurrentVersionNumber":7}}`
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
}
func TestRestoreRevalidatesIdentityAndRestoresOnlySelectedRuntimeSlots(t *testing.T) {
	dir := t.TempDir()
	backend := &secretsFixture{map[string][]byte{}}
	transport := &cloudFixtureTransport{}
	path := func() (string, error) { return dir + "/saved.json", nil }
	conn := connection.New()
	p := NewPlatformWithPersistence(cloud.NewWithTransport(transport), conn, credentials.NewWithBackend(backend), path)
	out := p.SignIn(context.Background(), dto.SignInRequest{AccessKey: "AK", SecretKey: "SK", Region: "cn-beijing", Remember: true})
	if !out.OK {
		t.Fatal(out.Error)
	}
	id := out.Value.ProfileID
	bind := p.BindRuntime(context.Background(), dto.RuntimeRequest{Region: "cn-beijing", ProjectName: "default", RuntimeID: "r1"})
	if !bind.OK {
		t.Fatal(bind.Error)
	}
	facade := Connection{Client: conn, Platform: p}
	identity := facade.SetBusinessIdentity(context.Background(), dto.IdentityRequest{ConnectionEpoch: bind.Value.ConnectionEpoch, AuthMode: "legacyJwt", LegacyJWT: "SECRET_APP"})
	if !identity.OK {
		t.Fatal(identity.Error)
	}
	channel := facade.ConfigureChannelAuth(context.Background(), dto.ChannelAuthRequest{ConnectionEpoch: identity.Value.ConnectionEpoch, AuthMode: "runtimeKey", ChannelKey: "SECRET_CHANNEL"})
	if !channel.OK {
		t.Fatal(channel.Error)
	}
	scope := conn.State().Scope
	newConn := connection.New()
	again := NewPlatformWithPersistence(cloud.NewWithTransport(transport), newConn, credentials.NewWithBackend(backend), path)
	restored := again.Restore(context.Background(), dto.RestoreRequest{ProfileID: id})
	if !restored.OK || !restored.Value.Remembered || !newConn.State().HasBusinessIdentity || scope != newConn.State().Scope || transport.requests != 4 {
		t.Fatalf("restore %+v conn=%+v requests=%d", restored, newConn.State(), transport.requests)
	}
	wire, _ := json.Marshal(restored)
	if strings.Contains(string(wire), "SECRET") {
		t.Fatal("restored secret in IPC")
	}
}

package ipc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/superops-team/OpenMPA/desktop/internal/credentials"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type savedBinding struct {
	Runtime       dto.RuntimeRequest  `json:"runtime"`
	Profile       credentials.Profile `json:"credentialProfile"`
	BusinessMode  string              `json:"businessMode"`
	TestSubject   string              `json:"testSubject,omitempty"`
	VerifiedTest  bool                `json:"verifiedTestDeployment"`
	ChannelMode   string              `json:"channelMode"`
	BusinessScope string              `json:"businessScope"`
}
type savedLogin struct {
	Profile         credentials.Profile `json:"credentialProfile"`
	Summary         dto.SavedProfile    `json:"summary"`
	ExpiresAt       string              `json:"expiresAt,omitempty"`
	Bindings        []savedBinding      `json:"bindings"`
	SelectedRuntime string              `json:"selectedRuntime,omitempty"`
}
type loginRegistry struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Profiles      map[string]savedLogin `json:"profiles"`
}
type loginPersistence struct {
	mu         sync.Mutex
	store      *credentials.Store
	path       func() (string, error)
	filename   string
	records    map[string]savedLogin
	activeID   string
	remembered bool
}

func newLoginPersistence(s *credentials.Store, path func() (string, error)) *loginPersistence {
	return &loginPersistence{store: s, path: path}
}
func secretFailure(err error) *dto.SafeError {
	code := "credentialStoreFailed"
	switch {
	case errors.Is(err, credentials.ErrTooLarge):
		code = "credentialTooLarge"
	case errors.Is(err, credentials.ErrNotFound):
		code = "credentialMissing"
	case errors.Is(err, credentials.ErrUnavailable):
		code = "credentialStoreUnavailable"
	case errors.Is(err, credentials.ErrRecoveryRequired):
		code = "credentialRecoveryRequired"
	case errors.Is(err, credentials.ErrCanceled):
		code = "staleConnection"
	}
	return dto.Failure(code, "transport", "never", 0)
}
func profileID(scope string) string {
	digest := sha256.Sum256([]byte(scope))
	return "saved-" + hex.EncodeToString(digest[:16])
}
func (p *loginPersistence) loadLocked() error {
	if p.records != nil {
		return nil
	}
	filename, e := p.path()
	if e != nil {
		return e
	}
	p.filename = filename
	records := map[string]savedLogin{}
	info, e := os.Lstat(filename)
	if os.IsNotExist(e) {
		p.records = records
		return nil
	}
	if e != nil {
		return e
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 1<<20 {
		return errors.New("invalid login registry")
	}
	b, e := os.ReadFile(filename)
	if e != nil {
		return e
	}
	var registry loginRegistry
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&registry) != nil || decoder.Decode(new(interface{})) != io.EOF || registry.SchemaVersion != 1 || len(registry.Profiles) > 100 {
		return errors.New("invalid login registry")
	}
	for id, r := range registry.Profiles {
		if id != r.Profile.ID || r.Profile.Scope != r.Summary.Scope || !validSaved(r) {
			return errors.New("invalid profile reference")
		}
	}
	p.records = registry.Profiles
	return nil
}
func validSaved(r savedLogin) bool {
	if r.Profile.ID == "" || r.Profile.Scope == "" || !strings.HasPrefix(r.Profile.ID, "saved-") || len(r.Profile.Scope) > 512 || strings.ContainsAny(r.Profile.Scope, "\r\n?=&") || len(r.Bindings) > 100 {
		return false
	}
	for _, binding := range r.Bindings {
		if binding.Runtime.RuntimeID == "" || binding.Profile.Scope == "" || binding.Profile.ID == "" || !strings.HasPrefix(binding.Profile.Scope, r.Profile.Scope+"/") {
			return false
		}
		if binding.BusinessMode != "" && binding.BusinessMode != "none" && binding.BusinessMode != "explicitTest" && binding.BusinessMode != "legacyJwt" {
			return false
		}
		if binding.ChannelMode != "" && binding.ChannelMode != "none" && binding.ChannelMode != "runtimeKey" && binding.ChannelMode != "legacyJwt" {
			return false
		}
	}
	return true
}
func (p *loginPersistence) writeLocked() error {
	if e := os.MkdirAll(filepath.Dir(p.filename), 0700); e != nil {
		return e
	}
	b, e := json.Marshal(loginRegistry{SchemaVersion: 1, Profiles: p.records})
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(filepath.Dir(p.filename), ".saved-logins-")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(b)
	}
	if e == nil {
		e = tmp.Sync()
	}
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(tmp.Name(), p.filename)
	}
	return e
}
func (p *loginPersistence) signIn(ctx context.Context, state dto.PlatformState, r dto.SignInRequest) *dto.SafeError {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.activeID = ""
	p.remembered = false
	if !r.Remember {
		return nil
	}
	if state.Principal == nil {
		return dto.Failure("authRequired", "platform", "never", 0)
	}
	if e := p.loadLocked(); e != nil {
		return dto.Failure("savedProfileUnavailable", "transport", "never", 0)
	}
	principal := state.Principal
	id := profileID(principal.Scope)
	profile := credentials.Profile{ID: id, Scope: principal.Scope}
	expiry := time.Time{}
	if r.ExpiresAt != "" {
		expiry, _ = time.Parse(time.RFC3339, r.ExpiresAt)
	}
	_, e := p.store.Save(ctx, profile, []credentials.Entry{{Slot: credentials.CloudAK, Value: []byte(r.AccessKey), ExpiresAt: expiry}, {Slot: credentials.CloudSK, Value: []byte(r.SecretKey), ExpiresAt: expiry}, {Slot: credentials.SecurityToken, Value: []byte(r.SecurityToken), ExpiresAt: expiry}}, credentials.SystemStore)
	if e != nil {
		return secretFailure(e)
	}
	record := p.records[id]
	record.Profile = profile
	record.Summary = dto.SavedProfile{ProfileID: id, Label: principal.AccountID + " · " + principal.IdentityType, AccountID: principal.AccountID, Scope: principal.Scope, Region: r.Region, Remembered: true}
	record.ExpiresAt = r.ExpiresAt
	p.records[id] = record
	if e = p.writeLocked(); e != nil {
		return dto.Failure("savedProfileUnavailable", "transport", "never", 0)
	}
	p.activeID = id
	p.remembered = true
	return nil
}
func (p *loginPersistence) decorate(state dto.PlatformState) dto.PlatformState {
	p.mu.Lock()
	defer p.mu.Unlock()
	state.ProfileID = p.activeID
	state.Remembered = p.remembered
	return state
}
func (p *loginPersistence) list() (dto.SavedProfiles, *dto.SafeError) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := dto.SavedProfiles{Profiles: []dto.SavedProfile{}}
	if e := p.loadLocked(); e != nil {
		return out, dto.Failure("savedProfileUnavailable", "transport", "never", 0)
	}
	for _, record := range p.records {
		out.Profiles = append(out.Profiles, record.Summary)
	}
	sort.Slice(out.Profiles, func(i, j int) bool { return out.Profiles[i].ProfileID < out.Profiles[j].ProfileID })
	return out, nil
}
func (p *loginPersistence) lookup(id string) (savedLogin, *dto.SafeError) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e := p.loadLocked(); e != nil {
		return savedLogin{}, dto.Failure("savedProfileUnavailable", "transport", "never", 0)
	}
	r, ok := p.records[id]
	if !ok {
		return savedLogin{}, dto.Failure("savedProfileMissing", "transport", "never", 0)
	}
	return r, nil
}
func (p *loginPersistence) activate(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.activeID = id
	p.remembered = true
}
func (p *loginPersistence) signOut(ctx context.Context) *dto.SafeError {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.activeID == "" {
		p.remembered = false
		return nil
	}
	record := p.records[p.activeID]
	if e := p.store.Delete(ctx, record.Profile); e != nil {
		return secretFailure(e)
	}
	for _, binding := range record.Bindings {
		if e := p.store.Delete(ctx, binding.Profile); e != nil {
			return secretFailure(e)
		}
	}
	delete(p.records, p.activeID)
	if e := p.writeLocked(); e != nil {
		return dto.Failure("savedProfileUnavailable", "transport", "never", 0)
	}
	p.activeID = ""
	p.remembered = false
	return nil
}
func slotValue(b credentials.Bundle, slot credentials.Slot) string {
	for _, entry := range b.Entries {
		if entry.Slot == slot {
			return string(entry.Value)
		}
	}
	return ""
}
func (p *loginPersistence) bind(runtime dto.RuntimeRequest) *dto.SafeError {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.remembered {
		return nil
	}
	r := p.records[p.activeID]
	scope := r.Profile.Scope + "/" + runtime.Region + "/" + runtime.ProjectName + "/" + runtime.RuntimeID
	found := false
	for _, b := range r.Bindings {
		if b.Profile.Scope == scope {
			found = true
		}
	}
	if !found {
		r.Bindings = append(r.Bindings, savedBinding{Runtime: runtime, Profile: credentials.Profile{ID: profileID(scope), Scope: scope}, BusinessScope: "/business/profile/" + profileID(scope)})
	}
	r.SelectedRuntime = runtime.RuntimeID
	r.Summary.RuntimeID = runtime.RuntimeID
	r.Summary.ProjectName = runtime.ProjectName
	r.Summary.Region = runtime.Region
	p.records[p.activeID] = r
	if e := p.writeLocked(); e != nil {
		return dto.Failure("savedProfileUnavailable", "transport", "never", 0)
	}
	return nil
}
func (p *loginPersistence) saveAuth(ctx context.Context, runtime dto.RuntimeRequest, identity *dto.IdentityRequest, channel *dto.ChannelAuthRequest) (string, *dto.SafeError) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.remembered {
		return "", nil
	}
	record := p.records[p.activeID]
	for index, binding := range record.Bindings {
		if binding.Runtime != runtime {
			continue
		}
		entries := []credentials.Entry{}
		if identity != nil {
			binding.BusinessMode = identity.AuthMode
			if identity.AuthMode == "legacyJwt" {
				binding.BusinessScope = "/business/profile/" + randomProfileID()
			}
			binding.TestSubject = identity.TestSubject
			binding.VerifiedTest = identity.VerifiedTestDeployment
			entries = append(entries, credentials.Entry{Slot: credentials.ApplicationJWT, Value: []byte(identity.LegacyJWT)}, credentials.Entry{Slot: credentials.TaskJWT, Value: []byte(identity.LegacyJWT)})
		}
		if channel != nil {
			binding.ChannelMode = channel.AuthMode
			entries = append(entries, credentials.Entry{Slot: credentials.ChannelManagement, Value: []byte(channel.ChannelKey)}, credentials.Entry{Slot: credentials.ChannelManagementJWT, Value: []byte(channel.LegacyJWT)})
		}
		if _, e := p.store.Save(ctx, binding.Profile, entries, credentials.SystemStore); e != nil {
			return "", secretFailure(e)
		}
		record.Bindings[index] = binding
		p.records[p.activeID] = record
		if e := p.writeLocked(); e != nil {
			return "", dto.Failure("savedProfileUnavailable", "transport", "never", 0)
		}
		return binding.BusinessScope, nil
	}
	return "", dto.Failure("savedProfileMissing", "transport", "never", 0)
}

func randomProfileID() string { b := make([]byte, 16); rand.Read(b); return hex.EncodeToString(b) }

// Package localization owns OpenMPA's display-language preference.
package localization

import (
	"context"
	"github.com/egoist/mygo"
	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
	"github.com/superops-team/OpenMPA/desktop/internal/dto"
	"github.com/superops-team/OpenMPA/desktop/locales"
	"sync"
	"sync/atomic"
	"time"
)

var current atomic.Pointer[string]

func init() { SetCurrentLocale(Chinese) }

// SetCurrentLocale mirrors the language MyGo resolved for native product
// strings without requiring a running App event loop to render or test them.
func SetCurrentLocale(locale string) {
	if locale != English && locale != Chinese {
		locale = Chinese
	}
	value := locale
	current.Store(&value)
}

func CurrentLocale() string { return *current.Load() }

const (
	System  = "system"
	Chinese = "zh-Hans"
	English = "en"
)

type State struct {
	Mode   string `json:"mode"`
	Locale string `json:"locale"`
}
type LanguageRequest struct {
	Mode string `json:"mode"`
}
type LanguageResult struct {
	OK    bool           `json:"ok"`
	Value *State         `json:"value,omitempty"`
	Error *dto.SafeError `json:"error,omitempty"`
}
type Service struct {
	mu        sync.Mutex
	store     *configstore.Store
	setLocale func(string)
	locale    func() string
	mode      string
}

func New(store *configstore.Store) *Service {
	return newWithDependencies(store, mygo.App.SetLocale, mygo.App.Locale)
}
func newWithDependencies(store *configstore.Store, setLocale func(string), locale func() string) *Service {
	return &Service{store: store, setLocale: setLocale, locale: locale, mode: System}
}
func valid(mode string) bool { return mode == System || mode == Chinese || mode == English }
func tag(mode string) string {
	if mode == System {
		return ""
	}
	return mode
}

// Load applies the persisted preference during trusted app assembly. It is a
// package helper, not an IPC method.
func Load(s *Service) *dto.SafeError { return s.load() }
func (s *Service) load() *dto.SafeError {
	s.mu.Lock()
	defer s.mu.Unlock()
	mode := System
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	snapshot, err := s.store.Snapshot(ctx)
	if err == nil && valid(snapshot.Language) {
		mode = snapshot.Language
	}
	s.mode = mode
	s.setLocale(tag(mode))
	SetCurrentLocale(s.locale())
	if err != nil {
		return dto.Failure("displayLanguageUnavailable", "desktop", "never", 0)
	}
	return nil
}

func (s *Service) Status() LanguageResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	value := State{Mode: s.mode, Locale: CurrentLocale()}
	return LanguageResult{OK: true, Value: &value}
}
func (s *Service) Set(request LanguageRequest) LanguageResult {
	if !valid(request.Mode) {
		return LanguageResult{Error: dto.Failure("invalidInput", "desktop", "never", 0)}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := s.store.SetLanguage(ctx, request.Mode)
	if err != nil {
		return LanguageResult{Error: dto.Failure("displayLanguageUnavailable", "desktop", "never", 0)}
	}
	s.mode = request.Mode
	s.setLocale(tag(request.Mode))
	SetCurrentLocale(s.locale())
	value := State{Mode: s.mode, Locale: CurrentLocale()}
	return LanguageResult{OK: true, Value: &value}
}

// Text formats a catalog key in the current app locale.
func Text(key string, args map[string]any) string {
	return locales.Catalog.Format(CurrentLocale(), key, args)
}

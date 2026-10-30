package localization

import (
	"github.com/superops-team/OpenMPA/desktop/internal/configstore"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *configstore.Store {
	t.Helper()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "openmpa.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestMissingPreferenceFollowsSystem(t *testing.T) {
	var applied string
	service := newWithDependencies(testStore(t), func(tag string) { applied = tag }, func() string { return "en" })
	if err := Load(service); err != nil {
		t.Fatal(err)
	}
	state := service.Status().Value
	if state.Mode != System || state.Locale != "en" || applied != "" {
		t.Fatalf("unexpected default: %+v applied=%q", state, applied)
	}
}

func TestExplicitLanguagePersistsAndSystemClearsOverride(t *testing.T) {
	store := testStore(t)
	current := "zh-Hans"
	newService := func() *Service {
		return newWithDependencies(store, func(tag string) {
			if tag == "" {
				current = "en"
			} else {
				current = tag
			}
		}, func() string { return current })
	}
	service := newService()
	if result := service.Set(LanguageRequest{Mode: English}); !result.OK || result.Value.Locale != English {
		t.Fatal(result)
	}
	restored := newService()
	if err := Load(restored); err != nil || restored.Status().Value.Mode != English {
		t.Fatal(err, restored.Status())
	}
	if result := restored.Set(LanguageRequest{Mode: System}); !result.OK || result.Value.Mode != System || result.Value.Locale != English {
		t.Fatal(result)
	}
}

func TestInvalidPreferenceAndRequestFailSafely(t *testing.T) {
	var applied string
	service := newWithDependencies(testStore(t), func(tag string) { applied = tag }, func() string { return "zh-Hans" })
	if err := Load(service); err != nil || service.Status().Value.Mode != System || applied != "" {
		t.Fatal(err, service.Status(), applied)
	}
	if result := service.Set(LanguageRequest{Mode: "fr"}); result.OK || result.Error == nil || result.Error.Code != "invalidInput" {
		t.Fatal(result)
	}
}

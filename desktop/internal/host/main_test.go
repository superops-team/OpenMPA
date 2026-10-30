package host

import (
	"github.com/superops-team/OpenMPA/desktop/internal/localization"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("MYGO_LOCALE", "en")
	localization.SetCurrentLocale(localization.English)
	os.Exit(m.Run())
}

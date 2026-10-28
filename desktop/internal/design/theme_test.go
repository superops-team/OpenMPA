package design

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestThemeHonorsAccessibilityAndDoesNotShareMutableValues(t *testing.T) {
	light := Theme(false, ui.Preferences{})
	dark := Theme(true, ui.Preferences{HighContrast: true, TextScale: 2})
	if dark.Border != Colors(true).ForegroundSubtle || dark.TextMuted != Colors(true).Foreground {
		t.Fatal("high contrast preference ignored")
	}
	if dark.FontSize != TypographyUiBase*2 || light.FontSize != TypographyUiBase {
		t.Fatal("text scaling leaks into other themes")
	}
	dark.Accent = ui.Transparent
	if Theme(true, ui.Preferences{}).Accent != Colors(true).Primary {
		t.Fatal("theme values share mutable state")
	}
}

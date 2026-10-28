package host

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestShellRendersAuthoritativeThemeColors(t *testing.T) {
	data, err := os.ReadFile("../../../design/react-desktop-v2.tokens.json")
	if err != nil {
		t.Fatal(err)
	}
	var source struct {
		Theme map[string]map[string]string `json:"theme"`
	}
	if err := json.Unmarshal(data, &source); err != nil {
		t.Fatal(err)
	}
	rail := &nativeRail{state: ShellState{View: "settings"}}
	tt := ui.NewTester(rail.view, 760, 700)
	for _, mode := range []string{"light", "dark", "light"} {
		rail.state.Theme = mode
		tt.Frame()
		img := tt.Image()
		for _, point := range []struct {
			x, y int
			role string
		}{
			{1, 400, "sidebar"}, {50, 400, "sidebar"}, {500, 2, "header"},
		} {
			want := ui.Hex(source.Theme[mode][point.role])
			r, g, b, a := img.At(point.x, point.y).RGBA()
			if uint8(r>>8) != want.R || uint8(g>>8) != want.G || uint8(b>>8) != want.B || uint8(a>>8) != want.A {
				t.Fatalf("%s %s pixel (%d,%d) = %d,%d,%d; want %v", mode, point.role, point.x, point.y, r>>8, g>>8, b>>8, want)
			}
		}
	}
}

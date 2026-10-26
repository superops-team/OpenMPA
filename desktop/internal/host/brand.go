package host

import (
	_ "embed"

	"github.com/egoist/mygo/ui"
)

//go:embed assets/brand/mark.svg
var nativeBrandSource []byte

var nativeBrand = ui.MustParseSVG(nativeBrandSource)

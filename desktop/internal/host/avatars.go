package host

import (
	"embed"
	"github.com/egoist/mygo/ui"
)

// The 96px thumbnails derive from the existing public/dots characters.
//
//go:embed assets/dots/*.png
var avatarAssets embed.FS
var nativeAvatars = func() []*ui.Bitmap {
	var images []*ui.Bitmap
	for _, color := range []string{"blue", "mint", "orange", "purple"} {
		data, err := avatarAssets.ReadFile("assets/dots/" + color + ".png")
		if err != nil {
			panic(err)
		}
		bitmap, err := ui.DecodeBitmap(data)
		if err != nil {
			panic(err)
		}
		images = append(images, bitmap)
	}
	return images
}()

func avatarFor(id string) *ui.Bitmap {
	var hash uint32
	for _, character := range id {
		hash = hash*31 + uint32(character)
	}
	return nativeAvatars[hash%uint32(len(nativeAvatars))]
}

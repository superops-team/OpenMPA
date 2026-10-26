package artifact

import (
	"context"
	"errors"

	"github.com/egoist/mygo"
)

type SaveOptions struct {
	Title         string
	SuggestedName string
	Markdown      bool
}
type Dialog interface {
	Save(context.Context, *mygo.Window, SaveOptions) (string, error)
	ConfirmOverwrite(context.Context, *mygo.Window) (bool, error)
}

// NativeDialog always parents dialogs to the calling window. An arbitrary Go
// context cannot create a file-selection grant on behalf of a renderer window.
type NativeDialog struct{}

func (NativeDialog) Save(ctx context.Context, parent *mygo.Window, options SaveOptions) (string, error) {
	if parent == nil || ctx.Err() != nil {
		return "", errors.New("native save unavailable")
	}
	opts := mygo.SaveDialogOptions{Parent: parent, Title: options.Title, DefaultPath: options.SuggestedName, ButtonLabel: "Save", CreateDirectories: true}
	if options.Markdown {
		opts.Filters = []mygo.FileFilter{{Name: "Markdown", Extensions: []string{"md"}}}
	}
	return mygo.Dialog.Save(opts)
}
func (NativeDialog) ConfirmOverwrite(ctx context.Context, parent *mygo.Window) (bool, error) {
	if parent == nil || ctx.Err() != nil {
		return false, errors.New("native confirmation unavailable")
	}
	r, e := mygo.Dialog.Message(mygo.MessageOptions{Parent: parent, Type: mygo.MessageWarning, Title: "Replace existing file?", Message: "Replace the selected file?", Detail: "The original file will be replaced only after the new file has been saved completely.", Buttons: []string{"Cancel", "Replace"}, DefaultButton: 0, CancelButton: 0})
	return e == nil && r.Button == 1, e
}

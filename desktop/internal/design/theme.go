package design

import "github.com/egoist/mygo/ui"

func Theme(dark bool, preferences ui.Preferences) *ui.Theme {
	c := Colors(dark)
	t := &ui.Theme{
		Dark: dark, Background: c.Background, Surface: c.Input,
		SurfaceHover: c.SurfaceHover, SurfacePressed: c.SurfacePressed,
		Border: c.Border, Text: c.Foreground, TextMuted: c.ForegroundSubtle,
		Accent: c.Primary, AccentHover: c.PrimaryHover, AccentPressed: c.PrimaryPressed,
		AccentText: c.PrimaryForeground, Danger: c.Destructive, Warning: c.Warning,
		Success: c.Success, Selection: c.Selection, Focus: c.Focus,
		Inverse: c.Tooltip, InverseText: c.TooltipForeground, Scrollbar: c.Scrollbar,
		Radius: RadiusMd, Spacing: Space1, FontSize: TypographyUiBase,
	}
	if preferences.HighContrast {
		t.Border = c.ForegroundSubtle
		t.TextMuted = c.Foreground
	}
	if preferences.TextScale > 0 {
		t.FontSize *= preferences.TextScale
	}
	return t
}

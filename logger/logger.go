package logger

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

const (
	neutral90 = "#dfdae1"
	neutral60 = "#94879b"
	purple70  = "#c289e6"
	blue70    = "#71b8ef"
	green70   = "#77cf8f"
	yellow70  = "#ebba00"
	red70     = "#ff8091"
)

var style = lipgloss.NewStyle().
	Bold(true).
	PaddingLeft(4).
	Width(80)

func Info(a ...any) {
	message := getFormattedMessage(a...)
	Infof("%s", message)
}

func Warn(a ...any) {
	message := getFormattedMessage(a...)
	Warnf("%s", message)
}

func Infof(format string, a ...any) {
	message := fmt.Sprintf(format, a...)
	fmt.Println(style.
		Foreground(lipgloss.Color(blue70)).
		Render("[INFO] " + message))
}

func Warnf(format string, a ...any) {
	message := fmt.Sprintf(format, a...)
	fmt.Println(style.
		Foreground(lipgloss.Color(yellow70)).
		Render("[WARN] " + message))
}

func Error(a ...any) {
	message := getFormattedMessage(a...)
	formattedMessage := style.
		Foreground(lipgloss.Color(red70)).
		PaddingTop(2).
		PaddingBottom(1).
		Render("[ERROR] " + message)

	fmt.Println(formattedMessage)
}

func Confirm(title, onYes, onNo string) (bool, error) {
	var confirm bool

	err := huh.NewConfirm().
		Title(title).
		Affirmative("yes").
		Negative("no").
		Value(&confirm).
		WithTheme(
			confirmTheme(),
		).
		Run()

	if err == nil {
		if confirm && onYes != "" {
			Info(onYes)
		} else if !confirm && onNo != "" {
			Info(onNo)
		}
	}

	return confirm, err
}

func confirmTheme() *huh.Theme {
	t := huh.ThemeBase()

	var (
		neutral = lipgloss.Color(neutral90)
		purple  = lipgloss.Color(purple70)
	)

	t.Focused.Base = t.Focused.Base.BorderForeground(purple).PaddingTop(1) // sideline
	t.Focused.Title = t.Focused.Title.Foreground(purple)                   // description

	t.Focused.FocusedButton = t.Focused.FocusedButton.Background(purple).Bold(true) // selected tile
	t.Focused.BlurredButton = t.Focused.BlurredButton.Background(neutral)           // unselected tile

	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())

	return t
}

func getFormattedMessage(a ...any) string {
	if len(a) < 1 {
		return ""
	} else if str, ok := a[0].(string); ok {
		return fmt.Sprintf(str, a[1:]...)
	} else if err, ok := a[0].(error); ok {
		return err.Error()
	}
	return fmt.Sprint(a...)
}

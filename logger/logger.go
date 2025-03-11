package logger

import (
	"fmt"
	"time"

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

func Success(a ...any) {
	message := getFormattedMessage(a...)
	Successf("%s", message)
}

func Info(a ...any) {
	message := getFormattedMessage(a...)
	Infof("%s", message)
}

func Warn(a ...any) {
	message := getFormattedMessage(a...)
	Warnf("%s", message)
}

func Successf(format string, a ...any) {
	message := fmt.Sprintf(format, a...)

	write("INFO", message, blue70, green70, green70)
}

func Infof(format string, a ...any) {
	message := fmt.Sprintf(format, a...)

	write("INFO", message, blue70, neutral60, neutral90)
}

func Warnf(format string, a ...any) {
	message := fmt.Sprintf(format, a...)

	write("WARN", message, yellow70, yellow70, yellow70)
}

func Error(a ...any) {
	message := getFormattedMessage(a...)
	write("ERROR", message, red70, red70, red70)
}

func PrintFormattedOutput(headerText, bodyText string) {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(purple70)).
		PaddingBottom(1)
	bodyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(purple70))
	frameStyle := lipgloss.NewStyle().
		MarginLeft(3).
		BorderForeground(lipgloss.AdaptiveColor{Dark: neutral90, Light: neutral60}).
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2)

	header := headerStyle.Render(headerText)
	body := bodyStyle.Render(bodyText)

	content := fmt.Sprintf("%s\n%s", header, body)
	framedContent := frameStyle.Render(content)

	fmt.Println(framedContent)
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
		neutral = lipgloss.AdaptiveColor{Dark: neutral90, Light: neutral60}
		purple  = lipgloss.Color(purple70)
	)

	t.Focused.Base = t.Focused.Base.BorderForeground(neutral).MarginTop(1).MarginLeft(3) // sideline
	t.Focused.Title = t.Focused.Title.Foreground(purple)                                 // description

	t.Focused.FocusedButton = t.Focused.FocusedButton.Background(purple).Bold(true) // selected tile
	t.Focused.BlurredButton = t.Focused.BlurredButton.Background(neutral)           // unselected tile

	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.RoundedBorder())

	return t
}

func write(tag, message string, tagColor, messageColorDark, messageColorLight string) {
	timestamp := time.Now().Format("15:04:05")
	tagStr := lipgloss.NewStyle().
		Foreground(lipgloss.Color(tagColor)).
		Width(7).
		Align(lipgloss.Right).
		PaddingLeft(2).
		PaddingRight(0).
		Render(tag)

	timeStr := lipgloss.NewStyle().
		PaddingLeft(1).
		Render(fmt.Sprintf("[%s]", timestamp))

	messageStr := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Dark: messageColorDark, Light: messageColorLight}).
		PaddingLeft(1).
		Render(message)

	formattedMessage := tagStr + timeStr + messageStr

	fmt.Println(formattedMessage)
}

func getFormattedMessage(a ...any) string {
	if len(a) < 1 {
		return ""
	} else if str, ok := a[0].(string); ok {
		if len(a) == 1 {
			return str
		}
		return fmt.Sprintf(str, a[1:]...)
	} else if err, ok := a[0].(error); ok {
		return err.Error()
	}
	return fmt.Sprint(a...)
}

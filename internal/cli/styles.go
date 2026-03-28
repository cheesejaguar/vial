package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// Color palette — matches the dashboard theme
var (
	colorPurple     = lipgloss.Color("#9F7AEA")
	colorPurpleDark = lipgloss.Color("#6B46C1")
	colorGold       = lipgloss.Color("#D69E2E")
	colorGoldLight  = lipgloss.Color("#F6E05E")
	colorGreen      = lipgloss.Color("#48BB78")
	colorRed        = lipgloss.Color("#FC8181")
	colorOrange     = lipgloss.Color("#F6AD55")
	colorMuted      = lipgloss.Color("#A0AEC0")
	colorDim        = lipgloss.Color("#718096")
)

// Reusable styles
var (
	styleSuccess = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	styleError   = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
	styleWarning = lipgloss.NewStyle().Foreground(colorOrange).Bold(true)
	styleInfo    = lipgloss.NewStyle().Foreground(colorPurple)
	styleKey     = lipgloss.NewStyle().Foreground(colorGold)
	styleMuted   = lipgloss.NewStyle().Foreground(colorMuted)
	styleDim     = lipgloss.NewStyle().Foreground(colorDim)
	styleBold    = lipgloss.NewStyle().Bold(true)
	styleHeader  = lipgloss.NewStyle().Foreground(colorPurple).Bold(true)
	styleBadge   = lipgloss.NewStyle().Foreground(colorPurpleDark)
	styleCount   = lipgloss.NewStyle().Foreground(colorPurple)
	styleArrow   = lipgloss.NewStyle().Foreground(colorGold)
	styleURL     = lipgloss.NewStyle().Foreground(colorGoldLight).Underline(true)
)

// isTTY returns true if stdout is a terminal (for color rendering).
func isTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// styled renders text with a lipgloss style, falling back to plain text for non-TTY.
func styled(style lipgloss.Style, text string) string {
	if !isTTY() {
		return text
	}
	return style.Render(text)
}

// --- Semantic helpers ---

func successIcon() string  { return styled(styleSuccess, "✓") }
func errorIcon() string    { return styled(styleError, "✗") }
func warningIcon() string  { return styled(styleWarning, "⚠") }
func skipIcon() string     { return styled(styleDim, "⊘") }
func arrowIcon() string    { return styled(styleArrow, "→") }
func agingIcon() string    { return styled(styleWarning, "●") }

func successMsg(msg string) string { return styled(styleSuccess, "✓ "+msg) }
func errorMsg(msg string) string   { return styled(styleError, "✗ "+msg) }
func warningMsg(msg string) string { return styled(styleWarning, "⚠ "+msg) }

func keyName(name string) string     { return styled(styleKey, name) }
func mutedText(text string) string   { return styled(styleMuted, text) }
func dimText(text string) string     { return styled(styleDim, text) }
func headerText(text string) string  { return styled(styleHeader, text) }
func boldText(text string) string    { return styled(styleBold, text) }
func badgeText(text string) string   { return styled(styleBadge, text) }
func countText(text string) string   { return styled(styleCount, text) }
func urlText(text string) string     { return styled(styleURL, text) }

// banner renders the Vial ASCII art logo.
func banner() string {
	if !isTTY() {
		return "vial"
	}

	art := `
   ██╗   ██╗██╗ █████╗ ██╗
   ██║   ██║██║██╔══██╗██║
   ██║   ██║██║███████║██║
   ╚██╗ ██╔╝██║██╔══██║██║
    ╚████╔╝ ██║██║  ██║███████╗
     ╚═══╝  ╚═╝╚═╝  ╚═╝╚══════╝`

	purple := lipgloss.NewStyle().Foreground(colorPurple)
	gold := lipgloss.NewStyle().Foreground(colorGold)

	lines := strings.Split(art, "\n")
	var colored []string
	for i, line := range lines {
		if line == "" {
			continue
		}
		if i <= 3 {
			colored = append(colored, purple.Render(line))
		} else {
			colored = append(colored, gold.Render(line))
		}
	}

	return strings.Join(colored, "\n")
}

// sectionHeader renders a styled section header like "🧪 Secret Health Report — 10 secret(s)"
func sectionHeader(emoji, title string) string {
	if !isTTY() {
		return fmt.Sprintf("%s %s", emoji, title)
	}
	return fmt.Sprintf("%s %s", emoji, headerText(title))
}

// stepNumber renders a colored circled number for setup steps.
func stepNumber(n int) string {
	numbers := []string{"①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨", "⑩"}
	if n < 1 || n > len(numbers) {
		return fmt.Sprintf("%d.", n)
	}
	return styled(styleInfo, numbers[n-1])
}

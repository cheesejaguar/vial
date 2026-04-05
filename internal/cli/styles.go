// styles.go defines the Vial CLI visual design system built on lipgloss.
//
// # Theme
//
// Vial uses a purple/gold duotone palette that mirrors the Svelte dashboard:
//   - Purple (#9F7AEA / #6B46C1) — primary brand colour used for headers,
//     informational text, badges, and counts.
//   - Gold (#D69E2E / #F6E05E) — accent colour used for key names, arrows, and
//     URLs, creating a visual hierarchy that draws attention to secret names.
//   - Semantic colours (green, red, orange) convey status: success, failure,
//     and warnings respectively.
//   - Muted/dim grey tones (#A0AEC0 / #718096) de-emphasise secondary
//     information such as file paths and timestamps.
//
// All rendering is gated on isTTY() so that piped or redirected output is
// always plain ASCII — scripts and log aggregators never receive ANSI escapes.
package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// Color palette — hex values are intentionally matched to the Svelte dashboard
// so that the CLI and web UI share a consistent visual identity.
var (
	colorPurple     = lipgloss.Color("#9F7AEA") // primary brand purple (lighter variant)
	colorPurpleDark = lipgloss.Color("#6B46C1") // darker purple for badges and subtle accents
	colorGold       = lipgloss.Color("#D69E2E") // primary gold for key names and arrows
	colorGoldLight  = lipgloss.Color("#F6E05E") // lighter gold for underlined URL text
	colorGreen      = lipgloss.Color("#48BB78") // success state
	colorRed        = lipgloss.Color("#FC8181") // error / failure state
	colorOrange     = lipgloss.Color("#F6AD55") // warning / aging state
	colorMuted      = lipgloss.Color("#A0AEC0") // secondary information (paths, timestamps)
	colorDim        = lipgloss.Color("#718096") // tertiary / disabled text
)

// Pre-built lipgloss styles. Each style is a value type; callers must not
// mutate these variables. New one-off styles should be constructed locally
// rather than added here unless they are used in three or more places.
var (
	styleSuccess = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)          // ✓ green bold
	styleError   = lipgloss.NewStyle().Foreground(colorRed).Bold(true)            // ✗ red bold
	styleWarning = lipgloss.NewStyle().Foreground(colorOrange).Bold(true)         // ⚠ orange bold
	styleInfo    = lipgloss.NewStyle().Foreground(colorPurple)                    // informational purple
	styleKey     = lipgloss.NewStyle().Foreground(colorGold)                      // vault key names
	styleMuted   = lipgloss.NewStyle().Foreground(colorMuted)                     // paths, secondary labels
	styleDim     = lipgloss.NewStyle().Foreground(colorDim)                       // skipped / disabled items
	styleBold    = lipgloss.NewStyle().Bold(true)                                 // emphasis without colour
	styleHeader  = lipgloss.NewStyle().Foreground(colorPurple).Bold(true)         // section headings
	styleBadge   = lipgloss.NewStyle().Foreground(colorPurpleDark)                // small badges / counts
	styleCount   = lipgloss.NewStyle().Foreground(colorPurple)                    // numeric counts
	styleArrow   = lipgloss.NewStyle().Foreground(colorGold)                      // → directional arrows
	styleURL     = lipgloss.NewStyle().Foreground(colorGoldLight).Underline(true) // clickable URLs
)

// isTTY reports whether stdout is connected to a real terminal. All styled
// helpers call this so that ANSI escape codes are suppressed when output is
// redirected to a file or pipe, keeping machine-readable output clean.
func isTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// styled applies a lipgloss style to text when stdout is a TTY, and returns
// the plain text unchanged otherwise. Centralising the TTY check here means
// individual helper functions do not need to guard themselves separately.
//
// The lipgloss.Style parameter is passed by value intentionally — lipgloss
// styles are designed as value types and copying is cheap (see nolint below).
func styled(style lipgloss.Style, text string) string { //nolint:gocritic // lipgloss.Style is a value type by design
	if !isTTY() {
		return text
	}
	return style.Render(text)
}

// Icon helpers return single-character status indicators rendered in the
// appropriate semantic colour. Using dedicated functions (rather than inline
// styled() calls) keeps output consistent and makes the intent clear at call
// sites: successIcon() communicates "this step succeeded" more clearly than
// styled(styleSuccess, "✓").

// successIcon returns a green bold checkmark.
func successIcon() string { return styled(styleSuccess, "✓") }

// errorIcon returns a red bold cross.
func errorIcon() string { return styled(styleError, "✗") }

// warningIcon returns an orange bold warning triangle.
func warningIcon() string { return styled(styleWarning, "⚠") }

// skipIcon returns a dim circle-slash used for non-fatal skipped steps.
func skipIcon() string { return styled(styleDim, "⊘") }

// arrowIcon returns a gold right-arrow used to indicate direction or the
// result of an operation (e.g. alias → canonical key).
func arrowIcon() string { return styled(styleArrow, "→") }

// agingIcon returns an orange filled circle used in the health report to flag
// secrets that have not been rotated recently.
func agingIcon() string { return styled(styleWarning, "●") }

// Full-line message helpers prefix text with the corresponding icon and apply
// the same colour to the entire string, making status lines visually distinct
// from plain output even in terminals that do not support colour.

// successMsg returns a green bold "✓ <msg>" line.
func successMsg(msg string) string { return styled(styleSuccess, "✓ "+msg) }

// errorMsg returns a red bold "✗ <msg>" line.
func errorMsg(msg string) string { return styled(styleError, "✗ "+msg) }

// warningMsg returns an orange bold "⚠ <msg>" line.
func warningMsg(msg string) string { return styled(styleWarning, "⚠ "+msg) }

// Text-styling helpers apply a single semantic style to an arbitrary string.
// They are the primary way that command output is formatted; direct calls to
// styled() or lipgloss in command files should be avoided for consistency.

// keyName renders a vault key name in gold — the primary accent colour — to
// draw the eye to secret identifiers in busy terminal output.
func keyName(name string) string { return styled(styleKey, name) }

// mutedText renders secondary information (file paths, counts, timestamps) in
// the muted grey tone so it recedes visually behind the primary content.
func mutedText(text string) string { return styled(styleMuted, text) }

// dimText renders disabled or skipped text in the darkest grey tone.
func dimText(text string) string { return styled(styleDim, text) }

// headerText renders a section heading in bold purple.
func headerText(text string) string { return styled(styleHeader, text) }

// boldText renders text in bold without applying a colour.
func boldText(text string) string { return styled(styleBold, text) }

// badgeText renders a small label (e.g. a count or tag) in dark purple.
func badgeText(text string) string { return styled(styleBadge, text) }

// countText renders a numeric count in the primary purple tone.
func countText(text string) string { return styled(styleCount, text) }

// urlText renders a clickable URL in light gold with an underline.
func urlText(text string) string { return styled(styleURL, text) }

// banner renders the Vial ASCII art logo in the purple/gold duotone palette.
// The first half of the letterforms uses purple and the bottom half transitions
// to gold, mimicking the gradient used on the marketing site. Falls back to
// the plain string "vial" in non-TTY environments.
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
		// Lines 1–3 are the upper portion of the letterforms (purple);
		// lines 4+ form the descenders and baseline (gold).
		if i <= 3 {
			colored = append(colored, purple.Render(line))
		} else {
			colored = append(colored, gold.Render(line))
		}
	}

	return strings.Join(colored, "\n")
}

// sectionHeader renders a styled section heading prefixed by an emoji, for
// example: "🧪 Secret Health Report". The emoji is always emitted as-is; only
// the title text receives lipgloss styling. In non-TTY mode the output is plain
// text with no ANSI codes.
func sectionHeader(emoji, title string) string {
	if !isTTY() {
		return fmt.Sprintf("%s %s", emoji, title)
	}
	return fmt.Sprintf("%s %s", emoji, headerText(title))
}

// stepNumber renders an encircled Unicode digit (① ② … ⑩) in the info purple
// colour, used by setup.go to number the onboarding steps so users can track
// progress at a glance. Numbers outside the range 1–10 fall back to "N.".
func stepNumber(n int) string {
	numbers := []string{"①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧", "⑨", "⑩"}
	if n < 1 || n > len(numbers) {
		return fmt.Sprintf("%d.", n)
	}
	return styled(styleInfo, numbers[n-1])
}

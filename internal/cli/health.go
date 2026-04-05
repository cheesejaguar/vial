package cli

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/vault"
)

// healthCmd reports the age and rotation status of every secret in the vault.
// It is intended to help operators identify credentials that may have been
// compromised due to long-lived use without rotation.
//
// Status thresholds (based on last-rotated timestamp, not created timestamp):
//   - ok      — rotated within 90 days
//   - warning — 90-180 days since last rotation
//   - danger  — more than 180 days since last rotation
//   - overdue — past the per-key rotation policy deadline (set via --set-rotation)
var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Show secret health report",
	Long: `Display the age, rotation status, and health of secrets in your vault.

Status indicators:
  ✓ — fresh (< 90 days since rotation)
  ● — aging (90-180 days)
  ✗ — stale (> 180 days)
  ⚠ — overdue for rotation

Examples:
  vial health
  vial health --set-rotation STRIPE_SECRET_KEY=90
  vial health --json`,
	RunE: runHealth,
}

// Health flag state.
var (
	// healthSetRotation encodes a rotation policy assignment as KEY=DAYS.
	// When set, the command updates the metadata for the named key instead
	// of printing a report. Zero days removes the policy.
	healthSetRotation string
	// healthJSON switches the output to a machine-readable JSON array so
	// the report can be consumed by dashboards or external tooling.
	healthJSON bool
)

func init() {
	healthCmd.Flags().StringVar(&healthSetRotation, "set-rotation", "", "Set rotation policy: KEY=DAYS (e.g. STRIPE_SECRET_KEY=90)")
	healthCmd.Flags().BoolVar(&healthJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(healthCmd)
}

// secretHealthEntry holds the computed health information for a single secret.
// It is intentionally separate from vault.SecretInfo so the health logic is
// decoupled from the storage layer.
type secretHealthEntry struct {
	// Key is the vault key name.
	Key string
	// AgeDays is the number of days since the secret was first added to the
	// vault (not necessarily when the value was last changed).
	AgeDays int
	// RotatedDays is the number of days since the secret value was last set
	// (created or explicitly rotated). This is the primary health signal.
	RotatedDays int
	// RotationDays is the user-configured rotation policy in days (0 = none).
	RotationDays int
	// Status is one of "ok", "warning", "danger", "overdue".
	Status string
}

// runHealth is the Cobra RunE handler for the health command. It dispatches to
// handleSetRotation when --set-rotation is provided; otherwise it builds and
// prints (or JSON-encodes) the health report.
func runHealth(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	// --set-rotation is a mutation, not a read; handle it before touching the
	// report path.
	if healthSetRotation != "" {
		return handleSetRotation(vm, healthSetRotation)
	}

	secrets := vm.ListSecrets()
	if len(secrets) == 0 {
		fmt.Println("No secrets in vault.")
		return nil
	}

	now := time.Now()
	health := buildHealthReport(secrets, now)

	if healthJSON {
		return printHealthJSON(health)
	}

	// Print report
	counts := map[string]int{}
	for _, h := range health {
		counts[h.Status]++
	}

	fmt.Printf("%s\n\n", sectionHeader("💊", fmt.Sprintf("Secret Health Report — %d secret(s)", len(health))))

	for _, h := range health {
		icon := styledStatusIcon(h.Status)
		name := keyName(h.Key)
		age := styledAge(h.RotatedDays, h.Status)
		line := fmt.Sprintf("  %s %-50s %s", icon, name, age)
		if h.RotationDays > 0 {
			line += mutedText(fmt.Sprintf("  (rotate every %dd)", h.RotationDays))
		}
		if h.Status == "overdue" {
			overdueDays := h.RotatedDays - h.RotationDays
			line += "  " + warningMsg(fmt.Sprintf("%dd overdue!", overdueDays))
		}
		fmt.Println(line)
	}

	fmt.Println()
	// Summary counts are written to stderr so they appear even when stdout is
	// piped, and so they do not pollute machine-readable stdout output.
	if counts["overdue"] > 0 {
		fmt.Fprintf(os.Stderr, "%s\n", warningMsg(fmt.Sprintf("%d secret(s) overdue for rotation", counts["overdue"])))
	}
	if counts["danger"] > 0 {
		fmt.Fprintf(os.Stderr, "%s\n", errorMsg(fmt.Sprintf("%d secret(s) stale (>180 days)", counts["danger"])))
	}
	if counts["warning"] > 0 {
		fmt.Fprintf(os.Stderr, "  %s\n", mutedText(fmt.Sprintf("%d secret(s) aging (>90 days)", counts["warning"])))
	}

	return nil
}

// buildHealthReport converts a slice of vault.SecretInfo (raw storage metadata)
// into a sorted slice of secretHealthEntry values with computed age and status.
// The report is sorted so the most urgent secrets (overdue, then danger, then
// warning) appear first; within a status tier, older secrets sort before newer
// ones so the worst offenders are always at the top.
func buildHealthReport(secrets []vault.SecretInfo, now time.Time) []secretHealthEntry {
	var health []secretHealthEntry

	for _, sec := range secrets {
		// AgeDays measures time since the key was first created.
		ageDays := int(math.Floor(now.Sub(sec.Metadata.Added).Hours() / 24))
		// RotatedDays measures time since the value was last written. This is
		// what the rotation thresholds apply to.
		rotatedDays := int(math.Floor(now.Sub(sec.Metadata.Rotated).Hours() / 24))

		status := "ok"
		if sec.Metadata.RotationDays > 0 && rotatedDays > sec.Metadata.RotationDays {
			// User has set a custom policy and the deadline has passed.
			status = "overdue"
		} else if rotatedDays > 180 {
			// Hard danger threshold regardless of custom policy.
			status = "danger"
		} else if rotatedDays > 90 {
			// Soft warning: approaching the default 180-day danger threshold.
			status = "warning"
		}

		health = append(health, secretHealthEntry{
			Key:          sec.Key,
			AgeDays:      ageDays,
			RotatedDays:  rotatedDays,
			RotationDays: sec.Metadata.RotationDays,
			Status:       status,
		})
	}

	// Sort by urgency first, then by rotatedDays descending so the stalest
	// secrets within each tier appear at the top of the list.
	sort.Slice(health, func(i, j int) bool {
		order := map[string]int{"overdue": 0, "danger": 1, "warning": 2, "ok": 3}
		if order[health[i].Status] != order[health[j].Status] {
			return order[health[i].Status] < order[health[j].Status]
		}
		return health[i].RotatedDays > health[j].RotatedDays
	})

	return health
}

// handleSetRotation parses a KEY=DAYS specification and updates the rotation
// policy stored in the secret's metadata. Passing DAYS=0 removes the policy.
// Only the metadata is updated; the secret value itself is not changed.
func handleSetRotation(vm *vault.VaultManager, spec string) error {
	parts := strings.SplitN(spec, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid format: use KEY=DAYS (e.g. STRIPE_SECRET_KEY=90)")
	}

	key := parts[0]
	days, err := strconv.Atoi(parts[1])
	if err != nil || days < 0 {
		return fmt.Errorf("invalid days value: %s", parts[1])
	}

	meta, err := vm.GetMetadata(key)
	if err != nil {
		return fmt.Errorf("secret %q not found", key)
	}

	meta.RotationDays = days
	if err := vm.SetMetadata(key, *meta); err != nil {
		return err
	}

	if days == 0 {
		fmt.Printf("%s Removed rotation policy for %s\n", successIcon(), keyName(key))
	} else {
		fmt.Printf("%s Set rotation policy for %s: every %s days\n", successIcon(), keyName(key), countText(strconv.Itoa(days)))
	}
	return nil
}

// styledStatusIcon returns a styled terminal icon that corresponds to the
// health status. Uses the shared icon helpers from styles.go so colour is
// automatically suppressed on non-TTY output.
func styledStatusIcon(status string) string {
	switch status {
	case "ok":
		return successIcon()
	case "warning":
		return agingIcon()
	case "danger":
		return errorIcon()
	case "overdue":
		return warningIcon()
	default:
		return " "
	}
}

// styledAge formats a "Xd old" string in the colour that matches the health
// status. This ties the visual severity of the number directly to its meaning.
func styledAge(days int, status string) string {
	text := fmt.Sprintf("%dd old", days)
	switch status {
	case "ok":
		return styled(styleSuccess, text)
	case "warning":
		return styled(styleWarning, text)
	case "danger", "overdue":
		return styled(styleError, text)
	default:
		return text
	}
}

// printHealthJSON emits a JSON array of health entries to stdout. Values are
// formatted inline (not via encoding/json) to keep the per-line structure
// readable while remaining valid JSON.
func printHealthJSON(health []secretHealthEntry) error {
	fmt.Println("[")
	for i, h := range health {
		comma := ","
		if i == len(health)-1 {
			comma = ""
		}
		fmt.Printf("  {\"key\": %q, \"age_days\": %d, \"rotated_days_ago\": %d, \"rotation_days\": %d, \"status\": %q}%s\n",
			h.Key, h.AgeDays, h.RotatedDays, h.RotationDays, h.Status, comma)
	}
	fmt.Println("]")
	return nil
}

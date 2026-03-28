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

var (
	healthSetRotation string
	healthJSON        bool
)

func init() {
	healthCmd.Flags().StringVar(&healthSetRotation, "set-rotation", "", "Set rotation policy: KEY=DAYS (e.g. STRIPE_SECRET_KEY=90)")
	healthCmd.Flags().BoolVar(&healthJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(healthCmd)
}

type secretHealthEntry struct {
	Key          string
	AgeDays      int
	RotatedDays  int
	RotationDays int
	Status       string
}

func runHealth(cmd *cobra.Command, args []string) error {
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	// Handle --set-rotation
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

func buildHealthReport(secrets []vault.SecretInfo, now time.Time) []secretHealthEntry {
	var health []secretHealthEntry

	for _, sec := range secrets {
		ageDays := int(math.Floor(now.Sub(sec.Metadata.Added).Hours() / 24))
		rotatedDays := int(math.Floor(now.Sub(sec.Metadata.Rotated).Hours() / 24))

		status := "ok"
		if sec.Metadata.RotationDays > 0 && rotatedDays > sec.Metadata.RotationDays {
			status = "overdue"
		} else if rotatedDays > 180 {
			status = "danger"
		} else if rotatedDays > 90 {
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

	sort.Slice(health, func(i, j int) bool {
		order := map[string]int{"overdue": 0, "danger": 1, "warning": 2, "ok": 3}
		if order[health[i].Status] != order[health[j].Status] {
			return order[health[i].Status] < order[health[j].Status]
		}
		return health[i].RotatedDays > health[j].RotatedDays
	})

	return health
}

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

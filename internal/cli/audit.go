package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/audit"
)

// auditCmd displays recent vault activity from the append-only audit log.
// The log records who (or what command) touched which secrets and when,
// giving operators a trail to consult after an incident or before rotation.
// The log file is stored as newline-delimited JSON alongside the vault file
// so it can be grepped or parsed independently of vial.
var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "View vault audit log",
	Long: `Display recent vault activity including secret access, modifications,
pour and brew operations.

The audit log is stored alongside your vault and records:
  - Secret access (pour, brew, get, export)
  - Modifications (set, remove, distill)
  - Vault operations (unlock, lock)

Examples:
  vial audit              # show last 20 entries
  vial audit --limit 50   # show last 50 entries
  vial audit --csv        # export as CSV`,
	RunE: runAudit,
}

// Audit flag state.
var (
	// auditLimit controls how many of the most recent entries are returned.
	// The log itself is unbounded; this is purely a display limit.
	auditLimit int
	// auditCSV switches the output to comma-separated format suitable for
	// import into spreadsheets or external audit tools.
	auditCSV bool
)

func init() {
	auditCmd.Flags().IntVarP(&auditLimit, "limit", "n", 20, "Number of entries to show")
	auditCmd.Flags().BoolVar(&auditCSV, "csv", false, "Export as CSV")
	rootCmd.AddCommand(auditCmd)
}

// runAudit is the Cobra RunE handler for the audit command. It does not
// require an unlocked vault because the log itself is plaintext metadata
// (key names and event types, never secret values).
func runAudit(cmd *cobra.Command, args []string) error {
	if err := loadConfig(); err != nil {
		return err
	}

	log := getAuditLog()
	entries, err := log.Read(auditLimit)
	if err != nil {
		return fmt.Errorf("reading audit log: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println(mutedText("No audit entries yet."))
		return nil
	}

	if auditCSV {
		fmt.Print(audit.ExportCSV(entries))
		return nil
	}

	fmt.Printf("%s\n\n", sectionHeader("📋", fmt.Sprintf("Audit Log — last %d entries", len(entries))))

	for _, e := range entries {
		// Timestamp is rendered in local time with second precision; UTC is
		// preserved in the underlying JSONL file for portability.
		ts := dimText(e.Timestamp.Local().Format("2006-01-02 15:04:05"))
		icon := auditEventIcon(e.Event)
		eventName := styledAuditEvent(e.Event)
		keys := ""
		if len(e.Keys) > 0 {
			keys = " " + keyName("["+strings.Join(e.Keys, ", ")+"]")
		}
		project := ""
		if e.Project != "" {
			// Show only the base name of the project path to keep lines short;
			// the full path is available in the raw JSONL if needed.
			project = " " + arrowIcon() + " " + mutedText(filepath.Base(e.Project))
		}
		detail := ""
		if e.Detail != "" {
			detail = " — " + dimText(e.Detail)
		}

		fmt.Printf("  %s %s %-18s%s%s%s\n", ts, icon, eventName, keys, project, detail)
	}

	return nil
}

// auditEventIcon returns an emoji that visually encodes the type of vault
// operation. Emojis are used here (not styled lipgloss icons) because they
// are universally supported in modern terminals and convey category at a
// glance without colour.
func auditEventIcon(event audit.EventType) string {
	switch event {
	case audit.EventPour, audit.EventBrew:
		return "🫗"
	case audit.EventGet:
		return "🔑"
	case audit.EventSet, audit.EventDistill:
		return "📝"
	case audit.EventRemove:
		return "🗑️"
	case audit.EventUnlock:
		return "🔓"
	case audit.EventLock:
		return "🔒"
	case audit.EventExport:
		return "📤"
	case audit.EventShare:
		return "🔗"
	default:
		return "  "
	}
}

// styledAuditEvent renders the event type name in a colour that reflects the
// sensitivity of the operation:
//   - green for vault access that produces output (pour, brew, unlock)
//   - gold for mutations (set, distill)
//   - red for destructive operations (remove)
//   - orange for high-risk plaintext outputs (export, share)
//   - muted for low-interest bookkeeping (lock)
func styledAuditEvent(event audit.EventType) string {
	name := string(event)
	switch event {
	case audit.EventGet:
		return styled(styleInfo, name)
	case audit.EventPour, audit.EventBrew:
		return styled(styleSuccess, name)
	case audit.EventSet, audit.EventDistill:
		return styled(styleKey, name)
	case audit.EventRemove:
		return styled(styleError, name)
	case audit.EventUnlock:
		return styled(styleSuccess, name)
	case audit.EventLock:
		return styled(styleMuted, name)
	case audit.EventExport, audit.EventShare:
		// Export and share emit plaintext; orange signals caution without
		// being as alarming as red (which is reserved for destructive ops).
		return styled(styleWarning, name)
	default:
		return name
	}
}

// getAuditLog constructs an audit.Log pointed at the file that lives next to
// the vault (audit.jsonl in the same directory as vault.json). Keeping both
// files together makes backup and migration straightforward.
func getAuditLog() *audit.Log {
	auditPath := filepath.Join(filepath.Dir(cfg.VaultPath), "audit.jsonl")
	return audit.NewLog(auditPath)
}

// recordAudit writes a single event to the audit log. Errors are logged at
// DEBUG level and swallowed so that an audit log write failure (e.g. disk
// full) never blocks the primary operation that triggered it. Callers must
// not assume the event was persisted.
func recordAudit(event audit.EventType, keys []string, project, detail string) {
	if cfg == nil {
		return
	}
	log := getAuditLog()
	if err := log.Record(event, keys, project, detail); err != nil {
		logger.Debug("audit log write failed", "err", err)
	}
}

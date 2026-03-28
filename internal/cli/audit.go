package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/audit"
)

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

var (
	auditLimit  int
	auditCSV    bool
)

func init() {
	auditCmd.Flags().IntVarP(&auditLimit, "limit", "n", 20, "Number of entries to show")
	auditCmd.Flags().BoolVar(&auditCSV, "csv", false, "Export as CSV")
	rootCmd.AddCommand(auditCmd)
}

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
		fmt.Println("No audit entries yet.")
		return nil
	}

	if auditCSV {
		fmt.Print(audit.ExportCSV(entries))
		return nil
	}

	fmt.Printf("Audit Log — last %d entries\n\n", len(entries))

	for _, e := range entries {
		ts := e.Timestamp.Local().Format("2006-01-02 15:04:05")
		icon := auditEventIcon(e.Event)
		keys := ""
		if len(e.Keys) > 0 {
			keys = " [" + strings.Join(e.Keys, ", ") + "]"
		}
		project := ""
		if e.Project != "" {
			project = " → " + filepath.Base(e.Project)
		}
		detail := ""
		if e.Detail != "" {
			detail = " — " + e.Detail
		}

		fmt.Printf("  %s %s %-8s%s%s%s\n", ts, icon, e.Event, keys, project, detail)
	}

	return nil
}

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

// getAuditLog returns the audit log for the configured vault.
func getAuditLog() *audit.Log {
	auditPath := filepath.Join(filepath.Dir(cfg.VaultPath), "audit.jsonl")
	return audit.NewLog(auditPath)
}

// recordAudit is a helper to record an audit event (errors are logged, not fatal).
func recordAudit(event audit.EventType, keys []string, project, detail string) {
	if cfg == nil {
		return
	}
	log := getAuditLog()
	if err := log.Record(event, keys, project, detail); err != nil {
		logger.Debug("audit log write failed", "err", err)
	}
}

// recordAuditTimestamp records an event with current time formatting.
func recordAuditTimestamp(event audit.EventType, detail string) {
	recordAudit(event, nil, "", detail)
}

func init() {
	// Ensure audit log timestamp is valid on startup
	_ = time.Now()
}

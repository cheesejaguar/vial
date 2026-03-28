package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/awnumar/memguard"
	"github.com/cheesejaguar/vial/internal/audit"
	"github.com/cheesejaguar/vial/internal/share"
)

var shareCmd = &cobra.Command{
	Use:   "share [KEYS...]",
	Short: "Create an encrypted secret bundle for sharing",
	Long: `Create an encrypted, time-limited bundle of secrets that can be shared
with teammates. The bundle is encrypted with a one-time passphrase
(not your vault master password).

The recipient imports with: vial distill --from=bundle <file>

Examples:
  vial share OPENAI_API_KEY STRIPE_SECRET_KEY
  vial share STRIPE_* --expires=24h
  vial share --all --expires=1h --output=team-secrets.vial`,
	RunE: runShare,
}

var shareReceiveCmd = &cobra.Command{
	Use:   "receive FILE",
	Short: "Import secrets from a shared bundle",
	Long: `Decrypt and import secrets from a shared bundle file.
You'll need the passphrase that was used to create the bundle.`,
	Args: cobra.ExactArgs(1),
	RunE: runShareReceive,
}

var (
	shareExpires string
	shareOutput  string
	shareAll     bool
)

func init() {
	shareCmd.Flags().StringVar(&shareExpires, "expires", "24h", "Bundle expiration (e.g. 1h, 24h, 7d)")
	shareCmd.Flags().StringVarP(&shareOutput, "output", "o", "", "Output file (default: vial-share-<timestamp>.bundle)")
	shareCmd.Flags().BoolVar(&shareAll, "all", false, "Share all secrets")
	shareCmd.AddCommand(shareReceiveCmd)
	rootCmd.AddCommand(shareCmd)
}

func runShare(cmd *cobra.Command, args []string) error {
	if len(args) == 0 && !shareAll {
		return fmt.Errorf("specify key names (e.g. vial share OPENAI_API_KEY) or use --all")
	}

	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	// Get all vault keys
	allKeys, err := vm.VaultKeyNames()
	if err != nil {
		return err
	}

	// Determine which keys to share
	var selectedKeys []string
	if shareAll {
		selectedKeys = allKeys
	} else {
		for _, pattern := range args {
			for _, key := range allKeys {
				matched, _ := filepath.Match(pattern, key)
				if matched || key == pattern {
					selectedKeys = append(selectedKeys, key)
				}
			}
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	var uniqueKeys []string
	for _, k := range selectedKeys {
		if !seen[k] {
			seen[k] = true
			uniqueKeys = append(uniqueKeys, k)
		}
	}

	if len(uniqueKeys) == 0 {
		return fmt.Errorf("no matching keys found")
	}

	// Collect secret values
	secrets := make(map[string]string, len(uniqueKeys))
	for _, key := range uniqueKeys {
		val, err := vm.GetSecret(key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ⊘ Skipping %s: %v\n", key, err)
			continue
		}
		secrets[key] = string(val.Bytes())
		val.Destroy()
	}

	if len(secrets) == 0 {
		return fmt.Errorf("no secrets to share")
	}

	// Parse expiry
	expiry, err := parseExpiry(shareExpires)
	if err != nil {
		return err
	}

	// Get passphrase
	fmt.Fprintln(os.Stderr, "Choose a passphrase for this bundle (share it with the recipient separately):")
	passphrase, err := readPassword("Passphrase: ")
	if err != nil {
		return err
	}
	defer passphrase.Destroy()

	if passphrase.Size() < 12 {
		return fmt.Errorf("passphrase too short (minimum 12 characters)")
	}

	// Create bundle
	bundle, err := share.CreateBundle(secrets, string(passphrase.Bytes()), expiry)
	if err != nil {
		return err
	}

	data, err := bundle.Marshal()
	if err != nil {
		return err
	}

	// Write output
	output := shareOutput
	if output == "" {
		output = fmt.Sprintf("vial-share-%s.bundle", time.Now().Format("20060102-150405"))
	}

	if err := os.WriteFile(output, data, 0600); err != nil {
		return fmt.Errorf("writing bundle: %w", err)
	}

	fmt.Printf("\n%s Bundle created: %s\n", successIcon(), boldText(output))
	fmt.Printf("  %s Contains %s secret(s)\n", arrowIcon(), countText(fmt.Sprintf("%d", len(secrets))))
	fmt.Printf("  %s Expires: %s\n", arrowIcon(), dimText(time.Now().Add(expiry).Format("2006-01-02 15:04 MST")))
	fmt.Printf("\n  %s\n", mutedText("Recipient imports with: vial share receive "+output))

	// Record audit event
	var keyNames []string
	for k := range secrets {
		keyNames = append(keyNames, k)
	}
	recordAudit(audit.EventShare, keyNames, "", fmt.Sprintf("bundle: %s", output))

	return nil
}

func runShareReceive(cmd *cobra.Command, args []string) error {
	bundlePath := args[0]

	data, err := os.ReadFile(bundlePath)
	if err != nil {
		return fmt.Errorf("reading bundle: %w", err)
	}

	bundle, err := share.UnmarshalBundle(data)
	if err != nil {
		return err
	}

	fmt.Printf("Bundle: %d secret(s), expires %s\n", bundle.KeyCount, bundle.ExpiresAt)

	passphrase, err := readPassword("Passphrase: ")
	if err != nil {
		return err
	}
	defer passphrase.Destroy()

	payload, err := share.OpenBundle(bundle, string(passphrase.Bytes()))
	if err != nil {
		return err
	}

	// Import into vault
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	imported := 0
	for key, value := range payload.Secrets {
		val := memguard.NewBufferFromBytes([]byte(value))
		if err := vm.SetSecret(key, val); err != nil {
			val.Destroy()
			fmt.Printf("  %s %s: %v\n", errorIcon(), keyName(key), err)
			continue
		}
		val.Destroy()
		fmt.Printf("  %s %s imported\n", successIcon(), keyName(key))
		imported++
	}

	fmt.Printf("\n%s %s secret(s) imported from bundle\n", arrowIcon(), countText(fmt.Sprintf("%d", imported)))
	return nil
}

func parseExpiry(s string) (time.Duration, error) {
	// Handle "Nd" format (days)
	if strings.HasSuffix(s, "d") {
		s = strings.TrimSuffix(s, "d")
		var days int
		if _, err := fmt.Sscanf(s, "%d", &days); err == nil {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	}
	return time.ParseDuration(s)
}

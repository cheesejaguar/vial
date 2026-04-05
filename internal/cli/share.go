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

// shareCmd creates an encrypted, time-limited bundle of secrets that can be
// handed to a teammate. The bundle is encrypted with a one-time passphrase
// chosen by the sender — not the vault master password — so the recipient can
// import the bundle without ever knowing the sender's vault credentials.
//
// Threat model: the bundle file is safe to transfer over an untrusted channel
// (email, Slack, S3) as long as the passphrase is shared via a separate
// out-of-band channel. After the expiry time the bundle is rejected on import.
//
// Security note: the passphrase is read interactively via readPassword (never
// as a positional argument) so it does not appear in shell history or process
// listings.
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

// shareReceiveCmd decrypts a bundle created by `vial share` and imports the
// secrets into the local vault. The passphrase prompt is handled interactively;
// the vault is unlocked after the bundle is opened so that the passphrase and
// vault credential prompts are clearly separated.
var shareReceiveCmd = &cobra.Command{
	Use:   "receive FILE",
	Short: "Import secrets from a shared bundle",
	Long: `Decrypt and import secrets from a shared bundle file.
You'll need the passphrase that was used to create the bundle.`,
	Args: cobra.ExactArgs(1),
	RunE: runShareReceive,
}

// Share flag state.
var (
	// shareExpires sets the bundle lifetime. Supports Go duration strings
	// (e.g. "1h", "24h") and a day shorthand (e.g. "7d"). The default of
	// "24h" balances convenience with limiting the exposure window.
	shareExpires string
	// shareOutput overrides the default filename. If empty, a timestamped
	// name is generated (vial-share-YYYYMMDD-HHMMSS.bundle).
	shareOutput string
	// shareAll includes every key in the vault rather than requiring the
	// caller to name them individually.
	shareAll bool
)

func init() {
	shareCmd.Flags().StringVar(&shareExpires, "expires", "24h", "Bundle expiration (e.g. 1h, 24h, 7d)")
	shareCmd.Flags().StringVarP(&shareOutput, "output", "o", "", "Output file (default: vial-share-<timestamp>.bundle)")
	shareCmd.Flags().BoolVar(&shareAll, "all", false, "Share all secrets")
	shareCmd.AddCommand(shareReceiveCmd)
	rootCmd.AddCommand(shareCmd)
}

// runShare is the Cobra RunE handler for the share command.
//
// Steps:
//  1. Validate that at least one key or --all is specified.
//  2. Unlock the vault and enumerate matching keys (glob patterns supported).
//  3. Deduplicate the selected key list in case patterns overlap.
//  4. Decrypt each selected secret into an in-memory map; destroy each
//     LockedBuffer immediately after copying.
//  5. Prompt for a bundle passphrase (minimum 12 characters) via readPassword.
//  6. Delegate bundle creation and encryption to share.CreateBundle.
//  7. Write the bundle to disk with 0600 permissions.
//  8. Record an audit event listing the shared key names.
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

	// Determine which keys to share. Positional arguments may be exact key
	// names or glob patterns (e.g. "STRIPE_*"); both are resolved against the
	// full vault key list.
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

	// Deduplicate in case multiple patterns matched the same key.
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

	// Collect secret values into a plain map. Each LockedBuffer is destroyed
	// right after the string copy to minimise the plaintext exposure window.
	secrets := make(map[string]string, len(uniqueKeys))
	for _, key := range uniqueKeys {
		val, err := vm.GetSecret(key)
		if err != nil {
			// Log the failure and skip rather than aborting; the caller can see
			// which keys were omitted from the bundle summary printed at the end.
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

	// Prompt for the bundle passphrase via stdin (never a CLI argument).
	// The passphrase must be at least 12 characters to reduce brute-force
	// risk on the bundle file if it is intercepted.
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

	// Write output. 0600 restricts the file to the owner; this is a defence-
	// in-depth measure since the contents are already encrypted.
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

// runShareReceive is the Cobra RunE handler for the `share receive` sub-command.
// It reads and decrypts the bundle file, prompting for the passphrase via stdin,
// then imports each secret into the local vault.
//
// The vault is unlocked after the bundle is decrypted so that the passphrase
// and vault prompts are clearly separate in the terminal — the user knows
// exactly what each credential is for.
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

	// Print bundle metadata before prompting so the user can confirm they have
	// the right file before entering the passphrase.
	fmt.Printf("Bundle: %d secret(s), expires %s\n", bundle.KeyCount, bundle.ExpiresAt)

	passphrase, err := readPassword("Passphrase: ")
	if err != nil {
		return err
	}
	defer passphrase.Destroy()

	// OpenBundle verifies the HMAC/AEAD tag and the expiry timestamp.
	// It returns an error if the passphrase is wrong or the bundle has expired.
	payload, err := share.OpenBundle(bundle, string(passphrase.Bytes()))
	if err != nil {
		return err
	}

	// Unlock vault only after successfully decrypting the bundle to avoid
	// leaving the vault unlocked if the passphrase is wrong.
	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	imported := 0
	for key, value := range payload.Secrets {
		// Wrap the value in a LockedBuffer before passing to SetSecret so that
		// the vault manager receives secrets in protected memory, consistent
		// with how all other write paths operate.
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

// parseExpiry converts a human-readable expiry string into a time.Duration.
// It extends Go's standard duration syntax with a "d" (days) suffix, because
// expressing multi-day bundle lifetimes in hours (e.g. "168h") is error-prone.
// Standard Go durations ("1h", "30m") continue to work unchanged.
func parseExpiry(s string) (time.Duration, error) {
	// Handle "Nd" format (days) before delegating to time.ParseDuration so
	// that "7d" works even though it is not a standard Go duration token.
	if strings.HasSuffix(s, "d") {
		s = strings.TrimSuffix(s, "d")
		var days int
		if _, err := fmt.Sscanf(s, "%d", &days); err == nil {
			return time.Duration(days) * 24 * time.Hour, nil
		}
	}
	return time.ParseDuration(s)
}

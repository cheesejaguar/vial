package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/cheesejaguar/vial/internal/alias"
	"github.com/cheesejaguar/vial/internal/matcher"
	"github.com/cheesejaguar/vial/internal/parser"
)

// brewCmd implements the "brew" command (alchemical name for "run").
// It injects vault secrets as environment variables into a child process
// without writing any files to disk. The standard alias "run" is also
// registered but hidden to preserve the alchemical naming convention.
//
// Alchemical metaphor: brewing combines the raw ingredients (vault secrets)
// into a prepared environment that feeds the target process.
//
// Unlike pour (which writes to .env), brew is ephemeral: secrets exist only
// in the child process's environment and disappear when the process exits.
//
// DisableFlagParsing is set so that flags intended for the child command
// (e.g. `vial brew -- node --inspect server.js`) are passed through
// unchanged rather than intercepted by Cobra.
var brewCmd = &cobra.Command{
	Use:     "brew -- COMMAND [ARGS...]",
	Aliases: []string{"run"},
	Short:   "Run a command with secrets injected as environment variables",
	Long: `Inject secrets from the vault as environment variables and execute a command.

Reads .env.example to determine which variables to inject, then runs the
specified command with those secrets in its environment. No .env file is written.

Example:
  vial brew -- node server.js
  vial brew -- python manage.py runserver`,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	RunE:               runBrew,
}

func init() {
	rootCmd.AddCommand(brewCmd)
}

// runBrew is the Cobra RunE handler for the brew command.
//
// Execution flow:
//  1. Handle --help manually (required because DisableFlagParsing is set).
//  2. Strip a leading "--" separator (conventional before child command args).
//  3. Unlock the vault via requireUnlockedVault.
//  4. Determine which keys to inject: prefer .env.example if it exists,
//     otherwise fall back to all vault keys.
//  5. Resolve each key against the vault using a 3-tier matcher chain
//     (Exact → Normalize → Alias).
//  6. Build the child process's env by copying the current process environment
//     and overriding/appending matched secrets.
//  7. Replace the current process with the child using syscall.Exec so that
//     the child inherits the correct PID and signal handling (important for
//     process managers and shells that rely on the child being the direct
//     process group leader).
//
// Security notes:
//   - Secrets are never passed as command-line arguments; they flow only
//     through the environment.
//   - The vault is locked via defer before Exec; however because Exec replaces
//     the process the defer will not run. The lock is kept for clarity on the
//     error paths before Exec is reached.
//   - The LockedBuffer returned by GetSecret is destroyed immediately after
//     the plaintext string is copied into the env slice.
func runBrew(cmd *cobra.Command, args []string) error {
	// DisableFlagParsing bypasses Cobra's --help handling, so we must do it
	// ourselves to avoid treating --help as the command to exec.
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return cmd.Help()
		}
	}

	// Users may write `vial brew -- node server.js`; strip the "--" separator
	// before treating args[0] as the executable name.
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	vm, err := requireUnlockedVault()
	if err != nil {
		return err
	}
	defer vm.Lock()

	if err := loadConfig(); err != nil {
		return err
	}

	// Determine which env vars to inject by consulting .env.example.
	// This scopes the injection to the variables the project actually declares,
	// avoiding leaking unrelated secrets into the child process.
	dir, _ := os.Getwd()
	templatePath := filepath.Join(dir, cfg.EnvExample)

	var keysToInject []string

	if f, err := os.Open(templatePath); err == nil {
		entries, parseErr := parser.Parse(f)
		f.Close()
		if parseErr == nil {
			keysToInject = parser.KeysNeeded(entries)
		}
	}

	// Fallback: if there is no .env.example, inject every key in the vault so
	// `vial brew` still works for projects that have not created a template.
	if len(keysToInject) == 0 {
		keysToInject, err = vm.VaultKeyNames()
		if err != nil {
			return err
		}
	}

	// Build matcher chain (tiers 1-3; LLM is not used for brew to keep startup
	// latency acceptable when launching interactive processes).
	vaultKeys, err := vm.VaultKeyNames()
	if err != nil {
		return err
	}

	aliasStore := alias.NewStore()
	aliasStore.LoadFromVault(loadAliasStoreFromVault(vm))

	chain := matcher.NewChain(
		&matcher.ExactMatcher{},
		&matcher.NormalizeMatcher{},
		&matcher.AliasMatcher{Store: aliasStore},
	)

	// Start with the current process environment so the child inherits PATH,
	// HOME, TERM, and any other ambient variables it needs to function.
	env := os.Environ()
	injected := 0

	for _, key := range keysToInject {
		result, _ := chain.Resolve(key, vaultKeys)
		if result == nil {
			// No vault entry found; leave the key absent rather than injecting
			// an empty value, which could cause subtle application failures.
			continue
		}

		val, err := vm.GetSecret(result.VaultKey)
		if err != nil {
			continue
		}
		// Copy the value out of protected memory and destroy the buffer before
		// appending to the env slice.
		secretValue := string(val.Bytes())
		val.Destroy()

		// setEnvVar overwrites any existing entry for this key so vault values
		// take precedence over whatever was already in the shell's environment.
		env = setEnvVar(env, key, secretValue)
		injected++
	}

	fmt.Fprintf(os.Stderr, "🧪 injected %s secrets, running %s\n", countText(fmt.Sprintf("%d", injected)), boldText(strings.Join(args, " ")))

	// Resolve the binary to an absolute path before calling Exec. LookPath
	// respects the PATH in the current environment (before our modifications),
	// which is what the user expects.
	binary, err := exec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("command not found: %s", args[0])
	}

	// syscall.Exec replaces the current process image entirely. The child
	// process has the same PID and inherits open file descriptors. This is
	// intentional: tools like npm, make, and shell scripts expect to be the
	// direct child of whatever launched vial brew.
	return syscall.Exec(binary, args, env)
}

// setEnvVar sets or overrides a single environment variable in a KEY=VALUE
// slice. It scans the slice for an existing entry with the same key prefix and
// replaces it in-place to avoid duplicate entries, which some programs handle
// unpredictably. If no existing entry is found the new pair is appended.
func setEnvVar(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

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

func runBrew(cmd *cobra.Command, args []string) error {
	// Strip leading -- if present
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

	// Determine which env vars to inject
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

	// If no .env.example, inject all vault keys
	if len(keysToInject) == 0 {
		keysToInject, err = vm.VaultKeyNames()
		if err != nil {
			return err
		}
	}

	// Build matcher chain
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

	// Resolve secrets and build env
	env := os.Environ()
	injected := 0

	for _, key := range keysToInject {
		result, _ := chain.Resolve(key, vaultKeys)
		if result == nil {
			continue
		}

		val, err := vm.GetSecret(result.VaultKey)
		if err != nil {
			continue
		}
		secretValue := string(val.Bytes())
		val.Destroy()

		env = setEnvVar(env, key, secretValue)
		injected++
	}

	fmt.Fprintf(os.Stderr, "🧪 injected %s secrets, running %s\n", countText(fmt.Sprintf("%d", injected)), boldText(strings.Join(args, " ")))

	// Find the command binary
	binary, err := exec.LookPath(args[0])
	if err != nil {
		return fmt.Errorf("command not found: %s", args[0])
	}

	// Exec replaces the current process
	return syscall.Exec(binary, args, env)
}

// setEnvVar sets or overrides an env var in a KEY=VALUE slice.
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

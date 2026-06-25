package compose

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/mattn/go-shellwords"
)

const (
	// SecretRefPrefix prefixes a reference to a secret defined in the top-level 'secrets' section,
	// e.g. 'secret://my_secret'.
	SecretRefPrefix = "secret://"
	// SecretCommandExtensionKey is the short-form secret extension providing the command to run,
	// equivalent to 'driver: exec' with 'driver_opts.command'.
	SecretCommandExtensionKey = "x-command"
	// secretExecDriver is a secret driver that resolves a secret by running an arbitrary command and using its output.
	secretExecDriver = "exec"
)

// secretRefName returns the secret name from the secret reference, for example, 'name' from 'secret://name'.
func secretRefName(ref string) (string, bool) {
	name, ok := strings.CutPrefix(ref, SecretRefPrefix)
	if !ok || name == "" {
		return "", false
	}
	return name, true
}

// validateSecrets validates the project's secrets and clears the transient 'external' marker that
// expandSecretCommandExtension sets on driver-based secrets during loading. A secret defines exactly one
// source: 'file', 'environment', or the 'exec' driver.
func validateSecrets(project *types.Project) error {
	for name, secret := range project.Secrets {
		if secret.Driver != "" {
			// Clear the transient external marker set during loading to pass compose-go's consistency check.
			secret.External = false
			project.Secrets[name] = secret

			if secret.Driver != secretExecDriver {
				return fmt.Errorf("secret '%s': unsupported driver '%s', only '%s' is supported",
					name, secret.Driver, secretExecDriver)
			}
			if secret.DriverOpts["command"] == "" {
				return fmt.Errorf("secret '%s': '%s' driver requires 'driver_opts.command'", name, secretExecDriver)
			}
			if secret.File != "" || secret.Environment != "" {
				return fmt.Errorf("secret '%s': a secret using a driver cannot also define 'file' or 'environment'",
					name)
			}
			continue
		}
		// Assume that compose-go already validated that a non-driver, non-external secret defines exactly one of
		// 'file' or 'environment'.
	}

	return nil
}

// ResolveSecrets resolves 'secret://name' references in the services' environment to actual secret values, setting each
// referenced variable to the secret's value in place. Each referenced secret is resolved at most once, even if
// referenced by multiple services. Secrets that are not referenced are never resolved.
func ResolveSecrets(ctx context.Context, project *types.Project) error {
	values := make(map[string]string)
	resolve := func(name string) (string, error) {
		if v, ok := values[name]; ok {
			return v, nil
		}
		secret, ok := project.Secrets[name]
		if !ok {
			return "", fmt.Errorf("secret '%s' referenced via '%s%s' is not defined in the top-level "+
				"'secrets' section", name, SecretRefPrefix, name)
		}
		v, err := secretValue(ctx, secret, project)
		if err != nil {
			return "", fmt.Errorf("get the value of secret '%s': %w", name, err)
		}
		values[name] = v
		return v, nil
	}

	// Resolve only secret references set as values for environment variables in enabled services.
	for _, service := range project.Services {
		for k, v := range service.Environment {
			if v == nil {
				continue
			}
			secretName, ok := secretRefName(*v)
			if !ok {
				continue
			}
			value, err := resolve(secretName)
			if err != nil {
				return err
			}
			service.Environment[k] = &value
		}
	}

	return nil
}

// secretValue resolves a secret to its value depending on its source: an 'exec' driver command,
// an environment variable, or a file. The command output is trimmed of surrounding whitespace as
// command-line tools commonly append a trailing newline; environment and file values are returned
// verbatim.
func secretValue(ctx context.Context, secret types.SecretConfig, project *types.Project) (string, error) {
	switch {
	case secret.Environment != "":
		value, ok := project.Environment[secret.Environment]
		if !ok {
			return "", fmt.Errorf("environment variable '%s' is not set", secret.Environment)
		}
		return value, nil
	case secret.File != "":
		content, err := os.ReadFile(secret.File)
		if err != nil {
			return "", fmt.Errorf("read secret file: %w", err)
		}
		return string(content), nil
	case secret.Driver == secretExecDriver:
		// Run with the resolved project environment (process env merged with .env files) so the command
		// can use the same variables available elsewhere in the Compose file.
		out, err := runSecretCommand(ctx, secret.DriverOpts["command"], project.WorkingDir,
			project.Environment.Values())
		if err != nil {
			return "", err
		}
		return strings.TrimSuffix(out, "\n"), nil
	default:
		return "", fmt.Errorf("secret has no source: define one of 'file', 'environment', '%s', or 'driver'",
			SecretCommandExtensionKey)
	}
}

// runSecretCommand runs the command in workingDir with the given environment and returns its stdout.
// The command runs directly without a shell, so shell features need an explicit shell, e.g. 'sh -c "cmd1 | cmd2"'.
// Its stdin and stderr are connected to the current process so it can prompt for authentication interactively.
func runSecretCommand(ctx context.Context, command, workingDir string, env []string) (string, error) {
	args, err := shellwords.Parse(command)
	if err != nil {
		return "", fmt.Errorf("parse command '%s': %w", command, err)
	}
	if len(args) == 0 {
		return "", fmt.Errorf("command '%s' is empty", command)
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = workingDir
	cmd.Env = env

	// Forward stdin and stderr to the user's terminal so prompts and progress are visible.
	var stdout, stderr bytes.Buffer
	cmd.Stdin = os.Stdin
	cmd.Stdout = &stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)

	if err := cmd.Run(); err != nil {
		// Include stderr but never stdout in the error as stdout may contain secret data.
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", fmt.Errorf("run command '%s': %w", command, err)
		}
		return "", fmt.Errorf("run command '%s': %w: %s", command, err, msg)
	}

	return stdout.String(), nil
}

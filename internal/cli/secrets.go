package cli

import (
	"context"
	"fmt"
	"regexp"
)

var secretNameRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// reservedSecretNames mirrors docs/interfaces/api.md's "Secrets" table:
// the guest's sshd material, delivered by hostd, never through this path.
var reservedSecretNames = map[string]bool{
	"ssh_host_ed25519_key":          true,
	"ssh_host_ed25519_key-cert.pub": true,
	"user_ca.pub":                   true,
}

// SecretsSetCmd implements `repose secrets set NAME` (07-cli.md §5.10).
func SecretsSetCmd(ctx context.Context, e *Env, projectArg, name string, value []byte) error {
	if !secretNameRe.MatchString(name) {
		return exitf(ExitUsage, "NAME must match [A-Z][A-Z0-9_]{0,63}")
	}
	if reservedSecretNames[name] {
		return exitf(ExitUsage, "%s is reserved for the guest's sshd material", name)
	}
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	res, err := e.Client.PutSecret(ctx, project.ID, name, value)
	if err != nil {
		return err
	}
	if res != nil && res.Pushed {
		_, _ = fmt.Fprintf(e.Out, "Set %s (pushed to running guest)\n", name)
	} else {
		_, _ = fmt.Fprintf(e.Out, "Set %s (will be delivered at next start)\n", name)
	}
	return nil
}

// SecretsListCmd implements `repose secrets list`.
func SecretsListCmd(ctx context.Context, e *Env, projectArg string) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	secrets, err := e.Client.ListSecrets(ctx, project.ID)
	if err != nil {
		return err
	}
	if e.JSON {
		return writeJSONOut(e.Out, secrets)
	}
	for _, s := range secrets {
		_, _ = fmt.Fprintf(e.Out, "%s\t%s\n", s.Name, s.UpdatedAt.Format("2006-01-02 15:04"))
	}
	return nil
}

// SecretsRmCmd implements `repose secrets rm NAME`.
func SecretsRmCmd(ctx context.Context, e *Env, projectArg, name string) error {
	project, err := requireProject(ctx, e, projectArg)
	if err != nil {
		return err
	}
	if err := e.Client.DeleteSecret(ctx, project.ID, name); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(e.Out, "Removed %s\n", name)
	return nil
}

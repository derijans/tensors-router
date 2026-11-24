package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"

	"tensors-router/internal/config"
	"tensors-router/internal/credentialstore"
)

type securityAction func(ctx context.Context, store *credentialstore.Store, cfg config.Config, output io.Writer) error

type securityCommand struct {
	summary string
	bind    func(flags *flag.FlagSet) securityAction
}

var securityCommands = map[string]securityCommand{
	"token list":    {summary: "list tokens (secrets are never shown)", bind: bindTokenList},
	"token create":  {summary: "--role inference|admin|metrics|cluster --name N [--value-file F|-]", bind: bindTokenCreate},
	"token delete":  {summary: "--name N (name or id)", bind: bindTokenDelete},
	"token show":    {summary: "--name N (cluster tokens only)", bind: bindTokenShow},
	"cert list":     {summary: "list API certificates, identities, authorities and trusted peers", bind: bindCertificateList},
	"cert generate": {summary: "--name N [--host H ...] [--activate] (self-signed identity)", bind: bindCertificateGenerate},
	"cert import":   {summary: "--kind api_certificate|identity|authority|trusted_authority|pinned_peer --name N --cert F [--key F] [--activate]", bind: bindCertificateImport},
	"cert activate": {summary: "--name N (API certificate served to clients, or identity used for peer calls)", bind: bindCertificateActivate},
	"cert delete":   {summary: "--name N [--force]", bind: bindCertificateDelete},
	"cert export":   {summary: "--name N --out F (public certificate, to pin or trust on other routers)", bind: bindCertificateExport},
	"ca create":     {summary: "--name N (local certificate authority)", bind: bindAuthorityCreate},
	"ca issue":      {summary: "--ca N --name N [--host H ...] (--out-cert F --out-key F | --install [--activate])", bind: bindAuthorityIssue},
	"trust-bundle":  {summary: "--out F (certificates that verify this router's HTTPS listener)", bind: bindTrustBundle},
}

func runSecurity(args []string) error {
	name := securityCommandName(args)
	command, ok := securityCommands[name]
	if !ok {
		printSecurityUsage(os.Stdout)
		if len(args) == 0 || name == "help" {
			return nil
		}
		return fmt.Errorf("unknown security command %q", strings.Join(args, " "))
	}
	flags := flag.NewFlagSet("security "+name, flag.ContinueOnError)
	configPath := flags.String("config", "config.yaml", "config file")
	securityProfile := flags.String("security-profile", "", "security profile")
	action := command.bind(flags)
	if err := flags.Parse(args[len(strings.Fields(name)):]); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected arguments %q; pass values with flags such as --name", flags.Args())
	}
	return runSecurityAction(*configPath, *securityProfile, action)
}

func securityCommandName(args []string) string {
	if len(args) == 0 {
		return ""
	}
	if len(args) >= 2 && !strings.HasPrefix(args[1], "-") {
		return args[0] + " " + args[1]
	}
	return args[0]
}

func runSecurityAction(configPath string, securityProfile string, action securityAction) error {
	cfg, err := loadCommandConfig(configPath, securityProfile)
	if err != nil {
		return err
	}
	ctx := context.Background()
	store, err := openCredentialStore(ctx, cfg, log.New(os.Stderr, "", 0))
	if err != nil {
		return err
	}
	defer store.Close()
	return action(ctx, store, cfg, os.Stdout)
}

func loadCommandConfig(configPath string, securityProfile string) (config.Config, error) {
	profileOverride := config.ResolveSecurityProfile(securityProfile, os.Getenv("TENSORS_ROUTER_SECURITY_PROFILE"))
	loadOptions, err := environmentLoadOptions(profileOverride)
	if err != nil {
		return config.Config{}, err
	}
	return config.LoadWithOptions(configPath, loadOptions)
}

func printSecurityUsage(output io.Writer) {
	names := make([]string, 0, len(securityCommands))
	for name := range securityCommands {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Fprintln(output, "usage: tensors-router security <command> [--config config.yaml] [flags]")
	for _, name := range names {
		fmt.Fprintf(output, "  %-14s %s\n", name, securityCommands[name].summary)
	}
}

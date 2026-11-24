package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"tensors-router/internal/config"
	"tensors-router/internal/credentialstore"
)

func bindAuthorityCreate(flags *flag.FlagSet) securityAction {
	name := flags.String("name", "", "unique authority name, also its common name")
	return func(ctx context.Context, store *credentialstore.Store, _ config.Config, output io.Writer) error {
		certificate, err := store.CreateAuthority(ctx, *name)
		if err != nil {
			return err
		}
		return reportCertificateWrite(ctx, store, certificate, false, output)
	}
}

type issueTarget struct {
	certificateFile string
	keyFile         string
	install         bool
	activate        bool
}

func (target issueTarget) validate() error {
	writesFiles := target.certificateFile != "" || target.keyFile != ""
	switch {
	case target.install && writesFiles:
		return errors.New("use either --install or --out-cert/--out-key, not both")
	case !target.install && (target.certificateFile == "" || target.keyFile == ""):
		return errors.New("--out-cert and --out-key are required unless --install is set")
	case target.activate && !target.install:
		return errors.New("--activate requires --install")
	}
	return nil
}

func bindAuthorityIssue(flags *flag.FlagSet) securityAction {
	authority := flags.String("ca", "", "authority name or id")
	name := flags.String("name", "", "identity name, also its common name")
	var hosts hostList
	flags.Var(&hosts, "host", "host name or IP the identity covers; repeat or comma-separate")
	var target issueTarget
	flags.StringVar(&target.certificateFile, "out-cert", "", "write the certificate chain to this new file")
	flags.StringVar(&target.keyFile, "out-key", "", "write the private key to this new file (mode 0600)")
	flags.BoolVar(&target.install, "install", false, "store the identity on this router instead of writing files")
	flags.BoolVar(&target.activate, "activate", false, "activate the installed identity")
	return func(ctx context.Context, store *credentialstore.Store, cfg config.Config, output io.Writer) error {
		if err := requireFlag("ca", *authority); err != nil {
			return err
		}
		if err := target.validate(); err != nil {
			return err
		}
		if len(hosts) == 0 && target.install {
			hosts = defaultIdentityHosts(cfg)
		}
		issued, err := store.IssueIdentity(ctx, *authority, *name, hosts)
		if err != nil {
			return err
		}
		if target.install {
			certificate, err := store.ImportCertificate(ctx, issued.AsIdentityImport(*name))
			if err != nil {
				return err
			}
			return reportCertificateWrite(ctx, store, certificate, target.activate, output)
		}
		if err := writeNewPrivateFile(target.certificateFile, issued.CertificatePEM); err != nil {
			return err
		}
		if err := writeNewPrivateFile(target.keyFile, issued.PrivateKeyPEM); err != nil {
			return err
		}
		fmt.Fprintf(output, "wrote %s and %s; on the target router run `tensors-router security cert import --kind identity --name %s --cert %s --key %s --activate` and import this authority there as trusted_authority\n",
			target.certificateFile, target.keyFile, *name, target.certificateFile, target.keyFile)
		return nil
	}
}

func bindTrustBundle(flags *flag.FlagSet) securityAction {
	outputPath := flags.String("out", "", "file to write")
	return func(ctx context.Context, store *credentialstore.Store, _ config.Config, output io.Writer) error {
		if err := requireFlag("out", *outputPath); err != nil {
			return err
		}
		bundle, err := store.TrustBundle(ctx)
		if err != nil {
			return err
		}
		if err := writeNewPrivateFile(*outputPath, string(bundle)); err != nil {
			return err
		}
		fmt.Fprintf(output, "wrote %s\n", *outputPath)
		return nil
	}
}

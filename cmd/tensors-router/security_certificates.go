package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"tensors-router/internal/config"
	"tensors-router/internal/credentialstore"
)

const restartForHTTPSNotice = "restart the router so its listener switches to HTTPS if it is still serving plain HTTP"

func bindCertificateList(*flag.FlagSet) securityAction {
	return func(ctx context.Context, store *credentialstore.Store, _ config.Config, output io.Writer) error {
		certificates, err := store.Certificates(ctx)
		if err != nil {
			return err
		}
		table := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(table, "ID\tNAME\tKIND\tACTIVE\tEXPIRES\tHOSTS\tSHA256")
		for _, certificate := range certificates {
			hosts := strings.Join(append(append([]string{}, certificate.DNSNames...), certificate.IPAddresses...), ",")
			fmt.Fprintf(table, "%s\t%s\t%s\t%t\t%s\t%s\t%s\n", certificate.ID, certificate.Name, certificate.Kind, certificate.Active,
				certificate.NotAfter.Format(time.RFC3339), hosts, certificate.Fingerprint)
		}
		return table.Flush()
	}
}

func bindCertificateGenerate(flags *flag.FlagSet) securityAction {
	name := flags.String("name", "", "unique certificate name, also the certificate common name")
	var hosts hostList
	flags.Var(&hosts, "host", "host name or IP the certificate covers; repeat or comma-separate (default: loopback, bind address and cluster.public_url host)")
	activate := flags.Bool("activate", false, "use this identity for the HTTPS listener and peer calls")
	return func(ctx context.Context, store *credentialstore.Store, cfg config.Config, output io.Writer) error {
		if len(hosts) == 0 {
			hosts = defaultIdentityHosts(cfg)
		}
		certificate, err := store.GenerateIdentity(ctx, *name, hosts)
		if err != nil {
			return err
		}
		return reportCertificateWrite(ctx, store, certificate, *activate, output)
	}
}

func bindCertificateImport(flags *flag.FlagSet) securityAction {
	kind := flags.String("kind", "", "identity, authority, trusted_authority or pinned_peer")
	name := flags.String("name", "", "unique certificate name")
	certificateFile := flags.String("cert", "", "PEM certificate file (identity: leaf first, then intermediates)")
	keyFile := flags.String("key", "", "PEM private key file (identity and authority only)")
	activate := flags.Bool("activate", false, "use this identity for the HTTPS listener and peer calls")
	return func(ctx context.Context, store *credentialstore.Store, _ config.Config, output io.Writer) error {
		parsedKind, err := credentialstore.ParseKind(*kind)
		if err != nil {
			return err
		}
		if err := requireFlag("cert", *certificateFile); err != nil {
			return err
		}
		certificatePEM, err := readSecurityInput(*certificateFile)
		if err != nil {
			return err
		}
		privateKeyPEM, err := readSecurityInput(*keyFile)
		if err != nil {
			return err
		}
		certificate, err := store.ImportCertificate(ctx, credentialstore.CertificateImport{Name: *name, Kind: parsedKind, CertificatePEM: certificatePEM, PrivateKeyPEM: privateKeyPEM})
		if err != nil {
			return err
		}
		return reportCertificateWrite(ctx, store, certificate, *activate, output)
	}
}

func bindCertificateActivate(flags *flag.FlagSet) securityAction {
	name := flags.String("name", "", "identity name or id")
	return func(ctx context.Context, store *credentialstore.Store, _ config.Config, output io.Writer) error {
		if err := requireFlag("name", *name); err != nil {
			return err
		}
		if err := store.Activate(ctx, *name); err != nil {
			return err
		}
		fmt.Fprintf(output, "activated identity %s; %s\n", *name, restartForHTTPSNotice)
		return nil
	}
}

func bindCertificateDelete(flags *flag.FlagSet) securityAction {
	name := flags.String("name", "", "certificate name or id")
	force := flags.Bool("force", false, "also delete the active identity")
	return func(ctx context.Context, store *credentialstore.Store, _ config.Config, output io.Writer) error {
		if err := requireFlag("name", *name); err != nil {
			return err
		}
		if err := store.DeleteCertificate(ctx, *name, *force); err != nil {
			return err
		}
		fmt.Fprintf(output, "deleted certificate %s\n", *name)
		return nil
	}
}

func reportCertificateWrite(ctx context.Context, store *credentialstore.Store, certificate credentialstore.Certificate, activate bool, output io.Writer) error {
	fmt.Fprintf(output, "stored %s %s (%s)\nsha256 %s\n", certificate.Kind, certificate.Name, certificate.ID, certificate.Fingerprint)
	if !activate {
		return nil
	}
	if err := store.Activate(ctx, certificate.ID); err != nil {
		return err
	}
	fmt.Fprintf(output, "activated; %s\n", restartForHTTPSNotice)
	return nil
}

func bindCertificateExport(flags *flag.FlagSet) securityAction {
	name := flags.String("name", "", "certificate name or id")
	outputPath := flags.String("out", "", "file to write")
	return func(ctx context.Context, store *credentialstore.Store, _ config.Config, output io.Writer) error {
		if err := requireFlag("name", *name); err != nil {
			return err
		}
		if err := requireFlag("out", *outputPath); err != nil {
			return err
		}
		certificatePEM, err := store.PublicCertificatePEM(ctx, *name)
		if err != nil {
			return err
		}
		if err := writeNewPrivateFile(*outputPath, certificatePEM); err != nil {
			return err
		}
		fmt.Fprintf(output, "wrote %s\n", *outputPath)
		return nil
	}
}

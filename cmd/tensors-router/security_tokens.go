package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"tensors-router/internal/config"
	"tensors-router/internal/credentialstore"
)

func bindTokenList(*flag.FlagSet) securityAction {
	return func(ctx context.Context, store *credentialstore.Store, _ config.Config, output io.Writer) error {
		tokens, err := store.Tokens(ctx)
		if err != nil {
			return err
		}
		table := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
		fmt.Fprintln(table, "ID\tNAME\tROLE\tSOURCE\tCREATED")
		for _, token := range tokens {
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", token.ID, token.Name, token.Role, token.Source, token.CreatedAt.Format(time.RFC3339))
		}
		return table.Flush()
	}
}

func bindTokenCreate(flags *flag.FlagSet) securityAction {
	role := flags.String("role", "", "inference, admin, metrics or cluster")
	name := flags.String("name", "", "unique token name")
	valueFile := flags.String("value-file", "", "use an existing value read from this file, or - for stdin (needed to give every node the same cluster token)")
	return func(ctx context.Context, store *credentialstore.Store, _ config.Config, output io.Writer) error {
		parsedRole, err := credentialstore.ParseRole(*role)
		if err != nil {
			return err
		}
		supplied, err := readSecurityInput(*valueFile)
		if err != nil {
			return err
		}
		token, secret, err := store.CreateToken(ctx, credentialstore.NewToken{Name: *name, Role: parsedRole, Supplied: supplied})
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "created %s token %s (%s)\n", token.Role, token.Name, token.ID)
		if token.Source == credentialstore.SourceCreated {
			fmt.Fprintf(output, "secret (shown once): %s\n", secret)
		}
		return nil
	}
}

func bindTokenDelete(flags *flag.FlagSet) securityAction {
	name := flags.String("name", "", "token name or id")
	return func(ctx context.Context, store *credentialstore.Store, _ config.Config, output io.Writer) error {
		if err := requireFlag("name", *name); err != nil {
			return err
		}
		if err := store.DeleteToken(ctx, *name); err != nil {
			return err
		}
		fmt.Fprintf(output, "deleted token %s\n", *name)
		return nil
	}
}

func bindTokenShow(flags *flag.FlagSet) securityAction {
	name := flags.String("name", "", "cluster token name or id")
	return func(ctx context.Context, store *credentialstore.Store, _ config.Config, output io.Writer) error {
		if err := requireFlag("name", *name); err != nil {
			return err
		}
		secret, err := store.ClusterSecret(ctx, *name)
		if err != nil {
			return err
		}
		fmt.Fprintln(output, secret)
		return nil
	}
}

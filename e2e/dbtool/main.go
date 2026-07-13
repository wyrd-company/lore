package main

import (
	"context"
	"fmt"
	"os"
	"regexp"

	"github.com/jackc/pgx/v5"
)

var schemaName = regexp.MustCompile(`^lore_e2e_[a-zA-Z0-9_]+$`)

func main() {
	if len(os.Args) != 4 || (os.Args[1] != "create" && os.Args[1] != "drop") {
		fmt.Fprintln(os.Stderr, "usage: dbtool <create|drop> <database-url> <schema>")
		os.Exit(2)
	}
	if !schemaName.MatchString(os.Args[3]) {
		fmt.Fprintln(os.Stderr, "refusing invalid e2e schema name")
		os.Exit(2)
	}
	connection, err := pgx.Connect(context.Background(), os.Args[2])
	if err != nil {
		fatal(err)
	}
	defer connection.Close(context.Background()) //nolint:errcheck
	identifier := pgx.Identifier{os.Args[3]}.Sanitize()
	statement := "CREATE SCHEMA " + identifier
	if os.Args[1] == "drop" {
		statement = "DROP SCHEMA IF EXISTS " + identifier + " CASCADE"
	}
	if _, err := connection.Exec(context.Background(), statement); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

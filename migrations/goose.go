// This file is fully taken from an exampe:
//   https://github.com/pressly/goose/blob/master/cmd/goose/main.go
// With only postgresql driver support
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"

	"github.com/pressly/goose"
)

var (
	flags      = flag.NewFlagSet("goose", flag.ExitOnError)
	dir        = flags.String("dir", ".", "directory with migration files")
	table      = flags.String("table", "goose_db_version", "migrations table name")
	verbose    = flags.Bool("v", false, "enable verbose mode")
	help       = flags.Bool("h", false, "print help")
	version    = flags.Bool("version", false, "print version")
	certfile   = flags.String("certfile", "", "file path to root CA's certificates in pem format (only support on mysql)")
	sequential = flags.Bool("s", false, "use sequential numbering for new migrations")
)

// Just because it is required by goose
func normalizeDBString(driver string, str string, certfile string) string {
	return str
}

func main() {
	flags.Usage = usage
	flags.Parse(os.Args[1:])

	if *version {
		fmt.Println(goose.VERSION)
		return
	}
	if *verbose {
		goose.SetVerbose(true)
	}
	if *sequential {
		goose.SetSequential(true)
	}
	goose.SetTableName(*table)

	args := flags.Args()
	if len(args) == 0 || *help {
		flags.Usage()
		return
	}

	switch args[0] {
	case "create":
		if err := goose.Run("create", nil, *dir, args[1:]...); err != nil {
			log.Fatalf("goose run: %v", err)
		}
		return
	case "fix":
		if err := goose.Run("fix", nil, *dir); err != nil {
			log.Fatalf("goose run: %v", err)
		}
		return
	}

	args = mergeArgs(args)
	if len(args) < 3 {
		flags.Usage()
		return
	}

	driver, dbstring, command := args[0], args[1], args[2]

	db, err := goose.OpenDBWithDriver(driver, normalizeDBString(driver, dbstring, *certfile))
	if err != nil {
		log.Fatalf("-dbstring=%q: %v\n", dbstring, err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Fatalf("goose: failed to close DB: %v\n", err)
		}
	}()

	arguments := []string{}
	if len(args) > 3 {
		arguments = append(arguments, args[3:]...)
	}

	if err := goose.Run(command, db, *dir, arguments...); err != nil {
		log.Fatalf("goose run: %v", err)
	}
}

const (
	envGooseDriver   = "GOOSE_DRIVER"
	envGooseDBString = "GOOSE_DBSTRING"
)

func mergeArgs(args []string) []string {
	if len(args) < 1 {
		return args
	}
	if d := os.Getenv(envGooseDriver); d != "" {
		args = append([]string{d}, args...)
	}
	if d := os.Getenv(envGooseDBString); d != "" {
		args = append([]string{args[0], d}, args[1:]...)
	}
	return args
}

func usage() {
	fmt.Println(usagePrefix)
	flags.PrintDefaults()
	fmt.Println(usageCommands)
}

var (
	usagePrefix = `Usage: goose [OPTIONS] DRIVER DBSTRING COMMAND

or

Set environment key
GOOSE_DRIVER=DRIVER
GOOSE_DBSTRING=DBSTRING

Usage: goose [OPTIONS] COMMAND

Drivers:
    postgres

Examples:
    goose postgres "user=postgres dbname=postgres sslmode=disable" status

    GOOSE_DRIVER=postgres GOOSE_DBSTRING="user=postgres dbname=postgres sslmode=disable" goose status

Options:
`

	usageCommands = `
Commands:
    up                   Migrate the DB to the most recent version available
    up-by-one            Migrate the DB up by 1
    up-to VERSION        Migrate the DB to a specific VERSION
    down                 Roll back the version by 1
    down-to VERSION      Roll back to a specific VERSION
    redo                 Re-run the latest migration
    reset                Roll back all migrations
    status               Dump the migration status for the current DB
    version              Print the current version of the database
    create NAME [sql|go] Creates new migration file with the current timestamp
    fix                  Apply sequential ordering to migrations
`
)

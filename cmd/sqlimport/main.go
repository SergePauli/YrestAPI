package main

import (
	"YrestAPI/internal"
	"YrestAPI/internal/importer/sqlimport"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var dsn string
	var help bool
	fs.StringVar(&dsn, "dsn", "", "PostgreSQL DSN (priority 1)")
	fs.StringVar(&dsn, "dns", "", "Deprecated alias for -dsn")
	fs.BoolVar(&help, "help", false, "show command help")
	fs.BoolVar(&help, "h", false, "show command help (shorthand)")
	schema := fs.String("schema", "public", "PostgreSQL schema to introspect")
	onlySimple := fs.Bool("only-simple", false, "import only tables without outgoing foreign keys")
	outDir := fs.String("out", "./db", "output directory for generated model YAML files")
	dryRun := fs.Bool("dry-run", false, "print generated YAML to stdout without writing files")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  make import ARGS=\"[options]\"")
		fmt.Fprintln(os.Stderr, "  go run ./cmd/sqlimport [options]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  make import ARGS=\"-dry-run -only-simple\"")
		fmt.Fprintln(os.Stderr, "  make import ARGS=\"-dsn 'postgres://user:pass@localhost:5432/app?sslmode=disable' -out ./db_generated\"")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Options:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			fs.Usage()
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "sqlimport: %v\n\n", err)
		fs.Usage()
		os.Exit(2)
	}
	if help {
		fs.Usage()
		return
	}

	resolvedDSN := strings.TrimSpace(dsn)
	if resolvedDSN == "" {
		_ = loadDotEnvFromRepoRoot()
		resolvedDSN = strings.TrimSpace(os.Getenv("POSTGRES_DSN"))
	}
	if resolvedDSN == "" {
		fmt.Fprintln(os.Stderr, "sqlimport: DSN is required (pass -dsn, -dns or set POSTGRES_DSN in .env/env)\n")
		fs.Usage()
		os.Exit(2)
	}
	if strings.TrimSpace(*schema) == "" {
		fmt.Fprintln(os.Stderr, "sqlimport: schema must not be empty\n")
		fs.Usage()
		os.Exit(2)
	}
	if strings.TrimSpace(*outDir) == "" {
		fmt.Fprintln(os.Stderr, "sqlimport: out directory must not be empty\n")
		fs.Usage()
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, resolvedDSN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sqlimport: connect failed: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	files, err := sqlimport.Generate(ctx, pool, sqlimport.GenerateOptions{
		Schema:     *schema,
		OnlySimple: *onlySimple,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "sqlimport: generation failed: %v\n", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Printf("sqlimport: no eligible tables found in schema %q\n", *schema)
		return
	}

	if *dryRun {
		for _, f := range files {
			fmt.Printf("--- %s ---\n%s\n", f.FileName, string(f.Content))
		}
		fmt.Printf("sqlimport: generated %d model files (dry-run, only-simple=%v)\n", len(files), *onlySimple)
		return
	}

	if err := sqlimport.WriteFiles(*outDir, files); err != nil {
		fmt.Fprintf(os.Stderr, "sqlimport: write failed: %v\n", err)
		os.Exit(1)
	}

	abs, _ := filepath.Abs(*outDir)
	fmt.Printf("sqlimport: generated %d model files in %s (only-simple=%v)\n", len(files), abs, *onlySimple)
}

func loadDotEnvFromRepoRoot() error {
	root, err := internal.FindRepoRoot()
	if err != nil {
		return err
	}
	return godotenv.Load(filepath.Join(root, ".env"))
}

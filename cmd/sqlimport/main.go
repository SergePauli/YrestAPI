package main

import (
	"YrestAPI/internal"
	"YrestAPI/internal/importer/graphqlimport"
	"YrestAPI/internal/importer/prismaimport"
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
	prismaSchema := fs.String("prisma-schema", "", "path to Prisma schema file (e.g. ./prisma/schema.prisma)")
	graphqlQueries := fs.String("graphql-queries", "", "path to GraphQL query file or directory")
	modelsDir := fs.String("models-dir", "./db", "directory with existing YAML models for GraphQL preset import")
	replacePresets := fs.Bool("replace-presets", false, "replace existing presets during GraphQL import")
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
		fmt.Fprintln(os.Stderr, "  make import ARGS=\"-prisma-schema ./prisma/schema.prisma -out ./db_generated\"")
		fmt.Fprintln(os.Stderr, "  make import ARGS=\"-graphql-queries ./gateway/queries -models-dir ./db -dry-run\"")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Options:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			fs.Usage()
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "sqlimport: %v\n", err)
		fmt.Fprintln(os.Stderr)
		fs.Usage()
		os.Exit(2)
	}
	if help {
		fs.Usage()
		return
	}

	if strings.TrimSpace(*outDir) == "" {
		fmt.Fprintln(os.Stderr, "sqlimport: out directory must not be empty")
		fmt.Fprintln(os.Stderr)
		fs.Usage()
		os.Exit(2)
	}

	var files []sqlimport.ModelFile
	prismaPath := strings.TrimSpace(*prismaSchema)
	graphqlPath := strings.TrimSpace(*graphqlQueries)
	var localeDefaults map[string]map[int]string
	var graphqlWarnings []string
	if graphqlPath != "" {
		res, err := graphqlimport.ImportFromPath(graphqlPath, graphqlimport.ImportOptions{
			ModelsDir:      *modelsDir,
			ReplacePresets: *replacePresets,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "sqlimport: generation failed: %v\n", err)
			os.Exit(1)
		}
		graphqlWarnings = res.Warnings
		files = make([]sqlimport.ModelFile, 0, len(res.Files))
		for _, f := range res.Files {
			files = append(files, sqlimport.ModelFile{
				FileName: f.FileName,
				Content:  f.Content,
			})
		}
	} else if prismaPath != "" {
		prismaResult, err := prismaimport.GenerateResultFromFile(prismaPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sqlimport: generation failed: %v\n", err)
			os.Exit(1)
		}
		localeDefaults = prismaResult.LocaleDefaults
		prismaFiles := prismaResult.Files
		files = make([]sqlimport.ModelFile, 0, len(prismaFiles))
		for _, f := range prismaFiles {
			files = append(files, sqlimport.ModelFile{
				FileName: f.FileName,
				Content:  f.Content,
			})
		}
	} else {
		resolvedDSN := strings.TrimSpace(dsn)
		if resolvedDSN == "" {
			_ = loadDotEnvFromRepoRoot()
			resolvedDSN = strings.TrimSpace(os.Getenv("POSTGRES_DSN"))
		}
		if resolvedDSN == "" {
			fmt.Fprintln(os.Stderr, "sqlimport: DSN is required (pass -dsn, -dns or set POSTGRES_DSN in .env/env), or pass -prisma-schema, or pass -graphql-queries")
			fmt.Fprintln(os.Stderr)
			fs.Usage()
			os.Exit(2)
		}
		if strings.TrimSpace(*schema) == "" {
			fmt.Fprintln(os.Stderr, "sqlimport: schema must not be empty")
			fmt.Fprintln(os.Stderr)
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

		files, err = sqlimport.Generate(ctx, pool, sqlimport.GenerateOptions{
			Schema:     *schema,
			OnlySimple: *onlySimple,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "sqlimport: generation failed: %v\n", err)
			os.Exit(1)
		}
	}

	if len(files) == 0 {
		if graphqlPath != "" {
			fmt.Printf("sqlimport: no eligible presets found in graphql queries %q\n", graphqlPath)
		} else if prismaPath != "" {
			fmt.Printf("sqlimport: no eligible models found in prisma schema %q\n", prismaPath)
		} else {
			fmt.Printf("sqlimport: no eligible tables found in schema %q\n", *schema)
		}
		return
	}

	if *dryRun {
		for _, f := range files {
			fmt.Printf("--- %s ---\n%s\n", f.FileName, string(f.Content))
		}
		if graphqlPath != "" {
			for _, warning := range graphqlWarnings {
				fmt.Printf("warning: %s\n", warning)
			}
		}
		if prismaPath != "" && len(localeDefaults) > 0 {
			if raw, err := prismaimport.LocaleDefaultsYAML(localeDefaults); err == nil && len(raw) > 0 {
				fmt.Printf("--- locale defaults ---\n%s\n", string(raw))
			}
		}
		if graphqlPath != "" {
			fmt.Printf("sqlimport: updated %d model files (dry-run, source=graphql)\n", len(files))
		} else if prismaPath != "" {
			fmt.Printf("sqlimport: generated %d model files (dry-run, source=prisma)\n", len(files))
		} else {
			fmt.Printf("sqlimport: generated %d model files (dry-run, only-simple=%v)\n", len(files), *onlySimple)
		}
		return
	}

	writeDir := *outDir
	if graphqlPath != "" {
		writeDir = *modelsDir
	}
	if err := sqlimport.WriteFiles(writeDir, files); err != nil {
		fmt.Fprintf(os.Stderr, "sqlimport: write failed: %v\n", err)
		os.Exit(1)
	}

	abs, _ := filepath.Abs(writeDir)
	if graphqlPath != "" {
		fmt.Printf("sqlimport: updated %d model files in %s (source=graphql)\n", len(files), abs)
		for _, warning := range graphqlWarnings {
			fmt.Printf("sqlimport: warning: %s\n", warning)
		}
	} else if prismaPath != "" {
		_ = loadDotEnvFromRepoRoot()
		localeName := strings.TrimSpace(os.Getenv("LOCALE"))
		if localeName == "" {
			localeName = "en"
		}
		localePath := filepath.Join("cfg", "locales", localeName+".yml")
		if err := prismaimport.MergeLocaleDefaults(localePath, localeDefaults); err != nil {
			fmt.Fprintf(os.Stderr, "sqlimport: locale write failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("sqlimport: generated %d model files in %s (source=prisma)\n", len(files), abs)
		fmt.Printf("sqlimport: locale defaults merged into %s\n", localePath)
	} else {
		fmt.Printf("sqlimport: generated %d model files in %s (only-simple=%v)\n", len(files), abs, *onlySimple)
	}
}

func loadDotEnvFromRepoRoot() error {
	root, err := internal.FindRepoRoot()
	if err != nil {
		return err
	}
	return godotenv.Load(filepath.Join(root, ".env"))
}

// Command seed generates the fictitious data of the demonstration condominium
// (administradora, condominium, blocks, units, people with history of owner and
// tenant swaps, one test account per role) and the fictitious import spreadsheet.
//
//	seed --profile=seed          1 block, 8 units: development and CI (make seed)
//	seed --profile=poc           3 blocks, 120 units, 300 people: the PoC (make seed-poc)
//	seed --profile=poc --reset-demo   delete the demonstration data first (make reset-demo)
//	seed --profile=poc --csv-only     print the import spreadsheet and touch nothing
//
// The same --seed always generates the same data. Everything carries is_demo;
// --reset-demo deletes only demonstration condominiums.
//
// Environment: DATABASE_URL (application role). --reset-demo also needs
// DATABASE_MIGRATIONS_URL (the schema owner: the application role cannot see
// other tenants). SEED_PASSWORD is the password of the test accounts; when it
// is not set a random one is generated and printed once. There is no built-in password.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/victorradael/condoguard/api/internal/db"
	"github.com/victorradael/condoguard/api/internal/seed"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	fs.SetOutput(stderr)
	profile := fs.String("profile", "seed", "data set size: seed (1 block, 8 units) or poc (3 blocks, 120 units, 300 people)")
	seedN := fs.Int64("seed", seed.DefaultSeed, "seed of the generator: the same seed generates the same data")
	reset := fs.Bool("reset-demo", false, "delete the demonstration data before generating it again")
	csvOnly := fs.Bool("csv-only", false, "print the import spreadsheet on stdout and do nothing else")
	csvBroken := fs.Bool("csv-errors", false, "with --csv-only: the spreadsheet with spoiled lines, to try the error report")
	if err := fs.Parse(args); err != nil {
		return err
	}
	prof, ok := seed.Profiles[*profile]
	if !ok {
		names := make([]string, 0, len(seed.Profiles))
		for n := range seed.Profiles {
			names = append(names, n)
		}
		sort.Strings(names)
		return fmt.Errorf("unknown profile %q (use one of %v)", *profile, names)
	}
	ds, err := seed.Generate(prof, *seedN)
	if err != nil {
		return err
	}
	if *csvOnly {
		if *csvBroken {
			return ds.WriteBrokenCSV(stdout)
		}
		return ds.WriteImportCSV(stdout)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	start := time.Now()

	if *reset {
		url := os.Getenv("DATABASE_MIGRATIONS_URL")
		if url == "" {
			return errors.New("--reset-demo needs DATABASE_MIGRATIONS_URL (the schema owner)")
		}
		owner, derr := db.NewPool(ctx, url)
		if derr != nil {
			return derr
		}
		removed, rerr := seed.Reset(ctx, owner)
		owner.Close()
		if rerr != nil {
			return rerr
		}
		fmt.Fprintf(stderr, "reset-demo: removed %d condominium(s), %d account(s), %d administradora(s), %d audit entries\n",
			removed.Condominiums, removed.Accounts, removed.Administrators, removed.AuditEntries)
	}

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL not set")
	}
	pool, err := db.NewPool(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()

	pw, generated := os.Getenv("SEED_PASSWORD"), false
	if pw == "" {
		if pw, err = seed.NewPassword(); err != nil {
			return err
		}
		generated = true
	}
	switch err = seed.Load(ctx, pool, ds, pw); {
	case errors.Is(err, seed.ErrExists):
		fmt.Fprintf(stderr, "seed: %q already exists; nothing to do (make reset-demo recreates it)\n", ds.Condominium)
		return nil
	case err != nil:
		return err
	}

	c := ds.Counts()
	fmt.Fprintf(stderr, "seed %q (seed %d) in %s: %d block(s), %d units, %d people, %d links (%d owner swaps, %d tenant swaps), %d accounts\n",
		prof.Name, *seedN, time.Since(start).Round(time.Millisecond), c.Blocks, c.Units, c.People, c.Links, c.OwnerSwaps, c.TenantSwaps, c.Accounts)
	fmt.Fprintf(stderr, "condominium %q (id %s)\ntest accounts (administradora, síndico and conselho enroll the second factor on first login):\n", ds.Condominium, ds.CondominiumID)
	for _, a := range ds.Accounts {
		fmt.Fprintf(stderr, "  %-30s %s\n", a.Email, a.Role)
	}
	if generated {
		fmt.Fprintf(stderr, "password of all of them (generated now, shown once; set SEED_PASSWORD to choose): %s\n", pw)
	}
	return nil
}

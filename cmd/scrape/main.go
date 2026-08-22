// Command scrape syncs vendor catalogues into the database.
//
//	go run ./cmd/scrape --vendor=dispozable   scrape one vendor now
//	go run ./cmd/scrape --due-now             scrape whichever vendors' hour
//	                                          slots have arrived (used by the
//	                                          hourly GitHub Actions workflow)
//	go run ./cmd/scrape --list                show every vendor and its state
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/R1yAA/Bat-ti/app/database"
	"github.com/R1yAA/Bat-ti/app/scraper/runner"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	vendorSlug := flag.String("vendor", "", "scrape this vendor slug now, ignoring its hour slot")
	dueNow := flag.Bool("due-now", false, "scrape every vendor whose hour slot has arrived")
	listVendors := flag.Bool("list", false, "list every vendor with its last scrape state")
	verbose := flag.Bool("verbose", false, "log every HTTP request")
	maxListings := flag.Int("max-listings", 0,
		"development only: stop after this many product pages on the per-product tiers (0 means no cap)")
	flag.Parse()

	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

	if err := run(logger, *vendorSlug, *dueNow, *listVendors, *maxListings); err != nil {
		logger.Error("scrape command failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, vendorSlug string, dueNow bool, listVendors bool, maxListings int) error {
	// Ctrl-C and the CI job's termination signal both cancel in-flight HTTP
	// requests rather than leaving a scrape run stuck in 'running'.
	ctx, stopSignalHandling := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stopSignalHandling()

	_ = godotenv.Load()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is not set; copy .env.example to .env or export it")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connecting to the database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("pinging the database: %w", err)
	}

	scrapeRunner, err := runner.New(pool, logger, maxListings)
	if err != nil {
		return err
	}

	// config/vendors.go is the source of truth, so every invocation reconciles
	// the vendors table against it before doing anything else.
	if err := scrapeRunner.SyncVendorRegistry(ctx); err != nil {
		return err
	}

	switch {
	case listVendors:
		return printVendorTable(ctx, pool)
	case vendorSlug != "":
		return scrapeRunner.RunVendorBySlug(ctx, vendorSlug)
	case dueNow:
		return scrapeRunner.RunDueVendors(ctx)
	default:
		flag.Usage()
		return fmt.Errorf("choose one of --vendor, --due-now or --list")
	}
}

func printVendorTable(ctx context.Context, pool *pgxpool.Pool) error {
	vendorRows, err := database.New(pool).ListVendors(ctx)
	if err != nil {
		return fmt.Errorf("listing vendors: %w", err)
	}

	fmt.Printf("%-14s %-18s %-6s %s\n", "SLUG", "TIER", "SLOT", "LAST SUCCESSFUL SCRAPE")
	for _, vendorRow := range vendorRows {
		lastScrape := "never"
		if vendorRow.LastSuccessfulScrapeTimestamp.Valid {
			lastScrape = vendorRow.LastSuccessfulScrapeTimestamp.Time.
				Local().Format(time.RFC1123)
		}
		if vendorRow.LastScrapeError.Valid {
			lastScrape += "  [last error: " + truncate(vendorRow.LastScrapeError.String, 60) + "]"
		}
		fmt.Printf("%-14s %-18s %02d:00  %s\n",
			vendorRow.VendorSlug, vendorRow.ScraperTier, vendorRow.ScrapeHourUtc, lastScrape)
	}
	return nil
}

func truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "…"
}

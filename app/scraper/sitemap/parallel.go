package sitemap

import (
	"context"
	"log/slog"
	"sync"

	"github.com/R1yAA/Bat-ti/app/scraper/httpclient"
)

// FetchedPage is one product page, or the reason it could not be read.
type FetchedPage struct {
	Location string
	Body     []byte
	Err      error
}

// FetchPages reads every location, up to workerCount at a time, and returns
// the results in the order given.
//
// Vendors that publish no catalogue feed are read one product page at a time,
// which for a storefront of a couple of thousand products is the whole cost of
// the scrape: Plutonious took an hour and two minutes for 1,463 products,
// against a job that is killed at ninety minutes. Almost all of that was
// waiting, not transferring — so the fix is to have several requests in flight
// rather than to ask for pages any faster.
//
// The client's own rate limiter still decides the overall pace, so raising
// workerCount cannot make the scraper less polite than the vendor's configured
// rate. It only stops one slow response from stalling the ones behind it.
//
// Ordering is preserved because a catalogue that reshuffles between runs makes
// diffs unreadable, and the delisting sweep compares what was seen against
// what is stored.
func FetchPages(
	ctx context.Context,
	client *httpclient.Client,
	locations []string,
	workerCount int,
	logger *slog.Logger,
) []FetchedPage {
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > len(locations) {
		workerCount = len(locations)
	}

	pages := make([]FetchedPage, len(locations))
	indexes := make(chan int)

	var waitGroup sync.WaitGroup
	for range workerCount {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for index := range indexes {
				location := locations[index]
				body, err := client.GetBytes(ctx, location)
				pages[index] = FetchedPage{Location: location, Body: body, Err: err}
			}
		}()
	}

	for index := range locations {
		// Stop handing out work once the run is cancelled; whatever has been
		// fetched so far is still returned, and the caller reports the rest as
		// unread rather than pretending the catalogue was complete.
		if ctx.Err() != nil {
			break
		}
		indexes <- index
	}
	close(indexes)
	waitGroup.Wait()

	if logger != nil {
		failedCount := 0
		for _, page := range pages {
			if page.Err != nil {
				failedCount++
			}
		}
		if failedCount > 0 {
			logger.Info("some product pages could not be read",
				"failed", failedCount, "of", len(locations))
		}
	}
	return pages
}

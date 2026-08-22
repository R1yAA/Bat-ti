// Package sitemap reads the XML sitemaps the non-API vendors publish. It is the
// only way to enumerate a catalogue on a storefront with no product feed.
package sitemap

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/R1yAA/Bat-ti/app/scraper/httpclient"
)

// Entry is one URL from a sitemap.
type Entry struct {
	Location     string
	LastModified *time.Time
}

type urlSet struct {
	URLs []struct {
		Location     string `xml:"loc"`
		LastModified string `xml:"lastmod"`
	} `xml:"url"`
}

type sitemapIndex struct {
	Sitemaps []struct {
		Location string `xml:"loc"`
	} `xml:"sitemap"`
}

// maximumIndexDepth stops a sitemap index that points at itself.
const maximumIndexDepth = 3

// Fetch reads a sitemap, following one level of sitemap index if the URL turns
// out to be an index rather than a URL set.
func Fetch(ctx context.Context, client *httpclient.Client, sitemapURL string) ([]Entry, error) {
	return fetchWithDepth(ctx, client, sitemapURL, 0)
}

func fetchWithDepth(
	ctx context.Context,
	client *httpclient.Client,
	sitemapURL string,
	depth int,
) ([]Entry, error) {
	if depth > maximumIndexDepth {
		return nil, fmt.Errorf("sitemap index nested more than %d levels at %s", maximumIndexDepth, sitemapURL)
	}

	sitemapBytes, err := client.GetBytes(ctx, sitemapURL)
	if err != nil {
		return nil, fmt.Errorf("fetching sitemap %s: %w", sitemapURL, err)
	}

	// An index and a URL set are both valid XML at the same endpoint, so which
	// one arrived is decided by which parses into content.
	var parsedIndex sitemapIndex
	if err := xml.Unmarshal(sitemapBytes, &parsedIndex); err == nil && len(parsedIndex.Sitemaps) > 0 {
		var allEntries []Entry
		for _, childSitemap := range parsedIndex.Sitemaps {
			childEntries, err := fetchWithDepth(ctx, client, childSitemap.Location, depth+1)
			if err != nil {
				return nil, err
			}
			allEntries = append(allEntries, childEntries...)
		}
		return allEntries, nil
	}

	var parsedURLSet urlSet
	if err := xml.Unmarshal(sitemapBytes, &parsedURLSet); err != nil {
		return nil, fmt.Errorf("parsing sitemap %s: %w", sitemapURL, err)
	}

	entries := make([]Entry, 0, len(parsedURLSet.URLs))
	for _, sitemapURLEntry := range parsedURLSet.URLs {
		location := strings.TrimSpace(sitemapURLEntry.Location)
		if location == "" {
			continue
		}
		entry := Entry{Location: location}
		if parsedTime, err := time.Parse(time.RFC3339, strings.TrimSpace(sitemapURLEntry.LastModified)); err == nil {
			entry.LastModified = &parsedTime
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// FilterByPathPrefix keeps only the entries whose URL path contains the given
// segment, which is how product URLs are separated from category and content
// pages in a single combined sitemap.
func FilterByPathPrefix(entries []Entry, pathSegment string) []Entry {
	filtered := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if strings.Contains(entry.Location, pathSegment) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

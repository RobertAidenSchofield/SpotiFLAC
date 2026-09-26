package backend

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type SpotifyHomeItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"` // "album", "playlist", "artist", "track"
	Subtitle    string `json:"subtitle"`
	Images      string `json:"images"`
	ExternalURL string `json:"external_urls"`
	ReleaseDate string `json:"release_date,omitempty"`
}

type SpotifyHomeSection struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Items       []SpotifyHomeItem `json:"items"`
}

type SpotifyHomeFeedResponse struct {
	Greeting   string               `json:"greeting"`
	QuickPicks []SpotifyHomeItem    `json:"quick_picks"`
	Sections   []SpotifyHomeSection `json:"sections"`
}

var (
	spotifyHomeFeedCacheMu sync.Mutex
	cachedHomeFeed         *SpotifyHomeFeedResponse
	cachedHomeFeedTime     time.Time
)

func getGreeting() string {
	hour := time.Now().Hour()
	if hour >= 5 && hour < 12 {
		return "Good morning"
	} else if hour >= 12 && hour < 18 {
		return "Good afternoon"
	}
	return "Good evening"
}

// GetSpotifyHomeFeed returns curated sections for the Home screen discovery feed
func GetSpotifyHomeFeed(ctx context.Context, forceRefresh bool) (*SpotifyHomeFeedResponse, error) {
	spotifyHomeFeedCacheMu.Lock()
	if !forceRefresh && cachedHomeFeed != nil && time.Since(cachedHomeFeedTime) < 15*time.Minute {
		feedCopy := *cachedHomeFeed
		feedCopy.Greeting = getGreeting()
		spotifyHomeFeedCacheMu.Unlock()
		return &feedCopy, nil
	}
	spotifyHomeFeedCacheMu.Unlock()

	var wg sync.WaitGroup
	var mu sync.Mutex

	var quickPicks []SpotifyHomeItem
	var newReleases []SpotifyHomeItem
	var trendingAlbums []SpotifyHomeItem
	var topPlaylists []SpotifyHomeItem
	var popularArtists []SpotifyHomeItem

	// 1. Quick Picks (Top Playlists)
	wg.Add(1)
	go func() {
		defer wg.Done()
		res, err := SearchSpotifyByType(ctx, "Top 50", "playlist", 6, 0)
		if err == nil && len(res) > 0 {
			items := make([]SpotifyHomeItem, 0, len(res))
			for _, r := range res {
				items = append(items, SpotifyHomeItem{
					ID:          r.ID,
					Name:        r.Name,
					Type:        "playlist",
					Subtitle:    r.Owner,
					Images:      r.Images,
					ExternalURL: r.ExternalURL,
				})
			}
			mu.Lock()
			quickPicks = items
			mu.Unlock()
		}
	}()

	// 2. New Releases (Albums)
	wg.Add(1)
	go func() {
		defer wg.Done()
		res, err := SearchSpotifyByType(ctx, "tag:new", "album", 12, 0)
		if err == nil && len(res) > 0 {
			items := make([]SpotifyHomeItem, 0, len(res))
			for _, r := range res {
				items = append(items, SpotifyHomeItem{
					ID:          r.ID,
					Name:        r.Name,
					Type:        "album",
					Subtitle:    r.Artists,
					Images:      r.Images,
					ExternalURL: r.ExternalURL,
					ReleaseDate: r.ReleaseDate,
				})
			}
			mu.Lock()
			newReleases = items
			mu.Unlock()
		}
	}()

	// 3. Trending & Year Releases
	wg.Add(1)
	go func() {
		defer wg.Done()
		yearQuery := fmt.Sprintf("year:%d", time.Now().Year())
		res, err := SearchSpotifyByType(ctx, yearQuery, "album", 12, 0)
		if err != nil || len(res) == 0 {
			res, _ = SearchSpotifyByType(ctx, "Top Albums", "album", 12, 0)
		}
		if len(res) > 0 {
			items := make([]SpotifyHomeItem, 0, len(res))
			for _, r := range res {
				items = append(items, SpotifyHomeItem{
					ID:          r.ID,
					Name:        r.Name,
					Type:        "album",
					Subtitle:    r.Artists,
					Images:      r.Images,
					ExternalURL: r.ExternalURL,
					ReleaseDate: r.ReleaseDate,
				})
			}
			mu.Lock()
			trendingAlbums = items
			mu.Unlock()
		}
	}()

	// 4. Featured Curated Playlists
	wg.Add(1)
	go func() {
		defer wg.Done()
		res, err := SearchSpotifyByType(ctx, "Today's Top Hits", "playlist", 12, 0)
		if err == nil && len(res) > 0 {
			items := make([]SpotifyHomeItem, 0, len(res))
			for _, r := range res {
				items = append(items, SpotifyHomeItem{
					ID:          r.ID,
					Name:        r.Name,
					Type:        "playlist",
					Subtitle:    r.Owner,
					Images:      r.Images,
					ExternalURL: r.ExternalURL,
				})
			}
			mu.Lock()
			topPlaylists = items
			mu.Unlock()
		}
	}()

	// 5. Popular Artists
	wg.Add(1)
	go func() {
		defer wg.Done()
		res, err := SearchSpotifyByType(ctx, "The Weeknd", "artist", 6, 0)
		if err == nil && len(res) > 0 {
			items := make([]SpotifyHomeItem, 0, len(res))
			for _, r := range res {
				items = append(items, SpotifyHomeItem{
					ID:          r.ID,
					Name:        r.Name,
					Type:        "artist",
					Subtitle:    "Artist",
					Images:      r.Images,
					ExternalURL: r.ExternalURL,
				})
			}
			mu.Lock()
			popularArtists = items
			mu.Unlock()
		}
	}()

	wg.Wait()

	sections := make([]SpotifyHomeSection, 0)

	if len(newReleases) > 0 {
		sections = append(sections, SpotifyHomeSection{
			ID:          "new-releases",
			Title:       "New Releases",
			Description: "Fresh new albums, EPs, and singles just released",
			Items:       newReleases,
		})
	}

	if len(trendingAlbums) > 0 {
		sections = append(sections, SpotifyHomeSection{
			ID:          "trending-albums",
			Title:       "Trending Albums",
			Description: "The most popular and trending albums right now",
			Items:       trendingAlbums,
		})
	}

	if len(topPlaylists) > 0 {
		sections = append(sections, SpotifyHomeSection{
			ID:          "featured-playlists",
			Title:       "Featured Playlists",
			Description: "Curated playlists by Spotify and listeners worldwide",
			Items:       topPlaylists,
		})
	}

	if len(popularArtists) > 0 {
		sections = append(sections, SpotifyHomeSection{
			ID:          "popular-artists",
			Title:       "Popular Artists",
			Description: "Explore complete discographies from top artists",
			Items:       popularArtists,
		})
	}

	feed := &SpotifyHomeFeedResponse{
		Greeting:   getGreeting(),
		QuickPicks: quickPicks,
		Sections:   sections,
	}

	spotifyHomeFeedCacheMu.Lock()
	cachedHomeFeed = feed
	cachedHomeFeedTime = time.Now()
	spotifyHomeFeedCacheMu.Unlock()

	return feed, nil
}

// GetSpotifyCategoryFeed returns albums and playlists for a specific genre/category
func GetSpotifyCategoryFeed(ctx context.Context, genre string) ([]SpotifyHomeItem, error) {
	genre = strings.TrimSpace(genre)
	if genre == "" {
		genre = "Pop"
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var items []SpotifyHomeItem

	// Search albums in this genre
	wg.Add(1)
	go func() {
		defer wg.Done()
		query := fmt.Sprintf("genre:%s", strings.ToLower(genre))
		res, err := SearchSpotifyByType(ctx, query, "album", 10, 0)
		if err != nil || len(res) == 0 {
			res, _ = SearchSpotifyByType(ctx, genre, "album", 10, 0)
		}
		if len(res) > 0 {
			mu.Lock()
			for _, r := range res {
				items = append(items, SpotifyHomeItem{
					ID:          r.ID,
					Name:        r.Name,
					Type:        "album",
					Subtitle:    r.Artists,
					Images:      r.Images,
					ExternalURL: r.ExternalURL,
					ReleaseDate: r.ReleaseDate,
				})
			}
			mu.Unlock()
		}
	}()

	// Search playlists in this genre
	wg.Add(1)
	go func() {
		defer wg.Done()
		res, err := SearchSpotifyByType(ctx, genre, "playlist", 8, 0)
		if err == nil && len(res) > 0 {
			mu.Lock()
			for _, r := range res {
				items = append(items, SpotifyHomeItem{
					ID:          r.ID,
					Name:        r.Name,
					Type:        "playlist",
					Subtitle:    r.Owner,
					Images:      r.Images,
					ExternalURL: r.ExternalURL,
				})
			}
			mu.Unlock()
		}
	}()

	wg.Wait()

	return items, nil
}

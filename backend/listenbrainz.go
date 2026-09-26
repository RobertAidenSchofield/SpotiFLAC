package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type ExploTrack struct {
	Title         string `json:"title"`
	Artist        string `json:"artist"`
	Album         string `json:"album,omitempty"`
	RecordingMBID string `json:"recording_mbid,omitempty"`
	SpotifyID     string `json:"spotify_id,omitempty"`
	DurationMs    int    `json:"duration_ms,omitempty"`
	ImageURL      string `json:"image_url,omitempty"`
	ReleaseDate   string `json:"release_date,omitempty"`
	PreviewURL    string `json:"preview_url,omitempty"`
}

type ExploPlaylist struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Creator     string       `json:"creator"`
	TrackCount  int          `json:"track_count"`
	Tracks      []ExploTrack `json:"tracks"`
}

type ListenBrainzClient struct {
	httpClient *http.Client
}

func NewListenBrainzClient() *ListenBrainzClient {
	return &ListenBrainzClient{
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

// jspfPlaylistWrapper matches JSPF JSON Shareable Playlist Format
type jspfPlaylistContainer struct {
	Playlists []struct {
		Playlist jspfPlaylist `json:"playlist"`
	} `json:"playlists"`
}

type jspfSinglePlaylistContainer struct {
	Playlist jspfPlaylist `json:"playlist"`
}

type jspfPlaylist struct {
	Title      string      `json:"title"`
	Creator    string      `json:"creator"`
	Annotation string      `json:"annotation"`
	Identifier string      `json:"identifier"`
	Track      []jspfTrack `json:"track"`
	Extension  struct {
		MusicBrainz struct {
			Identifier string `json:"identifier"`
			CreatedFor string `json:"created_for"`
			Creator    string `json:"creator"`
		} `json:"https://musicbrainz.org/doc/jspf#playlist"`
	} `json:"extension"`
}

type jspfTrack struct {
	Title      string `json:"title"`
	Creator    string `json:"creator"`
	Album      string `json:"album"`
	Identifier string `json:"identifier"`
	Duration   int    `json:"duration"`
	Extension  struct {
		TrackExt struct {
			AddedBy           string   `json:"added_by"`
			ArtistIdentifiers []string `json:"artist_identifiers"`
		} `json:"https://musicbrainz.org/doc/jspf#track"`
	} `json:"extension"`
}

func extractPlaylistID(rawID string) string {
	rawID = strings.TrimSpace(rawID)
	if strings.HasPrefix(rawID, "https://listenbrainz.org/playlist/") {
		rawID = strings.TrimPrefix(rawID, "https://listenbrainz.org/playlist/")
	}
	return strings.Trim(rawID, "/ ")
}

// FetchUserPlaylists gets the user's "created for you" playlists (Weekly Exploration, Jams) and public playlists
func (c *ListenBrainzClient) FetchUserPlaylists(username string) ([]ExploPlaylist, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("ListenBrainz username cannot be empty")
	}

	resultList := make([]ExploPlaylist, 0)
	seenIDs := make(map[string]bool)

	// 1. Fetch created-for playlists (Weekly Exploration, Daily Jams, etc.)
	createdForURL := fmt.Sprintf("https://api.listenbrainz.org/1/user/%s/playlists/createdfor", url.PathEscape(username))
	req, err := http.NewRequest(http.MethodGet, createdForURL, nil)
	if err == nil {
		req.Header.Set("User-Agent", "SpotiFLAC/1.0 (https://github.com/spotbye/SpotiFLAC)")
		resp, fetchErr := c.httpClient.Do(req)
		if fetchErr == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var container jspfPlaylistContainer
			if jsonErr := json.NewDecoder(resp.Body).Decode(&container); jsonErr == nil {
				for _, p := range container.Playlists {
					id := p.Playlist.Extension.MusicBrainz.Identifier
					if id == "" {
						id = extractPlaylistID(p.Playlist.Identifier)
					}
					if id != "" && !seenIDs[id] {
						seenIDs[id] = true
						tracks := parseJSPFTracks(p.Playlist.Track)
						resultList = append(resultList, ExploPlaylist{
							ID:          id,
							Title:       p.Playlist.Title,
							Description: p.Playlist.Annotation,
							Creator:     p.Playlist.Creator,
							TrackCount:  len(tracks),
							Tracks:      tracks,
						})
					}
				}
			}
		} else if resp != nil {
			resp.Body.Close()
		}
	}

	// 2. Fetch user's own playlists
	userPlaylistsURL := fmt.Sprintf("https://api.listenbrainz.org/1/user/%s/playlists", url.PathEscape(username))
	req2, err2 := http.NewRequest(http.MethodGet, userPlaylistsURL, nil)
	if err2 == nil {
		req2.Header.Set("User-Agent", "SpotiFLAC/1.0 (https://github.com/spotbye/SpotiFLAC)")
		resp2, fetchErr2 := c.httpClient.Do(req2)
		if fetchErr2 == nil && resp2.StatusCode == http.StatusOK {
			defer resp2.Body.Close()
			var container jspfPlaylistContainer
			if jsonErr := json.NewDecoder(resp2.Body).Decode(&container); jsonErr == nil {
				for _, p := range container.Playlists {
					id := p.Playlist.Extension.MusicBrainz.Identifier
					if id == "" {
						id = extractPlaylistID(p.Playlist.Identifier)
					}
					if id != "" && !seenIDs[id] {
						seenIDs[id] = true
						tracks := parseJSPFTracks(p.Playlist.Track)
						resultList = append(resultList, ExploPlaylist{
							ID:          id,
							Title:       p.Playlist.Title,
							Description: p.Playlist.Annotation,
							Creator:     p.Playlist.Creator,
							TrackCount:  len(tracks),
							Tracks:      tracks,
						})
					}
				}
			}
		} else if resp2 != nil {
			resp2.Body.Close()
		}
	}

	return resultList, nil
}

// FetchPlaylistTracks retrieves full track list for a given ListenBrainz playlist ID or URL
func (c *ListenBrainzClient) FetchPlaylistTracks(playlistID string) (*ExploPlaylist, error) {
	mbid := extractPlaylistID(playlistID)
	if mbid == "" {
		return nil, fmt.Errorf("invalid playlist ID or URL")
	}

	apiURL := fmt.Sprintf("https://api.listenbrainz.org/1/playlist/%s", url.PathEscape(mbid))
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SpotiFLAC/1.0 (https://github.com/spotbye/SpotiFLAC)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch playlist: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("ListenBrainz returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var container jspfSinglePlaylistContainer
	if err := json.NewDecoder(resp.Body).Decode(&container); err != nil {
		return nil, fmt.Errorf("failed to parse playlist response: %w", err)
	}

	tracks := parseJSPFTracks(container.Playlist.Track)
	return &ExploPlaylist{
		ID:          mbid,
		Title:       container.Playlist.Title,
		Description: container.Playlist.Annotation,
		Creator:     container.Playlist.Creator,
		TrackCount:  len(tracks),
		Tracks:      tracks,
	}, nil
}

// FetchRecommendations generates discovery tracks for a user or exploration query
func (c *ListenBrainzClient) FetchRecommendations(username string, recType string, query string) (*ExploPlaylist, error) {
	username = strings.TrimSpace(username)
	recType = strings.TrimSpace(strings.ToLower(recType))

	// If user provided a specific playlist ID or URL in query, fetch it directly
	if strings.Contains(query, "playlist") || strings.Contains(query, "-") && len(query) == 36 {
		return c.FetchPlaylistTracks(query)
	}

	// 1. Try to find user's Weekly Exploration / Jams if username is provided
	if username != "" && (recType == "exploration" || recType == "jams" || recType == "default") {
		playlists, err := c.FetchUserPlaylists(username)
		if err == nil && len(playlists) > 0 {
			targetKeyword := "exploration"
			if recType == "jams" {
				targetKeyword = "jam"
			}
			for _, p := range playlists {
				if strings.Contains(strings.ToLower(p.Title), targetKeyword) && len(p.Tracks) > 0 {
					return &p, nil
				}
			}
			// If exact keyword match wasn't found, return the first created playlist with tracks
			for _, p := range playlists {
				if len(p.Tracks) > 0 {
					return &p, nil
				}
			}
		}

		// Fallback: Query ListenBrainz Collaborative Filtering recommendation API
		cfURL := fmt.Sprintf("https://api.listenbrainz.org/1/cf/recommendation/user/%s/recordings?count=30", url.PathEscape(username))
		req, cfErr := http.NewRequest(http.MethodGet, cfURL, nil)
		if cfErr == nil {
			req.Header.Set("User-Agent", "SpotiFLAC/1.0 (https://github.com/spotbye/SpotiFLAC)")
			resp, doErr := c.httpClient.Do(req)
			if doErr == nil && resp.StatusCode == http.StatusOK {
				defer resp.Body.Close()
				var cfResp struct {
					Payload struct {
						Recordings []struct {
							RecordingMBID string `json:"recording_mbid"`
							TrackName     string `json:"track_name"`
							ArtistName    string `json:"artist_name"`
							ReleaseName   string `json:"release_name"`
						} `json:"recordings"`
					} `json:"payload"`
				}
				if decErr := json.NewDecoder(resp.Body).Decode(&cfResp); decErr == nil && len(cfResp.Payload.Recordings) > 0 {
					tracks := make([]ExploTrack, 0, len(cfResp.Payload.Recordings))
					for _, r := range cfResp.Payload.Recordings {
						if r.TrackName != "" && r.ArtistName != "" {
							tracks = append(tracks, ExploTrack{
								Title:         r.TrackName,
								Artist:        r.ArtistName,
								Album:         r.ReleaseName,
								RecordingMBID: r.RecordingMBID,
							})
						}
					}
					if len(tracks) > 0 {
						return &ExploPlaylist{
							ID:          fmt.Sprintf("cf-%s-%d", username, time.Now().Unix()),
							Title:       fmt.Sprintf("ListenBrainz Discovery for %s", username),
							Description: "Personalized algorithmic recommendations from ListenBrainz",
							Creator:     "ListenBrainz CF",
							TrackCount:  len(tracks),
							Tracks:      tracks,
						}, nil
					}
				}
			} else if resp != nil {
				resp.Body.Close()
			}
		}
	}

	// 2. Artist Radio / Query exploration mode
	if query != "" {
		return c.generateArtistRadio(query)
	}

	return nil, fmt.Errorf("no recommendations available. Try entering an artist name for Artist Radio, or a ListenBrainz username with scrobbles")
}

func (c *ListenBrainzClient) generateArtistRadio(artistOrGenre string) (*ExploPlaylist, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	// Search Spotify for tracks matching this artist/genre seed
	searchQuery := artistOrGenre
	if !strings.HasPrefix(strings.ToLower(artistOrGenre), "artist:") && !strings.HasPrefix(strings.ToLower(artistOrGenre), "genre:") {
		searchQuery = fmt.Sprintf("artist:%s", artistOrGenre)
	}

	results, err := SearchSpotifyByType(ctx, searchQuery, "track", 35, 0)
	if err != nil || len(results) == 0 {
		// Fallback to general track search
		results, err = SearchSpotifyByType(ctx, artistOrGenre, "track", 35, 0)
		if err != nil || len(results) == 0 {
			return nil, fmt.Errorf("no tracks found for '%s'", artistOrGenre)
		}
	}

	tracks := make([]ExploTrack, 0, len(results))
	for _, res := range results {
		tracks = append(tracks, ExploTrack{
			Title:       res.Name,
			Artist:      res.Artists,
			Album:       res.AlbumName,
			SpotifyID:   res.ID,
			DurationMs:  res.Duration,
			ImageURL:    res.Images,
			ReleaseDate: res.ReleaseDate,
		})
	}

	return &ExploPlaylist{
		ID:          fmt.Sprintf("radio-%s-%d", strings.ReplaceAll(artistOrGenre, " ", "-"), time.Now().Unix()),
		Title:       fmt.Sprintf("%s Radio", strings.Title(artistOrGenre)),
		Description: fmt.Sprintf("Curated discovery playlist inspired by %s", artistOrGenre),
		Creator:     "SpotiFLAC Explo",
		TrackCount:  len(tracks),
		Tracks:      tracks,
	}, nil
}

func parseJSPFTracks(tracks []jspfTrack) []ExploTrack {
	parsed := make([]ExploTrack, 0, len(tracks))
	for _, t := range tracks {
		title := strings.TrimSpace(t.Title)
		artist := strings.TrimSpace(t.Creator)
		if title == "" || artist == "" {
			continue
		}
		mbid := ""
		if strings.HasPrefix(t.Identifier, "https://musicbrainz.org/recording/") {
			mbid = strings.TrimPrefix(t.Identifier, "https://musicbrainz.org/recording/")
			mbid = strings.Trim(mbid, "/ ")
		}
		parsed = append(parsed, ExploTrack{
			Title:         title,
			Artist:        artist,
			Album:         t.Album,
			RecordingMBID: mbid,
			DurationMs:    t.Duration,
		})
	}
	return parsed
}

// ResolveExploTracksToSpotify searches Spotify for each track in parallel to resolve rich metadata
func ResolveExploTracksToSpotify(ctx context.Context, tracks []ExploTrack) ([]TrackMetadata, error) {
	if len(tracks) == 0 {
		return []TrackMetadata{}, nil
	}

	resolved := make([]TrackMetadata, len(tracks))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 6) // Max 6 concurrent Spotify searches

	for i, track := range tracks {
		wg.Add(1)
		go func(idx int, t ExploTrack) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// If Spotify ID already exists and has cover image, construct directly
			if t.SpotifyID != "" && t.ImageURL != "" {
				resolved[idx] = TrackMetadata{
					SpotifyID:   t.SpotifyID,
					Name:        t.Title,
					Artists:     t.Artist,
					AlbumName:   t.Album,
					DurationMS:  t.DurationMs,
					Images:      t.ImageURL,
					ReleaseDate: t.ReleaseDate,
					PreviewURL:  t.PreviewURL,
					TrackNumber: idx + 1,
					ExternalURL: fmt.Sprintf("https://open.spotify.com/track/%s", t.SpotifyID),
				}
				return
			}

			// Search Spotify for this track
			searchQuery := fmt.Sprintf("%s %s", t.Title, t.Artist)
			searchCtx, searchCancel := context.WithTimeout(ctx, 8*time.Second)
			defer searchCancel()

			results, err := SearchSpotifyByType(searchCtx, searchQuery, "track", 1, 0)
			if err == nil && len(results) > 0 {
				top := results[0]
				resolved[idx] = TrackMetadata{
					SpotifyID:   top.ID,
					Name:        top.Name,
					Artists:     top.Artists,
					AlbumName:   top.AlbumName,
					DurationMS:  top.Duration,
					Images:      top.Images,
					ReleaseDate: top.ReleaseDate,
					TrackNumber: idx + 1,
					ExternalURL: top.ExternalURL,
				}
				return
			}

			// Fallback: Return raw track metadata without Spotify ID
			resolved[idx] = TrackMetadata{
				Name:        t.Title,
				Artists:     t.Artist,
				AlbumName:   t.Album,
				DurationMS:  t.DurationMs,
				Images:      t.ImageURL,
				ReleaseDate: t.ReleaseDate,
				TrackNumber: idx + 1,
			}
		}(i, track)
	}

	wg.Wait()
	return resolved, nil
}

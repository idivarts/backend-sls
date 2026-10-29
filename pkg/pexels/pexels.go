// Package pexels is a thin client for Pexels photo search.
//
// Chosen over Unsplash deliberately: Unsplash's API terms route machine-learning
// and AI uses to a separate agreement, and Unsplash+ forbids them outright,
// while Pexels permits both placing a photo in generated work and using one as
// model input. Both of those are things this product does.
//
// The client is read-only and stateless. It is NOT a mediagen driver yet — when
// pkg/mediagen lands it absorbs this as its stock-search driver, which is a
// change at one call site. Keeping it standalone now is what lets real
// photography ship without waiting for the provider seam.
package pexels

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ErrNotConfigured means no API key is present. Callers treat this as "skip
// stock photography", never as a failure — a design without a stock photo is
// still a design, and a missing key is an ops gap, not a user error.
var ErrNotConfigured = errors.New("pexels: PEXELS_API_KEY is not set")

const (
	baseURL = "https://api.pexels.com/v1/search"
	timeout = 12 * time.Second
)

// Photo is the subset of a Pexels result this product uses.
type Photo struct {
	ID int `json:"id"`
	// URL is the hosted image to place. Always copy the bytes to our own
	// storage before persisting it into a design — see the package note on
	// why in the resolver.
	URL string `json:"url"`
	// Alt is Pexels' own description, used as the img alt text.
	Alt string `json:"alt"`
	// Photographer and PhotographerURL are the attribution that has to travel
	// with the photo and surface on publish. Pexels does not legally require
	// attribution, but dropping it is the kind of thing that is cheap to keep
	// and expensive to retrofit once designs are in the wild.
	Photographer    string `json:"photographer"`
	PhotographerURL string `json:"photographerUrl"`
	// PageURL is the photo's page on Pexels, for the credit link.
	PageURL string `json:"pageUrl"`
}

// Orientation values the API accepts. Anything else is sent as no filter.
const (
	OrientationPortrait  = "portrait"
	OrientationLandscape = "landscape"
	OrientationSquare    = "square"
)

func apiKey() string { return strings.TrimSpace(os.Getenv("PEXELS_API_KEY")) }

// IsConfigured reports whether stock search can run at all.
func IsConfigured() bool { return apiKey() != "" }

type apiResponse struct {
	Photos []struct {
		ID              int    `json:"id"`
		URL             string `json:"url"`
		Alt             string `json:"alt"`
		Photographer    string `json:"photographer"`
		PhotographerURL string `json:"photographer_url"`
		Src             struct {
			Original  string `json:"original"`
			Large2x   string `json:"large2x"`
			Large     string `json:"large"`
			Medium    string `json:"medium"`
			Portrait  string `json:"portrait"`
			Landscape string `json:"landscape"`
		} `json:"src"`
	} `json:"photos"`
}

// Search returns up to `count` photos matching the query.
//
// `orientation` is one of the Orientation constants, or "" for no filter.
func Search(ctx context.Context, query, orientation string, count int) ([]Photo, error) {
	key := apiKey()
	if key == "" {
		return nil, ErrNotConfigured
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("pexels: empty query")
	}
	if count < 1 {
		count = 1
	}
	if count > 15 {
		count = 15
	}

	q := url.Values{}
	q.Set("query", query)
	q.Set("per_page", fmt.Sprint(count))
	switch orientation {
	case OrientationPortrait, OrientationLandscape, OrientationSquare:
		q.Set("orientation", orientation)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pexels: search returned %d", resp.StatusCode)
	}

	var body apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	out := make([]Photo, 0, len(body.Photos))
	for _, p := range body.Photos {
		// large2x first: `original` is often many megabytes, which is wasted
		// bytes for something that renders at 1080px, and `medium` is too soft
		// once it fills a slide.
		src := firstNonEmpty(p.Src.Large2x, p.Src.Large, p.Src.Original, p.Src.Medium)
		if src == "" {
			continue
		}
		out = append(out, Photo{
			ID:              p.ID,
			URL:             src,
			Alt:             p.Alt,
			Photographer:    p.Photographer,
			PhotographerURL: p.PhotographerURL,
			PageURL:         p.URL,
		})
	}
	return out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

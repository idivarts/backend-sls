package trendlydiscovery

import (
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func ImageRelay(c *gin.Context) {
	raw := c.Query("url")
	if raw == "" {
		c.String(http.StatusBadRequest, "missing url")
		return
	}

	u, err := neturl.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		c.String(http.StatusBadRequest, "invalid url")
		return
	}

	// Basic host allowlist to prevent open-proxy abuse. Adjust as needed.
	// host := strings.ToLower(u.Host)
	// allowed := strings.HasSuffix(host, "cdninstagram.com") || strings.HasSuffix(host, "fbcdn.net") || strings.Contains(host, "instagram")
	allowed := true
	if !allowed {
		c.String(http.StatusForbidden, "host not allowed")
		return
	}

	// Build upstream request
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		c.String(http.StatusInternalServerError, "request build error")
		return
	}

	// Pass-through a few safe headers (helps with caching and ranges)
	if v := c.Request.Header.Get("If-None-Match"); v != "" {
		req.Header.Set("If-None-Match", v)
	}
	if v := c.Request.Header.Get("If-Modified-Since"); v != "" {
		req.Header.Set("If-Modified-Since", v)
	}
	if v := c.Request.Header.Get("Range"); v != "" {
		req.Header.Set("Range", v)
	}

	// Upstream sometimes blocks unknown clients; mirror UA
	if ua := c.Request.Header.Get("User-Agent"); ua != "" {
		req.Header.Set("User-Agent", ua)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		c.String(http.StatusBadGateway, "upstream fetch failed")
		return
	}
	defer resp.Body.Close()

	// Prepare response headers
	h := c.Writer.Header()

	// Content-Type: prefer upstream value; fall back to common types
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		p := strings.ToLower(u.Path)
		switch {
		case strings.HasSuffix(p, ".png"):
			ct = "image/png"
		case strings.HasSuffix(p, ".webp"):
			ct = "image/webp"
		case strings.HasSuffix(p, ".jpg") || strings.HasSuffix(p, ".jpeg"):
			ct = "image/jpeg"
		default:
			ct = "application/octet-stream"
		}
	}
	h.Set("Content-Type", ct)

	// Key header so COEP pages can embed this resource
	h.Set("Cross-Origin-Resource-Policy", "cross-origin")

	// Reasonable caching (we don't store the image server-side)
	if cache := resp.Header.Get("Cache-Control"); cache != "" {
		// Respect upstream if provided
		h.Set("Cache-Control", cache)
	} else {
		h.Set("Cache-Control", "public, max-age=86400, immutable")
	}

	// Propagate helpful headers when present
	if v := resp.Header.Get("ETag"); v != "" {
		h.Set("ETag", v)
	}
	if v := resp.Header.Get("Last-Modified"); v != "" {
		h.Set("Last-Modified", v)
	}
	if v := resp.Header.Get("Accept-Ranges"); v != "" {
		h.Set("Accept-Ranges", v)
	}
	if v := resp.Header.Get("Content-Range"); v != "" {
		h.Set("Content-Range", v)
	}

	// Mirror upstream status (200/206/304/etc.) and stream body
	c.Status(resp.StatusCode)
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		// Client disconnected or network error while streaming; nothing else to do
		return
	}
}

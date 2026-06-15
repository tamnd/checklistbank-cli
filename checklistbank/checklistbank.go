// Package checklistbank is the library behind the checklistbank command line:
// the HTTP client, request shaping, and the typed data models for ChecklistBank.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
// Build your endpoint calls and JSON decoding on top of it.
package checklistbank

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Host is the site this client talks to, and the host the URI driver in
// domain.go claims.
const Host = "api.checklistbank.org"

// BaseURL is the root every request is built from.
const BaseURL = "https://" + Host

// DefaultUserAgent identifies the client to ChecklistBank. A real, honest
// User-Agent is both polite and the thing most likely to keep you unblocked.
const DefaultUserAgent = "checklistbank-cli/0.1.0"

// Wire types — match the ChecklistBank JSON shapes exactly.

type wireName struct {
	ID              string `json:"id"`
	ScientificName  string `json:"scientificName"`
	Rank            string `json:"rank"`
	Genus           string `json:"genus"`
	SpecificEpithet string `json:"specificEpithet"`
	Code            string `json:"code"`
}

type wireUsage struct {
	ID         string    `json:"id"`
	DatasetKey int       `json:"datasetKey"`
	Status     string    `json:"status"`
	Label      string    `json:"label"`
	Name       wireName  `json:"name"`
}

type wireClassEntry struct {
	Name string `json:"name"`
	Rank string `json:"rank"`
}

type wireResult struct {
	Usage          wireUsage        `json:"usage"`
	Classification []wireClassEntry `json:"classification"`
}

type wireSearchResp struct {
	Total  int          `json:"total"`
	Result []wireResult `json:"result"`
}

type wireDataset struct {
	Key   int    `json:"key"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

type wireDatasetResp struct {
	Total  int           `json:"total"`
	Result []wireDataset `json:"result"`
}

// Taxon is the public record type: one taxonomic name from ChecklistBank.
type Taxon struct {
	ID             string `json:"id"              kit:"id"`
	ScientificName string `json:"scientific_name"`
	Rank           string `json:"rank"`
	Genus          string `json:"genus"`
	Species        string `json:"species"`
	Status         string `json:"status"`
	DatasetKey     int    `json:"dataset_key"`
	Code           string `json:"code"`
	Label          string `json:"label"`
	Classification string `json:"classification"`
}

// Checklist is the public record type: one checklist/dataset from ChecklistBank.
type Checklist struct {
	ID    string `json:"id"    kit:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
	Size  int    `json:"size"`
}

func taxonFromWire(r wireResult) *Taxon {
	parts := make([]string, 0, len(r.Classification))
	for _, c := range r.Classification {
		if c.Name != "" {
			parts = append(parts, c.Name)
		}
	}
	return &Taxon{
		ID:             r.Usage.ID,
		ScientificName: r.Usage.Name.ScientificName,
		Rank:           r.Usage.Name.Rank,
		Genus:          r.Usage.Name.Genus,
		Species:        r.Usage.Name.SpecificEpithet,
		Status:         r.Usage.Status,
		DatasetKey:     r.Usage.DatasetKey,
		Code:           r.Usage.Name.Code,
		Label:          r.Usage.Label,
		Classification: strings.Join(parts, " > "),
	}
}

func taxonFromUsage(u wireUsage) *Taxon {
	return &Taxon{
		ID:             u.ID,
		ScientificName: u.Name.ScientificName,
		Rank:           u.Name.Rank,
		Genus:          u.Name.Genus,
		Species:        u.Name.SpecificEpithet,
		Status:         u.Status,
		DatasetKey:     u.DatasetKey,
		Code:           u.Name.Code,
		Label:          u.Label,
	}
}

// Client talks to ChecklistBank over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults: a 30s timeout, a 300ms
// minimum gap between requests, and three retries on transient errors.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		UserAgent: DefaultUserAgent,
		Rate:      300 * time.Millisecond,
		Retries:   3,
	}
}

// SearchTaxa queries /nameusage/search and returns matching taxa plus the total count.
func (c *Client) SearchTaxa(ctx context.Context, query string, limit, offset int) ([]Taxon, int, error) {
	if limit <= 0 {
		limit = 20
	}
	u := BaseURL + "/nameusage/search?q=" + url.QueryEscape(query) +
		"&limit=" + strconv.Itoa(limit) +
		"&offset=" + strconv.Itoa(offset)

	body, err := c.get(ctx, u)
	if err != nil {
		return nil, 0, err
	}
	return parseSearchResp(body)
}

// GetTaxon fetches a single taxon by dataset key and usage ID.
func (c *Client) GetTaxon(ctx context.Context, datasetKey int, id string) (*Taxon, error) {
	u := BaseURL + "/dataset/" + strconv.Itoa(datasetKey) + "/taxon/" + url.PathEscape(id)

	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	return parseTaxonResp(body)
}

// ListDatasets queries /dataset and returns matching checklists plus the total count.
func (c *Client) ListDatasets(ctx context.Context, query string, limit int) ([]Checklist, int, error) {
	if limit <= 0 {
		limit = 20
	}
	u := BaseURL + "/dataset?limit=" + strconv.Itoa(limit)
	if query != "" {
		u += "&q=" + url.QueryEscape(query)
	}

	body, err := c.get(ctx, u)
	if err != nil {
		return nil, 0, err
	}
	return parseDatasetResp(body)
}

// get fetches a URL and returns the response body, with pacing and retries.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// parseSearchResp decodes a raw /nameusage/search JSON response body into taxa + total.
func parseSearchResp(body []byte) ([]Taxon, int, error) {
	var resp wireSearchResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("decode search response: %w", err)
	}
	taxa := make([]Taxon, 0, len(resp.Result))
	for _, r := range resp.Result {
		taxa = append(taxa, *taxonFromWire(r))
	}
	return taxa, resp.Total, nil
}

// parseTaxonResp decodes a raw single usage JSON response body into a Taxon.
func parseTaxonResp(body []byte) (*Taxon, error) {
	var u wireUsage
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("decode taxon response: %w", err)
	}
	return taxonFromUsage(u), nil
}

// parseDatasetResp decodes a raw /dataset JSON response body into checklists + total.
func parseDatasetResp(body []byte) ([]Checklist, int, error) {
	var resp wireDatasetResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("decode dataset response: %w", err)
	}
	lists := make([]Checklist, 0, len(resp.Result))
	for _, d := range resp.Result {
		lists = append(lists, Checklist{
			ID:    strconv.Itoa(d.Key),
			Title: d.Title,
			Type:  d.Type,
		})
	}
	return lists, resp.Total, nil
}

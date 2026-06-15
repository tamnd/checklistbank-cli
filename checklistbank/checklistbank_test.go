package checklistbank

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSearchTaxa(t *testing.T) {
	const resp = `{
		"offset": 0,
		"limit": 20,
		"total": 432913583,
		"empty": false,
		"result": [
			{
				"usage": {
					"id": "abc123",
					"datasetKey": 9,
					"status": "accepted",
					"label": "Homo sapiens Linnaeus, 1758",
					"name": {
						"id": "n1",
						"scientificName": "Homo sapiens",
						"rank": "species",
						"genus": "Homo",
						"specificEpithet": "sapiens",
						"code": "zoological"
					}
				},
				"classification": [
					{"name": "Animalia", "rank": "kingdom"},
					{"name": "Chordata", "rank": "phylum"},
					{"name": "Mammalia", "rank": "class"}
				]
			},
			{
				"usage": {
					"id": "def456",
					"datasetKey": 9,
					"status": "synonym",
					"label": "Homo neanderthalensis",
					"name": {
						"id": "n2",
						"scientificName": "Homo neanderthalensis",
						"rank": "species",
						"genus": "Homo",
						"specificEpithet": "neanderthalensis",
						"code": "zoological"
					}
				},
				"classification": [
					{"name": "Animalia", "rank": "kingdom"}
				]
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resp))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	taxa, total, err := searchTaxaAt(context.Background(), c, srv.URL, "Homo sapiens", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 432913583 {
		t.Errorf("total = %d, want 432913583", total)
	}
	if len(taxa) != 2 {
		t.Fatalf("len(taxa) = %d, want 2", len(taxa))
	}
	if taxa[0].ScientificName != "Homo sapiens" {
		t.Errorf("ScientificName = %q, want Homo sapiens", taxa[0].ScientificName)
	}
	if taxa[0].Classification != "Animalia > Chordata > Mammalia" {
		t.Errorf("Classification = %q, want Animalia > Chordata > Mammalia", taxa[0].Classification)
	}
	if taxa[0].Status != "accepted" {
		t.Errorf("Status = %q, want accepted", taxa[0].Status)
	}
	if taxa[1].ScientificName != "Homo neanderthalensis" {
		t.Errorf("ScientificName = %q, want Homo neanderthalensis", taxa[1].ScientificName)
	}
}

func TestGetTaxon(t *testing.T) {
	const resp = `{
		"id": "abc123",
		"datasetKey": 9,
		"status": "accepted",
		"label": "Homo sapiens Linnaeus, 1758",
		"name": {
			"id": "n1",
			"scientificName": "Homo sapiens",
			"rank": "species",
			"genus": "Homo",
			"specificEpithet": "sapiens",
			"code": "zoological"
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resp))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	taxon, err := getTaxonAt(context.Background(), c, srv.URL, 9, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if taxon.Status != "accepted" {
		t.Errorf("Status = %q, want accepted", taxon.Status)
	}
	if taxon.Label != "Homo sapiens Linnaeus, 1758" {
		t.Errorf("Label = %q, want Homo sapiens Linnaeus, 1758", taxon.Label)
	}
	if taxon.ScientificName != "Homo sapiens" {
		t.Errorf("ScientificName = %q, want Homo sapiens", taxon.ScientificName)
	}
}

func TestListDatasets(t *testing.T) {
	const resp = `{
		"total": 64425,
		"result": [
			{"key": 9, "type": "EXTERNAL", "title": "Catalogue of Life"},
			{"key": 1002, "type": "EXTERNAL", "title": "World Flora Online"}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resp))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	lists, total, err := listDatasetsAt(context.Background(), c, srv.URL, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 64425 {
		t.Errorf("total = %d, want 64425", total)
	}
	if len(lists) != 2 {
		t.Fatalf("len(lists) = %d, want 2", len(lists))
	}
	if lists[0].Title != "Catalogue of Life" {
		t.Errorf("Title = %q, want Catalogue of Life", lists[0].Title)
	}
	if lists[1].Title != "World Flora Online" {
		t.Errorf("Title = %q, want World Flora Online", lists[1].Title)
	}
}

func TestRetryOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":1,"result":[{"key":1,"type":"EXTERNAL","title":"Test"}]}`))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	lists, _, err := listDatasetsAt(context.Background(), c, srv.URL, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if len(lists) != 1 {
		t.Errorf("len(lists) = %d, want 1", len(lists))
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

// testable variants that accept an explicit base URL.

func searchTaxaAt(ctx context.Context, c *Client, base, query string, limit, offset int) ([]Taxon, int, error) {
	import_url := base + "/nameusage/search?q=" + urlEncode(query) +
		"&limit=" + intStr(limit) +
		"&offset=" + intStr(offset)
	body, err := c.get(ctx, import_url)
	if err != nil {
		return nil, 0, err
	}
	return parseSearchResp(body)
}

func getTaxonAt(ctx context.Context, c *Client, base string, datasetKey int, id string) (*Taxon, error) {
	u := base + "/dataset/" + intStr(datasetKey) + "/taxon/" + id
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	return parseTaxonResp(body)
}

func listDatasetsAt(ctx context.Context, c *Client, base, query string, limit int) ([]Checklist, int, error) {
	u := base + "/dataset?limit=" + intStr(limit)
	if query != "" {
		u += "&q=" + urlEncode(query)
	}
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, 0, err
	}
	return parseDatasetResp(body)
}

func urlEncode(s string) string {
	import_url := ""
	for _, b := range []byte(s) {
		switch {
		case b == ' ':
			import_url += "+"
		case (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') ||
			b == '-' || b == '_' || b == '.' || b == '~':
			import_url += string(rune(b))
		default:
			import_url += "%" + hexByte(b)
		}
	}
	return import_url
}

func hexByte(b byte) string {
	const hex = "0123456789ABCDEF"
	return string([]byte{hex[b>>4], hex[b&0xf]})
}

func intStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

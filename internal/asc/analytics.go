package asc

// App Store Connect analytics reports (Analytics Reports API): store impressions, product page views and
// first-time downloads by source.
//
// Structure: app → report request (ONGOING produces data daily, ONE_TIME_SNAPSHOT is a one-off history) → report
// (by name) → instance (per processing date, DAILY granularity) → segment (pre-signed download URL, gzip TSV).
// Each DAILY instance is a full snapshot of the last few days and corrects earlier days, so rows are stored by
// replacing each (app, date) as a whole, in processing-date order.

import (
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// Store impressions and product page views (by source and country)
	ReportDiscovery = "App Store Discovery and Engagement Standard"
	// Downloads (by download type, source and country); same source as impressions, so per-source conversion is consistent
	ReportDownloads = "App Downloads Standard"
)

// ReportRequest is one report request of an app.
type ReportRequest struct {
	ID      string
	Access  string // ONGOING / ONE_TIME_SNAPSHOT
	Stopped bool   // Apple stopped generating it because it was not read for a long time
}

// ReportInstance is the data a report produced on one processing date.
type ReportInstance struct {
	ID             string
	ProcessingDate string // YYYY-MM-DD
}

func (c *Client) ReportRequests(appID string) ([]ReportRequest, error) {
	body, err := c.get("/v1/apps/"+appID+"/analyticsReportRequests", url.Values{"limit": {"50"}})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []struct {
			ID         string `json:"id"`
			Attributes struct {
				AccessType             string `json:"accessType"`
				StoppedDueToInactivity bool   `json:"stoppedDueToInactivity"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	var out []ReportRequest
	for _, d := range resp.Data {
		out = append(out, ReportRequest{ID: d.ID, Access: d.Attributes.AccessType, Stopped: d.Attributes.StoppedDueToInactivity})
	}
	return out, nil
}

// CreateReportRequest enables analytics reports for an app (ONGOING or ONE_TIME_SNAPSHOT); Apple starts producing data 1-2 days later.
func (c *Client) CreateReportRequest(appID, access string) error {
	payload := map[string]any{"data": map[string]any{
		"type":          "analyticsReportRequests",
		"attributes":    map[string]string{"accessType": access},
		"relationships": map[string]any{"app": map[string]any{"data": map[string]string{"type": "apps", "id": appID}}},
	}}
	_, err := c.post("/v1/analyticsReportRequests", payload)
	return err
}

// ReportID finds a report by name under a report request; returns "" if it does not exist.
func (c *Client) ReportID(requestID, name string) (string, error) {
	path := "/v1/analyticsReportRequests/" + requestID + "/reports?" + url.Values{"limit": {"200"}}.Encode()
	for path != "" {
		body, err := c.get(path, nil)
		if err != nil {
			return "", err
		}
		var resp struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					Name string `json:"name"`
				} `json:"attributes"`
			} `json:"data"`
			Links struct {
				Next string `json:"next"`
			} `json:"links"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return "", err
		}
		for _, d := range resp.Data {
			if d.Attributes.Name == name {
				return d.ID, nil
			}
		}
		path = c.relative(resp.Links.Next)
	}
	return "", nil
}

// Instances lists all DAILY instances of a report.
func (c *Client) Instances(reportID string) ([]ReportInstance, error) {
	var out []ReportInstance
	path := "/v1/analyticsReports/" + reportID + "/instances?" + url.Values{"filter[granularity]": {"DAILY"}, "limit": {"200"}}.Encode()
	for path != "" {
		body, err := c.get(path, nil)
		if err != nil {
			return nil, err
		}
		var resp struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					ProcessingDate string `json:"processingDate"`
				} `json:"attributes"`
			} `json:"data"`
			Links struct {
				Next string `json:"next"`
			} `json:"links"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, err
		}
		for _, d := range resp.Data {
			out = append(out, ReportInstance{ID: d.ID, ProcessingDate: d.Attributes.ProcessingDate})
		}
		path = c.relative(resp.Links.Next)
	}
	return out, nil
}

// InstanceRows downloads and parses all segments of an instance.
func (c *Client) InstanceRows(instanceID string) ([]map[string]string, error) {
	body, err := c.get("/v1/analyticsReportInstances/"+instanceID+"/segments", url.Values{"limit": {"200"}})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []struct {
			Attributes struct {
				URL string `json:"url"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	var out []map[string]string
	for _, seg := range resp.Data {
		time.Sleep(requestGap)
		b, err := c.download(seg.Attributes.URL)
		if err != nil {
			return nil, err
		}
		rows, err := ParseTSV(b)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

// download fetches a segment file from its pre-signed URL, without an Authorization header.
func (c *Client) download(u string) ([]byte, error) {
	resp, err := c.http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("asc: segment download HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
}

// relative turns the absolute next URL returned by pagination into a relative path ("" means no next page).
func (c *Client) relative(next string) string {
	if next == "" {
		return ""
	}
	u, err := url.Parse(next)
	if err != nil {
		return ""
	}
	return u.RequestURI()
}

func (c *Client) post(path string, payload any) ([]byte, error) {
	tok, err := c.jwt()
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", c.base+path, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		msg := string(body)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, &apiError{Status: resp.StatusCode, Body: msg}
	}
	return body, nil
}

// ParseTSV parses a gzip-compressed (or uncompressed) tab-separated report into rows of column name → value.
func ParseTSV(body []byte) ([]map[string]string, error) {
	var r io.Reader = bytes.NewReader(body)
	if len(body) > 2 && body[0] == 0x1f && body[1] == 0x8b {
		gz, err := gzip.NewReader(r)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	}
	cr := csv.NewReader(r)
	cr.Comma = '\t'
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	recs, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, nil
	}
	head := recs[0]
	if len(head) > 0 {
		head[0] = strings.TrimPrefix(head[0], "\uFEFF")
	}
	out := make([]map[string]string, 0, len(recs)-1)
	for _, rec := range recs[1:] {
		m := make(map[string]string, len(head))
		for i, h := range head {
			if i < len(rec) {
				m[h] = rec[i]
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// StoreSource maps Apple's Source Type to a fixed set of categories.
func StoreSource(s string) string {
	switch s {
	case "App Store search":
		return "search"
	case "App Store browse":
		return "browse"
	case "App referrer":
		return "app_referrer"
	case "Web referrer":
		return "web_referrer"
	default:
		return "other"
	}
}

// FunnelKey is the stored aggregation grain: date x app (Apple ID) x source x country.
type FunnelKey struct {
	Date, AppleID, Source, Territory string
}

// FunnelCounts holds the counts of one aggregated cell.
type FunnelCounts struct {
	Impressions, ImpressionsUnique int
	PageViews, PageViewsUnique     int
	FirstDownloads, Redownloads    int
}

// AggregateFunnel aggregates detail rows of both report types into FunnelKey; other dimensions (device, OS version, page type, ...) are merged.
func AggregateFunnel(report string, rows []map[string]string) map[FunnelKey]*FunnelCounts {
	out := map[FunnelKey]*FunnelCounts{}
	for _, r := range rows {
		k := FunnelKey{Date: r["Date"], AppleID: r["App Apple Identifier"], Source: StoreSource(r["Source Type"]), Territory: r["Territory"]}
		if k.Date == "" || k.AppleID == "" {
			continue
		}
		n, _ := strconv.Atoi(r["Counts"])
		c := out[k]
		if c == nil {
			c = &FunnelCounts{}
			out[k] = c
		}
		switch report {
		case ReportDiscovery:
			u, _ := strconv.Atoi(r["Unique Counts"])
			switch r["Event"] {
			case "Impression":
				c.Impressions += n
				c.ImpressionsUnique += u
			case "Page view":
				c.PageViews += n
				c.PageViewsUnique += u
			}
		case ReportDownloads:
			switch r["Download Type"] {
			case "First-time download":
				c.FirstDownloads += n
			case "Redownload":
				c.Redownloads += n
			}
		}
	}
	return out
}

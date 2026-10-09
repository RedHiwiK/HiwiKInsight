package asc

import (
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// SalesRow is one row of the Summary Sales Report (1_1), keeping only the columns the dashboard needs.
type SalesRow struct {
	SKU             string
	ParentSKU       string // in-app purchase rows: SKU of the parent app
	Title           string
	ProductType     string // Product Type Identifier, see Category
	Units           float64
	ProceedsPerUnit float64 // Developer Proceeds (per unit, after commission and taxes)
	ProceedsCur     string
	CustomerPrice   float64
	CustomerCur     string
	Country         string // two-letter country code
	AppleID         string // Apple Identifier: the app ID for app rows, the in-app purchase ID for in-app purchase rows
	Subscription    string // New / Renewal (subscription rows)
	Period          string
	BeginDate       string // MM/DD/YYYY
}

// Category classifies a Product Type Identifier:
//
//	download: first-time download; redownload: redownload; update: update; iap: non-consumable / consumable in-app purchase; subscription: auto-renewable subscription
func Category(productType string) string {
	switch strings.TrimSuffix(productType, "-M") {
	case "1", "1F", "1T", "F1", "1E", "1EP", "1EU":
		return "download"
	case "3", "3F", "3T", "F3":
		return "redownload"
	case "7", "7F", "7T", "F7":
		return "update"
	case "IAY":
		return "subscription"
	case "IA1", "IA9", "IAC", "FI1":
		return "iap"
	}
	return "other"
}

// Sales downloads the summary sales report for one day (DAILY, date=YYYY-MM-DD) or one month (MONTHLY, date=YYYY-MM).
// Returns ErrNotFound when there were no sales or the report has not been generated yet.
func (c *Client) Sales(frequency, date string) ([]SalesRow, error) {
	version := "1_1"
	if frequency == "MONTHLY" {
		version = "1_0"
	}
	q := url.Values{
		"filter[frequency]":     {frequency},
		"filter[reportType]":    {"SALES"},
		"filter[reportSubType]": {"SUMMARY"},
		"filter[vendorNumber]":  {c.cfg.VendorNumber},
		"filter[reportDate]":    {date},
		"filter[version]":       {version},
	}
	body, err := c.getAccept("/v1/salesReports", q, "application/a-gzip")
	if err != nil {
		return nil, err
	}
	return ParseSales(body)
}

// ParseSales parses a gzip-compressed (or already decompressed) TSV report.
func ParseSales(body []byte) ([]SalesRow, error) {
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
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("asc: sales header: %w", err)
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.TrimSpace(strings.TrimPrefix(h, "\uFEFF"))] = i
	}
	for _, need := range []string{"SKU", "Product Type Identifier", "Units", "Developer Proceeds", "Currency of Proceeds"} {
		if _, ok := idx[need]; !ok {
			return nil, fmt.Errorf("asc: sales report missing column %q", need)
		}
	}
	col := func(rec []string, name string) string {
		if i, ok := idx[name]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	num := func(s string) float64 {
		f, _ := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
		return f
	}
	var out []SalesRow
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(rec) < 5 || col(rec, "SKU") == "" {
			continue
		}
		out = append(out, SalesRow{
			SKU: col(rec, "SKU"), ParentSKU: col(rec, "Parent Identifier"), Title: col(rec, "Title"),
			ProductType: col(rec, "Product Type Identifier"), Units: num(col(rec, "Units")),
			ProceedsPerUnit: num(col(rec, "Developer Proceeds")), ProceedsCur: col(rec, "Currency of Proceeds"),
			CustomerPrice: num(col(rec, "Customer Price")), CustomerCur: col(rec, "Customer Currency"),
			Country: col(rec, "Country Code"), AppleID: col(rec, "Apple Identifier"),
			Subscription: col(rec, "Subscription"), Period: col(rec, "Period"), BeginDate: col(rec, "Begin Date"),
		})
	}
	return out, nil
}

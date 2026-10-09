package asc

// Released build list: a fallback for environment detection.
//
// The SDK uses AppTransaction to tell App Store from TestFlight. When AppTransaction fails (e.g. noAccount on
// iOS 27 TestFlight devices) and there is no receipt file, the SDK can only fall back to production, and TestFlight
// data would be counted as real users. The server checks again whether the build was ever released on the App Store:
// data marked production whose build was never released is always recorded as sandbox.

import (
	"encoding/json"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/store"
)

const releasesEvery = 15 * time.Minute // a newly released version is recognized within 15 minutes

// Version states that are (or are becoming) publicly available; in review, pending developer release, etc. do not count
var releasedStates = map[string]bool{
	"READY_FOR_SALE":              true,
	"READY_FOR_DISTRIBUTION":      true,
	"PROCESSING_FOR_APP_STORE":    true,
	"PROCESSING_FOR_DISTRIBUTION": true,
	"REPLACED_WITH_NEW_VERSION":   true,
	"REMOVED_FROM_SALE":           true,
	"DEVELOPER_REMOVED_FROM_SALE": true,
}

// ReleasedBuilds returns "version|build" for every released version of an app.
func (c *Client) ReleasedBuilds(appID string) ([]string, error) {
	q := url.Values{
		"filter[platform]":         {"IOS"},
		"include":                  {"build"},
		"fields[appStoreVersions]": {"versionString,appStoreState,appVersionState,build"},
		"fields[builds]":           {"version"},
		"limit":                    {"200"},
	}
	body, err := c.get("/v1/apps/"+appID+"/appStoreVersions", q)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []struct {
			Attributes struct {
				VersionString   string `json:"versionString"`
				AppStoreState   string `json:"appStoreState"`
				AppVersionState string `json:"appVersionState"`
			} `json:"attributes"`
			Relationships struct {
				Build struct {
					Data *struct {
						ID string `json:"id"`
					} `json:"data"`
				} `json:"build"`
			} `json:"relationships"`
		} `json:"data"`
		Included []struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				Version string `json:"version"`
			} `json:"attributes"`
		} `json:"included"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	builds := map[string]string{}
	for _, in := range resp.Included {
		if in.Type == "builds" {
			builds[in.ID] = in.Attributes.Version
		}
	}
	var out []string
	for _, v := range resp.Data {
		a := v.Attributes
		if !releasedStates[a.AppStoreState] && !releasedStates[a.AppVersionState] {
			continue
		}
		if d := v.Relationships.Build.Data; d != nil && builds[d.ID] != "" {
			out = append(out, a.VersionString+"|"+builds[d.ID])
		}
	}
	return out, nil
}

// Releases is the in-memory released build list (bundle ID → set of "version|build"), refreshed periodically from App Store Connect.
type Releases struct {
	mu       sync.RWMutex
	byBundle map[string]map[string]bool
}

func NewReleases() *Releases { return &Releases{byBundle: map[string]map[string]bool{}} }

// Released reports whether a build has been released; known = false means the list for this app is not loaded yet,
// and callers must not reclassify based on it.
func (r *Releases) Released(bundle, version, build string) (released, known bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	set, ok := r.byBundle[bundle]
	if !ok || len(set) == 0 {
		return false, false
	}
	return set[version+"|"+build], true
}

// Loop refreshes once at startup and then every 15 minutes; after each refresh, historical data mislabeled as
// production is moved back to TestFlight (sandbox). apps maps analytics app keys → bundle IDs.
func (r *Releases) Loop(c *Client, st *store.Store, apps map[string]string) {
	for {
		r.refresh(c, st, apps)
		time.Sleep(releasesEvery)
	}
}

func (r *Releases) refresh(c *Client, st *store.Store, apps map[string]string) {
	list, err := c.Apps()
	if err != nil {
		slog.Error("asc releases refresh failed", "error", err)
		return
	}
	for _, a := range list {
		time.Sleep(requestGap)
		builds, err := c.ReleasedBuilds(a.ID)
		if err != nil {
			slog.Error("asc releases refresh failed", "app", a.BundleID, "error", err)
			continue
		}
		if len(builds) == 0 {
			continue // empty list (never released, or an API problem): keep the old list and reclassify nothing
		}
		set := map[string]bool{}
		for _, b := range builds {
			set[b] = true
		}
		r.mu.Lock()
		r.byBundle[a.BundleID] = set
		r.mu.Unlock()

		for key, bundle := range apps {
			if bundle != a.BundleID {
				continue
			}
			n, err := st.ReclassifyUnreleased(key, builds)
			if err != nil {
				slog.Error("reclassify unreleased builds failed", "app", key, "error", err)
			} else if n > 0 {
				slog.Info("reclassified unreleased builds as sandbox", "app", key, "events", n)
			}
		}
	}
}

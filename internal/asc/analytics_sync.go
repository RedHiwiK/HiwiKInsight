package asc

import (
	"log/slog"
	"sort"
	"time"

	"github.com/RedHiwiK/HiwiKInsight/internal/store"
)

// SyncAnalytics fetches unprocessed instances from each app's analytics reports and returns the number of instances stored.
// When there is no usable ONGOING request it creates one (Apple starts producing data 1-2 days later); apps that never
// requested a historical snapshot also get one, to backfill the history before reports were enabled.
func (s *Syncer) SyncAnalytics() (int, error) {
	apps, err := s.client.Apps()
	if err != nil {
		return 0, err
	}
	done, err := s.store.AnalyticsInstanceIDs()
	if err != nil {
		return 0, err
	}
	total := 0
	for _, a := range apps {
		n, err := s.syncAppAnalytics(a, done)
		total += n
		if err != nil {
			// A failure for one app does not affect the others
			slog.Error("asc analytics sync failed", "app", a.BundleID, "error", err, "instances", n)
		}
	}
	return total, nil
}

func (s *Syncer) syncAppAnalytics(a App, done map[string]bool) (int, error) {
	reqs, err := s.client.ReportRequests(a.ID)
	if err != nil {
		return 0, err
	}
	hasOngoing, hasSnapshot := false, false
	for _, r := range reqs {
		switch r.Access {
		case "ONGOING":
			hasOngoing = hasOngoing || !r.Stopped
		case "ONE_TIME_SNAPSHOT":
			hasSnapshot = true
		}
	}
	for access, has := range map[string]bool{"ONGOING": hasOngoing, "ONE_TIME_SNAPSHOT": hasSnapshot} {
		if has {
			continue
		}
		time.Sleep(requestGap)
		if err := s.client.CreateReportRequest(a.ID, access); err != nil {
			slog.Warn("asc analytics request not created", "app", a.BundleID, "access", access, "error", err)
		} else {
			slog.Info("asc analytics request created", "app", a.BundleID, "access", access)
		}
	}

	n := 0
	for _, report := range []string{ReportDiscovery, ReportDownloads} {
		// The same report has instances under both the ONGOING and the snapshot request; they are stored together in
		// processing-date order, so later ones overwrite earlier ones
		var pending []ReportInstance
		for _, r := range reqs {
			time.Sleep(requestGap)
			rid, err := s.client.ReportID(r.ID, report)
			if err != nil {
				return n, err
			}
			if rid == "" {
				continue
			}
			time.Sleep(requestGap)
			insts, err := s.client.Instances(rid)
			if err != nil {
				return n, err
			}
			for _, in := range insts {
				if !done[in.ID] {
					pending = append(pending, in)
				}
			}
		}
		sort.SliceStable(pending, func(i, j int) bool { return pending[i].ProcessingDate < pending[j].ProcessingDate })
		for _, in := range pending {
			time.Sleep(requestGap)
			rows, err := s.client.InstanceRows(in.ID)
			if err != nil {
				return n, err
			}
			var recs []store.FunnelRecord
			for k, c := range AggregateFunnel(report, rows) {
				if k.AppleID != a.ID {
					continue
				}
				recs = append(recs, store.FunnelRecord{
					Date: k.Date, BundleID: a.BundleID, Source: k.Source, Territory: k.Territory,
					Impressions: c.Impressions, ImpressionsUnique: c.ImpressionsUnique,
					PageViews: c.PageViews, PageViewsUnique: c.PageViewsUnique,
					FirstDownloads: c.FirstDownloads, Redownloads: c.Redownloads,
				})
			}
			if err := s.store.ApplyAnalyticsInstance(in.ID, report, a.BundleID, in.ProcessingDate, report == ReportDiscovery, recs); err != nil {
				return n, err
			}
			done[in.ID] = true
			n++
		}
	}
	return n, nil
}

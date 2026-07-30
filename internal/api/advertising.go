// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"net/http"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/middleware"
)


// AdvertisingHandler handles the AdvertisingHandler HTTP request.
func AdvertisingHandler(dataDir string) http.HandlerFunc {
	tm := economy.LoadTagManager(dataDir)

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		action := r.URL.Query().Get("action")

		switch r.Method {
		case "GET":
			switch action {
			case "search":
				tag := r.URL.Query().Get("tag")
				if tag == "" {
					http.Error(w, "tag required", http.StatusBadRequest)
					return
				}
				ads := tm.SearchAds(tag)
				writeJSON(w, map[string]any{
					"tag":  tag,
					"ads":  ads,
					"hits": len(ads),
				})

			case "popular":
				popular := tm.PopularTags(20)
				writeJSON(w, map[string]any{
					"popular_tags": popular,
				})

			case "deflation":
				ledger := economy.LoadLedger(dataDir)
				summary := tm.DeflationSummary(ledger)
				ledger.Save(dataDir)
				writeJSON(w, summary)

			case "tags":
				tags := make([]map[string]any, 0)
				for _, t := range tm.Tags {
					adCount := 0
					for _, a := range tm.Ads {
						if a.Tag == t.Tag && a.Active {
							adCount++
						}
					}
					tags = append(tags, map[string]any{
						"tag":          t.Tag,
						"owner":        t.Owner,
						"price_ng":     t.PriceNg,
						"active":       t.Active,
						"purchased_at": t.PurchasedAt,
						"expires_at":   t.ExpiresAt,
						"ad_count":     adCount,
					})
				}
				writeJSON(w, map[string]any{
					"tags":             tags,
					"total_burned_ng":  tm.TotalBurnedNg,
					"total_ads":        len(tm.Ads),
				})

			default:
				writeJSON(w, map[string]any{
					"tags_total":       len(tm.Tags),
					"total_burned_ng":  tm.TotalBurnedNg,
					"total_ads":        len(tm.Ads),
				})
			}

		case "POST":
			switch action {
			case "buy-tag":
				var req struct {
					Tag   string `json:"tag"`
					Owner string `json:"owner"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "invalid json", http.StatusBadRequest)
					return
				}
				if req.Tag == "" || req.Owner == "" {
					http.Error(w, "tag and owner required", http.StatusBadRequest)
					return
				}
				ledger := economy.LoadLedger(dataDir)
				tag, err := tm.BuyTag(dataDir, req.Tag, req.Owner, ledger)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				ledger.Save(dataDir)
				tm.Save(dataDir)
				writeJSON(w, tag)

			case "place-ad":
				var req struct {
					Tag     string `json:"tag"`
					Owner   string `json:"owner"`
					Title   string `json:"title"`
					Desc    string `json:"description"`
					Contact string `json:"contact"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "invalid json", http.StatusBadRequest)
					return
				}
				if req.Tag == "" || req.Owner == "" || req.Title == "" {
					http.Error(w, "tag, owner, title required", http.StatusBadRequest)
					return
				}
				ledger := economy.LoadLedger(dataDir)
				ad, err := tm.PlaceAd(dataDir, req.Tag, req.Owner, req.Title, req.Desc, req.Contact, ledger)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				ledger.Save(dataDir)
				tm.Save(dataDir)
				writeJSON(w, ad)

			default:
				http.Error(w, "unknown action", http.StatusBadRequest)
			}

		default:
			http.Error(w, "GET or POST only", 400)
		}
	}
}

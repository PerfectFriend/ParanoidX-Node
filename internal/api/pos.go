// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"ParanoidX/internal/economy"
	"ParanoidX/internal/middleware"
	"rsc.io/qr"
)

func intParam(r *http.Request, name string, defaultVal int) int {
	v := r.URL.Query().Get(name)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return defaultVal
	}
	return n
}


// POSHandler handles the POSHandler HTTP request.
func POSHandler(dataDir string) http.HandlerFunc {
	pm := economy.LoadPOSManager(dataDir)

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		action := r.URL.Query().Get("action")

		switch r.Method {
		case "GET":
			switch action {
			case "invoice":
				id := r.URL.Query().Get("id")
				if id == "" {
					http.Error(w, "id required", http.StatusBadRequest)
					return
				}
				inv, err := pm.GetInvoice(id)
				if err != nil {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				writeJSON(w, inv)

			case "list":
				merchant := r.URL.Query().Get("merchant")
				if merchant == "" {
					http.Error(w, "merchant required", http.StatusBadRequest)
					return
				}
				limit, offset := intParam(r, "limit", 50), intParam(r, "offset", 0)
				invoices := pm.ListMerchantInvoices(merchant)
				total := len(invoices)
				if offset > total {
					offset = total
				}
				end := offset + limit
				if end > total {
					end = total
				}
				page := invoices[offset:end]
				writeJSON(w, map[string]any{
					"merchant":  merchant,
					"invoices":  page,
					"total":     total,
					"limit":     limit,
					"offset":    offset,
					"revenue":   pm.MerchantRevenue(merchant),
				})

			case "list-vouchers":
				vouchers := pm.ListVouchers()
				writeJSON(w, map[string]any{
					"vouchers": vouchers,
					"total":    len(vouchers),
				})

			case "merchant-stats":
				merchant := r.URL.Query().Get("merchant")
				if merchant == "" {
					http.Error(w, "merchant required", http.StatusBadRequest)
					return
				}
				invoices := pm.ListMerchantInvoices(merchant)
				paidCount := 0
				pendingCount := 0
				var volume int64
				var commission int64
				for _, inv := range invoices {
					volume += inv.AmountNg
					commission += inv.CommissionNg
					switch inv.Status {
					case "paid":
						paidCount++
					case "pending":
						pendingCount++
					}
				}
				writeJSON(w, map[string]any{
					"merchant":         merchant,
					"total_invoices":   len(invoices),
					"paid":             paidCount,
					"pending":          pendingCount,
					"volume_ng":        volume,
					"commission_ng":    commission,
					"net_revenue_ng":   volume - commission,
				})

			case "merchants":
				merchantSet := map[string]bool{}
				for _, inv := range pm.Invoices {
					merchantSet[inv.Merchant] = true
				}
				var merchants []map[string]any
				for m := range merchantSet {
					invs := pm.ListMerchantInvoices(m)
					var vol, comm int64
					for _, inv := range invs {
						vol += inv.AmountNg
						comm += inv.CommissionNg
					}
					merchants = append(merchants, map[string]any{
						"merchant":        m,
						"total_invoices":  len(invs),
						"volume_ng":       vol,
						"net_revenue_ng":  vol - comm,
					})
				}
				writeJSON(w, map[string]any{
					"merchants": merchants,
					"total":     len(merchants),
				})

			case "stats":
				totalInv := len(pm.Invoices)
				paidCount := 0
				pendingCount := 0
				expiredCount := 0
				var totalVolume int64
				var totalCommission int64
				for _, inv := range pm.Invoices {
					totalVolume += inv.AmountNg
					totalCommission += inv.CommissionNg
					switch inv.Status {
					case "paid":
						paidCount++
					case "pending":
						pendingCount++
					default:
						expiredCount++
					}
				}
				writeJSON(w, map[string]any{
					"total_invoices":   totalInv,
					"paid":             paidCount,
					"pending":          pendingCount,
					"expired":          expiredCount,
					"total_volume_ng":  totalVolume,
					"total_commission": totalCommission,
					"fee_bps":          economy.POSCommissionBPS,
					"expiry_minutes":   economy.POSInvoiceExpiryMinutes,
				})

			default:
				writeJSON(w, map[string]any{
					"total_invoices": len(pm.Invoices),
					"status":         "POS terminal active",
					"fee_bps":        economy.POSCommissionBPS,
					"expiry_minutes": economy.POSInvoiceExpiryMinutes,
				})
			}

		case "POST":
			switch action {
			case "create-voucher":
				var req struct {
					Merchant string `json:"merchant"`
					AmountNg int64  `json:"amount_ng"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "invalid json", http.StatusBadRequest)
					return
				}
				v, err := pm.CreateVoucher(req.Merchant, req.AmountNg)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				writeJSON(w, v)

			case "redeem-voucher":
				var req struct {
					Code     string `json:"code"`
					Redeemer string `json:"redeemer"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "invalid json", http.StatusBadRequest)
					return
				}
				ledger := economy.LoadLedger(dataDir)
				v, err := pm.RedeemVoucher(req.Code, req.Redeemer, ledger)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				ledger.Save(dataDir)
				writeJSON(w, v)

			case "create-invoice":
				var req struct {
					Merchant    string `json:"merchant"`
					AmountNg    int64  `json:"amount_ng"`
					Description string `json:"description"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "invalid json", http.StatusBadRequest)
					return
				}
				inv, err := pm.CreateInvoice(req.Merchant, req.AmountNg, req.Description)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				pm.Save(dataDir)
				writeJSON(w, inv)

			case "pay":
				var req struct {
					InvoiceID string `json:"invoice_id"`
					Payer     string `json:"payer"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					http.Error(w, "invalid json", http.StatusBadRequest)
					return
				}
				if req.InvoiceID == "" || req.Payer == "" {
					http.Error(w, "invoice_id and payer required", http.StatusBadRequest)
					return
				}
				ledger := economy.LoadLedger(dataDir)
				inv, err := pm.PayInvoice(req.InvoiceID, req.Payer, ledger)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				ledger.Save(dataDir)
				pm.Save(dataDir)
				writeJSON(w, inv)

			default:
				http.Error(w, "unknown action", http.StatusBadRequest)
			}

		default:
			http.Error(w, "GET or POST only", 400)
		}
	}
}


// QRHandler handles the QRHandler HTTP request.
func QRHandler(dataDir string) http.HandlerFunc {
	pm := economy.LoadPOSManager(dataDir)

	return func(w http.ResponseWriter, r *http.Request) {
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}

		invID := r.URL.Query().Get("id")
		if invID == "" {
			invID = r.URL.Query().Get("invoice_id")
		}
		if invID == "" {
			http.Error(w, "invoice_id required", http.StatusBadRequest)
			return
		}

		inv, err := pm.GetInvoice(invID)
		if err != nil {
			http.Error(w, "invoice not found", http.StatusNotFound)
			return
		}

		payload := "simplex-node://pay/" + invID
		if inv.Description != "" {
			payload = "simplex-node://pay/" + invID + "?m=" + inv.Merchant + "&a=" + strconv.FormatInt(inv.AmountNg, 10)
		}

		code, err := qr.Encode(payload, qr.M)
		if err != nil {
			http.Error(w, "qr encode error", 500)
			return
		}

		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", strconv.Itoa(len(code.PNG())))
		w.Write(code.PNG())
	}
}

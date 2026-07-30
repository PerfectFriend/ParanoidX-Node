// Package economy — P2P-обмен банкнотами между держателями напрямую (минуя аукцион).
// Позволяет создавать офферы (предложения купли/продажи), принимать, отклонять и отменять их.
// Все офферы хранятся в JSON-файле p2p_offers.json в dataDir.
// Доступ к P2P-функциям ограничен — только для аудиторов (auditor-only).
package economy

import (
	"crypto/rand"
	"encoding/hex"
		"fmt"
		"path/filepath"
	"time"

	"ParanoidX/internal/fileutil"
)

// P2POffer — предложение купить банкноту напрямую у держателя.
type P2POffer struct {
	OfferID    string `json:"offer_id"`
	FromPubkey string `json:"from_pubkey"`  // кто хочет купить
	ToPubkey   string `json:"to_pubkey"`    // кому адресовано
	TargetSerial string `json:"target_serial"` // какую банкноту хочет
	PriceNg    int64  `json:"price_ng"`     // цена предложения
	Message    string `json:"message"`      // "продай мне её, я маньяк коллекционер!"
	Status     string `json:"status"`       // pending, accepted, rejected, cancelled
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// GenerateOfferID генерирует уникальный идентификатор оффера в формате "offer_<8 hex>".
func GenerateOfferID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return "offer_" + hex.EncodeToString(b)
}

// LoadOffers загружает все P2P-офферы из JSON-файла.
func LoadOffers(dataDir string) []P2POffer {
	var offers []P2POffer
	fileutil.ReadJSON(filepath.Join(dataDir, "p2p_offers.json"), &offers)
	if offers == nil {
		offers = []P2POffer{}
	}
	return offers
}

// SaveOffers сохраняет все P2P-офферы в JSON-файл.
func SaveOffers(dataDir string, offers []P2POffer) {
	p := filepath.Join(dataDir, "p2p_offers.json")
	fileutil.WriteJSON(p, offers)
}

// CreateOffer создаёт новое предложение.
func CreateOffer(dataDir, fromPubkey, toPubkey, targetSerial, message string, priceNg int64) (*P2POffer, error) {
	offers := LoadOffers(dataDir)

	// Проверяем что цель существует в реестре
	banknotes, _ := LoadBanknotesV2(dataDir)
	found := false
	for _, b := range banknotes {
		if b.Serial == targetSerial && b.Holder == toPubkey && b.Status == "active" {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("банкнота %s не найдена у держателя %s", targetSerial, toPubkey)
	}

	// Проверяем что нет уже pending оффера на этот serial от этого покупателя
	for _, o := range offers {
		if o.TargetSerial == targetSerial && o.FromPubkey == fromPubkey && o.Status == "pending" {
			return nil, fmt.Errorf("у вас уже есть активное предложение на эту банкноту")
		}
	}

	offer := &P2POffer{
		OfferID:    GenerateOfferID(),
		FromPubkey: fromPubkey,
		ToPubkey:   toPubkey,
		TargetSerial: targetSerial,
		PriceNg:    priceNg,
		Message:    message,
		Status:     "pending",
		CreatedAt:  time.Now().Format(time.RFC3339),
		UpdatedAt:  time.Now().Format(time.RFC3339),
	}

	offers = append(offers, *offer)
	SaveOffers(dataDir, offers)
	return offer, nil
}

// AcceptOffer принимает предложение — банкнота переходит к покупателю, ng — к продавцу.
func AcceptOffer(dataDir, offerID, toPubkey string) (*P2POffer, error) {
	offers := LoadOffers(dataDir)
	var idx int = -1
	for i, o := range offers {
		if o.OfferID == offerID && o.ToPubkey == toPubkey && o.Status == "pending" {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, fmt.Errorf("предложение не найдено или не принадлежит вам")
	}

	offer := &offers[idx]

	// Проверяем баланс покупателя
	ledger := LoadLedger(dataDir)
	if ledger.Balance(offer.FromPubkey) < offer.PriceNg {
		return nil, fmt.Errorf("у покупателя недостаточно Liquid Taler")
	}

	// Проверяем что банкнота всё ещё у продавца
	banknotes, _ := LoadBanknotesV2(dataDir)
	bFound := false
	for i, b := range banknotes {
		if b.Serial == offer.TargetSerial && b.Holder == offer.ToPubkey && b.Status == "active" {
			// Переводим банкноту
			banknotes[i].Holder = offer.FromPubkey
			bFound = true
			break
		}
	}
	if !bFound {
		// Если кто-то уже купил через аукцион — оффер протух
		offer.Status = "rejected"
		SaveOffers(dataDir, offers)
		return nil, fmt.Errorf("банкнота больше не доступна")
	}
	SaveBanknotesV2(dataDir, banknotes)

	// Переводим ng: покупатель → продавец
	if err := ledger.Transfer(offer.FromPubkey, offer.ToPubkey, offer.PriceNg); err != nil {
		// Откатываем банкноту
		for i, b := range banknotes {
			if b.Serial == offer.TargetSerial {
				banknotes[i].Holder = offer.ToPubkey
				break
			}
		}
		SaveBanknotesV2(dataDir, banknotes)
		return nil, fmt.Errorf("перевод средств не удался: %v", err)
	}
	ledger.Save(dataDir)

	offer.Status = "accepted"
	offer.UpdatedAt = time.Now().Format(time.RFC3339)
	SaveOffers(dataDir, offers)

	return offer, nil
}

// RejectOffer отклоняет предложение.
func RejectOffer(dataDir, offerID, toPubkey string) (*P2POffer, error) {
	offers := LoadOffers(dataDir)
	for i, o := range offers {
		if o.OfferID == offerID && o.ToPubkey == toPubkey && o.Status == "pending" {
			offers[i].Status = "rejected"
			offers[i].UpdatedAt = time.Now().Format(time.RFC3339)
			SaveOffers(dataDir, offers)
			return &offers[i], nil
		}
	}
	return nil, fmt.Errorf("предложение не найдено или не принадлежит вам")
}

// CancelOffer отменяет своё предложение.
func CancelOffer(dataDir, offerID, fromPubkey string) (*P2POffer, error) {
	offers := LoadOffers(dataDir)
	for i, o := range offers {
		if o.OfferID == offerID && o.FromPubkey == fromPubkey && o.Status == "pending" {
			offers[i].Status = "cancelled"
			offers[i].UpdatedAt = time.Now().Format(time.RFC3339)
			SaveOffers(dataDir, offers)
			return &offers[i], nil
		}
	}
	return nil, fmt.Errorf("предложение не найдено или не принадлежит вам")
}

// GetMyOffers возвращает все предложения пользователя (отправленные и полученные).
func GetMyOffers(dataDir, pubkey string) (sent, received []P2POffer) {
	offers := LoadOffers(dataDir)
	for _, o := range offers {
		if o.FromPubkey == pubkey {
			sent = append(sent, o)
		}
		if o.ToPubkey == pubkey {
			received = append(received, o)
		}
	}
	return
}

// ExploreHolders возвращает всех держателей банкнот с их балансами (только для аудиторов).
func ExploreHolders(dataDir string) []map[string]any {
	banknotes, _ := LoadBanknotesV2(dataDir)
	ledger := LoadLedger(dataDir)

	holdersMap := make(map[string]map[string]any)
	for _, b := range banknotes {
		if b.Status != "active" {
			continue
		}
		entry, ok := holdersMap[b.Holder]
		if !ok {
			entry = map[string]any{
				"pubkey":     b.Holder,
				"liquid_ng":  ledger.Balance(b.Holder),
				"banknotes":  []map[string]any{},
			}
		}
		bnList := entry["banknotes"].([]map[string]any)
		bnList = append(bnList, map[string]any{
			"serial":  b.Serial,
			"rarity":  b.Rarity,
			"denom_ng": b.DenominationNg,
			"denom_tlr": NGtoTLR(b.DenominationNg),
			"is_golden": b.IsGolden,
			"special":   b.SpecialSeries,
		})
		entry["banknotes"] = bnList
		holdersMap[b.Holder] = entry
	}

	result := []map[string]any{}
	for _, v := range holdersMap {
		result = append(result, v)
	}
	return result
}

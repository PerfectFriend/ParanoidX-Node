// Genesis Init — разовый скрипт для первого запуска экономики.
// Минтит Liquid Taler на 420k USDT, создаёт долг, 9 Genesis банкнот, 3 пакета.
//
// ЗАПУСК:
//   cd /home/tomas/simplex-node && go run ./scripts/genesis/genesis-init.go -data ~/.local/share/simplex-node
//
// ОСТОРОЖНО: скрипт идемпотентен — если долг уже создан, он не перезаписывается.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"ParanoidX/internal/economy"
)

// GenesisConfig — параметры первой эмиссии экономики Острова.
type GenesisConfig struct {
	USDTAmount      float64 // 420,000 — сумма первого взноса в USDT
	InvestorPubkey  string  // публичный ключ первого инвестора
	InvestorLabel   string  // метка инвестора (по умолч. "First Investor")
}

func main() {
	dataDir := flag.String("data", filepath.Join(os.Getenv("HOME"), ".local/share/simplex-node"), "data dir")
	investorPubkey := flag.String("investor", "", "публичный ключ первого инвестора (если пусто — создаётся новый)")
	flag.Parse()

	if *dataDir == "" {
		log.Fatal("data dir required")
	}

	cfg := GenesisConfig{
		USDTAmount:     420000,
		InvestorPubkey: *investorPubkey,
		InvestorLabel:  "First Investor",
	}

	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║     GENESIS INIT — ЗАПУСК ПЕРВОЙ ЭМИССИИ       ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")

	// 1. Создаём / проверяем кошелёк инвестора
	ledger := economy.LoadLedger(*dataDir)
	if cfg.InvestorPubkey == "" {
		pubkey, privkey, mnemonic, err := economy.GenerateKeypair()
		if err != nil {
			log.Fatalf("generate keypair: %v", err)
		}
		cfg.InvestorPubkey = pubkey
		fmt.Printf("\n🔑 Создан новый кошелёк инвестора:\n")
		fmt.Printf("   pubkey:  %s\n", pubkey)
		fmt.Printf("   privkey: %s\n", privkey)
		fmt.Printf("   mnemonic: %s\n", mnemonic)
		fmt.Printf("   ⚠️  Сохрани сид-фразу!\n")
	}

	ledger.EnsureAccount(cfg.InvestorPubkey)

	// 2. Проверяем существующий долг
	debt := economy.LoadDebt(*dataDir)
	if debt.RepaidAt != "" {
		log.Fatal("Долг первого инвестора уже погашен. Genesis уже был.")
	}
	if debt.PrincipalNG > 0 {
		fmt.Println("\nℹ️  Долг уже создан, пропускаем.")
	} else {
		// Конвертируем 420k USDT в ng по споту
		ngAmount := economy.USDTtoNG(cfg.USDTAmount)
		fmt.Printf("\n💵 %g USDT = %d ng (спот $%g/oz)\n", cfg.USDTAmount, ngAmount, economy.SilverSpotUSDperOZ)

		// Создаём долг (вся сумма в казну)
		debt = &economy.FirstInvestorDebt{
			PrincipalUSDT: cfg.USDTAmount,
			PrincipalNG:   ngAmount,
			IssuedAt:      time.Now().Format(time.RFC3339),
		}
		economy.SaveDebt(*dataDir, debt)
		fmt.Printf("📝 Долг записан: %g USDT / %d ng\n", cfg.USDTAmount, ngAmount)

		// Минтим Liquid Taler в казну (весь 5,600 TLR)
		// Казна = тот же ledger, pubkey казны — special "treasury"
		treasuryPubkey := "treasury_saint_mary_island"
		ledger.EnsureAccount(treasuryPubkey)
		ledger.Mint(treasuryPubkey, ngAmount)
		ledger.Save(*dataDir)
		fmt.Printf("💰 Liquid Taler заминчено: %d ng (%.2f TLR)\n", ngAmount, float64(ngAmount)/float64(economy.NGPerTLR))
	}

	// 3. Создаём токен аудитора
	auditors := economy.LoadAuditors(*dataDir)
	found := false
	for _, a := range auditors {
		if a.Pubkey == cfg.InvestorPubkey {
			found = true
			break
		}
	}
	if !found {
		auditors = append(auditors, economy.AuditorEntry{
			Pubkey:    cfg.InvestorPubkey,
			Label:     cfg.InvestorLabel,
			GrantedAt: time.Now().Format(time.RFC3339),
			Type:      "first_investor",
		})
		economy.SaveAuditors(*dataDir, auditors)
		fmt.Println("👁 Токен аудитора выдан первому инвестору")
	} else {
		fmt.Println("ℹ️  Токен аудитора уже есть")
	}

	// 4. Создаём 9 Genesis банкнот — Священный Грааль Острова
	//    9 добродетелей: Peace → Love → Unity → Respect → Knowledge → Wisdom → Truth → Freedom → Creator
	type genesisCard struct {
		DenomTLR int64
		Name     string
	}
	genesisCards := []genesisCard{
		{1, "Peace"},
		{5, "Love"},
		{10, "Unity"},
		{25, "Respect"},
		{50, "Knowledge"},
		{100, "Wisdom"},
		{250, "Truth"},
		{500, "Freedom"},
		{1000, "Creator"},
	}
	entries := economy.LoadPreMint(*dataDir)

	// Проверяем не созданы ли уже
	existingGenesis := 0
	for _, e := range entries {
		if e.Rarity == "genesis" {
			existingGenesis++
		}
	}

	if existingGenesis >= 9 {
		fmt.Println("\nℹ️  9 Genesis банкнот уже в pre-mint, пропускаем")
	} else {
		fmt.Println("\n🃏 Куётся Священный Грааль — 9 добродетелей Острова:")
		for _, card := range genesisCards {
			serial := fmt.Sprintf("MB-FIRST-2026-%06d", card.DenomTLR*1000)
			// checking for duplicates
			dup := false
			for _, e := range entries {
				if e.Serial == serial {
					dup = true
					break
				}
			}
			if dup {
				continue
			}

			entries = append(entries, economy.PreMintEntry{
				Serial:         serial,
				DenominationNg: card.DenomTLR * economy.NGPerTLR,
				Rarity:         "genesis",
				Status:         "genesis_reserved",
				PdfPath:        fmt.Sprintf("pre-mint/%s.pdf", serial),
				SigPath:        fmt.Sprintf("pre-mint/%s.sig", serial),
				AddedAt:        time.Now().Format(time.RFC3339),
			})
			fmt.Printf("   ⚜️ %-8s %s — %3d TLR (Genesis ×5)\n", card.Name, serial, card.DenomTLR)
		}
		economy.SavePreMint(*dataDir, entries)
		fmt.Println("✅ Genesis банкноты зарезервированы в pre-mint")
	}

	// 4b. Добавляем Genesis банкноты в banknotes_v2 (чтобы инвестор видел их в холдингах)
	banknotes, _ := economy.LoadBanknotesV2(*dataDir)
	genesisInV2 := 0
	for _, b := range banknotes {
		if b.Rarity == "genesis" {
			genesisInV2++
		}
	}
	if genesisInV2 < 9 {
		for _, card := range genesisCards {
			serial := fmt.Sprintf("MB-FIRST-2026-%06d", card.DenomTLR*1000)
			dup := false
			for _, b := range banknotes {
				if b.Serial == serial {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			banknotes = append(banknotes, economy.BanknoteV2{
				Serial:         serial,
				DenominationNg: card.DenomTLR * economy.NGPerTLR,
				Rarity:         "genesis",
				Multiplier:     5,
				SpecialSeries:  card.Name,
				Holder:         cfg.InvestorPubkey,
				FrozenNg:       0,
				Status:         "genesis_locked",
				MintedAt:       time.Now().UTC().Format(time.RFC3339),
			})
		}
		economy.SaveBanknotesV2(*dataDir, banknotes)
		fmt.Println("👁 Genesis банкноты добавлены в реестр v2 (status: genesis_locked)")
	} else {
		fmt.Println("ℹ️  Genesis банкноты уже в реестре v2")
	}

	// 4c. Создаём оповещение о Genesis
	genesisAnnouncement := fmt.Sprintf(`⚜️ СВЯТОЙ ГРААЛЬ ГЕНЕЗИСА ⚜️

Свершилось! Первый инвестор получил 9 бесценных банкнот Genesis — источник вечной жизни и бесконечного успеха.

9 добродетелей Острова:
  ⚜️ Peace     ⚜️ Love      ⚜️ Unity
  ⚜️ Respect   ⚜️ Knowledge ⚜️ Wisdom
  ⚜️ Truth     ⚜️ Freedom   ⚜️ Creator

Каждая — уникальный артефакт с множителем ×5, напечатанный в единственном экземпляре. Они запечатаны в 3 священных пакета:
  Pack A — Основание (Peace·Knowledge·Freedom)
  Pack B — Сердце (Love·Wisdom·Truth)
  Pack C — Вершина (Unity·Respect·Creator)

Пока они locked — видны, но неприкосновенны. Как только казна достигнет 3× месячных операций — врата рая откроются.

Общий номинал: 5600.00 TLR
Дата пре-минта: %s

«Кто владеет Genesis — владеет вечностью.»`, time.Now().Format("2006-01-02 15:04:05"))

	annPath := filepath.Join(*dataDir, "genesis_announcement.txt")
	if _, err := os.Stat(annPath); os.IsNotExist(err) {
		os.WriteFile(annPath, []byte(genesisAnnouncement), 0644)
		fmt.Println("📜 Оповещение Genesis сохранено")
	}

	// 5. Формируем 3 пакета по 3 карты (алхимические триады)
	//    Pack A: Peace(1) | Knowledge(50) | Freedom(500)     — Основание
	//    Pack B: Love(5)  | Wisdom(100)   | Truth(250)       — Сердце
	//    Pack C: Unity(10)| Respect(25)   | Creator(1000)    — Вершина
	packs := [][]string{
		{"MB-FIRST-2026-001000", "MB-FIRST-2026-050000", "MB-FIRST-2026-500000"},
		{"MB-FIRST-2026-005000", "MB-FIRST-2026-100000", "MB-FIRST-2026-250000"},
		{"MB-FIRST-2026-010000", "MB-FIRST-2026-025000", "MB-FIRST-2026-001000000"},
	}
	packNames := []string{"A — Основание (Peace·Knowledge·Freedom)", "B — Сердце (Love·Wisdom·Truth)", "C — Вершина (Unity·Respect·Creator)"}

	inv := economy.LoadInventory(*dataDir, cfg.InvestorPubkey)
	genesisPackCount := 0
	for _, p := range inv {
		if p.PackType == "genesis" {
			genesisPackCount++
		}
	}

	if genesisPackCount < 3 {
		fmt.Println("\n📦 Куются 3 священных пакета:")
		for i, serials := range packs {
			packID := fmt.Sprintf("genesis_pack_%s", string(rune('A'+i)))
			// проверяем не создан ли уже
			exists := false
			for _, p := range inv {
				if p.PackID == packID {
					exists = true
					break
				}
			}
			if exists {
				continue
			}

			totalPrice := int64(0)
			for _, e := range entries {
				for _, s := range serials {
					if e.Serial == s {
						totalPrice += e.DenominationNg
					}
				}
			}

			inv = append(inv, economy.Pack{
				PackID:    packID,
				Sealed:    true,
				Banknotes: serials,
				PriceNg:   totalPrice,
				Owner:     cfg.InvestorPubkey,
				CreatedAt: time.Now().Format(time.RFC3339),
				PackType:  "genesis",
			})
			fmt.Printf("   ⚜️ Pack %s — 3 карты Genesis, сумма %d ng (%.2f TLR)\n",
				packNames[i], totalPrice, float64(totalPrice)/float64(economy.NGPerTLR))
		}
		economy.SaveInventory(*dataDir, cfg.InvestorPubkey, inv)
		fmt.Println("✅ Пакеты созданы в инвентаре инвестора")
	} else {
		fmt.Println("ℹ️  Genesis пакеты уже созданы")
	}

	// 6. Итог
	ledger = economy.LoadLedger(*dataDir)
	preMint := economy.LoadPreMint(*dataDir)
	auditors = economy.LoadAuditors(*dataDir)

	// Считаем сколько genesis в pre-mint
	genesisCount := 0
	for _, e := range preMint {
		if e.Rarity == "genesis" {
			genesisCount++
		}
	}

	fmt.Println("\n╔══════════════════════════════════════════════════╗")
	fmt.Println("║     GENESIS INIT — СВЯЩЕННЫЙ ГРААЛЬ ВЫКОВАН!  ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Printf("   🏦 Казна:        %d ng (%.2f TLR)\n",
		ledger.Balance("treasury_saint_mary_island"),
		float64(ledger.Balance("treasury_saint_mary_island"))/float64(economy.NGPerTLR))
	fmt.Printf("   💵 Долг:          %g USDT / %d ng\n", debt.PrincipalUSDT, debt.PrincipalNG)
	fmt.Printf("   👁 Аудиторы:      %d шт\n", len(auditors))
	fmt.Println("")
	fmt.Println("   ⚜️ 9 ДОБРОДЕТЕЛЕЙ ГЕНЕЗИСА:")
	for _, card := range genesisCards {
		serial := fmt.Sprintf("MB-FIRST-2026-%06d", card.DenomTLR*1000)
		fmt.Printf("      %-8s %s — %3d TLR 🔒\n", card.Name, serial, card.DenomTLR)
	}
	// пересчитываем после добавления
	inv = economy.LoadInventory(*dataDir, cfg.InvestorPubkey)
	fmt.Println("")
	fmt.Println("   📦 3 СВЯЩЕННЫХ ПАКЕТА:")
	for _, p := range inv {
		if p.PackType == "genesis" {
			fmt.Printf("      %s (%s) — %d карт\n", p.PackID, p.PackType, len(p.Banknotes))
		}
	}
	fmt.Printf("\n   🔑 Pubkey инвестора: %s\n", cfg.InvestorPubkey)
	fmt.Println("")
}

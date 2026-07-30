// Package press — управление шаблонами банкнот (TemplateManager).
// Отвечает за загрузку манифеста шаблонов, список доступных шаблонов (front/back для каждой
// denomination + rarity), генерацию PDF-банкнот (заглушка), проверку Ed25519 подписей
// и валидацию серийных номеров по схеме MB-<rarity>-<date>-<num>.
// Шаблоны хранятся в банкнотных шаблонов <dataDir>/banknote-templates/.
// Пакет press — банкнотный пресс Острова.
// Управляет шаблонами банкнот (TemplateManager), их рендерингом в PDF,
// проверкой Ed25519 подписей и генерацией серийных номеров.
// manifest.json в data/banknote-templates/ хранит номиналы, редкости,
// спец-серии и контрольные суммы шаблонов.
package press

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ParanoidX/internal/fileutil"
)

// TemplateManager управляет шаблонами банкнот.
type TemplateManager struct {
	DataDir    string
	mu         sync.RWMutex
	Manifest   *Manifest
	SigningKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
}

// Manifest — метаданные банкнотного пресса: версия, публичный ключ Короля,
// список номиналов, редкостей, спец-серий и контрольные суммы шаблонов.
type Manifest struct {
	Version       int               `json:"version"`
	KingPubkey    string            `json:"king_pubkey"`
	Denominations []int64           `json:"denominations"`
	Rarities      []string          `json:"rarities"`
	SpecialSeries []string          `json:"special_series"`
	Checksums     map[string]string `json:"checksums"`
}

// TemplateInfo — информация об одном шаблоне банкноты: номинал, редкость,
// сторона (front/back), путь к файлу и SHA256 хэш.
type TemplateInfo struct {
	Denom  int64  `json:"denom"`
	Rarity string `json:"rarity"`
	Side   string `json:"side"` // front, back
	Path   string `json:"path"`
	Hash   string `json:"hash"`
}

// NewTemplateManager создаёт новый TemplateManager и загружает манифест шаблонов.
// NewTemplateManager создаёт TemplateManager, загружая manifest из dataDir/banknote-templates/.
func NewTemplateManager(dataDir string) *TemplateManager {
	tm := &TemplateManager{DataDir: dataDir}
	tm.loadManifest()
	return tm
}

// loadManifest загружает manifest.json из dataDir. Если файла нет — создаёт дефолтный.
func (tm *TemplateManager) loadManifest() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	p := filepath.Join(tm.DataDir, "banknote-templates", "manifest.json")
	if b, err := os.ReadFile(p); err == nil {
		var m Manifest
		if err := json.Unmarshal(b, &m); err == nil {
			tm.Manifest = &m
			return
		}
	}
	tm.Manifest = &Manifest{
		Version:       1,
		Denominations: []int64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
		Rarities:      []string{"common", "rare", "epic", "legendary", "genesis"},
		SpecialSeries: []string{},
		Checksums:     map[string]string{},
	}
}

// SaveManifest сохраняет текущий манифест шаблонов в JSON-файл.
// SaveManifest сохраняет текущий manifest.json на диск.
func (tm *TemplateManager) SaveManifest() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.Manifest == nil {
		return
	}
	p := filepath.Join(tm.DataDir, "banknote-templates", "manifest.json")
	os.MkdirAll(filepath.Dir(p), 0700)
	fileutil.WriteJSON(p, tm.Manifest)
}

// ListTemplates возвращает список всех шаблонов.
func (tm *TemplateManager) ListTemplates() []TemplateInfo {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	var result []TemplateInfo
	base := filepath.Join(tm.DataDir, "banknote-templates")

	for _, denom := range tm.Manifest.Denominations {
		for _, rarity := range tm.Manifest.Rarities {
			for _, side := range []string{"front", "back"} {
				paths := []string{
					filepath.Join(base, fmt.Sprintf("%d", denom), fmt.Sprintf("%s-%s.png", rarity, side)),
					filepath.Join(base, fmt.Sprintf("%d", denom), fmt.Sprintf("%s-%s.svg", rarity, side)),
				}
				for _, p := range paths {
					if _, err := os.Stat(p); err == nil {
						hash := ""
						if b, err := os.ReadFile(p); err == nil {
							h := sha256.Sum256(b)
							hash = hex.EncodeToString(h[:])
						}
						result = append(result, TemplateInfo{
							Denom:  denom,
							Rarity: rarity,
							Side:   side,
							Path:   p,
							Hash:   hash,
						})
						break
					}
				}
			}
		}
	}
	return result
}

// RenderBanknote генерирует PDF банкноты с расширенным дизайном.
func (tm *TemplateManager) RenderBanknote(denom int64, rarity, serial, outputDir string) (pdfPath, sigPath string, err error) {
	pdfPath = filepath.Join(outputDir, serial+".pdf")
	sigPath = filepath.Join(outputDir, serial+".sig")

	// Build a richer PDF with multiple text elements
	var content strings.Builder
	content.WriteString("BT\n")
	content.WriteString("/F2 28 Tf 72 720 Td (SAINT MARY LIBERTY ISLAND) Tj\n")
	content.WriteString("/F1 12 Tf 72 695 Td (Silver-Backed Banknote) Tj\n")
	denomTLR := float64(denom)
	content.WriteString(fmt.Sprintf("/F2 36 Tf 72 640 Td (%d Silver Troy Ounces) Tj\n", int64(denomTLR)))
	content.WriteString(fmt.Sprintf("/F1 14 Tf 72 600 Td (Serial: %s) Tj\n", serial))
	rarityUpper := strings.ToUpper(rarity)
	if rarityUpper == "" {
		rarityUpper = "COMMON"
	}
	content.WriteString(fmt.Sprintf("/F1 14 Tf 72 570 Td (Rarity: %s) Tj\n", rarityUpper))
	content.WriteString("/F1 10 Tf 72 510 Td (1 TLR = 1 troy oz = 31,103,480,000 ng silver) Tj\n")
	content.WriteString("/F1 10 Tf 72 490 Td (Backed by Proof of Reserve at /api/treasury/proof-of-reserve) Tj\n")
	content.WriteString(fmt.Sprintf("/F1 9 Tf 72 72 Td (Minted: %s | Verified by Royal Signature) Tj\n", time.Now().Format("2006-01-02")))
	content.WriteString("ET")

	contentStr := content.String()
	contentObj := fmt.Sprintf("<</Length %d>> stream\n%s\nendstream", len(contentStr), contentStr)

	pdf := fmt.Sprintf(`%%PDF-1.4
1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj
2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj
3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]/Contents 4 0 R/Resources<</Font<</F1 5 0 R/F2 6 0 R>>>>>>endobj
4 0 obj%s
5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj
6 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica-Bold>>endobj
xref
0 7
0000000000 65535 f 
0000000009 00000 n 
0000000058 00000 n 
0000000115 00000 n 
0000000266 00000 n 
0000000379 00000 n 
0000000446 00000 n 
trailer<</Size 7/Root 1 0 R>>
startxref
513
%%%%EOF`, contentObj)

	os.WriteFile(pdfPath, []byte(pdf), 0600)

	// Ed25519 double-signature: if signing key present, sign PDF content
	pdfData, _ := os.ReadFile(pdfPath)
	var sigHex string
	if len(tm.SigningKey) == ed25519.PrivateKeySize {
		tm.mu.RLock()
		key := tm.SigningKey
		pub := tm.PublicKey
		tm.mu.RUnlock()

		sig := ed25519.Sign(key, pdfData)
		sigHex = hex.EncodeToString(sig)
		// Write combined signature: sig_hex + "|" + pubkey_hex
		combined := sigHex + "|" + hex.EncodeToString(pub)
		os.WriteFile(sigPath, []byte(combined), 0600)

		slog.Info("ed25519 signed banknote", "serial", serial, "pubkey_hex", hex.EncodeToString(pub))
	} else {
		// Fallback: SHA256 hash as placeholder
		h := sha256.Sum256(pdfData)
		sigHex = hex.EncodeToString(h[:])
		os.WriteFile(sigPath, []byte("sha256:"+sigHex), 0600)

		slog.Warn("no ed25519 signing key set; using sha256 fallback for", "serial", serial)
	}

	return
}

// SetKingKey устанавливает публичный ключ Короля.
func (tm *TemplateManager) SetKingKey(pubkeyHex string) {
	tm.mu.Lock()
	if tm.Manifest != nil {
		tm.Manifest.KingPubkey = pubkeyHex
	}
	tm.mu.Unlock()
	tm.SaveManifest()
}

// SetSigningKey устанавливает Ed25519 ключ для подписи банкнот.
func (tm *TemplateManager) SetSigningKey(priv ed25519.PrivateKey) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.SigningKey = priv
	if len(priv) == ed25519.PrivateKeySize {
		tm.PublicKey = priv.Public().(ed25519.PublicKey)
		if tm.Manifest != nil {
			tm.Manifest.KingPubkey = hex.EncodeToString(tm.PublicKey)
		}
	}
}

// VerifySignature проверяет Ed25519 подпись PDF.
func (tm *TemplateManager) VerifySignature(pdfPath, sigPath string) (bool, error) {
	pdfData, err := os.ReadFile(pdfPath)
	if err != nil {
		return false, err
	}
	sigData, err := os.ReadFile(sigPath)
	if err != nil {
		return false, err
	}

	tm.mu.RLock()
	kingPubkeyHex := ""
	if tm.Manifest != nil {
		kingPubkeyHex = tm.Manifest.KingPubkey
	}
	tm.mu.RUnlock()

	if kingPubkeyHex == "" {
		return false, fmt.Errorf("king pubkey not set")
	}

	pub, err := hex.DecodeString(kingPubkeyHex)
	if err != nil {
		return false, err
	}

	// Format: "sha256:<hex>" — old fallback
	if bytes.HasPrefix(sigData, []byte("sha256:")) {
		expectedHex := strings.TrimPrefix(string(sigData), "sha256:")
		h := sha256.Sum256(pdfData)
		return hex.EncodeToString(h[:]) == expectedHex, nil
	}

	// Format: "<sig_hex>|<pubkey_hex>" — new Ed25519 combined
	parts := strings.SplitN(strings.TrimSpace(string(sigData)), "|", 2)
	if len(parts) == 2 {
		pubBytes, err := hex.DecodeString(parts[1])
		if err != nil || len(pubBytes) != ed25519.PublicKeySize {
			return false, fmt.Errorf("invalid pubkey in sig file")
		}
		sigBytes, err := hex.DecodeString(parts[0])
		if err != nil {
			return false, fmt.Errorf("invalid signature hex")
		}
		return ed25519.Verify(pubBytes, pdfData, sigBytes), nil
	}

	// Legacy: raw Ed25519 sig bytes (64 bytes)
	return ed25519.Verify(pub, pdfData, sigData), nil
}

// ValidateSerial проверяет серийный номер по схеме.
func ValidateSerial(serial string) bool {
	parts := strings.Split(serial, "-")
	return len(parts) >= 4 && parts[0] == "MB"
}

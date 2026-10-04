// nodeid_test.go фиксирует контракт GenerateNodeID: 128-битное тело,
// соль crypto/rand, привязка к публичному ключу. Ожидания в этих
// тестах — литералы контракта, а не константы реализации: тест,
// сверяющийся с константой из исходника, остаётся зелёным при
// сужении ID (находка 10-03, мутация M1 прошла мимо такого теста).
package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// TestGenerateNodeIDFormat проверяет форму идентификатора:
// "node-" + ровно 32 hex-символа (128-битное тело). Сужение тела
// обратно до 32 бит (8 hex) или чужие символы (например, фигурные
// скобки из "%x" по struct{uint32}) обязаны упасть здесь.
func TestGenerateNodeIDFormat(t *testing.T) {
	id := GenerateNodeID("pk-format")
	if len(id) != 5+32 {
		t.Fatalf("expected 37-char ID (5 prefix + 32 hex), got %d: %q", len(id), id)
	}
	if id[:5] != "node-" {
		t.Fatalf("expected node- prefix, got %q", id[:5])
	}
	for i := 5; i < len(id); i++ {
		c := id[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("expected lowercase hex at position %d, got %q in %q", i, string(c), id)
		}
	}
}

// TestGenerateNodeIDKeyBinding закрепляет хвост тела: последние 8
// байт обязаны совпадать с первыми 8 байтами SHA-256(pubkey).
// Ключевое-ответ вычисляется здесь, в тесте, поэтому удаление
// участия ключа из реализации ломает именно это утверждение.
func TestGenerateNodeIDKeyBinding(t *testing.T) {
	const pk = "pk-binding"
	id := GenerateNodeID(pk)
	if len(id) < 16 {
		t.Fatalf("ID too short to check key tail: %q", id)
	}
	sum := sha256.Sum256([]byte(pk))
	want := hex.EncodeToString(sum[:8])
	if got := id[len(id)-16:]; got != want {
		t.Fatalf("key tail mismatch: want %s got %s", want, got)
	}

	other := GenerateNodeID("pk-other")
	if len(other) < 16 {
		t.Fatalf("ID too short to check key tail: %q", other)
	}
	if id[len(id)-16:] == other[len(other)-16:] {
		t.Fatal("different keys produced identical key tails")
	}
}

// TestGenerateNodeIDSalt: повторный вызов с тем же ключом обязан
// дать другой идентификатор — без соли ID прибивался бы к ключу,
// и каждый перезапуск ноды с утерянной конфигурацией рождал бы
// один и тот же ID.
func TestGenerateNodeIDSalt(t *testing.T) {
	id1 := GenerateNodeID("pk-salt")
	id2 := GenerateNodeID("pk-salt")
	if id1 == id2 {
		t.Fatalf("expected different IDs for the same key, got %q twice", id1)
	}
}

// TestGenerateNodeIDUniqueness: 200 000 выпусков без коллизий.
// При 128-битном теле вероятность коллизии ~10^-28; при старом
// 32-битном теле по парадоксу дней рождения ожидаемо
// ~n^2/2^33 ≈ 4.7 коллизии — тест детерминированно ловит сужение.
func TestGenerateNodeIDUniqueness(t *testing.T) {
	const n = 200000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := GenerateNodeID("pk-uniq")
		if _, dup := seen[id]; dup {
			t.Fatalf("collision at i=%d: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}

// TestGenerateNodeIDEmptyKey: пустой ключ — нормальный ввод
// (нода ещё не обменяла ключи), и он обязан удовлетворять тому же
// контракту, включая хвост-пальцевый след SHA-256("").
func TestGenerateNodeIDEmptyKey(t *testing.T) {
	id := GenerateNodeID("")
	if len(id) != 5+32 {
		t.Fatalf("expected 37-char ID, got %d: %q", len(id), id)
	}
	sum := sha256.Sum256(nil)
	want := hex.EncodeToString(sum[:8])
	if got := id[len(id)-16:]; got != want {
		t.Fatalf("empty-key tail mismatch: want %s got %s", want, got)
	}
}

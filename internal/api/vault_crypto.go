// Package api provides HTTP handlers and API endpoints for the simplex-node server
package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"ParanoidX/internal/vault"
)

type encryptRequest struct {
	FileName string `json:"file_name"`
	Key      string `json:"key"` // base64-encoded 32-byte key
}

type decryptRequest struct {
	FileName string `json:"file_name"`
	Key      string `json:"key"` // base64-encoded 32-byte key
}

func generateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

func encryptFile(data, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return aesGCM.Seal(nonce, nonce, data, nil), nil
}

func decryptFile(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return aesGCM.Open(nil, nonce, ciphertext, nil)
}


// VaultEncryptHandler handles the VaultEncryptHandler HTTP request.
func VaultEncryptHandler(vaultSvc *vault.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req encryptRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		key, err := base64.StdEncoding.DecodeString(req.Key)
		if err != nil {
			key = make([]byte, 32)
			if _, err2 := rand.Read(key); err2 != nil {
				http.Error(w, "key generation failed", 500)
				return
			}
		}
		srcPath := vaultSvc.Download(req.FileName)
		data, err := os.ReadFile(srcPath)
		if err != nil {
			http.Error(w, "file not found: "+err.Error(), 404)
			return
		}
		encrypted, err := encryptFile(data, key)
		if err != nil {
			http.Error(w, "encryption failed: "+err.Error(), 500)
			return
		}
		encName := req.FileName + ".enc"
		encPath := filepath.Join(vaultSvc.Path, encName)
		if err := os.WriteFile(encPath, encrypted, 0600); err != nil {
			http.Error(w, "write failed: "+err.Error(), 500)
			return
		}
		keyB64 := base64.StdEncoding.EncodeToString(key)
		writeJSON(w, map[string]any{
			"ok":          true,
			"encrypted":   encName,
			"key":         keyB64,
			"size":        len(encrypted),
			"original":    req.FileName,
		})
	}
}


// VaultDecryptHandler handles the VaultDecryptHandler HTTP request.
func VaultDecryptHandler(vaultSvc *vault.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var req decryptRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		key, err := base64.StdEncoding.DecodeString(req.Key)
		if err != nil {
			http.Error(w, "invalid key", 400)
			return
		}
		srcPath := vaultSvc.Download(req.FileName)
		data, err := os.ReadFile(srcPath)
		if err != nil {
			http.Error(w, "file not found: "+err.Error(), 404)
			return
		}
		decrypted, err := decryptFile(data, key)
		if err != nil {
			http.Error(w, "decryption failed: "+err.Error(), 500)
			return
		}
		outName := strings.TrimSuffix(req.FileName, ".enc")
		outPath := filepath.Join(vaultSvc.Path, "decrypted_"+outName)
		if err := os.WriteFile(outPath, decrypted, 0600); err != nil {
			http.Error(w, "write failed: "+err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{
			"ok":         true,
			"decrypted":  "decrypted_" + outName,
			"size":       len(decrypted),
		})
	}
}

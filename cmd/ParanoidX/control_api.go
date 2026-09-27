// control_api.go — HTTP-мост между дашбордом/браузером и node-control.py.
//
// Зачем: dashboard.html — статическая страница без доступа к systemd. Здесь
// живёт локальный API, который дёргает тот же node-control.py, что и трей.
// Одна кодовая база для обоих путей — иначе кнопки в UI и в трее разойдутся.
//
// БЕЗОПАСНОСТЬ: эндпоинты меняют состояние ноды, поэтому
//   • только POST (никакого GET-по-ссылке из почты/чата);
//   • только локальный адрес или onion (тот же guard, что у /api/rotate);
//   • никакой аутентификации — поэтому на onion это привилегированный
//     вызов. Список ниже ЗАКРЫТЫЙ, произвольные имена не принимаются.
package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ParanoidX/internal/middleware"
)

// controlScript — путь к node-control.py. Рядом с бинарём ноды.
func controlScript() string {
	exe, err := os.Executable()
	if err == nil {
		cand := filepath.Join(filepath.Dir(exe), "node-control.py")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return "/home/tomas/simplex-node/node-control.py"
}

// controlAllowed — закрытый список. Произвольные имена не проходят.
var controlAllowed = map[string]bool{
	"daemon":  true,
	"xray":    true,
	"docker":  true,
	"monitor": true,
}

var (
	controlMu   sync.Mutex
	controlBusy = map[string]bool{}
)

// controlAction — что делаем с компонентом.
func controlAction(component, action string) (string, bool) {
	switch action {
	case "stop-all":
		return "stop_all", true
	case "rotate":
		return "rotate", true
	case "suspend":
		return "suspend", true
	case "subscription":
		return "subscription", true
	}
	return "", false
}

func writeControlJSON(w http.ResponseWriter, code int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

// registerControlAPI вешает /api/control/* рядом с остальными хендлерами.
func registerControlAPI() {
	// /api/control/<component>/<action>
	http.HandleFunc("/api/control/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		// Эндпоинты останавливают ноду. Доступ — только с локального адреса
		// или с onion, тем же guard'ом, что у /api/rotate и /api/restart.
		// Без него любой, кто дотянется до порта 8080, гасит ноду.
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/control/"), "/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			writeControlJSON(w, http.StatusBadRequest, map[string]any{
				"ok": false, "error": "expected /api/control/<component>/<action>",
			})
			return
		}

		component := parts[0]
		action := ""
		if len(parts) > 1 {
			action = parts[1]
		}

		// Глобальное действие без компонента.
		if !controlAllowed[component] {
			if len(parts) == 1 {
				if _, ok := controlAction("", component); !ok {
					writeControlJSON(w, http.StatusBadRequest, map[string]any{
						"ok": false, "error": "unknown action: " + component,
					})
					return
				}
				runControlBackground(w, component)
				return
			}
			writeControlJSON(w, http.StatusBadRequest, map[string]any{
				"ok": false, "error": "unknown component: " + component,
			})
			return
		}

		if len(parts) != 2 {
			writeControlJSON(w, http.StatusBadRequest, map[string]any{
				"ok": false, "error": "expected /api/control/<component>/<action>",
			})
			return
		}
		switch action {
		case "start", "stop", "restart", "test":
		default:
			writeControlJSON(w, http.StatusBadRequest, map[string]any{
				"ok": false, "error": "unknown action: " + action,
			})
			return
		}
		runComponentAction(w, component, action)
	})

	// /api/control/state — текущее состояние всех компонентов.
	http.HandleFunc("/api/control/state", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		if middleware.DenyIfNotLocalOrOnion(w, r) {
			return
		}
		out, err := runControl("state")
		writeControlJSON(w, http.StatusOK, map[string]any{
			"ok":    err == nil,
			"state": strings.TrimSpace(out),
			"error": errStr(err),
		})
	})
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// runComponentAction выполняет act <component> <action> и ждёт результат.
func runComponentAction(w http.ResponseWriter, component, action string) {
	// Монитор уже занят этой операцией? Не дублируем.
	controlMu.Lock()
	key := component + "/" + action
	if controlBusy[key] {
		controlMu.Unlock()
		writeControlJSON(w, http.StatusTooManyRequests, map[string]any{
			"ok": false, "error": "operation already in progress: " + key,
		})
		return
	}
	controlBusy[key] = true
	controlMu.Unlock()
	defer func() {
		controlMu.Lock()
		delete(controlBusy, key)
		controlMu.Unlock()
	}()

	cmd := exec.Command("python3", controlScript(), "cli", component, action)
	cmd.Dir = filepath.Dir(controlScript())
	// Без глобального Tor-прокси: systemctl и docker работают напрямую.
	cmd.Env = append(os.Environ(), "no_proxy=*", "NO_PROXY=*")

	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.CombinedOutput()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Second):
		_ = cmd.Process.Kill()
		writeControlJSON(w, http.StatusGatewayTimeout, map[string]any{
			"ok": false, "error": "timeout after 200s",
		})
		return
	}

	slog.Info("control action", "component", component, "action", action, "rc", err)
	payload := map[string]any{
		"ok":      err == nil,
		"message": strings.TrimSpace(string(out)),
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	writeControlJSON(w, http.StatusOK, payload)
}

// runControlBackground запускает глобальное действие (stop-all, rotate,
// subscription) и отвечает сразу — они долгие.
func runControlBackground(w http.ResponseWriter, action string) {
	controlMu.Lock()
	if controlBusy[action] {
		controlMu.Unlock()
		writeControlJSON(w, http.StatusTooManyRequests, map[string]any{
			"ok": false, "error": "already in progress: " + action,
		})
		return
	}
	controlBusy[action] = true
	controlMu.Unlock()
	go func() {
		defer func() {
			controlMu.Lock()
			delete(controlBusy, action)
			controlMu.Unlock()
		}()
		cmd := exec.Command("python3", controlScript(), "cli", "--global", action)
		cmd.Dir = filepath.Dir(controlScript())
		cmd.Env = append(os.Environ(), "no_proxy=*", "NO_PROXY=*")
		out, err := cmd.CombinedOutput()
		slog.Info("control global", "action", action, "rc", err, "out", truncate(string(out), 400))
	}()

	writeControlJSON(w, http.StatusAccepted, map[string]any{
		"ok":      true,
		"message": action + " запущено в фоне, результат придёт в Telegram",
	})
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func runControl(args ...string) (string, error) {
	cmd := exec.Command("python3", append([]string{controlScript(), "cli"}, args...)...)
	cmd.Dir = filepath.Dir(controlScript())
	cmd.Env = append(os.Environ(), "no_proxy=*", "NO_PROXY=*")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

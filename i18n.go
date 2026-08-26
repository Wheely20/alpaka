package main

import (
	"os"
	"strings"
)

// getSystemLanguage liest die Sprache des Betriebssystems aus (Fallback: Englisch)
func getSystemLanguage() string {
	langEnv := os.Getenv("LANG")
	if strings.HasPrefix(langEnv, "de") {
		return "de"
	}

	return "en"
}

var translations = map[string]map[string]string{
	"de": {
		"root_use":             "alpaka",
		"root_short":           "Alpaka – Ein leichter Manager für llama.cpp",
		"root_long":            "Alpaka vereinfacht die Nutzung lokaler KI-Modelle mit llama.cpp",
		"run_use":              "run [modellname]",
		"run_short":            "Startet ein KI-Modell im Chat-Modus",
		"run_ctx":              "Legt die maximale Kontextgröße fest",
		"run_sys":              "Definiert einen eigenen System-Prompt für die KI",
		"serve_short":          "Startet einen lokalen API-Server für das Modell",
		"load_use":             "load [url] (als [name])",
		"load_short":           "Lädt ein GGUF-Modell herunter",
		"list_use":             "list",
		"list_short":           "Zeigt alle heruntergeladenen Modelle an",
		"delete_use":           "delete [modellname]",
		"delete_short":         "Löscht ein heruntergeladenes Modell von der Festplatte",
		"config_use":           "config",
		"config_short":         "Verwaltet Alpaka-Konfiguration",
		"config_long":          "Zeigt und setzt Einstellungen",
		"config_set_cli_use":   "set-cli [pfad]",
		"config_set_cli_short": "Speichert einen festen Pfad zur llama-cli in ~/.alpaka/config.json",
		"config_show_use":      "show",
		"config_show_short":    "Zeigt die aktuell gespeicherte Konfiguration",
		"invalid_load_syntax":  "ungültige Syntax. Nutze: alpaka download <url> as <name>",
		"run_err":              "Modell-Datei nicht gefunden",
		"selfupdate_use":       "selfupdate",
		"selfupdate_short":     "Aktualisiert Alpaka auf die neueste Version",
	},
}

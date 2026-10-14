package main

import (
	"net/http"
	"testing"
)

func TestLlamaDownloadURL(t *testing.T) {
	// 1. Tag abfragen
	tag, err := getLatestLlamaTag()
	if err != nil {
		t.Fatalf("Fehler beim Abrufen des neuesten Tags: %v", err)
	}

	// 2. URL für das aktuelle System generieren
	url, err := getLlamaDownloadURL(tag)
	if err != nil {
		t.Fatalf("Fehler beim Generieren der URL: %v", err)
	}

	// 3. Nur den Header abfragen, um Traffic zu sparen (prüft auf 404)
	resp, err := http.Head(url)
	if err != nil {
		t.Fatalf("URL nicht erreichbar: %v", err)
	}
	defer resp.Body.Close()

	// 302 (Found/Redirect) ist bei GitHub Releases auch in Ordnung
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		t.Errorf("URL ungültig! Status: %s | URL: %s", resp.Status, url)
	}
}

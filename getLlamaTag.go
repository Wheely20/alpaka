package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// githubRelease spiegelt nur das Feld wider, das wir aus der API brauchen
type githubRelease struct {
	TagName string `json:"tag_name"`
}

// getLatestLlamaTag fragt die GitHub API nach der neuesten Version
func getLatestLlamaTag() (string, error) {
	url := "https://api.github.com/repos/ggml-org/llama.cpp/releases"

	client := &http.Client{}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Alpaka-CLI")

	// Token auslesen und als Header mitschicken (falls vorhanden)
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API Fehler: %s", resp.Status)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return "", err
	}

	if len(releases) > 0 {
		return releases[0].TagName, nil
	}

	return "", fmt.Errorf("No releases found")
}

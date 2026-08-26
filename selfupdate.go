package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/schollz/progressbar/v3"
)

func getLatestAlpakaVersion() (string, error) {
	url := "https://api.github.com/repos/Wheely20/alpaka/releases/latest"
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API Error: %s", resp.Status)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}

func performUpdate() error {
	// 1. OS & Architektur ermitteln
	osName := runtime.GOOS
	arch := runtime.GOARCH

	filename := fmt.Sprintf("alpaka-%s-%s", osName, arch)
	if osName == "windows" {
		filename += ".exe"
	}

	// 2. Herausfinden, wo die ausführbare Alpaka-Datei aktuell liegt
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("Couldn't find path to executable: %w", err)
	}

	// Symlinks auflösen
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("Couldn't resolve symlinks: %w", err)
	}

	// Package-Manager Exceptions:
	// MacPorts installiert nach /opt/local/bin
	if strings.HasPrefix(exePath, "/opt/local/bin") {
		return fmt.Errorf("❌ Alpaka was installed via MacPorts. Please use 'sudo port upgrade alpaka' for the update.")
	}

	// System-Paketmanager (APT, DNF, Pacman) installieren meistens nach /usr/bin
	if strings.HasPrefix(exePath, "/usr/bin") {
		return fmt.Errorf("❌ Alpaka was installed via a system package manager. Please use that for the update.")
	}

	fmt.Println("🔍 Checking for Updates...")

	// Version-Check
	latestVersion, err := getLatestAlpakaVersion()
	if err != nil {
		return fmt.Errorf("Couldn't query latest version: %w", err)
	}

	if latestVersion == version {
		fmt.Printf("✨ Alpaka is already up to date (%s)!\n", version)
		return nil
	}

	fmt.Printf("🔄 New update found! Updating from %s to %s...\n", version, latestVersion)

	// 3. Temporäre Datei für den Download neben der echten Datei erstellen
	tempPath := exePath + ".new"
	out, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("Couldn't create temporary file (missing admin/sudo privileges?): %w", err)
	}

	// URL zum neuesten GitHub-Release
	url := fmt.Sprintf("https://github.com/Wheely20/alpaka/releases/latest/download/%s", filename)

	client := &http.Client{}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Alpaka-CLI")

	// 4. Herunterladen
	resp, err := client.Do(req)
	if err != nil {
		out.Close()
		os.Remove(tempPath)
		return fmt.Errorf("Download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		out.Close()
		os.Remove(tempPath)
		return fmt.Errorf("No update found. Status: %s", resp.Status)
	}

	// Ladebalken
	bar := progressbar.DefaultBytes(
		resp.ContentLength,
		"⬇️ Downloading Update",
	)

	_, err = io.Copy(io.MultiWriter(out, bar), resp.Body)
	if err != nil {
		out.Close()
		os.Remove(tempPath)
		return fmt.Errorf("Error saving file: %w", err)
	}

	// Datei schließen
	out.Close()

	// 5. Rechte setzen
	if osName != "windows" {
		if err := os.Chmod(tempPath, 0755); err != nil {
			return fmt.Errorf("Couldn't set file permissions: %w", err)
		}
	}

	// 6. Austsausch der alten Datei gegen die neue
	oldPath := exePath + ".old"

	// Alte .old-Datei von vorherigen Updates löschen
	os.Remove(oldPath)

	// Laufende Datei in .old umbenennen
	if err := os.Rename(exePath, oldPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("Error renaming current file: %w", err)
	}

	// Neue Datei an die Stelle der alten setzen
	if err := os.Rename(tempPath, exePath); err != nil {
		// Rollback: Alte Datei wiederherstellen
		os.Rename(oldPath, exePath)
		return fmt.Errorf("Error replacing file: %w", err)
	}

	fmt.Println("✅ Alpaka was successfully updated to the latest version!")
	return nil
}

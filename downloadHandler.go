package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/sync/errgroup"
)

// downloadFile lädt eine Datei von einer URL herunter und speichert sie am Zielpfad
func downloadFile(url string, destPath string) error {
	fmt.Printf("⬇️  Downloading file from: %s\n", url)

	// 1. HTTP GET-Anfrage erstellen
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("Error downloading: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Bad HTTP status: %s", resp.Status)
	}

	// 2. Lokale Datei erstellen
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("Could not create destination file: %w", err)
	}
	defer out.Close()

	// 3. Ladebalken initialisieren
	// resp.ContentLength liefert die Gesamtgröße der Datei in Bytes
	bar := progressbar.DefaultBytes(
		resp.ContentLength,
		"Downloading",
	)

	// 4. Daten aus dem Internet direkt in die Datei schreiben (streamen)
	_, err = io.Copy(io.MultiWriter(out, bar), resp.Body)
	if err != nil {
		return fmt.Errorf("Error saving data: %w", err)
	}

	fmt.Println("✅ Download successful!")
	return nil
}

// downloadFileParallel lädt eine Datei parallel in Chunks herunter, um Server-Drosselungen zu umgehen.
func downloadFileParallel(url string, destPath string) error {
	fmt.Printf("⬇️ Download started for: %s\n", url)

	// 1. HEAD-Anfrage, um die Dateigröße (Content-Length) zu ermitteln
	resp, err := http.Head(url)
	if err != nil {
		return fmt.Errorf("Error in HEAD request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Bad HTTP status in HEAD: %s", resp.Status)
	}

	totalSize := resp.ContentLength
	if totalSize <= 0 {
		return fmt.Errorf("Invalid file size reported by server")
	}

	// 2. Lokale Datei in voller Größe vordefinieren (Wichtig für wahlfreien Zugriff)
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("Could not create destination file: %w", err)
	}
	defer out.Close()

	if err := out.Truncate(totalSize); err != nil {
		return fmt.Errorf("Could not reserve file space: %w", err)
	}

	// 3. Ladebalken initialisieren (ist standardmäßig thread-sicher)
	bar := progressbar.DefaultBytes(totalSize, "Downloading in parallel")

	// 4. Download in 4 Chunks aufteilen (Sweet Spot für Hugging Face)
	numChunks := 4
	chunkSize := totalSize / int64(numChunks)

	g, ctx := errgroup.WithContext(context.Background())

	for i := 0; i < numChunks; i++ {
		// Variablen für die Goroutine fixieren
		chunkID := i
		start := int64(chunkID) * chunkSize
		end := start + chunkSize - 1

		// Der letzte Chunk übernimmt die restlichen Bytes
		if chunkID == numChunks-1 {
			end = totalSize - 1
		}

		g.Go(func() error {
			// HTTP GET mit Range-Header für diesen spezifischen Chunk erstellen
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return err
			}
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

			// Optional: Füge hier deinen Hugging Face Token hinzu, falls vorhanden
			// req.Header.Set("Authorization", "Bearer hf_DEIN_TOKEN")

			client := &http.Client{}
			chunkResp, err := client.Do(req)
			if err != nil {
				return err
			}
			defer chunkResp.Body.Close()

			if chunkResp.StatusCode != http.StatusPartialContent && chunkResp.StatusCode != http.StatusOK {
				return fmt.Errorf("Server does not support range downloads (Status: %s)", chunkResp.Status)
			}

			// Eigene Datei-Instanz pro Goroutine öffnen, um Race Conditions beim Schreiben zu verhindern
			filePart, err := os.OpenFile(destPath, os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			defer filePart.Close()

			// Schreib-Zeiger an den Startpunkt des Chunks setzen
			_, err = filePart.Seek(start, io.SeekStart)
			if err != nil {
				return err
			}

			// Daten streamen und gleichzeitig den globalen Ladebalken füttern
			buffer := make([]byte, 32*1024)
			for {
				n, readErr := chunkResp.Body.Read(buffer)
				if n > 0 {
					// In den richtigen Abschnitt der Datei schreiben
					_, writeErr := filePart.Write(buffer[:n])
					if writeErr != nil {
						return writeErr
					}

					// Progressbar direkt aktualisieren (ist thread-sicher)
					_ = bar.Add(n)
				}
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					return readErr
				}
			}

			return nil
		})
	}

	// 5. Warten, bis alle Goroutines fertig sind oder ein Fehler auftritt
	if err := g.Wait(); err != nil {
		return fmt.Errorf("Error during parallel download: %w", err)
	}

	fmt.Println("\n✅ Download successful and file assembled!")
	return nil
}

// getLlamaDownloadURL ermittelt die passende Download-URL für das aktuelle System.
func getLlamaDownloadURL(tag string) (string, error) {
	// Basis-URL (GitHub Releases von llama.cpp)
	baseURL := fmt.Sprintf("https://github.com/ggml-org/llama.cpp/releases/download/%s/", tag)
	var filename string

	// runtime.GOOS liefert das Betriebssystem (darwin = macOS, linux = Linux, windows = Windows)
	// runtime.GOARCH liefert die Architektur (amd64 = Intel/AMD 64-bit, arm64 = ARM 64-bit / Apple Silicon)
	switch runtime.GOOS {
	case "darwin": // macOS
		if runtime.GOARCH == "arm64" {
			filename = fmt.Sprintf("llama-%s-bin-macos-arm64.tar.gz", tag) // Für M1/M2/M3 Macs
		} else {
			filename = fmt.Sprintf("llama-%s-bin-macos-x64.tar.gz", tag) // Für Intel Macs
		}
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			filename = fmt.Sprintf("llama-%s-bin-ubuntu-x64.tar.gz", tag)
		case "arm64":
			filename = fmt.Sprintf("llama-%s-bin-ubuntu-arm64.tar.gz", tag)
		case "s390x":
			filename = fmt.Sprintf("llama-%s-bin-ubuntu-s390x.tar.gz", tag)
		default:
			return "", fmt.Errorf("Linux architecture '%s' is currently unsupported", runtime.GOARCH)
		}
	case "windows":
		if runtime.GOARCH == "amd64" {
			filename = fmt.Sprintf("llama-%s-bin-win-cpu-x64.zip", tag)
		} else {
			return "", fmt.Errorf("Windows architecture '%s' is currently unsupported", runtime.GOARCH)
		}
	default:
		return "", fmt.Errorf("Operating system '%s' is not supported by Alpaka", runtime.GOOS)
	}

	return baseURL + filename, nil
}

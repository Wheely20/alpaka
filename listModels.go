package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// listModels liest den models-Ordner aus und zeigt alle GGUF-Dateien an
func listModels() error {
	alpakaDir, err := getAlpakaDir()
	if err != nil {
		return fmt.Errorf("Konnte Alpaka-Verzeichnis nicht laden: %w", err)
	}

	modelsDir := filepath.Join(alpakaDir, "models")

	// Lese den Inhalt des Verzeichnisses
	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		// Wenn der Ordner (noch) nicht existiert, fangen wir das sauber ab
		if os.IsNotExist(err) {
			fmt.Println("No models found. Use 'alpaka load <URL>'.")
			return nil
		}
		return fmt.Errorf("Error reading the model directory: %w", err)
	}

	// Einträge filtern und nur GGUF-Dateien behalten
	var ggufModels []os.DirEntry

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".gguf") {
			ggufModels = append(ggufModels, entry)
		}
	}

	// Prüfen, ob Modelle gefunden wurden und diese ausgeben
	if len(ggufModels) > 0 {
		fmt.Println("Available models:")

		for _, entry := range ggufModels {
			// Dateigröße ermitteln
			info, err := entry.Info()
			sizeStr := ""
			if err == nil {
				sizeMB := float64(info.Size()) / (1024 * 1024)
				if sizeMB >= 1024 {
					sizeStr = fmt.Sprintf("(%.2f GB)", sizeMB/1024)
				} else {
					sizeStr = fmt.Sprintf("(%.0f MB)", sizeMB)
				}
			}

			// ".gguf" abschneiden
			modelName := strings.TrimSuffix(entry.Name(), ".gguf")
			fmt.Printf("  - %s %s\n", modelName, sizeStr)
		}
	} else {
		fmt.Println("No GGUF models found in the directory")
	}

	return nil
}

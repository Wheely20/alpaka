package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// loadModel kümmert sich um die Vorbereitung und den Download eines GGUF-Modells
func loadModel(modelURL string, customName string) error {
	alpakaDir, err := getAlpakaDir()
	if err != nil {
		return fmt.Errorf("Could not load Alpaka directory: %w", err)
	}
	// Stelle sicher, dass der models-Ordner existiert
	if err := initAlpakaDir(); err != nil {
		return fmt.Errorf("Could not initialize directories: %w", err)
	}

	// Extrahiere den Dateinamen aus der URL (z.B. url.../tinyllama.gguf -> tinyllama.gguf)
	fileName := filepath.Base(modelURL)

	if customName != "" {
		fileName = customName
		// Sicherstellen, dass die Endung immer .gguf ist
		if !strings.HasSuffix(fileName, ".gguf") {
			fileName += ".gguf"
		}
	}

	// Kleine Warnung, falls der Nutzer aus Versehen eine Webseite statt einer Datei angibt
	if !strings.HasSuffix(fileName, ".gguf") {
		fmt.Println("⚠️  Warning: The URL does not end with .gguf. Make sure it is a direct GGUF model.")
	}

	destPath := filepath.Join(alpakaDir, "models", fileName)

	// Prüfen, ob das Modell schon existiert, um unnötige Downloads zu vermeiden
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("Model already exists at: %s", destPath)
	}

	fmt.Printf("🚀 Starting download for model: %s\n", fileName)

	// Wir nutzen deine bereits existierende Funktion!
	err = downloadFile(modelURL, destPath)
	if err != nil {
		return fmt.Errorf("Error downloading the model: %w", err)
	}

	fmt.Printf("✅ Model successfully saved under: %s\n", destPath)
	return nil
}

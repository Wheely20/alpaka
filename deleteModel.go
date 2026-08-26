package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// deleteModel löscht ein lokales GGUF-Modell
func deleteModel(modelName string) error {
	alpakaDir, err := getAlpakaDir()
	if err != nil {
		return fmt.Errorf("Could not load Alpaka directory: %w", err)
	}

	// Falls ".gguf" nicht angegeben wurde
	modelName = ensureGGUFSuffix(modelName)

	modelPath := filepath.Join(alpakaDir, "models", modelName)

	// Prüfen, ob die Datei überhaupt existiert
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return fmt.Errorf("Model '%s' was not found", modelName)
	}

	// Datei endgültig löschen
	err = os.Remove(modelPath)
	if err != nil {
		return fmt.Errorf("Error deleting model: %w", err)
	}

	fmt.Printf("'%s' was successfully deleted.\n", modelName)
	return nil
}

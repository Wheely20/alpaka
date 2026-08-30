package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// getPersonasPath gibt den Pfad zur personas.json zurück
func getPersonasPath() (string, error) {
	alpakaDir, err := getAlpakaDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(alpakaDir, "personas.json"), nil
}

// loadPersonas liest die personas.json und gibt eine Map zurück
func loadPersonas() (map[string]string, error) {
	path, err := getPersonasPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Wenn die Datei nicht existiert, gib eine leere Map zurück
			return make(map[string]string), nil
		}
		return nil, err
	}

	var personas map[string]string
	if err := json.Unmarshal(data, &personas); err != nil {
		return nil, err
	}

	return personas, nil
}

// savePersonas schreibt die Persona-Map in die personas.json
func savePersonas(personas map[string]string) error {
	path, err := getPersonasPath()
	if err != nil {
		return err
	}

	// Map in JSON umwandeln
	data, err := json.MarshalIndent(personas, "", "  ")
	if err != nil {
		return err
	}

	// Datei schreiben (0644 = Lese-/Schreibrechte für den Besitzer)
	return os.WriteFile(path, data, 0644)
}

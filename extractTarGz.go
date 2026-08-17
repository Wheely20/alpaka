package main

import (
	"fmt"
	"os"
	"os/exec"
)

// extractTarGz entpackt das Archiv in das Zielverzeichnis
func extractTarGz(archivePath string, destDir string) error {
	fmt.Println("📦 Extracting archive...")

	// Führt den Befehl aus: tar -xzf [archiv] -C [zielordner]
	cmd := exec.Command("tar", "-xzf", archivePath, "-C", destDir)

	// Fängt eventuelle Fehlermeldungen von 'tar' ab
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("Error extracting archive: %w\nOutput: %s", err, string(output))
	}

	// Räume auf: Lösche das komprimierte Archiv nach erfolgreichem Entpacken
	fmt.Println("Deleting temporary archive...")
	return os.Remove(archivePath)
}

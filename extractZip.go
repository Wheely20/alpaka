package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// extractZip entpackt ein ZIP-Archiv in das Zielverzeichnis
func extractZip(archivePath string, destDir string) error {
	fmt.Println("📦 Extracting ZIP archive...")

	// Öffne die ZIP-Datei
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("Error opening ZIP file: %w", err)
	}
	defer r.Close()

	// Gehe durch alle Dateien im ZIP-Archiv
	for _, f := range r.File {
		cleanDest := filepath.Clean(destDir)
		fpath := filepath.Join(cleanDest, f.Name)
		if !strings.HasPrefix(fpath, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("Illegal file path in zip: %s", f.Name)
		}

		// Wenn Ordner; erstellen
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		// Überprüfe, dass Ordner für die Datei existiert
		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		// Zieldatei erstellen
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		// Öffne die Datei innerhalb des ZIPs
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		// Inhalt kopieren
		_, err = io.Copy(outFile, rc)

		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}

	fmt.Println("Deleting temporary ZIP archive...")
	return os.Remove(archivePath)
}

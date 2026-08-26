package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// extractTarGz entpackt ein .tar.gz-Archiv in das Zielverzeichnis
func extractTarGz(archivePath string, destDir string) error {
	fmt.Println("📦 Extracting archive...")

	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("Error opening archive: %w", err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("Error creating gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	cleanDest := filepath.Clean(destDir)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break // Ende des Archivs
		}
		if err != nil {
			return fmt.Errorf("Error reading tar entry: %w", err)
		}

		targetPath := filepath.Join(cleanDest, header.Name)

		// Schutz vor Path Traversal / Tar Slip
		if !strings.HasPrefix(targetPath, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("Illegal file path in tar archive: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("Error creating directory: %w", err)
			}
		case tar.TypeReg:
			// Sicherstellen, dass der übergeordnete Ordner existiert
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("Error creating parent directory: %w", err)
			}

			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("Error creating file: %w", err)
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("Error writing file content: %w", err)
			}
			outFile.Close()
		}
	}

	fmt.Println("Deleting temporary archive...")
	return os.Remove(archivePath)
}

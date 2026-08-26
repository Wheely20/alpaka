package main

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractTarGz(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	destDir := filepath.Join(tmpDir, "extracted")

	// Erstelle ein Test-.tar.gz Archiv
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Failed to create test archive: %v", err)
	}

	gw := gzip.NewWriter(file)
	tw := tar.NewWriter(gw)

	body := []byte("hello alpaka")
	hdr := &tar.Header{
		Name: "test.txt",
		Mode: 0644,
		Size: int64(len(body)),
	}

	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("Failed to write header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("Failed to write body: %v", err)
	}

	tw.Close()
	gw.Close()
	file.Close()

	// Führe extractTarGz aus
	if err := extractTarGz(archivePath, destDir); err != nil {
		t.Fatalf("extractTarGz failed: %v", err)
	}

	// Überprüfe, ob die Datei korrekt entpackt wurde
	extractedFile := filepath.Join(destDir, "test.txt")
	content, err := os.ReadFile(extractedFile)
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}

	if string(content) != "hello alpaka" {
		t.Errorf("Expected 'hello alpaka', got '%s'", string(content))
	}
}

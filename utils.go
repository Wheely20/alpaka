package main

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// askForConfirmation fragt den Nutzer nach einer Bestätigung [y/N]
func askForConfirmation(prompt string) bool {
	reader := bufio.NewReader(os.Stdin)

	// Ausgabe z.B.: "llama.cpp herunterladen? [y/N]: "
	fmt.Printf("%s [y/N]: ", prompt)

	response, err := reader.ReadString('\n')
	if err != nil {
		return false // Bei Fehlern abrechen
	}

	// Whitespaces und Zeilenumbrüche entfernen und in Kleinbuchstaben umwandeln
	response = strings.ToLower(strings.TrimSpace(response))

	return response == "y" || response == "yes"
}

func ensureGGUFSuffix(name string) string {
	if !strings.HasSuffix(strings.ToLower(name), ".gguf") {
		return name + ".gguf"
	}
	return name
}

func waitForServer(url string) error {
	spinner := []string{"/", "-", "\\", "|"}
	client := http.Client{Timeout: 2 * time.Second}

	// Wait max 40 seconds
	for i := 0; i < 400; i++ {
		// \r rewrites the current line
		fmt.Printf("\rLoading model... %s", spinner[i%len(spinner)])

		if i%10 == 0 {
			resp, err := client.Get(url + "/health")
			if err == nil && resp.StatusCode == 200 {
				// when server is ready
				fmt.Printf("\rLoading model...\n")
				return nil
			}
		}

		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println()
	return fmt.Errorf("Error: Model took too long to load.")
}

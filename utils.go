package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
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

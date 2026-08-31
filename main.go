package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/spf13/cobra"
)

// initAlpakaDir stellt sicher, dass die grundlegende Ordnerstruktur existiert
func initAlpakaDir() error {
	alpakaDir, err := getAlpakaDir()
	if err != nil {
		return err
	}

	// Erstelle den .alpaka Ordner
	if err := os.MkdirAll(alpakaDir, 0755); err != nil {
		return err
	}
	// Erstelle Unterordner
	if err := os.MkdirAll(filepath.Join(alpakaDir, "bin"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(alpakaDir, "models"), 0755); err != nil {
		return err
	}

	return nil
}

// Gibt den Pfad zum Alpaka-Verzeichnis zurück (~/.alpaka)
func getAlpakaDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	alpakaDir := filepath.Join(homeDir, ".alpaka")
	return alpakaDir, nil
}

// Config speichert persistente Einstellungen in ~/.alpaka/config.json
type Config struct {
	LlamaCliPath string `json:"llama_cli_path,omitempty"`
}

func getConfigPath() (string, error) {
	alpakaDir, err := getAlpakaDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(alpakaDir, "config.json"), nil
}

func loadConfig() (*Config, error) {
	cfgPath, err := getConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &Config{}, err
	}
	return &cfg, nil
}

func saveConfig(cfg *Config) error {
	cfgPath, err := getConfigPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, data, 0644)
}

// Hilfsfunktion: Gibt den Pfad zur llama-cli zurück.
// Reihenfolge: LLAMA_CLI_PATH env, config.json (wenn vorhanden und gültig), PATH (LookPath), ~/.alpaka/bin
func getLlamaCliPath() (string, error) {
	if custom := os.Getenv("LLAMA_CLI_PATH"); custom != "" {
		return custom, nil
	}

	// Prüfe config.json
	cfg, cfgErr := loadConfig()
	if cfgErr == nil && cfg.LlamaCliPath != "" {
		if _, err := os.Stat(cfg.LlamaCliPath); err == nil {
			return cfg.LlamaCliPath, nil
		}
	}

	// Suche im PATH
	if path, err := exec.LookPath("llama-cli"); err == nil {
		// Versuche zu speichern, falls möglich
		if cfg == nil {
			cfg = &Config{}
		}
		cfg.LlamaCliPath = path
		if err := saveConfig(cfg); err != nil {
			fmt.Printf("Could not save config: %v\n", err)
		}
		return path, nil
	}

	// Fallback: ~/.alpaka/bin/llama-cli
	alpakaDir, err := getAlpakaDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(alpakaDir, "bin", "llama-cli"), nil
}

// Prüfung, ob llama.cpp existiert
func ensureLlamaInstalled() (string, error) {
	llamaPath, err := getLlamaCliPath()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(llamaPath); os.IsNotExist(err) {
		fmt.Println("🦙 llama.cpp was not found.")
		fmt.Printf("Operating system: %s (%s)\n", runtime.GOOS, runtime.GOARCH)

		// Nutzer um Bestätigung bitten
		if !askForConfirmation("Download llama.cpp (~20 MB)?") {
			// Wenn Nutzer ablehnt
			return "", fmt.Errorf("Download cancelled. Alpaka needs llama.cpp to function; please install it manually.")
		}

		fmt.Println("➡️  Downloading llama.cpp...")

		if err := initAlpakaDir(); err != nil {
			return "", fmt.Errorf("Could not create Alpaka directories: %w", err)
		}

		tag, err := getLatestLlamaTag()
		if err != nil {
			return "", fmt.Errorf("Could not determine latest llama.cpp tag: %w", err)
		}

		downloadURL, err := getLlamaDownloadURL(tag)
		if err != nil {
			return "", fmt.Errorf("Could not determine download URL: %w", err)
		}

		// 1. Temporären Pfad für das Archiv definieren
		archivePath := llamaPath + ".archive"

		// 2. Das Archiv herunterladen
		err = downloadFile(downloadURL, archivePath)
		if err != nil {
			return "", fmt.Errorf("Download failed: %w", err)
		}

		// 3. Archiv in den bin-Ordner entpacken
		destDir := filepath.Dir(llamaPath)
		if runtime.GOOS == "windows" {
			err = extractZip(archivePath, destDir)
		} else {
			err = extractTarGz(archivePath, destDir)
		}
		if err != nil {
			return "", fmt.Errorf("Could not extract archive: %w", err)
		}

		// 4. llama-cli finden und in den richtigen Ordner verschieben
		binaryName := filepath.Base(llamaPath)
		err = filepath.Walk(destDir, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			// Wenn Datei in Unterordner
			if !info.IsDir() && info.Name() == binaryName && path != llamaPath {
				// Nach ~/.alpaka/bin/llama-cli verschieben
				os.Rename(path, llamaPath)
			}
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("Could not locate binary after extraction: %w", err)
		}

		if err := os.Chmod(llamaPath, 0755); err != nil {
			return "", fmt.Errorf("Could not make file executable: %w", err)
		}
	}

	return llamaPath, nil
}

func runModel(modelName string, ctxSize int, systemPrompt string) error {
	// 1. Pfad zur llama-server Binary finden
	llamaCliPath, err := ensureLlamaInstalled()
	if err != nil {
		return err
	}
	serverName := "llama-server"
	if runtime.GOOS == "windows" {
		serverName = "llama-server.exe"
	}
	serverPath := filepath.Join(filepath.Dir(llamaCliPath), serverName)

	alpakaDir, _ := getAlpakaDir()
	modelPath := filepath.Join(alpakaDir, "models", ensureGGUFSuffix(modelName))

	fmt.Printf("Starting background server for model: %s\n", modelName)
	ctxString := strconv.Itoa(ctxSize)

	// 2. Server im Hintergrund auf Port 8081 starten
	cmd := exec.Command(serverPath, "-m", modelPath, "-c", ctxString, "--port", "8081", "--host", "127.0.0.1")
	// Wir verstecken die Server-Logs, indem wir Stdout/Stderr NICHT mit os.Stdout verknüpfen
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Could not start backend server: %w", err)
	}

	// Kill the server process when alpaka exits
	defer cmd.Process.Kill()

	// 3. MCP Client initialisieren
	ctx := context.Background()
	mcpClient, err := startMCPClient(ctx)
	if err != nil {
		return fmt.Errorf("Could not load internal tools: %w", err)
	}
	defer mcpClient.Close()

	// 4. Werkzeuge abfragen
	tools, err := getAvailableTools(ctx, mcpClient)
	if err != nil {
		return fmt.Errorf("Error while fetching tools: %w", err)
	}
	if len(tools) > 0 {
		fmt.Printf("Native MCP Tools active (%d tools loaded)\n", len(tools))
	}

	// Kurze Pause, damit der llama-server hochfahren kann (in Zukunft eleganter über einen Ping lösbar)
	fmt.Println("Loading model into memory... (this may take a moment)")

	// 5. Chat starten
	startTerminalChat(ctx, "http://127.0.0.1:8081", mcpClient, systemPrompt, tools)

	return nil
}

// runServer startet den llama.cpp API-Server
func runServer(modelName string, ctxSize int, port int) error {
	// 1. Sicherstellen, dass llama.cpp installiert ist
	llamaCliPath, err := ensureLlamaInstalled()
	if err != nil {
		return err
	}

	// 2. Den Pfad zur Server-Binary finden (selber Ordner wie llama-cli)
	binDir := filepath.Dir(llamaCliPath)
	serverName := "llama-server"
	if runtime.GOOS == "windows" {
		serverName = "llama-server.exe"
	}
	serverPath := filepath.Join(binDir, serverName)

	if _, err := os.Stat(serverPath); os.IsNotExist(err) {
		return fmt.Errorf("Server binary ('%s') not found in: %s", serverName, binDir)
	}

	// 3. Modell-Pfad auflösen
	alpakaDir, _ := getAlpakaDir()
	modelPath := filepath.Join(alpakaDir, "models", ensureGGUFSuffix(modelName))

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return fmt.Errorf("Model file '%s' not found at: %s", modelName, modelPath)
	}

	fmt.Printf("🌐 Starting server on port %d with model: %s\n", port, modelName)

	ctxString := strconv.Itoa(ctxSize)

	// 4. Server-Befehl zusammenbauen
	cmd := exec.Command(serverPath,
		"-m", modelPath,
		"-c", ctxString, // context size
		"--port", fmt.Sprintf("%d", port),
		"--host", "127.0.0.1",
	)

	// Verbindet Stdin, Stdout und Stderr mit dem Terminal
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// --- COBRA CLI CONFIGURATION ---

var version = "dev"

var rootCmd = &cobra.Command{
	Use:     "alpaka",
	Short:   "Alpaka – A lightweight manager for llama.cpp",
	Long:    `Alpaka simplifies the usage of local AI models with llama.cpp`,
	Version: version,
}

var ctxSize int
var sysPrompt string
var personaName string

var runCmd = &cobra.Command{
	Use:          "run [modelname]",
	Short:        "Starts an AI model in chat mode",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1), // Erfordert genau ein Argument (z.B. llama3)
	RunE: func(cmd *cobra.Command, args []string) error {
		modelName := args[0]

		// Überprüfen, ob eine Persona angefordert wurde
		if personaName != "" {
			personas, err := loadPersonas()
			if err != nil {
				return fmt.Errorf("Could not load personas: %w", err)
			}

			// Prüfen, ob die Map existiert und die Persona enthält
			if prompt, exists := personas[personaName]; exists {
				sysPrompt = prompt // Überschreibe den -s / --sys Prompt
			} else {
				return fmt.Errorf("Persona '%s' was not found", personaName)
			}
		}

		return runModel(modelName, ctxSize, sysPrompt)
	},
}

var serverPort int

var serveCmd = &cobra.Command{
	Use:          "serve [modellname]",
	Short:        "Starts a local API server for the model",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		modelName := args[0]
		return runServer(modelName, ctxSize, serverPort)
	},
}

var loadCmd = &cobra.Command{
	Use:          "load [url] (as [name])",
	Short:        "Downloads a GGUF model",
	SilenceUsage: true,
	Args:         cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		modelURL := args[0]
		customName := ""

		// Prüfe, ob "as name" genutzt wurde
		if len(args) == 3 && args[1] == "as" {
			customName = args[2]
		} else if len(args) == 2 {
			// Erlaubt die Kurzform "alpaka download <url> <name>"
			customName = args[1]
		} else if len(args) == 3 {
			// Wenn das mittlere Argument nicht "as" ist
			return fmt.Errorf("Invalid Syntax. Use: alpaka load <url> as <name>")
		}
		return loadModel(modelURL, customName)
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "Shows all downloaded models",
	RunE: func(cmd *cobra.Command, args []string) error {
		return listModels()
	},
}

var deleteCmd = &cobra.Command{
	Use:          "delete [modelname]",
	Aliases:      []string{"rm", "remove"},
	Short:        "Deletes a downloaded model",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		modelName := args[0]
		return deleteModel(modelName)
	},
}

var personaCmd = &cobra.Command{
	Use:   "persona",
	Short: "Manages your AI profiles (personas)",
}

var personaAddCmd = &cobra.Command{
	Use:   "add [name] [prompt]",
	Short: "Creates a new persona or updates an existing one",
	Args:  cobra.ExactArgs(2), // Erfordert Name und Prompt
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		prompt := args[1]

		// 1. Bestehende Personas laden
		personas, err := loadPersonas()
		if err != nil {
			return fmt.Errorf("Could not load personas: %w", err)
		}

		// 2. Neue Persona einfügen
		personas[name] = prompt

		// 3. Speichern
		if err := savePersonas(personas); err != nil {
			return fmt.Errorf("Error saving persona: %w", err)
		}

		fmt.Printf("Persona '%s' was successfully saved!\n", name)
		return nil
	},
}

var personaListCmd = &cobra.Command{
	Use:   "list",
	Short: "Shows all saved personas",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Personas laden
		personas, err := loadPersonas()
		if err != nil {
			return fmt.Errorf("Could not load personas: %w", err)
		}

		// 2. Prüfen, ob Personas existieren
		if len(personas) == 0 {
			fmt.Println("No personas found. Create one with 'alpaka persona add <name> <prompt>'.")
			return nil
		}

		// 3. Personas in der Konsole ausgeben
		fmt.Println("Saved personas:")
		for name, prompt := range personas {
			// Kürze den Prompt für die Anzeige, falls er zu lang ist
			displayPrompt := prompt
			if len(displayPrompt) > 60 {
				displayPrompt = displayPrompt[:57] + "..."
			}
			fmt.Printf("  - %-10s %s\n", name+":", displayPrompt)
		}

		return nil
	},
}

var personaDeleteCmd = &cobra.Command{
	Use:     "delete [name]",
	Aliases: []string{"rm", "remove"},
	Short:   "Deletes a saved persona",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		// 1. Personas laden
		personas, err := loadPersonas()
		if err != nil {
			return fmt.Errorf("Could not load personas: %w", err)
		}

		// 2. Prüfen, ob die Persona existiert
		if _, exists := personas[name]; !exists {
			return fmt.Errorf("Persona '%s' was not found", name)
		}

		// 3. Aus der Map entfernen
		delete(personas, name)

		// 4. Aktualisierte Map speichern
		if err := savePersonas(personas); err != nil {
			return fmt.Errorf("Error saving personas after deletion: %w", err)
		}

		fmt.Printf("Persona '%s' was successfully deleted!\n", name)
		return nil
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manages Alpaka configuration",
	Long:  `Shows and configures settings`,
}

var configSetCliCmd = &cobra.Command{
	Use:   "set-cli [path]",
	Short: "Saves a fixed path to llama-cli",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cliPath := args[0]
		if _, err := os.Stat(cliPath); err != nil {
			fmt.Printf("❌ Error: File not found or not readable: %s\n", cliPath)
			os.Exit(1)
		}

		cfg, err := loadConfig()
		if err != nil {

			fmt.Printf("❌ Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		cfg.LlamaCliPath = cliPath
		if err := saveConfig(cfg); err != nil {
			fmt.Printf("❌ Error saving configuration: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ llama-cli-Path saved: %s\n", cliPath)
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Shows the currently saved configuration",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := loadConfig()
		if err != nil {
			fmt.Printf("❌ Error loading configuration: %v\n", err)
			os.Exit(1)
		}

		if cfg.LlamaCliPath == "" {
			fmt.Println("No llama-cli configuration saved.")
			return
		}

		fmt.Printf("llama-cli:\n  %s\n", cfg.LlamaCliPath)
	},
}

var selfupdateCmd = &cobra.Command{
	Use:   "selfupdate",
	Short: "Updates Alpaka to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		return performUpdate()
	},
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	runCmd.Flags().IntVarP(&ctxSize, "ctx", "c", 2048, "Sets the maximum context size")
	runCmd.Flags().StringVarP(&sysPrompt, "sys", "s", "", "Defines a custom system prompt for the AI")
	runCmd.Flags().StringVarP(&personaName, "persona", "p", "", "Use a pre-defined persona")

	serveCmd.Flags().IntVarP(&ctxSize, "ctx", "c", 2048, "Sets the maximum context size")
	serveCmd.Flags().IntVarP(&serverPort, "port", "p", 8080, "Port for the local server")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(loadCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(deleteCmd)

	//persona commands
	rootCmd.AddCommand(personaCmd)
	personaCmd.AddCommand(personaAddCmd)
	personaCmd.AddCommand(personaListCmd)
	personaCmd.AddCommand(personaDeleteCmd)

	//config commands
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetCliCmd)
	configCmd.AddCommand(configShowCmd)

	rootCmd.AddCommand(selfupdateCmd)

	// Internal MCP command
	rootCmd.AddCommand(internalMcpCmd)
}

// applyTranslations überschreibt die Cobra-Texte
func applyTranslations(lang string) {
	if lang == "en" {
		return
	}

	t, ok := translations[lang]
	if !ok {
		return
	}

	rootCmd.Use = t["root_use"]
	rootCmd.Short = t["root_short"]
	rootCmd.Long = t["root_long"]

	runCmd.Use = t["run_use"]
	runCmd.Short = t["run_short"]
	if flag := runCmd.Flags().Lookup("ctx"); flag != nil {
		flag.Usage = t["run_ctx"]
	}
	if flag := runCmd.Flags().Lookup("sys"); flag != nil {
		flag.Usage = t["run_sys"]
	}

	serveCmd.Short = t["serve_short"]

	loadCmd.Use = t["load_use"]
	loadCmd.Short = t["load_short"]

	listCmd.Use = t["list_use"]
	listCmd.Short = t["list_short"]

	deleteCmd.Use = t["delete_use"]
	deleteCmd.Short = t["delete_short"]

	//Config commands
	configCmd.Use = t["config_use"]
	configCmd.Short = t["config_short"]
	configCmd.Long = t["config_long"]

	configSetCliCmd.Use = t["config_set_cli_use"]
	configSetCliCmd.Short = t["config_set_cli_short"]

	configShowCmd.Use = t["config_show_use"]
	configShowCmd.Short = t["config_show_short"]

	selfupdateCmd.Use = t["selfupdate_use"]
	selfupdateCmd.Short = t["selfupdate_short"]
}

func main() {
	lang := getSystemLanguage()
	applyTranslations(lang)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

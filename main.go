package main

import (
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

// Hauptfunktion zum Starten des Modells
func runModel(modelName string, ctxSize int, systemPrompt string) error {
	llamaPath, err := ensureLlamaInstalled()
	if err != nil {
		return err
	}

	alpakaDir, err := getAlpakaDir()
	if err != nil {
		return fmt.Errorf("Could not load Alpaka directory: %w", err)
	}

	// Vorübergehendes Verzeichnis für Modelle
	modelPath := filepath.Join(alpakaDir, "models", modelName+".gguf")

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return fmt.Errorf("Model file '%s' not found at: %s", modelName, modelPath)
	}

	fmt.Printf("🚀 Starting chat with model: %s\n\n", modelName)

	ctxString := strconv.Itoa(ctxSize)

	// llama-cli Aufruf zusammenstellen
	args := []string{
		"-m", modelPath,
		"-c", ctxString, // Kontextgröße
		"-cnv",            // Conversation Mode
		"--color", "auto", // Farbige Terminal-Ausgabe
	}

	if systemPrompt != "" {
		args = append(args, "-sys", systemPrompt)
	}

	cmd := exec.Command(llamaPath, args...)

	// Verbindet Stdin, Stdout und Stderr mit dem Terminal
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
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
	modelPath := filepath.Join(alpakaDir, "models", modelName+".gguf")

	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		return fmt.Errorf("Model file '%s' not found at: %s", modelName, modelPath)
	}

	fmt.Printf("🌐 Starting server on port %d with model: %s\n", port, modelName)

	ctxString := strconv.Itoa(ctxSize)

	// 4. Server-Befehl zusammenbauen
	cmd := exec.Command(serverPath,
		"-m", modelPath,
		"-c", ctxString, // Kontextgröße
		"--port", fmt.Sprintf("%d", port),
		"--host", "127.0.0.1", // Standardmäßig nur lokal erreichbar
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
	Long:    `Alpaka streamlines the usage of local AI models with llama.cpp`,
	Version: version,
}

var ctxSize int
var sysPrompt string

var runCmd = &cobra.Command{
	Use:          "run [modelname]",
	Short:        "Starts an AI model in chat mode",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1), // Erfordert genau ein Argument (z.B. llama3)
	RunE: func(cmd *cobra.Command, args []string) error {
		modelName := args[0]
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
			// Wenn jemand 3 Argumente tippt, aber das mittlere nicht "as" ist
			return fmt.Errorf("ungültige Syntax. Nutze: alpaka download <url> as <name>")
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
	Aliases:      []string{"rm", "remove"}, // Erlaubt auch: alpaka rm <modell>
	Short:        "Deletes a downloaded model",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		modelName := args[0]
		return deleteModel(modelName)
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

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	runCmd.Flags().IntVarP(&ctxSize, "ctx", "c", 2048, "Sets the maximum context size")
	runCmd.Flags().StringVarP(&sysPrompt, "sys", "s", "", "Defines a custom system prompt for the AI")

	serveCmd.Flags().IntVarP(&ctxSize, "ctx", "c", 2048, "Sets the maximum context size")
	serveCmd.Flags().IntVarP(&serverPort, "port", "p", 8080, "Port for the local server")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(loadCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configSetCliCmd)
	configCmd.AddCommand(configShowCmd)
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

	configCmd.Use = t["config_use"]
	configCmd.Short = t["config_short"]
	configCmd.Long = t["config_long"]

	configSetCliCmd.Use = t["config_set_cli_use"]
	configSetCliCmd.Short = t["config_set_cli_short"]

	configShowCmd.Use = t["config_show_use"]
	configShowCmd.Short = t["config_show_short"]
}

func main() {
	lang := getSystemLanguage()
	applyTranslations(lang)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

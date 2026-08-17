# 🦙 Alpaka

> A lightweight, zero-configuration CLI manager for `llama.cpp`.

Alpaka simplifies the process of downloading, managing, and running local AI models (GGUF format) using `llama.cpp`.
## ✨ Features

- **Zero-Setup `llama.cpp`**: Automatically detects your operating system and architecture (Windows, macOS, Linux) and downloads the highly optimized, correct `llama.cpp` release from GitHub.
- **Model Management**: Easily download, list, and delete `.gguf` models directly from the CLI with built-in progress bars.
- **Interactive Chat**: Start a colored, interactive chat session right in your terminal.
- **Local API Server**: Spin up an OpenAI-compatible local API server with a single command.

## 🚀 Installation

Ensure you have [Go installed](https://go.dev/doc/install) (version 1.21+ recommended).

Clone the repository and install it globally using `go install`:

```bash
git clone https://github.com/Wheely20/alpaka.git
cd alpaka
go install
```

*Note: Make sure your `$(go env GOPATH)/bin` directory is added to your system's `PATH` so you can run `alpaka` from anywhere.*

## 📖 Usage

### Quick Start

1. **Download a Model**: Find a `.gguf` model on Hugging Face (e.g., TinyLlama or Llama 3) and copy the direct download link.
   ```bash
   alpaka load https://huggingface.co/.../model.gguf my-model
   ```

2. **Start Chatting**:
   ```bash
   alpaka run my-model
   ```
   *(If this is your first time running Alpaka, it will automatically download and set up `llama.cpp` in the background)*

---

### Command Overview

#### `load` - Download Models
Downloads a GGUF model and saves it to your local model directory. You can optionally provide a custom name.
```bash
alpaka load https://example.com/model.gguf
alpaka load https://example.com/model.gguf custom-name
```

#### `list` - View Models
Lists all locally downloaded models along with their file sizes.
```bash
alpaka list
```

#### `run` - Interactive Chat
Starts a model in interactive chat mode.
```bash
alpaka run [modelname]
```
**Flags:**
- `--ctx`, `-c`: Set the context size (default: 2048)
- `--sys`, `-s`: Define a custom system prompt

*Example:* `alpaka run my-model -c 4096 -s "You are a helpful coding assistant."`

#### `serve` - Local API Server
Starts an OpenAI-compatible local server. Ideal for connecting third-party UIs (like Chatbox or typingmind) or local scripts.
```bash
alpaka serve [modelname]
```
**Flags:**
- `--port`, `-p`: Port for the local server (default: 8080)

#### `delete` - Remove Models
Deletes a model from your hard drive to free up space.
```bash
alpaka delete [modelname]
```

#### `config` - Configuration
Manage Alpaka settings, like setting a custom path to a pre-existing `llama.cpp` installation.
```bash
alpaka config show
alpaka config set-cli /path/to/custom/llama-cli
```

## 📁 Directory Structure

Alpaka stores all its files safely in your user home directory under `~/.alpaka/`:

- `~/.alpaka/bin/` - Contains the downloaded `llama.cpp` executables (`llama-cli` and `llama-server`).
- `~/.alpaka/models/` - Your downloaded `.gguf` model files.
- `~/.alpaka/config.json` - Persistent configuration settings.

## 🤝 Contributing

Contributions, issues, and feature requests are welcome! Feel free to check the issues page.

## 📝 License

This project is open-source and available under the MIT License.

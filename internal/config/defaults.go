package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// resolveConfigPath implements smart config path resolution with fallback
func resolveConfigPath(configPath string) (string, error) {
	// If explicit path provided, try to use it - create if it doesn't exist
	if configPath != "" {
		expanded := ExpandPath(configPath)
		if _, err := os.Stat(expanded); err != nil {
			// If explicit path doesn't exist, try to create it
			if err := createDefaultConfig(expanded); err != nil {
				return "", fmt.Errorf("specified config file not found and could not create: %s", expanded)
			}
			fmt.Printf("Created config file at: %s\n", expanded)
		}
		return expanded, nil
	}

	// Priority order for auto-resolution:
	// 1. ~/.config/mcp-server-manager/config.yaml (user config)
	// 2. ./config.yaml (current directory)
	// 3. configs/config.yaml (relative to binary)
	// 4. Auto-create user config if none found

	candidates := []string{
		ExpandPath("~/.config/mcp-server-manager/config.yaml"),
		"./config.yaml",
		DefaultConfigPath,
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// No config found, auto-create user config
	userConfigPath := ExpandPath("~/.config/mcp-server-manager/config.yaml")
	if err := createDefaultConfig(userConfigPath); err != nil {
		return "", fmt.Errorf("failed to create default config: %w", err)
	}

	fmt.Printf("Created default config file at: %s\n", userConfigPath)
	fmt.Println("Please edit this file to configure your MCP servers and clients.")

	return userConfigPath, nil
}

// createDefaultConfig creates a default config file with example configuration
func createDefaultConfig(configPath string) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	defaultConfig := `# MCP Server Manager Configuration v2.0
# This matches standard MCP client config format for maximum compatibility
# Edit this file to configure your MCP servers and clients

server_port: 6543

# MCP Servers - Standard format matching MCP clients
# Server names are keys; configurations are values (pass through to clients)
mcpServers:
  # STDIO Transport Example (command-based)
  filesystem:
    command: "npx"
    args: ["@modelcontextprotocol/server-filesystem", "/path/to/your/directory"]
    env:
      NODE_ENV: "production"
    timeout: 30000  # Optional: request timeout in ms
    trust: false    # Optional: bypass tool confirmations

  # HTTP Transport Example (with type field for VS Code compatibility)
  context7-vscode:
    type: "http"
    url: "https://mcp.context7.com/mcp"
    headers:
      CONTEXT7_API_KEY: "ADD_YOUR_API_KEY"
      Accept: "application/json, text/event-stream"
    timeout: 10000

  # HTTP Transport Example (httpUrl variant for Gemini CLI)
  context7-gemini:
    httpUrl: "https://mcp.context7.com/mcp"
    headers:
      CONTEXT7_API_KEY: "ADD_YOUR_API_KEY"
      Accept: "application/json, text/event-stream"

# MCP Clients - Configure which targets each config file uses
clients:
  claude_code:
    format: json
    config_path: "~/.claude.json"
    enabled:
      - filesystem

  gemini_cli:
    format: json
    config_path: "~/.gemini/settings.json"
    enabled:
      # - context7-gemini
      # - filesystem

  codex:
    format: toml
    config_path: "~/.codex/config.toml"
    enabled:
      # - filesystem

# Notes:
# - The clients section really defines output targets: format + config_path + enabled servers
# - format defaults to json for backward compatibility
# - Supports JSON targets (Gemini, Claude, Antigravity-style files) and TOML targets (Codex)
# - Use 'enabled' array per target to control which servers each target uses
`

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// ExpandPath expands ~ to the user's home directory
func ExpandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

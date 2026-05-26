package setup

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed plugin.ts
var pluginContent string

// OpenCodeConfig represents the structure of opencode.json.
type OpenCodeConfig struct {
	Plugin []string       `json:"plugin,omitempty"`
	MCP    map[string]any `json:"mcp,omitempty"`
}

// SetupOpenCode configures OpenCode to use Echo.
func SetupOpenCode() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", "opencode")
	configPath := filepath.Join(configDir, "opencode.json")
	pluginPath := filepath.Join(configDir, "plugins", "echo.ts")

	// 1. Ensure config directory exists.
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	// 2. Ensure plugins directory exists.
	if err := os.MkdirAll(filepath.Dir(pluginPath), 0o755); err != nil {
		return fmt.Errorf("create plugins directory: %w", err)
	}

	// 3. Create plugin file if it doesn't exist.
	if _, err := os.Stat(pluginPath); os.IsNotExist(err) {
		if err := os.WriteFile(pluginPath, []byte(pluginContent), 0o644); err != nil {
			return fmt.Errorf("write plugin file: %w", err)
		}
		fmt.Println("✅ Created plugin: ~/.config/opencode/plugins/echo.ts")
	} else {
		fmt.Println("ℹ️  Plugin already exists: ~/.config/opencode/plugins/echo.ts")
	}

	// 4. Read existing config.
	var cfg OpenCodeConfig
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read config: %w", err)
	}

	// Initialize maps/slices if nil.
	if cfg.Plugin == nil {
		cfg.Plugin = []string{}
	}
	if cfg.MCP == nil {
		cfg.MCP = map[string]any{}
	}

	// 5. Add plugin if not present.
	pluginEntry := "./plugins/echo.ts"
	found := false
	for _, p := range cfg.Plugin {
		if strings.HasSuffix(p, "echo.ts") {
			found = true
			break
		}
	}
	if !found {
		cfg.Plugin = append(cfg.Plugin, pluginEntry)
		fmt.Println("✅ Added plugin to opencode.json")
	} else {
		fmt.Println("ℹ️  Plugin already registered in opencode.json")
	}

	// 6. Add MCP entry if not present.
	if _, exists := cfg.MCP["echo"]; !exists {
		cfg.MCP["echo"] = map[string]any{
			"command": []string{"echo-mcp", "serve"},
			"enabled": true,
			"type":    "local",
		}
		fmt.Println("✅ Added MCP server to opencode.json")
	} else {
		fmt.Println("ℹ️  MCP server already configured in opencode.json")
	}

	// 7. Write updated config.
	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Println("\n🚀 Echo is now configured for OpenCode!")
	fmt.Println("Restart OpenCode to apply changes.")

	return nil
}

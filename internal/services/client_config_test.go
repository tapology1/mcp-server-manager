package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vlazic/mcp-server-manager/internal/models"
	"github.com/vlazic/mcp-server-manager/internal/services/testutil"
)

// ClientConfigService Tests
//
// These tests verify the critical functionality of syncing MCP server configurations
// to individual AI client config files (e.g., ~/.claude.json, ~/.gemini/settings.json).
//
// Key test coverage:
// - Field preservation: ALL fields from server configs must be preserved (v2.0 fix)
// - Backup creation: Automatic timestamped backups before each write
// - Non-MCP settings preservation: Client theme, auth, etc. must not be affected
// - Error handling: Malformed JSON, permission errors, non-existent servers
// - Empty/missing config handling: Graceful creation of default structures
//
// IMPORTANT: The field preservation test (TestFieldPreservation) validates the fix
// for a critical bug where only command/args were copied, losing fields like url,
// headers, type, etc. This is essential for HTTP transport support.

// setupClientConfigTest creates a test environment with ClientConfigService
func setupClientConfigTest(t *testing.T, servers []models.MCPServer, enabled []string) (*ClientConfigService, string) {
	t.Helper()
	tempDir := t.TempDir()
	clientConfigPath := filepath.Join(tempDir, testutil.TestClientJSON)

	cfg := &models.Config{
		MCPServers: servers,
		Clients: map[string]*models.Client{
			"test_client": {
				ConfigPath: clientConfigPath,
				Enabled:    enabled,
			},
		},
	}

	service := NewClientConfigService(cfg)
	return service, clientConfigPath
}

func TestReadClientConfig(t *testing.T) {
	service, clientConfigPath := setupClientConfigTest(t, []models.MCPServer{}, []string{})

	t.Run("Read non-existent config", func(t *testing.T) {
		rawConfig, err := service.ReadClientConfig("test_client")
		if err != nil {
			t.Fatalf(testutil.ErrReadClientConfigFailedFmt, err)
		}

		// Should return empty config with mcpServers section
		mcpServers, ok := rawConfig["mcpServers"].(map[string]interface{})
		if !ok {
			t.Fatal("mcpServers section not found")
		}

		if len(mcpServers) != 0 {
			t.Errorf("Expected empty mcpServers, got %d", len(mcpServers))
		}
	})

	t.Run("Read existing config", func(t *testing.T) {
		// Create a client config file
		clientData := map[string]interface{}{
			"mcpServers": map[string]interface{}{
				testutil.TestServerName: map[string]interface{}{
					"command": "echo",
					"args":    []interface{}{"test"},
				},
			},
			"theme": "dark",
		}

		data, _ := json.MarshalIndent(clientData, "", "  ")
		if err := os.WriteFile(clientConfigPath, data, 0644); err != nil {
			t.Fatalf("Failed to write client config: %v", err)
		}

		rawConfig, err := service.ReadClientConfig("test_client")
		if err != nil {
			t.Fatalf(testutil.ErrReadClientConfigFailedFmt, err)
		}

		// Verify mcpServers section
		mcpServers, ok := rawConfig["mcpServers"].(map[string]interface{})
		if !ok {
			t.Fatal("mcpServers section not found")
		}

		if len(mcpServers) != 1 {
			t.Errorf("Expected 1 server, got %d", len(mcpServers))
		}

		// Verify theme is preserved
		if rawConfig["theme"] != "dark" {
			t.Error("Non-MCP fields not preserved")
		}
	})

	t.Run("Client not found", func(t *testing.T) {
		_, err := service.ReadClientConfig("nonexistent")
		if err == nil {
			t.Error("Expected error for non-existent client")
		}
	})
}

func TestWriteClientConfig(t *testing.T) {
	service, clientConfigPath := setupClientConfigTest(t, []models.MCPServer{}, []string{})
	// Override path with subdirectory to test directory creation
	tempDir := filepath.Dir(clientConfigPath)
	clientConfigPath = filepath.Join(tempDir, "subdir", testutil.TestClientJSON)
	service.config.Clients["test_client"].ConfigPath = clientConfigPath

	rawConfig := map[string]interface{}{
		"mcpServers": map[string]interface{}{
			testutil.TestServerName: map[string]interface{}{
				"command": "npx",
				"args":    []interface{}{"test"},
			},
		},
		"theme": "light",
	}

	// Write config
	if err := service.WriteClientConfig("test_client", rawConfig); err != nil {
		t.Fatalf(testutil.ErrWriteClientConfigFailedFmt, err)
	}

	// Verify file was created
	if _, err := os.Stat(clientConfigPath); os.IsNotExist(err) {
		t.Fatal("Client config file was not created")
	}

	// Read back and verify
	data, err := os.ReadFile(clientConfigPath)
	if err != nil {
		t.Fatalf("Failed to read client config: %v", err)
	}

	var readBack map[string]interface{}
	if err := json.Unmarshal(data, &readBack); err != nil {
		t.Fatalf("Failed to parse client config: %v", err)
	}

	if readBack["theme"] != "light" {
		t.Error("Theme field not preserved")
	}

	mcpServers := readBack["mcpServers"].(map[string]interface{})
	if len(mcpServers) != 1 {
		t.Errorf("Expected 1 server, got %d", len(mcpServers))
	}
}

func TestBackupConfig(t *testing.T) {
	service, clientConfigPath := setupClientConfigTest(t, []models.MCPServer{}, []string{})
	tempDir := filepath.Dir(clientConfigPath)

	// Create initial config
	initialData := map[string]interface{}{
		"mcpServers": map[string]interface{}{},
		"version":    "1.0",
	}
	data, _ := json.Marshal(initialData)
	if err := os.WriteFile(clientConfigPath, data, 0644); err != nil {
		t.Fatalf(testutil.ErrWriteInitialConfigFailedFmt, err)
	}

	// Write new config (should create backup)
	newData := map[string]interface{}{
		"mcpServers": map[string]interface{}{},
		"version":    "2.0",
	}

	if err := service.WriteClientConfig("test_client", newData); err != nil {
		t.Fatalf(testutil.ErrWriteClientConfigFailedFmt, err)
	}

	// Check for backup file
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to read temp dir: %v", err)
	}

	backupFound := false
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "client.json.backup.") {
			backupFound = true

			// Verify backup contains old data
			backupPath := filepath.Join(tempDir, file.Name())
			backupData, err := os.ReadFile(backupPath)
			if err != nil {
				t.Fatalf("Failed to read backup: %v", err)
			}

			var backupConfig map[string]interface{}
			if err := json.Unmarshal(backupData, &backupConfig); err != nil {
				t.Fatalf("Failed to parse backup: %v", err)
			}

			if backupConfig["version"] != "1.0" {
				t.Error("Backup does not contain original data")
			}
			break
		}
	}

	if !backupFound {
		t.Error("Backup file was not created")
	}
}

func TestUpdateMCPServerStatus(t *testing.T) {
	servers := []models.MCPServer{
		{
			Name: testutil.TestServerName,
			Config: map[string]interface{}{
				"command": "npx",
				"args":    []interface{}{"test"},
				"env": map[string]interface{}{
					"NODE_ENV": "production",
				},
			},
		},
	}
	service, _ := setupClientConfigTest(t, servers, []string{})

	t.Run("Enable server", func(t *testing.T) {
		if err := service.UpdateMCPServerStatus("test_client", testutil.TestServerName, true); err != nil {
			t.Fatalf(testutil.ErrUpdateMCPStatusFailedFmt, err)
		}

		// Verify server was added to client config
		rawConfig, err := service.ReadClientConfig("test_client")
		if err != nil {
			t.Fatalf(testutil.ErrReadClientConfigFailedFmt, err)
		}

		mcpServers := rawConfig["mcpServers"].(map[string]interface{})
		serverConfig, exists := mcpServers[testutil.TestServerName]
		if !exists {
			t.Fatal("Server was not added to client config")
		}

		// Verify all fields were copied
		sc := serverConfig.(map[string]interface{})
		if sc["command"] != "npx" {
			t.Error("Command field not copied")
		}
		if sc["env"] == nil {
			t.Error("Env field not copied")
		}
	})

	t.Run("Disable server", func(t *testing.T) {
		if err := service.UpdateMCPServerStatus("test_client", testutil.TestServerName, false); err != nil {
			t.Fatalf(testutil.ErrUpdateMCPStatusFailedFmt, err)
		}

		// Verify server was removed from client config
		rawConfig, err := service.ReadClientConfig("test_client")
		if err != nil {
			t.Fatalf(testutil.ErrReadClientConfigFailedFmt, err)
		}

		mcpServers := rawConfig["mcpServers"].(map[string]interface{})
		if _, exists := mcpServers[testutil.TestServerName]; exists {
			t.Error("Server was not removed from client config")
		}
	})
}

func TestUpdateMCPServerStatus_GeminiJSONOmitsStartupTimeout(t *testing.T) {
	tempDir := t.TempDir()
	clientConfigPath := filepath.Join(tempDir, "settings.json")

	cfg := &models.Config{
		MCPServers: []models.MCPServer{
			{
				Name: testutil.TestServerName,
				Config: map[string]interface{}{
					"command":             "npx",
					"args":                []interface{}{"test"},
					"startup_timeout_sec": 180,
				},
			},
		},
		Clients: map[string]*models.Client{
			"gemini_cli": {
				Format:     "json",
				ConfigPath: clientConfigPath,
			},
		},
	}

	service := NewClientConfigService(cfg)

	if err := service.UpdateMCPServerStatus("gemini_cli", testutil.TestServerName, true); err != nil {
		t.Fatalf("UpdateMCPServerStatus failed: %v", err)
	}

	rawConfig, err := service.ReadClientConfig("gemini_cli")
	if err != nil {
		t.Fatalf("ReadClientConfig failed: %v", err)
	}

	mcpServers := rawConfig["mcpServers"].(map[string]interface{})
	serverConfig := mcpServers[testutil.TestServerName].(map[string]interface{})

	if _, exists := serverConfig["startup_timeout_sec"]; exists {
		t.Fatal("gemini_cli config should omit startup_timeout_sec")
	}
	if serverConfig["command"] != "npx" {
		t.Fatalf("expected command to be preserved, got %#v", serverConfig["command"])
	}
}

func TestGetMCPServerStatus(t *testing.T) {
	servers := []models.MCPServer{
		{
			Name: testutil.TestServerName,
			Config: map[string]interface{}{
				"command": "echo",
			},
		},
	}
	service, _ := setupClientConfigTest(t, servers, []string{testutil.TestServerName})

	// Enable the server
	service.UpdateMCPServerStatus("test_client", testutil.TestServerName, true)

	// Check status
	enabled, err := service.GetMCPServerStatus("test_client", testutil.TestServerName)
	if err != nil {
		t.Fatalf(testutil.ErrGetMCPStatusFailedFmt, err)
	}

	if !enabled {
		t.Error("Expected server to be enabled")
	}

	// Disable and check again
	service.UpdateMCPServerStatus("test_client", testutil.TestServerName, false)

	enabled, err = service.GetMCPServerStatus("test_client", testutil.TestServerName)
	if err != nil {
		t.Fatalf(testutil.ErrGetMCPStatusFailedFmt, err)
	}

	if enabled {
		t.Error("Expected server to be disabled")
	}
}

func TestFieldPreservation(t *testing.T) {
	// This test verifies the critical fix: ALL fields are preserved when syncing
	tempDir := t.TempDir()
	clientConfigPath := filepath.Join(tempDir, testutil.TestClientJSON)

	cfg := &models.Config{
		MCPServers: []models.MCPServer{
			{
				Name: testutil.HTTPServerName,
				Config: map[string]interface{}{
					"type":        "http",
					"url":         "https://example.com",
					"customField": "customValue",
					"headers": map[string]interface{}{
						"Authorization": "Bearer token",
						"Accept":        "application/json",
					},
					"timeout": 10000,
					"nested": map[string]interface{}{
						"foo": "bar",
						"baz": 123,
					},
				},
			},
		},
		Clients: map[string]*models.Client{
			"test_client": {
				ConfigPath: clientConfigPath,
				Enabled:    []string{},
			},
		},
	}

	service := NewClientConfigService(cfg)

	// Enable server
	if err := service.UpdateMCPServerStatus("test_client", testutil.HTTPServerName, true); err != nil {
		t.Fatalf(testutil.ErrUpdateMCPStatusFailedFmt, err)
	}

	// Read client config
	rawConfig, err := service.ReadClientConfig("test_client")
	if err != nil {
		t.Fatalf(testutil.ErrReadClientConfigFailedFmt, err)
	}

	mcpServers := rawConfig["mcpServers"].(map[string]interface{})
	serverConfig := mcpServers[testutil.HTTPServerName].(map[string]interface{})

	// Verify ALL fields are preserved
	if serverConfig["type"] != "http" {
		t.Error("type field not preserved")
	}
	if serverConfig["url"] != "https://example.com" {
		t.Error("url field not preserved")
	}
	if serverConfig["customField"] != "customValue" {
		t.Error("customField not preserved")
	}
	if serverConfig["headers"] == nil {
		t.Error("headers not preserved")
	}
	if serverConfig["timeout"] == nil {
		t.Error("timeout not preserved")
	}
	if serverConfig["nested"] == nil {
		t.Error("nested custom field not preserved")
	}

	// Verify nested fields
	headers := serverConfig["headers"].(map[string]interface{})
	if headers["Authorization"] != "Bearer token" {
		t.Error("Authorization header not preserved")
	}

	nested := serverConfig["nested"].(map[string]interface{})
	if nested["foo"] != "bar" {
		t.Error("Nested custom field not preserved")
	}
}

func TestReadClientConfig_MalformedJSON(t *testing.T) {
	tempDir := t.TempDir()
	clientConfigPath := filepath.Join(tempDir, testutil.TestClientJSON)

	cfg := &models.Config{
		MCPServers: []models.MCPServer{},
		Clients: map[string]*models.Client{
			"test_client": {
				ConfigPath: clientConfigPath,
				Enabled:    []string{},
			},
		},
	}

	service := NewClientConfigService(cfg)

	// Write malformed JSON
	malformedJSON := `{"mcpServers": {"test": "value"` // Missing closing braces
	if err := os.WriteFile(clientConfigPath, []byte(malformedJSON), 0644); err != nil {
		t.Fatalf("Failed to write malformed JSON: %v", err)
	}

	_, err := service.ReadClientConfig("test_client")
	if err == nil {
		t.Error("Expected error for malformed JSON")
	}
}

func TestWriteClientConfig_ReadOnlyDirectory(t *testing.T) {
	// Skip on systems where we can't reliably test read-only directories
	if os.Getuid() == 0 {
		t.Skip("Skipping read-only test when running as root")
	}

	tempDir := t.TempDir()
	readOnlyDir := filepath.Join(tempDir, "readonly")
	if err := os.Mkdir(readOnlyDir, 0500); err != nil {
		t.Fatalf("Failed to create read-only dir: %v", err)
	}
	defer os.Chmod(readOnlyDir, 0755) // Restore permissions for cleanup

	clientConfigPath := filepath.Join(readOnlyDir, testutil.TestClientJSON)

	cfg := &models.Config{
		MCPServers: []models.MCPServer{},
		Clients: map[string]*models.Client{
			"test_client": {
				ConfigPath: clientConfigPath,
				Enabled:    []string{},
			},
		},
	}

	service := NewClientConfigService(cfg)

	rawConfig := map[string]interface{}{
		"mcpServers": map[string]interface{}{},
	}

	err := service.WriteClientConfig("test_client", rawConfig)
	if err == nil {
		t.Error("Expected error when writing to read-only directory")
	}
}

func TestUpdateMCPServerStatus_NonExistentServer(t *testing.T) {
	tempDir := t.TempDir()
	clientConfigPath := filepath.Join(tempDir, testutil.TestClientJSON)

	cfg := &models.Config{
		MCPServers: []models.MCPServer{
			{Name: "existing-server", Config: map[string]interface{}{"command": "echo"}},
		},
		Clients: map[string]*models.Client{
			"test_client": {
				ConfigPath: clientConfigPath,
				Enabled:    []string{},
			},
		},
	}

	service := NewClientConfigService(cfg)

	// Try to enable a server that doesn't exist in config
	err := service.UpdateMCPServerStatus("test_client", "nonexistent-server", true)
	if err == nil {
		t.Error("Expected error when enabling non-existent server")
	}
}

func TestBackupConfig_CreatesBackup(t *testing.T) {
	// Test that a backup is created when overwriting existing config
	// Note: Multiple rapid writes may create backups with identical timestamps,
	// so we test backup functionality rather than timestamp uniqueness
	tempDir := t.TempDir()
	clientConfigPath := filepath.Join(tempDir, testutil.TestClientJSON)

	cfg := &models.Config{
		MCPServers: []models.MCPServer{},
		Clients: map[string]*models.Client{
			"test_client": {
				ConfigPath: clientConfigPath,
				Enabled:    []string{},
			},
		},
	}

	service := NewClientConfigService(cfg)

	// Create initial config
	initialData := map[string]interface{}{"mcpServers": map[string]interface{}{}, "version": "1.0"}
	data, _ := json.Marshal(initialData)
	if err := os.WriteFile(clientConfigPath, data, 0644); err != nil {
		t.Fatalf(testutil.ErrWriteInitialConfigFailedFmt, err)
	}

	// Overwrite config - should create backup
	newData := map[string]interface{}{"mcpServers": map[string]interface{}{}, "version": "2.0"}
	if err := service.WriteClientConfig("test_client", newData); err != nil {
		t.Fatalf(testutil.ErrWriteClientConfigFailedFmt, err)
	}

	// Count backup files
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to read temp dir: %v", err)
	}

	backupCount := 0
	var backupFile string
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "client.json.backup.") {
			backupCount++
			backupFile = file.Name()
		}
	}

	// We expect at least 1 backup
	if backupCount < 1 {
		t.Error("Expected at least 1 backup file, got 0")
	}

	// Verify backup contains original data
	if backupFile != "" {
		backupPath := filepath.Join(tempDir, backupFile)
		backupData, err := os.ReadFile(backupPath)
		if err != nil {
			t.Fatalf("Failed to read backup: %v", err)
		}

		var backupConfig map[string]interface{}
		if err := json.Unmarshal(backupData, &backupConfig); err != nil {
			t.Fatalf("Failed to parse backup: %v", err)
		}

		if backupConfig["version"] != "1.0" {
			t.Errorf("Backup should contain original version '1.0', got '%v'", backupConfig["version"])
		}
	}
}

func TestGetMCPServerStatus_EmptyConfig(t *testing.T) {
	tempDir := t.TempDir()
	clientConfigPath := filepath.Join(tempDir, testutil.TestClientJSON)

	cfg := &models.Config{
		MCPServers: []models.MCPServer{
			{Name: testutil.TestServerName, Config: map[string]interface{}{"command": "echo"}},
		},
		Clients: map[string]*models.Client{
			"test_client": {
				ConfigPath: clientConfigPath,
				Enabled:    []string{},
			},
		},
	}

	service := NewClientConfigService(cfg)

	// Check status on non-existent config file (should return false, not error)
	enabled, err := service.GetMCPServerStatus("test_client", testutil.TestServerName)
	if err != nil {
		t.Fatalf(testutil.ErrGetMCPStatusFailedFmt, err)
	}

	if enabled {
		t.Error("Expected server to be disabled on empty config")
	}
}

func TestUpdateMCPServerStatus_PreserveOtherSettings(t *testing.T) {
	// Verify that enabling/disabling servers doesn't affect other client settings
	tempDir := t.TempDir()
	clientConfigPath := filepath.Join(tempDir, testutil.TestClientJSON)

	cfg := &models.Config{
		MCPServers: []models.MCPServer{
			{Name: testutil.TestServerName, Config: map[string]interface{}{"command": "echo"}},
		},
		Clients: map[string]*models.Client{
			"test_client": {
				ConfigPath: clientConfigPath,
				Enabled:    []string{},
			},
		},
	}

	service := NewClientConfigService(cfg)

	// Create initial config with additional settings
	initialConfig := map[string]interface{}{
		"mcpServers": map[string]interface{}{},
		"theme":      "dark",
		"apiKey":     "secret123",
		"settings": map[string]interface{}{
			"autoSave": true,
			"timeout":  5000,
		},
	}
	data, _ := json.MarshalIndent(initialConfig, "", "  ")
	if err := os.WriteFile(clientConfigPath, data, 0644); err != nil {
		t.Fatalf(testutil.ErrWriteInitialConfigFailedFmt, err)
	}

	// Enable server
	if err := service.UpdateMCPServerStatus("test_client", testutil.TestServerName, true); err != nil {
		t.Fatalf(testutil.ErrUpdateMCPStatusFailedFmt, err)
	}

	// Read back and verify other settings are preserved
	rawConfig, err := service.ReadClientConfig("test_client")
	if err != nil {
		t.Fatalf(testutil.ErrReadClientConfigFailedFmt, err)
	}

	if rawConfig["theme"] != "dark" {
		t.Error("Theme setting not preserved")
	}
	if rawConfig["apiKey"] != "secret123" {
		t.Error("API key not preserved")
	}
	if rawConfig["settings"] == nil {
		t.Error("Settings section not preserved")
	}
}

func TestSyncClientServers_ReplacesStaleServerConfig(t *testing.T) {
	tempDir := t.TempDir()
	clientConfigPath := filepath.Join(tempDir, "config.toml")

	cfg := &models.Config{
		MCPServers: []models.MCPServer{
			{
				Name: "cloudflare",
				Config: map[string]interface{}{
					"type":                  "http",
					"url":                   "https://mcp.cloudflare.com/mcp",
					"bridge_http_via_stdio": true,
				},
			},
		},
		Clients: map[string]*models.Client{
			"codex": {
				Format:     "toml",
				ConfigPath: clientConfigPath,
				Enabled:    []string{"cloudflare"},
			},
		},
	}

	service := NewClientConfigService(cfg)

	initial := []byte("[mcp_servers.cloudflare]\nargs = ['-y', 'mcp-remote', 'https://mcp.cloudflare.com/mcp', '--header', 'Authorization: Bearer stale']\ncommand = 'npx'\nenabled = true\nstartup_timeout_sec = 180\n")
	if err := os.WriteFile(clientConfigPath, initial, 0644); err != nil {
		t.Fatalf("Failed to seed TOML config: %v", err)
	}

	if err := service.SyncClientServers("codex", []string{"cloudflare"}); err != nil {
		t.Fatalf("SyncClientServers failed: %v", err)
	}

	data, err := os.ReadFile(clientConfigPath)
	if err != nil {
		t.Fatalf("Failed to read synced TOML config: %v", err)
	}

	text := string(data)
	if strings.Contains(text, "Authorization: Bearer stale") {
		t.Fatal("stale bearer header should have been removed during sync")
	}
	if !strings.Contains(text, "mcp.cloudflare.com/mcp") {
		t.Fatal("cloudflare server should remain in synced config")
	}
}

func TestSyncClientServers_GeminiJSONOmitsStartupTimeout(t *testing.T) {
	tempDir := t.TempDir()
	clientConfigPath := filepath.Join(tempDir, "settings.json")

	cfg := &models.Config{
		MCPServers: []models.MCPServer{
			{
				Name: testutil.TestServerName,
				Config: map[string]interface{}{
					"command":             "npx",
					"args":                []interface{}{"test"},
					"startup_timeout_sec": 120,
				},
			},
		},
		Clients: map[string]*models.Client{
			"gemini_cli": {
				Format:     "json",
				ConfigPath: clientConfigPath,
			},
		},
	}

	service := NewClientConfigService(cfg)

	if err := service.SyncClientServers("gemini_cli", []string{testutil.TestServerName}); err != nil {
		t.Fatalf("SyncClientServers failed: %v", err)
	}

	data, err := os.ReadFile(clientConfigPath)
	if err != nil {
		t.Fatalf("failed to read synced config: %v", err)
	}

	var rawConfig map[string]interface{}
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		t.Fatalf("failed to parse synced config: %v", err)
	}

	mcpServers := rawConfig["mcpServers"].(map[string]interface{})
	serverConfig := mcpServers[testutil.TestServerName].(map[string]interface{})

	if _, exists := serverConfig["startup_timeout_sec"]; exists {
		t.Fatal("gemini_cli sync should omit startup_timeout_sec")
	}
	if serverConfig["command"] != "npx" {
		t.Fatalf("expected command to be preserved, got %#v", serverConfig["command"])
	}
}

func TestTranslateServerConfigToTOML_BridgeHTTPViaStdio(t *testing.T) {
	service, _ := setupClientConfigTest(t, []models.MCPServer{}, []string{})

	translated := service.translateServerConfigToTOML(map[string]interface{}{
		"type":                  "http",
		"url":                   "https://mcp.cloudflare.com/mcp",
		"bridge_http_via_stdio": true,
		"headers": map[string]interface{}{
			"Authorization": "Bearer token",
			"Accept":        "application/json",
		},
	})

	if translated["command"] != "npx" {
		t.Fatalf("expected npx command, got %#v", translated["command"])
	}

	expectedArgs := []string{
		"-y",
		"mcp-remote",
		"https://mcp.cloudflare.com/mcp",
		"--header",
		"Accept: application/json",
		"--header",
		"Authorization: Bearer token",
	}

	args, ok := translated["args"].([]string)
	if !ok {
		t.Fatalf("expected []string args, got %T", translated["args"])
	}

	if !reflect.DeepEqual(args, expectedArgs) {
		t.Fatalf("unexpected args: %#v", args)
	}

	if _, exists := translated["url"]; exists {
		t.Fatal("bridge config should not emit direct url")
	}

	if _, exists := translated["http_headers"]; exists {
		t.Fatal("bridge config should not emit direct http_headers")
	}

	if translated["startup_timeout_sec"] != 20 {
		t.Fatalf("expected default startup timeout 20, got %#v", translated["startup_timeout_sec"])
	}
}

func TestTranslateServerConfigToTOML_BridgePreservesConfiguredTimeout(t *testing.T) {
	service, _ := setupClientConfigTest(t, []models.MCPServer{}, []string{})

	translated := service.translateServerConfigToTOML(map[string]interface{}{
		"type":                  "http",
		"url":                   "https://mcp.cloudflare.com/mcp",
		"bridge_http_via_stdio": true,
		"startup_timeout_sec":   180,
	})

	if translated["startup_timeout_sec"] != 180 {
		t.Fatalf("expected startup timeout 180, got %#v", translated["startup_timeout_sec"])
	}
}

package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/vlazic/mcp-server-manager/internal/config"
	"github.com/vlazic/mcp-server-manager/internal/models"
)

type ClientConfigService struct {
	config    *models.Config
	validator *ValidatorService
}

func NewClientConfigService(cfg *models.Config) *ClientConfigService {
	return &ClientConfigService{
		config:    cfg,
		validator: NewValidatorService(),
	}
}

func (s *ClientConfigService) ReadClientConfig(clientName string) (map[string]interface{}, error) {
	client := s.findClient(clientName)
	if client == nil {
		return nil, fmt.Errorf("client '%s' not found", clientName)
	}

	configPath := config.ExpandPath(client.ConfigPath)
	format := s.clientFormat(client)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return s.emptyConfigForFormat(format), nil
		}
		return nil, fmt.Errorf("failed to read client config '%s': %w", configPath, err)
	}

	switch format {
	case "toml":
		return s.readTOMLClientConfig(configPath, data)
	default:
		return s.readJSONClientConfig(configPath, data)
	}
}

func (s *ClientConfigService) WriteClientConfig(clientName string, rawConfig map[string]interface{}) error {
	client := s.findClient(clientName)
	if client == nil {
		return fmt.Errorf("client '%s' not found", clientName)
	}

	configPath := config.ExpandPath(client.ConfigPath)
	format := s.clientFormat(client)

	if err := s.backupConfig(configPath); err != nil {
		return fmt.Errorf("failed to backup config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	var (
		data []byte
		err  error
	)

	switch format {
	case "toml":
		data, err = toml.Marshal(rawConfig)
		if err != nil {
			return fmt.Errorf("failed to marshal TOML client config: %w", err)
		}
	default:
		data, err = json.MarshalIndent(rawConfig, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON client config: %w", err)
		}
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write client config '%s': %w", configPath, err)
	}

	return nil
}

func (s *ClientConfigService) UpdateMCPServerStatus(clientName, serverName string, enabled bool) error {
	rawConfig, err := s.ReadClientConfig(clientName)
	if err != nil {
		return err
	}

	client := s.findClient(clientName)
	if client == nil {
		return fmt.Errorf("client '%s' not found", clientName)
	}

	format := s.clientFormat(client)
	serversKey := s.serversKeyForFormat(format)

	mcpServers, ok := rawConfig[serversKey].(map[string]interface{})
	if !ok {
		mcpServers = make(map[string]interface{})
		rawConfig[serversKey] = mcpServers
	}

	if enabled {
		var serverConfig map[string]interface{}
		found := false
		for _, srv := range s.config.MCPServers {
			if srv.Name == serverName {
				serverConfig = srv.Config
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("MCP server '%s' not found in app config", serverName)
		}

		copiedConfig := make(map[string]interface{})
		for key, value := range serverConfig {
			copiedConfig[key] = value
		}

		if format == "toml" {
			copiedConfig = s.translateServerConfigToTOML(copiedConfig)
		}

		mcpServers[serverName] = copiedConfig
	} else {
		delete(mcpServers, serverName)
	}

	return s.WriteClientConfig(clientName, rawConfig)
}

func (s *ClientConfigService) GetMCPServerStatus(clientName, serverName string) (bool, error) {
	rawConfig, err := s.ReadClientConfig(clientName)
	if err != nil {
		return false, err
	}

	client := s.findClient(clientName)
	if client == nil {
		return false, fmt.Errorf("client '%s' not found", clientName)
	}

	serversKey := s.serversKeyForFormat(s.clientFormat(client))
	mcpServers, ok := rawConfig[serversKey].(map[string]interface{})
	if !ok {
		return false, nil
	}

	_, exists := mcpServers[serverName]
	return exists, nil
}

func (s *ClientConfigService) findClient(name string) *models.Client {
	if client, exists := s.config.Clients[name]; exists {
		return client
	}
	return nil
}

func (s *ClientConfigService) backupConfig(configPath string) error {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil
	}

	backupPath := configPath + ".backup." + time.Now().Format("20060102-150405")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	return os.WriteFile(backupPath, data, 0644)
}

func (s *ClientConfigService) clientFormat(client *models.Client) string {
	format := strings.ToLower(strings.TrimSpace(client.Format))
	if format == "" {
		return "json"
	}
	return format
}

func (s *ClientConfigService) serversKeyForFormat(format string) string {
	if format == "toml" {
		return "mcp_servers"
	}
	return "mcpServers"
}

func (s *ClientConfigService) emptyConfigForFormat(format string) map[string]interface{} {
	return map[string]interface{}{
		s.serversKeyForFormat(format): make(map[string]interface{}),
	}
}

func (s *ClientConfigService) readJSONClientConfig(configPath string, data []byte) (map[string]interface{}, error) {
	var rawConfig map[string]interface{}
	if err := json.Unmarshal(data, &rawConfig); err != nil {
		return nil, fmt.Errorf("failed to parse client config '%s': %w", configPath, err)
	}
	if rawConfig["mcpServers"] == nil {
		rawConfig["mcpServers"] = make(map[string]interface{})
	}
	return rawConfig, nil
}

func (s *ClientConfigService) readTOMLClientConfig(configPath string, data []byte) (map[string]interface{}, error) {
	var rawConfig map[string]interface{}
	if err := toml.Unmarshal(data, &rawConfig); err != nil {
		return nil, fmt.Errorf("failed to parse client config '%s': %w", configPath, err)
	}
	if rawConfig["mcp_servers"] == nil {
		rawConfig["mcp_servers"] = make(map[string]interface{})
	}
	return rawConfig, nil
}

func (s *ClientConfigService) translateServerConfigToTOML(serverConfig map[string]interface{}) map[string]interface{} {
	if shouldBridgeHTTPViaStdio(serverConfig) {
		return bridgeHTTPServerConfigToTOML(serverConfig)
	}

	translated := make(map[string]interface{})
	for key, value := range serverConfig {
		switch key {
		case "httpUrl":
			translated["url"] = value
		case "headers":
			translated["http_headers"] = value
		case "type":
			// Codex infers transport from url vs command; no direct type field needed.
			continue
		default:
			translated[key] = value
		}
	}
	if _, ok := translated["enabled"]; !ok {
		translated["enabled"] = true
	}
	if _, hasURL := translated["url"]; hasURL {
		if _, ok := translated["startup_timeout_sec"]; !ok {
			translated["startup_timeout_sec"] = 20
		}
	}
	if _, hasCmd := translated["command"]; hasCmd {
		if _, ok := translated["startup_timeout_sec"]; !ok {
			translated["startup_timeout_sec"] = 20
		}
	}
	return translated
}

func shouldBridgeHTTPViaStdio(serverConfig map[string]interface{}) bool {
	value, ok := serverConfig["bridge_http_via_stdio"]
	if !ok {
		return false
	}

	flag, ok := value.(bool)
	return ok && flag
}

func bridgeHTTPServerConfigToTOML(serverConfig map[string]interface{}) map[string]interface{} {
	url, ok := httpServerURL(serverConfig)
	if !ok {
		return defaultTOMLServerConfig(serverConfig)
	}

	args := []string{"-y", "mcp-remote", url}

	if headers, ok := serverConfig["headers"].(map[string]interface{}); ok {
		keys := make([]string, 0, len(headers))
		for key := range headers {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			args = append(args, "--header", fmt.Sprintf("%s: %v", key, headers[key]))
		}
	}

	return map[string]interface{}{
		"command":             "npx",
		"args":                args,
		"enabled":             true,
		"startup_timeout_sec": 20,
	}
}

func httpServerURL(serverConfig map[string]interface{}) (string, bool) {
	if url, ok := serverConfig["url"].(string); ok && strings.TrimSpace(url) != "" {
		return url, true
	}

	if httpURL, ok := serverConfig["httpUrl"].(string); ok && strings.TrimSpace(httpURL) != "" {
		return httpURL, true
	}

	return "", false
}

func defaultTOMLServerConfig(serverConfig map[string]interface{}) map[string]interface{} {
	translated := make(map[string]interface{})
	for key, value := range serverConfig {
		switch key {
		case "httpUrl":
			translated["url"] = value
		case "headers":
			translated["http_headers"] = value
		case "type", "bridge_http_via_stdio":
			continue
		default:
			translated[key] = value
		}
	}
	if _, ok := translated["enabled"]; !ok {
		translated["enabled"] = true
	}
	if _, hasURL := translated["url"]; hasURL {
		if _, ok := translated["startup_timeout_sec"]; !ok {
			translated["startup_timeout_sec"] = 20
		}
	}
	if _, hasCmd := translated["command"]; hasCmd {
		if _, ok := translated["startup_timeout_sec"]; !ok {
			translated["startup_timeout_sec"] = 20
		}
	}
	return translated
}

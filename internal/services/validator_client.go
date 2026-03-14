package services

import (
	"fmt"
	"strings"

	"github.com/vlazic/mcp-server-manager/internal/models"
)

// ValidateClient validates a client configuration
func (v *ValidatorService) ValidateClient(clientName string, client *models.Client) error {
	if strings.TrimSpace(clientName) == "" {
		return fmt.Errorf("client name cannot be empty")
	}

	if client.ConfigPath == "" {
		return fmt.Errorf("client config path cannot be empty")
	}

	format := strings.ToLower(strings.TrimSpace(client.Format))
	if format != "" && format != "json" && format != "toml" {
		return fmt.Errorf("client format must be json or toml")
	}

	// Don't require the directory to exist - we'll create it if needed
	return nil
}

// ValidateClientConfig validates a client's MCP server configuration
func (v *ValidatorService) ValidateClientConfig(clientConfig *models.ClientConfig) error {
	if clientConfig.MCPServers == nil {
		return nil
	}

	for name, serverInterface := range clientConfig.MCPServers {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("server name cannot be empty")
		}

		// Validate if it's a proper server config map
		serverMap, ok := serverInterface.(map[string]interface{})
		if !ok {
			continue
		}

		// Reuse the same transport detection logic
		_, _, err := detectTransportType(serverMap)
		if err != nil {
			return fmt.Errorf("server '%s': %w", name, err)
		}
	}

	return nil
}

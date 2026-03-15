package services

import (
	"fmt"

	"github.com/vlazic/mcp-server-manager/internal/config"
	"github.com/vlazic/mcp-server-manager/internal/models"
)

type MCPManagerService struct {
	config              *models.Config
	clientConfigService *ClientConfigService
	validator           *ValidatorService
	configPath          string
}

func NewMCPManagerService(cfg *models.Config, configPath string) *MCPManagerService {
	return &MCPManagerService{
		config:              cfg,
		clientConfigService: NewClientConfigService(cfg),
		validator:           NewValidatorService(),
		configPath:          configPath,
	}
}

// GetMCPServers returns the ordered server slice
func (s *MCPManagerService) GetMCPServers() []models.MCPServer {
	return s.config.MCPServers
}

// GetClients returns the client map
func (s *MCPManagerService) GetClients() map[string]*models.Client {
	return s.config.Clients
}

// ToggleClientMCPServer enables or disables a server for a specific client
func (s *MCPManagerService) ToggleClientMCPServer(clientName, serverName string, enabled bool) error {
	// Validate client exists
	client, exists := s.config.Clients[clientName]
	if !exists {
		return fmt.Errorf("client '%s' not found", clientName)
	}

	// Validate server exists
	if !s.serverExists(serverName) {
		return fmt.Errorf("MCP server '%s' not found", serverName)
	}

	// Initialize enabled list if nil
	if client.Enabled == nil {
		client.Enabled = []string{}
	}

	// Update enabled list using utility functions
	if enabled {
		client.Enabled = addUnique(client.Enabled, serverName)
	} else {
		client.Enabled = removeItem(client.Enabled, serverName)
	}

	// Save config
	if err := s.saveConfig(); err != nil {
		return err
	}

	// Update client config file
	return s.clientConfigService.UpdateMCPServerStatus(clientName, serverName, enabled)
}

// serverExists checks if a server exists in the configuration
func (s *MCPManagerService) serverExists(serverName string) bool {
	for _, srv := range s.config.MCPServers {
		if srv.Name == serverName {
			return true
		}
	}
	return false
}

// GetServerStatus returns server configuration by name
func (s *MCPManagerService) GetServerStatus(serverName string) (map[string]interface{}, error) {
	for _, srv := range s.config.MCPServers {
		if srv.Name == serverName {
			return srv.Config, nil
		}
	}
	return nil, fmt.Errorf("MCP server '%s' not found", serverName)
}

// SyncAllClients synchronizes all client configurations based on enabled lists
func (s *MCPManagerService) SyncAllClients() error {
	for clientName, client := range s.config.Clients {
		if err := s.clientConfigService.SyncClientServers(clientName, client.Enabled); err != nil {
			return fmt.Errorf("failed to sync client '%s': %w", clientName, err)
		}
	}
	return nil
}

func (s *MCPManagerService) GetConfig() *models.Config {
	return s.config
}

func (s *MCPManagerService) ValidateConfig() error {
	return s.validator.ValidateConfig(s.config)
}

// AddServer adds a new MCP server to the configuration
func (s *MCPManagerService) AddServer(serverName string, serverConfig map[string]interface{}) error {
	// Validate the server config
	if err := s.validator.ValidateMCPServerConfig(serverName, serverConfig); err != nil {
		return fmt.Errorf("server validation failed: %w", err)
	}

	// Check if server with this name already exists
	for _, srv := range s.config.MCPServers {
		if srv.Name == serverName {
			return fmt.Errorf("server with name '%s' already exists", serverName)
		}
	}

	// Add the server to the config (appends to end)
	s.config.MCPServers = append(s.config.MCPServers, models.MCPServer{
		Name:   serverName,
		Config: serverConfig,
	})

	// Save the config
	return s.saveConfig()
}

func (s *MCPManagerService) saveConfig() error {
	if err := s.ValidateConfig(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}
	return config.SaveConfig(s.config, s.configPath)
}

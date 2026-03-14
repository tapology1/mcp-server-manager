package handlers

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/vlazic/mcp-server-manager/internal/services"
)

type APIHandler struct {
	mcpManager *services.MCPManagerService
	configPath string
}

func NewAPIHandler(mcpManager *services.MCPManagerService, configPath ...string) *APIHandler {
	resolvedConfigPath := ""
	if len(configPath) > 0 {
		resolvedConfigPath = configPath[0]
	}
	return &APIHandler{
		mcpManager: mcpManager,
		configPath: resolvedConfigPath,
	}
}

func (h *APIHandler) GetMCPServers(c *gin.Context) {
	servers := h.mcpManager.GetMCPServers()
	c.JSON(http.StatusOK, gin.H{"servers": servers})
}

func (h *APIHandler) GetClients(c *gin.Context) {
	clients := h.mcpManager.GetClients()
	c.JSON(http.StatusOK, gin.H{"clients": clients})
}

func (h *APIHandler) ToggleClientServer(c *gin.Context) {
	clientName := c.Param("client")
	serverName := c.Param("server")
	enabledStr := c.PostForm("enabled")

	enabled, err := strconv.ParseBool(enabledStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid enabled value"})
		return
	}

	if err := h.mcpManager.ToggleClientMCPServer(clientName, serverName, enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *APIHandler) GetServerStatus(c *gin.Context) {
	serverName := c.Param("server")

	server, err := h.mcpManager.GetServerStatus(serverName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, server)
}

func (h *APIHandler) SyncAllClients(c *gin.Context) {
	if err := h.mcpManager.SyncAllClients(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *APIHandler) RestartManager(c *gin.Context) {
	exePath, err := os.Executable()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to locate executable: " + err.Error()})
		return
	}

	cmd, err := buildRestartCommand(exePath, h.configPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := cmd.Start(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule restart: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Manager restarting"})
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
}

func buildRestartCommand(exePath, configPath string) (*exec.Cmd, error) {
	switch runtime.GOOS {
	case "windows":
		quotedExe := strings.ReplaceAll(exePath, "'", "''")
		quotedConfig := strings.ReplaceAll(configPath, "'", "''")
		script := fmt.Sprintf("Start-Sleep -Seconds 1; Start-Process -FilePath '%s' -ArgumentList @('--config','%s') -WindowStyle Hidden", quotedExe, quotedConfig)
		return exec.Command("powershell", "-NoProfile", "-Command", script), nil
	default:
		shellCmd := fmt.Sprintf("sleep 1; '%s' --config '%s' >/dev/null 2>&1 &", strings.ReplaceAll(exePath, "'", "'\\''"), strings.ReplaceAll(configPath, "'", "'\\''"))
		return exec.Command("sh", "-c", shellCmd), nil
	}
}

func (h *APIHandler) AddServer(c *gin.Context) {
	var requestBody struct {
		MCPServers map[string]map[string]interface{} `json:"mcpServers"`
	}

	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON: " + err.Error()})
		return
	}

	if len(requestBody.MCPServers) != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Must provide exactly one server in mcpServers"})
		return
	}

	var serverName string
	var serverConfig map[string]interface{}
	for name, config := range requestBody.MCPServers {
		serverName = name
		serverConfig = config
		break
	}

	if err := h.mcpManager.AddServer(serverName, serverConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"server": map[string]interface{}{
			"name":   serverName,
			"config": serverConfig,
		},
	})
}


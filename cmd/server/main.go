package main

import (
	"flag"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/vlazic/mcp-server-manager/internal/assets"
	"github.com/vlazic/mcp-server-manager/internal/config"
	"github.com/vlazic/mcp-server-manager/internal/handlers"
	"github.com/vlazic/mcp-server-manager/internal/services"
)

func main() {
	var configPath = flag.String("config", "", "Path to config file (default: smart resolution)")
	var configShort = flag.String("c", "", "Path to config file (short form)")
	flag.Parse()

	finalConfigPath := *configPath
	if *configShort != "" {
		finalConfigPath = *configShort
	}

	cfg, actualConfigPath, err := config.LoadConfig(finalConfigPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	mcpManager := services.NewMCPManagerService(cfg, actualConfigPath)

	r := gin.Default()

	funcMap := template.FuncMap{
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, fmt.Errorf("invalid dict call")
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
	}

	tmpl, err := assets.ParseTemplates(funcMap)
	if err != nil {
		log.Fatalf("Failed to parse embedded templates: %v", err)
	}
	r.SetHTMLTemplate(tmpl)

	staticFS, err := fs.Sub(assets.GetStaticFS(), "web/static")
	if err != nil {
		log.Fatalf("Failed to create static subdirectory: %v", err)
	}
	r.StaticFS("/static", http.FS(staticFS))

	apiHandler := handlers.NewAPIHandler(mcpManager, actualConfigPath)
	webHandler := handlers.NewWebHandler(mcpManager)
	configHandler := handlers.NewConfigViewerHandler(mcpManager, actualConfigPath)

	r.GET("/", webHandler.Index)
	r.GET("/config/app", configHandler.GetAppConfig)
	r.GET("/config/client/:client", configHandler.GetClientConfig)

	api := r.Group("/api")
	{
		api.GET("/servers", apiHandler.GetMCPServers)
		api.POST("/servers", apiHandler.AddServer)
		api.GET("/clients", apiHandler.GetClients)
		api.POST("/clients/:client/servers/:server/toggle", apiHandler.ToggleClientServer)
		api.GET("/servers/:server", apiHandler.GetServerStatus)
		api.POST("/sync", apiHandler.SyncAllClients)
		api.POST("/restart", apiHandler.RestartManager)
	}

	htmx := r.Group("/htmx")
	{
		htmx.POST("/clients/:client/servers/:server/toggle", webHandler.ToggleClientServerHTMX)
	}

	address := fmt.Sprintf(":%d", cfg.ServerPort)
	log.Printf("Starting MCP Manager server on %s", address)
	if err := r.Run(address); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

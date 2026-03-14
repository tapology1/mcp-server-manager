$ErrorActionPreference = "Stop"
$configPath = "C:\Users\owner\.config\mcp-server-manager\config.yaml"
$exePath = "C:\Users\owner\tools\mcp-server-manager\mcp-server-manager.exe"

$existing = Get-Process mcp-server-manager -ErrorAction SilentlyContinue
if ($existing) {
  Write-Output "mcp-server-manager is already running."
  exit 0
}

Start-Process -FilePath $exePath -ArgumentList @("--config", $configPath) -WindowStyle Hidden
Start-Sleep -Seconds 2
Start-Process "http://localhost:6543"

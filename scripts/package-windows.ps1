$ErrorActionPreference = "Stop"

Set-Location (Join-Path $PSScriptRoot "..")

Write-Host "Checking Wails..."
wails3 doctor

Write-Host "Packaging Windows installer..."
wails3 package GOOS=windows GOARCH=amd64

Write-Host ""
Write-Host "Expected NSIS installer location:"
Write-Host "  build/windows/nsis/DevStack-installer.exe"
Write-Host "The exact filename may follow the app name configured in build/config.yml."

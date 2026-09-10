$ErrorActionPreference = "Stop"

Write-Host "WSL status:"
wsl --status

Write-Host ""
Write-Host "Installed distributions:"
wsl --list --verbose

Write-Host ""
Write-Host "If WSL is missing, run from an elevated PowerShell:"
Write-Host "  wsl --install"
Write-Host ""
Write-Host "Then choose an Ubuntu/Debian WSL2 distribution in DevStack > Engine."

Write-Host "🦙 Installing Alpaka for Windows..."

$Repo = "Wheely20/alpaka"
$Url = "https://github.com/$Repo/releases/latest/download/alpaka-windows-amd64.exe"

# In den Alpaka Ordner installieren
$InstallDir = "$env:USERPROFILE\.alpaka\bin"

# Ordner erstellen, falls er nicht existiert
if (-not (Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
}

Write-Host "⬇️  Downloading Alpaka..."
Invoke-WebRequest -Uri $Url -OutFile "$InstallDir\alpaka.exe"

# Den Ordner zum PATH hinzufügen
$UserPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($UserPath -notmatch [regex]::Escape($InstallDir)) {
    [Environment]::SetEnvironmentVariable("PATH", "$UserPath;$InstallDir", "User")
    Write-Host "⚠️ Important: Please restart your terminal so that the PATH is updated!" -ForegroundColor Yellow
}

Write-Host "✅ Installation completed! Type 'alpaka', to start."
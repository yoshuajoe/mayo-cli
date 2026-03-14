# Mayo - Unified Installation Script for Windows (PowerShell)
# Mayo by Teleskop.id

Write-Host "🐶 Mayo Setup - Windows Intelligence Installer" -ForegroundColor Blue

# 1. Check Go installation
if (!(Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "❌ Go is not installed. Please install Go from https://golang.org/dl/" -ForegroundColor Red
    exit
}

# 2. Build the application
Write-Host "🔨 Building binary..." -ForegroundColor Yellow
if (!(Test-Path bin)) { New-Item -ItemType Directory -Path bin }
go build -o bin/mayo.exe main.go

# 3. Define installation directory
$GOPATH = go env GOPATH
$GOBIN = Join-Path $GOPATH "bin"
if (!(Test-Path $GOBIN)) { New-Item -ItemType Directory -Path $GOBIN }

# 4. Copy binary
Write-Host "🚀 Installing to $GOBIN..." -ForegroundColor Yellow
Copy-Item bin/mayo.exe -Destination $GOBIN

# 5. Handle configuration folders
$HOME_DIR = [System.Environment]::GetFolderPath("UserProfile")
$CONFIG_DIR = Join-Path $HOME_DIR ".mayo-cli"
if (!(Test-Path $CONFIG_DIR)) { New-Item -ItemType Directory -Path $CONFIG_DIR }
New-Item -ItemType Directory -Force -Path (Join-Path $CONFIG_DIR "sessions")
New-Item -ItemType Directory -Force -Path (Join-Path $CONFIG_DIR "data")

# 6. Path Verification
$CurrentPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($CurrentPath -like "*$GOBIN*") {
    Write-Host "✅ Installation successful!" -ForegroundColor Green
    Write-Host "You can now run 'mayo' from anywhere." -ForegroundColor Blue
} else {
    Write-Host "⚠️  Installation complete, but $GOBIN is not in your PATH." -ForegroundColor Yellow
    Write-Host "Adding to User PATH..." -ForegroundColor Cyan
    [Environment]::SetEnvironmentVariable("Path", $CurrentPath + ";" + $GOBIN, "User")
    Write-Host "✅ Added to PATH. Please restart your terminal/PowerShell for changes to take effect." -ForegroundColor Green
}

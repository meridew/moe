# MOE — Development Helper
# Usage: .\dev.ps1 <command>
# Commands: build, run, test, clean, reset

param([string]$Command = "build")

$env:PATH = "C:\Program Files\Go\bin;" + $env:PATH

switch ($Command) {
    "build" {
        Write-Host "Building..." -ForegroundColor Cyan
        go build ./...
        if ($LASTEXITCODE -eq 0) { Write-Host "OK" -ForegroundColor Green }
    }
    "run" {
        Write-Host "Starting MOE on :8080..." -ForegroundColor Cyan
        go run ./cmd/moe
    }
    "test" {
        Write-Host "Running tests..." -ForegroundColor Cyan
        go test ./...
    }
    "clean" {
        Write-Host "Removing moe.db..." -ForegroundColor Yellow
        Remove-Item moe.db -ErrorAction SilentlyContinue
        Write-Host "Done" -ForegroundColor Green
    }
    "reset" {
        Write-Host "Clean rebuild + fresh DB..." -ForegroundColor Yellow
        Remove-Item moe.db -ErrorAction SilentlyContinue
        go build ./...
        if ($LASTEXITCODE -eq 0) {
            Write-Host "Build OK, starting..." -ForegroundColor Green
            go run ./cmd/moe
        }
    }
    default {
        Write-Host "Unknown command: $Command" -ForegroundColor Red
        Write-Host "Usage: .\dev.ps1 <build|run|test|clean|reset>"
    }
}

# Installs Snapshot on Windows and starts it at logon, elevated.
#
#   powershell -ExecutionPolicy Bypass -File packaging\install-windows.ps1
#
# Run it from an elevated prompt. The VSS backend makes Volume Shadow
# Copies, and that is an Administrator operation on every Windows edition.
#
# This registers a scheduled task, not a Windows service. snapshotd is a
# console program: it does not answer the service control protocol, so
# sc.exe would start it and then kill it for never reporting that it
# started. A real service needs service support compiled in, which is on
# the roadmap and not in v0.1.0.
[CmdletBinding()]
param(
    [string]$InstallDir = "$env:ProgramFiles\Snapshot",
    [string]$DataDir    = "$env:ProgramData\Snapshot",
    [string]$Address    = "127.0.0.1:7099"
)

$ErrorActionPreference = "Stop"

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw "Run this from an elevated prompt. A shadow copy needs Administrator."
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null

foreach ($exe in @("snapshotd.exe", "snapctl.exe")) {
    if (-not (Test-Path ".\$exe")) { throw "$exe is not in this directory. Unpack the release first." }
    Copy-Item ".\$exe" -Destination $InstallDir -Force
}

$action = New-ScheduledTaskAction -Execute "$InstallDir\snapshotd.exe" `
    -Argument "-addr $Address -data-dir `"$DataDir`""
$trigger = New-ScheduledTaskTrigger -AtLogOn
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
Register-ScheduledTask -TaskName "Snapshot daemon" -Action $action -Trigger $trigger `
    -Settings $settings -RunLevel Highest -Force | Out-Null

Start-ScheduledTask -TaskName "Snapshot daemon"

# Put snapctl on PATH for this machine.
$path = [Environment]::GetEnvironmentVariable("Path", "Machine")
if ($path -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$path;$InstallDir", "Machine")
}

Write-Output "Installed to $InstallDir. Data directory is $DataDir."
Write-Output "Open a new terminal, then: snapctl workset proj C:\path\to\work"

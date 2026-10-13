# Developer checks only. Product installation still requires Windows 11 Client.
[CmdletBinding()]
param([Parameter(Mandatory = $true)][string]$OutputDirectory)
Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT' -or [IntPtr]::Size -ne 8) {
    throw 'Native developer checks require a 64-bit Windows PowerShell process.'
}
$shellVersion = $PSVersionTable.PSVersion
if (!(($shellVersion.Major -eq 5 -and $shellVersion.Minor -eq 1) -or $shellVersion.Major -eq 7)) {
    throw 'Native developer checks require PowerShell 5.1 or 7.'
}
# A distinct read-only developer guard: never override the product guard or
# registry/environment to make a Server machine impersonate Windows Client.
if (-not ('Jaekit.DeveloperPlatform' -as [type])) {
    Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
namespace Jaekit {
  public static class DeveloperPlatform {
    [StructLayout(LayoutKind.Sequential)] private struct SYSTEM_INFO {
      public ushort architecture, reserved; public uint pageSize;
      public IntPtr minAddress, maxAddress, activeMask;
      public uint processorCount, processorType, allocationGranularity;
      public ushort processorLevel, processorRevision;
    }
    [DllImport("kernel32.dll")] private static extern void GetNativeSystemInfo(out SYSTEM_INFO value);
    public static bool IsX64() { SYSTEM_INFO value; GetNativeSystemInfo(out value); return value.architecture == 9; }
  }
}
'@
}
if (-not [Jaekit.DeveloperPlatform]::IsX64()) { throw 'Native developer checks require Windows x64, without ARM64 emulation.' }
$windows = Get-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
$installation = [string]$windows.InstallationType
$build = [int]$windows.CurrentBuildNumber
if (!(($installation -eq 'Client' -and $build -ge 22000) -or $installation -in @('Server', 'Server Core'))) {
    throw 'Developer checks support Windows 11 Client or Windows Server x64.'
}
$shellExecutable = (Get-Process -Id $PID).Path
& python (Join-Path $PSScriptRoot 'native_checks.py') --output $OutputDirectory `
    --shell-executable $shellExecutable --shell-version ($shellVersion.ToString()) `
    --shell-edition $PSVersionTable.PSEdition --installation-type $installation --os-build $build
if ($LASTEXITCODE -ne 0) { throw 'Windows native developer checks failed; inspect summary.json and the captured logs.' }

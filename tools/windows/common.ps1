# Shared by the local-candidate installer and uninstaller. No system settings
# change when this file is loaded. Compatible with Windows PowerShell 5.1/7.
Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'

function Assert-WindowsPlatform {
    if ($env:OS -ne 'Windows_NT') { throw 'This installer requires Windows 11 x64.' }
    $v = $PSVersionTable.PSVersion
    if (!(($v.Major -eq 5 -and $v.Minor -eq 1) -or $v.Major -eq 7)) {
        throw 'Use Windows PowerShell 5.1 or PowerShell 7.'
    }
    if ([IntPtr]::Size -ne 8) { throw 'Use the 64-bit PowerShell process.' }
    if (-not ('Jaekit.InstallPlatform' -as [type])) {
        Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
namespace Jaekit {
  public static class InstallPlatform {
    [StructLayout(LayoutKind.Sequential)] private struct SYSTEM_INFO {
      public ushort architecture, reserved; public uint pageSize;
      public IntPtr minAddress, maxAddress, activeMask;
      public uint processorCount, processorType, allocationGranularity;
      public ushort processorLevel, processorRevision;
    }
    [DllImport("kernel32.dll")] private static extern void GetNativeSystemInfo(out SYSTEM_INFO value);
    [DllImport("user32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    private static extern IntPtr SendMessageTimeout(IntPtr window, uint message, UIntPtr parameter,
      string text, uint flags, uint timeout, out UIntPtr result);
    public static bool IsX64() { SYSTEM_INFO value; GetNativeSystemInfo(out value); return value.architecture == 9; }
    public static void NotifyEnvironment() {
      UIntPtr result; SendMessageTimeout(new IntPtr(0xffff), 0x1a, UIntPtr.Zero, "Environment", 2, 2000, out result);
    }
  }
}
'@
    }
    if (-not [Jaekit.InstallPlatform]::IsX64()) { throw 'Only native Windows x64 is supported; ARM64 emulation is not supported.' }
    $windowsVersion = Get-ItemProperty -LiteralPath 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
    $build = [int]$windowsVersion.CurrentBuildNumber
    if ($build -lt 22000 -or $windowsVersion.InstallationType -ne 'Client') { throw 'Windows 11 client is required.' }
}

function Assert-OrdinaryPath([string]$Path) {
    $full = [IO.Path]::GetFullPath($Path)
    if ($full -notmatch '^[A-Za-z]:\\') { throw "Only local drive paths are supported: $full" }
    $item = $full
    while ($item) {
        # GetAttributes observes a reparse point itself, including dangling links.
        try { $attrs = [IO.File]::GetAttributes($item) }
        catch [IO.FileNotFoundException] { $attrs = $null }
        catch [IO.DirectoryNotFoundException] { $attrs = $null }
        if ($null -ne $attrs -and ($attrs -band [IO.FileAttributes]::ReparsePoint)) {
            throw "Link or reparse-point path is not allowed: $item"
        }
        $parent = [IO.Directory]::GetParent($item)
        if ($null -eq $parent) { break }
        $item = $parent.FullName
    }
    return $full
}

function Get-InstallRoot {
    $base = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
    if ([string]::IsNullOrEmpty($base)) { throw 'The current user has no LocalApplicationData directory.' }
    return (Assert-OrdinaryPath (Join-Path $base 'Jaekit'))
}

function Get-Sha256([string]$Path) {
    Assert-OrdinaryPath $Path | Out-Null
    return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant()
}

function Initialize-InstallStreams {
    if ('Jaekit.InstallStreams' -as [type]) { return }
    Add-Type -TypeDefinition @'
using System;
using System.Collections.Generic;
using System.ComponentModel;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;
namespace Jaekit {
  public static class InstallStreams {
    private const uint FileNamedStreams = 0x00040000;
    private const int ErrorHandleEof = 38;
    private static readonly IntPtr InvalidHandle = new IntPtr(-1);
    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Unicode)]
    private struct StreamData {
      public long Size;
      [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 296)] public string Name;
    }
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, ExactSpelling = true, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool GetVolumePathNameW(string path, StringBuilder volumePath, uint bufferLength);
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, ExactSpelling = true, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool GetVolumeInformationW(string root, StringBuilder volumeName, uint volumeNameSize,
      out uint serial, out uint maximumComponentLength, out uint flags, StringBuilder fileSystemName, uint fileSystemNameSize);
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, ExactSpelling = true, SetLastError = true)]
    private static extern IntPtr FindFirstStreamW(string path, int level, out StreamData data, uint flags);
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, ExactSpelling = true, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool FindNextStreamW(IntPtr handle, out StreamData data);
    [DllImport("kernel32.dll", ExactSpelling = true, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool FindClose(IntPtr handle);

    private static IOException Failure(string path, string operation, int error) {
      return new IOException("Cannot inspect named data streams: " + path + " (" + operation +
        ", Win32 error " + error + ").", new Win32Exception(error));
    }

    public static string[] Read(string path) {
      // A successful volume query without FILE_NAMED_STREAMS is positive evidence
      // of unsupported streams. An enumeration error alone is never that evidence.
      File.GetAttributes(path);
      StringBuilder volumePath = new StringBuilder(32768);
      if (!GetVolumePathNameW(path, volumePath, (uint)volumePath.Capacity))
        throw Failure(path, "GetVolumePathNameW", Marshal.GetLastWin32Error());
      uint serial, maximumComponentLength, flags;
      if (!GetVolumeInformationW(volumePath.ToString(), null, 0, out serial,
          out maximumComponentLength, out flags, null, 0))
        throw Failure(path, "GetVolumeInformationW", Marshal.GetLastWin32Error());
      if ((flags & FileNamedStreams) == 0) return new string[0];

      StreamData data;
      IntPtr handle = FindFirstStreamW(path, 0, out data, 0);
      if (handle == InvalidHandle) {
        int error = Marshal.GetLastWin32Error();
        if (error == ErrorHandleEof) return new string[0];
        throw Failure(path, "FindFirstStreamW", error);
      }
      try {
        List<string> names = new List<string>();
        while (true) {
          // This API enumerates $DATA only, not NTFS structural metadata. Do not
          // filter by size or exempt download metadata such as Zone.Identifier.
          if (!String.Equals(data.Name, "::$DATA", StringComparison.OrdinalIgnoreCase)) {
            if (String.IsNullOrEmpty(data.Name) || data.Name.Length <= 7 ||
                !data.Name.StartsWith(":", StringComparison.Ordinal) ||
                !data.Name.EndsWith(":$DATA", StringComparison.OrdinalIgnoreCase))
              throw new IOException("Cannot inspect named data streams: " + path + " (unexpected stream name).");
            names.Add(data.Name);
          }
          if (!FindNextStreamW(handle, out data)) {
            int error = Marshal.GetLastWin32Error();
            if (error != ErrorHandleEof) throw Failure(path, "FindNextStreamW", error);
            break;
          }
        }
        return names.ToArray();
      } finally {
        if (!FindClose(handle)) throw Failure(path, "FindClose", Marshal.GetLastWin32Error());
      }
    }
  }
}
'@
}

function Get-ItemDataStreams([string]$Path) {
    Initialize-InstallStreams
    return [Jaekit.InstallStreams]::Read($Path)
}

function Assert-NoNamedStreams([string]$Path) {
    $full = Assert-OrdinaryPath $Path
    foreach ($stream in @(Get-ItemDataStreams $full)) {
        throw "Named data stream; nothing will be replaced or deleted: $full ($stream)"
    }
}

function Enter-InstallLock([string]$Root) {
    $sha = [Security.Cryptography.SHA256]::Create()
    try { $id = [BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($Root.ToUpperInvariant()))).Replace('-', '') }
    finally { $sha.Dispose() }
    $mutex = New-Object Threading.Mutex($false, ('Global\JaekitInstall-' + $id))
    try {
        try { $held = $mutex.WaitOne(0) }
        catch [Threading.AbandonedMutexException] { $held = $true }
        if (-not $held) { throw 'Another Jaekit installation operation is in progress.' }
        return $mutex
    } catch { $mutex.Dispose(); throw }
}

function Assert-PackageName([string]$Name) {
    # Conservative package policy, independent of OS-specific device resolution.
    # A console name with any extension is refused in every path component.
    if ([string]::IsNullOrEmpty($Name) -or $Name.Contains('\') -or $Name.StartsWith('/')) { throw "Unsafe package path: $Name" }
    foreach ($part in $Name.Split('/')) {
        if (!$part -or $part -eq '.' -or $part -eq '..' -or $part -match '[ .]$' -or
            $part -match '[\x00-\x1f:<>"|?*]' -or $part -match '^(?i:CON|CONIN\$|CONOUT\$|PRN|AUX|NUL|COM[1-9\u00b9\u00b2\u00b3]|LPT[1-9\u00b9\u00b2\u00b3])(?:\..*)?$') {
            throw "Unsafe package path: $Name"
        }
    }
}

function Join-PackagePath([string]$Base, [string]$Name) {
    Assert-PackageName $Name
    return (Join-Path $Base $Name.Replace('/', '\'))
}

function Read-PackageManifest([string]$Directory) {
    $path = Join-Path $Directory 'package.json'
    Assert-OrdinaryPath $path | Out-Null
    if ((Get-Item -LiteralPath $path).Length -gt 1048576) { throw 'Package manifest is too large.' }
    $manifest = [IO.File]::ReadAllText($path, [Text.Encoding]::UTF8) | ConvertFrom-Json
    if ($manifest.version -isnot [string] -or $manifest.files -isnot [Array] -or
        $manifest.schema -ne 'jaekit-windows-package/v1' -or $manifest.os -ne 'windows' -or $manifest.arch -ne 'amd64' -or
        $manifest.version -notmatch '^\d+\.\d+\.\d+(?:-[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*)?$') { throw 'Unsupported package manifest.' }
    $files = @{}
    $total = 0L
    foreach ($entry in @($manifest.files)) {
        if ($entry.path -isnot [string] -or $entry.sha256 -isnot [string] -or
            -not ($entry.size -is [int] -or $entry.size -is [long])) { throw 'Invalid package file entry types.' }
        Assert-PackageName $entry.path
        if ($entry.path -eq 'package.json' -or $files.ContainsKey($entry.path) -or
            $entry.sha256 -cnotmatch '^[a-f0-9]{64}$' -or $entry.size -lt 0 -or $entry.size -gt 33554432) {
            throw "Invalid or duplicate manifest file: $($entry.path)"
        }
        $files[$entry.path] = $entry
        $total += $entry.size
    }
    if ($files.Count -gt 2047 -or $total -gt 134217728) { throw 'Package manifest exceeds limits.' }
    foreach ($required in @('ha.exe', 'LICENSE', 'guides/WINDOWS.md', 'guides/WINDOWS.en.md', 'tools/windows/common.ps1', 'tools/windows/uninstall.ps1')) {
        if (-not $files.ContainsKey($required)) { throw "Missing package file: $required" }
    }
    return @{ manifest = $manifest; files = $files }
}

function Assert-TreeInventory([string]$Directory, [hashtable]$Expected) {
    Assert-OrdinaryPath $Directory | Out-Null
    Assert-NoNamedStreams $Directory
    $allowedDirs = @{}
    foreach ($name in $Expected.Keys) {
        $parts = $name.Split('/')
        for ($i = 1; $i -lt $parts.Count; $i++) { $allowedDirs[($parts[0..($i - 1)] -join '/')] = $true }
    }
    $seen = @{}
    $stack = New-Object 'Collections.Generic.Stack[string]'
    $stack.Push($Directory)
    while ($stack.Count -gt 0) {
        $dir = $stack.Pop()
        foreach ($path in [IO.Directory]::GetFileSystemEntries($dir)) {
            Assert-OrdinaryPath $path | Out-Null
            Assert-NoNamedStreams $path
            $name = $path.Substring($Directory.Length + 1).Replace('\', '/')
            $attrs = [IO.File]::GetAttributes($path)
            if ($attrs -band [IO.FileAttributes]::Directory) {
                if (-not $allowedDirs.ContainsKey($name)) { throw "Unmanaged directory; nothing will be replaced: $path" }
                $stack.Push($path)
            } else {
                if (-not $Expected.ContainsKey($name)) { throw "Unmanaged file; nothing will be replaced: $path" }
                if ((Get-Sha256 $path) -ne $Expected[$name]) { throw "Installed/package file differs: $path" }
                $seen[$name] = $true
            }
        }
    }
    if ($seen.Count -ne $Expected.Count) { throw 'Installation/package files are missing.' }
}

function Expand-CheckedPackage([string]$Archive, [string]$Checksums, [string]$Destination) {
    $archivePath = Assert-OrdinaryPath $Archive
    $checksumPath = Assert-OrdinaryPath $Checksums
    if ((Get-Item -LiteralPath $archivePath).Length -gt 134217728 -or (Get-Item -LiteralPath $checksumPath).Length -gt 1048576) {
        throw 'Candidate archive or checksums exceed size limits.'
    }
    $leaf = [IO.Path]::GetFileName($archivePath)
    $checksumMatches = @()
    foreach ($line in [IO.File]::ReadAllLines($checksumPath)) {
        if ($line -match '^([a-fA-F0-9]{64}) [ *](.+)$' -and $Matches[2] -ceq $leaf) { $checksumMatches += $Matches[1].ToLowerInvariant() }
    }
    if ($checksumMatches.Count -ne 1 -or (Get-Sha256 $archivePath) -ne $checksumMatches[0]) { throw 'Archive checksum is missing, duplicated or does not match.' }
    Add-Type -AssemblyName System.IO.Compression
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $stream = [IO.File]::Open($archivePath, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::Read)
    $zip = $null
    try {
        $zip = New-Object IO.Compression.ZipArchive($stream, [IO.Compression.ZipArchiveMode]::Read, $false)
        # .NET Framework (Windows PowerShell 5.1) cannot list entries whose
        # names hold characters a path may not contain. Read as a property,
        # that failure would become $null; the method call keeps it.
        try {
            $entries = $zip.get_Entries()
        } catch [ArgumentException] {
            throw 'Unsafe package path: an archive entry name is not a valid path.'
        }
        $names = @{}
        $total = 0L
        if ($entries.Count -gt 2048) { throw 'Too many archive entries.' }
        foreach ($entry in $entries) {
            $name = $entry.FullName
            Assert-PackageName $name
            $type = ([int64]$entry.ExternalAttributes -shr 16) -band 0xF000
            if ($names.ContainsKey($name) -or ($type -ne 0 -and $type -ne 0x8000) -or
                ($entry.ExternalAttributes -band 0x410) -ne 0 -or $entry.Length -gt 33554432) { throw "Unsafe or duplicate archive entry: $name" }
            $names[$name] = $true
            $total += $entry.Length
        }
        if ($total -gt 134217728) { throw 'Expanded archive exceeds size limit.' }
        # Validate file/directory collisions before writing any archive content.
        foreach ($name in $names.Keys) {
            $parts = $name.Split('/')
            for ($i = 1; $i -lt $parts.Count; $i++) {
                if ($names.ContainsKey(($parts[0..($i - 1)] -join '/'))) { throw "Archive file/directory collision: $name" }
            }
        }
        [IO.Directory]::CreateDirectory($Destination) | Out-Null
        foreach ($entry in $entries) {
            $target = Join-PackagePath $Destination $entry.FullName
            [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target)) | Out-Null
            $inputStream = $entry.Open()
            $outputStream = [IO.File]::Open($target, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
            try {
                $buffer = New-Object byte[] 65536
                $written = 0L
                while (($count = $inputStream.Read($buffer, 0, $buffer.Length)) -gt 0) {
                    $written += $count
                    if ($written -gt $entry.Length -or $written -gt 33554432) { throw 'Archive stream exceeds declared size.' }
                    $outputStream.Write($buffer, 0, $count)
                }
                if ($written -ne $entry.Length) { throw 'Truncated archive entry.' }
            } finally { $outputStream.Dispose(); $inputStream.Dispose() }
        }
    } finally { if ($null -ne $zip) { $zip.Dispose() }; $stream.Dispose() }
    $package = Read-PackageManifest $Destination
    if ($leaf -cne ('ha_' + $package.manifest.version + '_windows_amd64.zip')) { throw 'Archive name and manifest version differ.' }
    $expected = @{'package.json' = (Get-Sha256 (Join-Path $Destination 'package.json'))}
    foreach ($name in $package.files.Keys) {
        $file = Join-PackagePath $Destination $name
        if ((Get-Item -LiteralPath $file).Length -ne $package.files[$name].size) { throw "Package file size differs: $name" }
        $expected[$name] = $package.files[$name].sha256
    }
    Assert-TreeInventory $Destination $expected
    return $package
}

function Read-Installation([string]$Root) {
    if (-not [IO.Directory]::Exists($Root)) {
        if ([IO.File]::Exists($Root)) { throw "Installation root is not a directory: $Root" }
        return $null
    }
    Assert-OrdinaryPath $Root | Out-Null
    $statePath = Join-Path $Root 'managed.json'
    if (-not [IO.File]::Exists($statePath)) { throw "Unmanaged installation directory; leave it intact: $Root" }
    Assert-OrdinaryPath $statePath | Out-Null
    if ((Get-Item -LiteralPath $statePath).Length -gt 1048576) { throw 'Installation state is too large.' }
    $state = [IO.File]::ReadAllText($statePath, [Text.Encoding]::UTF8) | ConvertFrom-Json
    if ($state.schema -ne 'jaekit-windows-install/v1' -or $state.owns_path -isnot [bool] -or $state.created_path_value -isnot [bool] -or
        $state.versions -isnot [Array] -or $state.current_version -isnot [string]) { throw 'Invalid installation state.' }
    $expected = @{'managed.json' = (Get-Sha256 $statePath)}
    $versions = @{}
    foreach ($version in @($state.versions)) {
        if ($version.version -isnot [string] -or $version.manifest_sha256 -isnot [string] -or
            $version.manifest_sha256 -cnotmatch '^[a-f0-9]{64}$' -or
            $version.version -notmatch '^\d+\.\d+\.\d+(?:-[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*)?$' -or $versions.ContainsKey($version.version)) { throw 'Invalid installed version.' }
        $base = Join-Path (Join-Path $Root 'versions') $version.version
        $manifestPath = Join-Path $base 'package.json'
        if ((Get-Sha256 $manifestPath) -ne $version.manifest_sha256) { throw "Installed version manifest differs: $base" }
        $package = Read-PackageManifest $base
        if ($package.manifest.version -cne $version.version) { throw 'Installed version identity differs.' }
        $expected['versions/' + $version.version + '/package.json'] = $version.manifest_sha256
        foreach ($name in $package.files.Keys) { $expected['versions/' + $version.version + '/' + $name] = $package.files[$name].sha256 }
        $versions[$version.version] = $package
    }
    if (-not $versions.ContainsKey($state.current_version)) { throw 'Current installation version is missing.' }
    $expected['bin/ha.exe'] = $versions[$state.current_version].files['ha.exe'].sha256
    Assert-TreeInventory $Root $expected
    return @{state = $state; files = $expected; versions = $versions}
}

function Copy-Inventory([string]$From, [string]$To, [hashtable]$Files) {
    foreach ($name in $Files.Keys) {
        $source = Join-PackagePath $From $name
        Assert-OrdinaryPath $source | Out-Null
        $target = Join-PackagePath $To $name
        [IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($target)) | Out-Null
        [IO.File]::Copy($source, $target, $false)
    }
}

function Assert-ExecutableReplaceable([string]$Root) {
    $exe = Join-Path $Root 'bin\ha.exe'
    if ([IO.File]::Exists($exe)) {
        try { $probe = [IO.File]::Open($exe, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None); $probe.Dispose() }
        catch { throw "The current executable cannot be replaced (in use or access denied); stop its users and retry: $exe" }
    }
}

function Read-UserPath {
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $false)
    try {
        if ($null -eq $key) { return @{value = ''; exists = $false; kind = [Microsoft.Win32.RegistryValueKind]::ExpandString} }
        $value = $key.GetValue('Path', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        $exists = $null -ne $value
        $kind = [Microsoft.Win32.RegistryValueKind]::ExpandString
        if ($exists) { $kind = $key.GetValueKind('Path') }
        if ($kind -ne [Microsoft.Win32.RegistryValueKind]::String -and $kind -ne [Microsoft.Win32.RegistryValueKind]::ExpandString) { throw 'User PATH is not a string.' }
        return @{value = [string]$value; exists = $exists; kind = $kind}
    } finally { if ($null -ne $key) { $key.Dispose() } }
}

function Test-PathEntry([string]$Entry, [string]$Bin) {
    $expanded = [Environment]::ExpandEnvironmentVariables($Entry.Trim().Trim('"')).TrimEnd('\', '/')
    return $expanded.Equals($Bin.TrimEnd('\', '/'), [StringComparison]::OrdinalIgnoreCase)
}

function Get-OwnedPathRemoval([string]$Value, [string]$Bin, [bool]$Owned) {
    if (-not $Owned) { return @{value = $Value; changed = $false} }
    $entries = New-Object 'Collections.Generic.List[string]'
    foreach ($entry in $Value.Split(';')) { $entries.Add($entry) }
    # Installation appends only when no equivalent entry exists. Preserve
    # duplicate literals appended later, including their position among other
    # entries. The saved state does not reconstruct arbitrary user reordering.
    for ($i = 0; $i -lt $entries.Count; $i++) {
        if ($entries[$i] -ceq $Bin) {
            $entries.RemoveAt($i)
            return @{value = ($entries -join ';'); changed = $true}
        }
    }
    return @{value = $Value; changed = $false}
}

function Write-UserPath([hashtable]$Before, [string]$After, [bool]$DeleteValue) {
    $current = Read-UserPath
    if ($current.value -cne $Before.value -or $current.exists -ne $Before.exists -or $current.kind -ne $Before.kind) {
        throw 'User PATH changed concurrently; nothing was overwritten. Retry after reviewing PATH.'
    }
    $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
    try {
        if ($DeleteValue) { $key.DeleteValue('Path', $false) }
        else { $key.SetValue('Path', $After, $Before.kind) }
    } finally { $key.Dispose() }
}

function Update-ProcessPath([string]$Bin, [bool]$Add) {
    if ($Add) {
        $entries = New-Object 'Collections.Generic.List[string]'
        foreach ($entry in $env:Path.Split(';')) { $entries.Add($entry) }
        $index = -1
        for ($i = $entries.Count - 1; $i -ge 0; $i--) {
            if (Test-PathEntry $entries[$i] $Bin) { $index = $i; break }
        }
        if ($index -lt 0) { $entries.Add($Bin) }
        $env:Path = $entries -join ';'
    } else {
        $removal = Get-OwnedPathRemoval $env:Path $Bin $true
        $env:Path = $removal.value
    }
    [Jaekit.InstallPlatform]::NotifyEnvironment()
}

function Remove-OrdinaryTree([string]$Directory) {
    if (-not [IO.Directory]::Exists($Directory)) { return }
    Assert-OrdinaryPath $Directory | Out-Null
    Assert-NoNamedStreams $Directory
    $stack = New-Object 'Collections.Generic.Stack[string]'
    $stack.Push($Directory)
    while ($stack.Count -gt 0) {
        foreach ($path in [IO.Directory]::GetFileSystemEntries($stack.Pop())) {
            Assert-OrdinaryPath $path | Out-Null
            Assert-NoNamedStreams $path
            if ([IO.Directory]::Exists($path)) { $stack.Push($path) }
        }
    }
    # Finish the entire inspection before deleting any item. A refused candidate
    # or backup must not lose its extra data through the caller's finally block.
    [IO.Directory]::Delete($Directory, $true)
}

function Save-InstallState([string]$Path, [object]$State) {
    $json = ConvertTo-Json -InputObject $State -Depth 8
    [IO.File]::WriteAllText($Path, $json + "`n", (New-Object Text.UTF8Encoding($false)))
}

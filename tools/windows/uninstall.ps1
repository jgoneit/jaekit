<#
.SYNOPSIS
Remove only the current user's inventoried Jaekit installation and owned PATH entry.
#>
[CmdletBinding()]
param()
. (Join-Path $PSScriptRoot 'common.ps1')

$mutex = $null
$backup = $null
$committed = $false
try {
    Assert-WindowsPlatform
    $root = Get-InstallRoot
    $bin = Join-Path $root 'bin'
    $mutex = Enter-InstallLock $root
    $installed = Read-Installation $root
    if ($null -eq $installed) { Write-Output "No managed installation at $root"; return }
    Assert-ExecutableReplaceable $root
    $beforePath = Read-UserPath
    $afterPath = $beforePath.value
    $pathChanged = $false
    if ($installed.state.owns_path) {
        $entries = New-Object 'Collections.Generic.List[string]'
        foreach ($entry in $beforePath.value.Split(';')) { $entries.Add($entry) }
        # One entry was added. Keep unrelated entries, their order and any
        # duplicate someone added later; never replace the whole PATH snapshot.
        for ($i = $entries.Count - 1; $i -ge 0; $i--) {
            if ($entries[$i] -ceq $bin) { $entries.RemoveAt($i); $pathChanged = $true; break }
        }
        $afterPath = $entries -join ';'
    }
    $backup = Join-Path ([IO.Path]::GetDirectoryName($root)) ('.jaekit-remove-' + [Guid]::NewGuid().ToString('N'))
    [IO.Directory]::Move($root, $backup)
    try {
        if ($pathChanged) {
            $deleteValue = -not $afterPath -and $installed.state.created_path_value
            Write-UserPath $beforePath $afterPath $deleteValue
        }
        $committed = $true
    } catch {
        $failure = $_
        try { [IO.Directory]::Move($backup, $root); $backup = $null }
        catch { throw "Removal failed and rollback needs attention. Preserved installation: $backup. Original error: $failure. Rollback error: $_" }
        throw $failure
    }
    if ($pathChanged) {
        try { Update-ProcessPath $bin $false }
        catch { Write-Warning "Removal committed; reopen parent apps to refresh PATH: $_" }
    }
    Write-Output "Removed the managed installation at $root"
    Write-Output 'Project files, copied examples, goals, records, Git log directories and other installations were not removal targets.'
    if ($pathChanged) { Write-Output 'Removed one installer-owned user PATH entry. Restart parent apps before opening new terminals.' }
} catch {
    Write-Error -ErrorAction Continue ("Removal failed: " + $_)
    exit 1
} finally {
    if ($committed -and $backup) {
        try { Remove-OrdinaryTree $backup }
        catch { Write-Warning "Removal committed, but preserved installation backup could not be cleaned: $backup ($_)" }
    }
    if ($null -ne $mutex) { $mutex.ReleaseMutex(); $mutex.Dispose() }
}

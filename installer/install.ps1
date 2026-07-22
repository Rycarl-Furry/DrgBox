$ErrorActionPreference = 'Stop'
$target = 'D:\Car1N0tCat\DRGBOX'
New-Item -ItemType Directory -Force -Path $target | Out-Null
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'DrgBoxDesktop.exe') -Destination (Join-Path $target 'DRGBOX.exe') -Force

$shell = New-Object -ComObject WScript.Shell
$desktop = [Environment]::GetFolderPath('Desktop')
$shortcut = $shell.CreateShortcut((Join-Path $desktop 'DRGBOX 工具箱.lnk'))
$shortcut.TargetPath = Join-Path $target 'DRGBOX.exe'
$shortcut.WorkingDirectory = $target
$shortcut.IconLocation = "$($shortcut.TargetPath),0"
$shortcut.Description = 'DRGBOX 本地分类工具箱'
$shortcut.Save()

Start-Process -FilePath (Join-Path $target 'DRGBOX.exe')

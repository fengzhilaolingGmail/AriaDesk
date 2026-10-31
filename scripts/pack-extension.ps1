# 打包浏览器插件为 zip，已存在的同名包先删除再重新打包
# 输出 build/bin/ariadesk-extension-<version>.zip
# 用法：powershell -ExecutionPolicy Bypass -File scripts\pack-extension.ps1
$ErrorActionPreference = 'Stop'

$root = Split-Path $PSScriptRoot -Parent
$manifestPath = Join-Path $root 'extension\manifest.json'
if (-not (Test-Path $manifestPath)) { throw "未找到 $manifestPath" }

$version = (Get-Content $manifestPath -Raw | ConvertFrom-Json).version
$outDir = Join-Path $root 'build\bin'
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$zip = Join-Path $outDir "ariadesk-extension-$version.zip"

if (Test-Path $zip) {
    Remove-Item $zip -Force
    Write-Host "已删除旧文件: $zip"
}
Compress-Archive -Path (Join-Path $root 'extension\*') -DestinationPath $zip
if (-not (Test-Path $zip)) { throw "打包失败，未生成 $zip" }
Write-Host "已打包: $zip"

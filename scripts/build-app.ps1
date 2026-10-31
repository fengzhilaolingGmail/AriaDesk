# 编译 AriaDesk 桌面软件（wails build 会先自动执行前端 npm run build）
# 已存在的旧产物先删除再重新编译，输出 build/bin/AriaDesk.exe
# 用法：powershell -ExecutionPolicy Bypass -File scripts\build-app.ps1
$ErrorActionPreference = 'Stop'

$root = Split-Path $PSScriptRoot -Parent
Push-Location $root
try {
    $exe = Join-Path $root 'build\bin\AriaDesk.exe'
    if (Test-Path $exe) {
        Remove-Item $exe -Force
        Write-Host "已删除旧文件: $exe"
    }
    wails build
    if ($LASTEXITCODE -ne 0) { throw "wails build 失败（退出码 $LASTEXITCODE）" }
    if (-not (Test-Path $exe)) { throw "编译完成但未找到 $exe" }
    Write-Host "已编译: $exe"
} finally {
    Pop-Location
}

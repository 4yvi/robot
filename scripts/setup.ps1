# Windows setup: downloads Vosk (model + lib), whisper.cpp server and model,
# Piper and a Russian voice into .\models and .\third_party.
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
Set-Location $Root
$Models = Join-Path $Root "models"
New-Item -ItemType Directory -Force -Path $Models, "third_party" | Out-Null

$VoskModel = "vosk-model-small-ru-0.22"
$VoskLib = "vosk-win64-0.3.42"
$WhisperModel = "ggml-small.bin"
$PiperVer = "2023.11.14-2"
$VoiceBase = "https://huggingface.co/rhasspy/piper-voices/resolve/main/ru/ru_RU/irina/medium"

function Get-File($url, $out) {
    Write-Host "Downloading $url"
    Invoke-WebRequest -Uri $url -OutFile $out -UseBasicParsing
}

Write-Host "==> Vosk model"
if (-not (Test-Path "$Models\vosk-model-small-ru")) {
    Get-File "https://alphacephei.com/vosk/models/$VoskModel.zip" "$Models\vosk.zip"
    Expand-Archive "$Models\vosk.zip" -DestinationPath $Models -Force
    Rename-Item "$Models\$VoskModel" "vosk-model-small-ru"
    Remove-Item "$Models\vosk.zip"
}

Write-Host "==> libvosk"
if (-not (Test-Path "third_party\vosk\libvosk.dll")) {
    Get-File "https://github.com/alphacep/vosk-api/releases/download/v0.3.42/$VoskLib.zip" "third_party\vosk.zip"
    Expand-Archive "third_party\vosk.zip" -DestinationPath "third_party" -Force
    Rename-Item "third_party\$VoskLib" "vosk"
    Remove-Item "third_party\vosk.zip"
}

Write-Host "==> whisper.cpp server"
if (-not (Test-Path "$Models\whisper\whisper-server.exe")) {
    $rel = Invoke-RestMethod "https://api.github.com/repos/ggml-org/whisper.cpp/releases/latest"
    $asset = $rel.assets | Where-Object { $_.name -eq "whisper-bin-x64.zip" } | Select-Object -First 1
    if (-not $asset) { throw "whisper-bin-x64.zip not found in the latest release; download whisper-server manually." }
    Get-File $asset.browser_download_url "$Models\whisper.zip"
    Expand-Archive "$Models\whisper.zip" -DestinationPath "$Models\whisper-tmp" -Force
    New-Item -ItemType Directory -Force -Path "$Models\whisper" | Out-Null
    Get-ChildItem "$Models\whisper-tmp" -Recurse -File | Copy-Item -Destination "$Models\whisper" -Force
    Remove-Item "$Models\whisper-tmp", "$Models\whisper.zip" -Recurse -Force
}
if (-not (Test-Path "$Models\whisper\$WhisperModel")) {
    Get-File "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/$WhisperModel" "$Models\whisper\$WhisperModel"
}

Write-Host "==> Piper"
if (-not (Test-Path "$Models\piper\piper.exe")) {
    Get-File "https://github.com/rhasspy/piper/releases/download/$PiperVer/piper_windows_amd64.zip" "$Models\piper.zip"
    Expand-Archive "$Models\piper.zip" -DestinationPath $Models -Force
    Remove-Item "$Models\piper.zip"
}
foreach ($f in "ru_RU-irina-medium.onnx", "ru_RU-irina-medium.onnx.json") {
    if (-not (Test-Path "$Models\piper\$f")) { Get-File "$VoiceBase/$f" "$Models\piper\$f" }
}

if (-not (Test-Path "config.yaml")) {
    (Get-Content config.example.yaml) -replace 'models/piper/piper"', 'models/piper/piper.exe"' | Set-Content config.yaml -Encoding UTF8
}

Write-Host ""
Write-Host "Done. Start the whisper server:"
Write-Host "  $Models\whisper\whisper-server.exe -m $Models\whisper\$WhisperModel -l ru --port 8080"
Write-Host "Build:"
Write-Host "  `$env:CGO_CPPFLAGS='-I$Root\third_party\vosk'; `$env:CGO_LDFLAGS='-L$Root\third_party\vosk -lvosk'"
Write-Host "  go build -tags vosk -ldflags '-H=windowsgui' -o robot.exe ./cmd/robot"

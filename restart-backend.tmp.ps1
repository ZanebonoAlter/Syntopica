# Restart the Go backend (kill old go-build process, start fresh go run).
Stop-Process -Id 6652 -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1
$proc = Start-Process -FilePath "go" -ArgumentList "run","cmd/server/main.go" `
  -WorkingDirectory "D:/project/Syntopica/backend-go" `
  -RedirectStandardOutput "D:/project/Syntopica/backend-go/server.out.log" `
  -RedirectStandardError "D:/project/Syntopica/backend-go/server.err.log" `
  -PassThru -WindowStyle Hidden
Write-Output "NEW_PID=$($proc.Id)"

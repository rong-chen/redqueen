$f = 'd:\project\redqueen\controllers\voice_ws_controller.go'
$lines = [System.IO.File]::ReadAllLines($f)
$kept = $lines[0..1349]
[System.IO.File]::WriteAllLines($f, $kept)
Write-Host "Done, file now has $($kept.Count) lines"

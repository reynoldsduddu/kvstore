# run_benchmark.ps1
$cpuMonitor = "cpu_monitor.py"
$benchmark = "bench.py"
$plotter = "plot_cpu.py"

# Settings
$mode = "cabinet"
$concurrency = 10
$ops = 1000

Write-Host "Starting CPU monitoring..."
Start-Process -FilePath "python" -ArgumentList "$cpuMonitor --interval 1 --duration 70"

Write-Host "Running benchmark..."
python $benchmark --mode $mode --concurrency $concurrency --ops $ops | Tee-Object -FilePath "benchmark_log.txt"

Write-Host "Plotting CPU usage..."
python $plotter

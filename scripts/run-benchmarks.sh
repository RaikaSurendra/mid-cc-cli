#!/bin/bash

# Dedicated benchmark runner with detailed analysis

echo "╔════════════════════════════════════════════════════════════╗"
echo "║            Performance Benchmark Suite                    ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""

# Create benchmark output directory
mkdir -p benchmarks
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

echo "Running comprehensive benchmarks..."
echo "Results will be saved to: benchmarks/benchmark_$TIMESTAMP.txt"
echo ""

# Run benchmarks with detailed output
echo "=== Session Manager Benchmarks ===" | tee benchmarks/benchmark_$TIMESTAMP.txt
go test -bench=. -benchmem -benchtime=3s ./internal/session/ | tee -a benchmarks/benchmark_$TIMESTAMP.txt
echo "" | tee -a benchmarks/benchmark_$TIMESTAMP.txt

echo "=== Server Benchmarks ===" | tee -a benchmarks/benchmark_$TIMESTAMP.txt
go test -bench=. -benchmem -benchtime=3s ./internal/server/ | tee -a benchmarks/benchmark_$TIMESTAMP.txt
echo "" | tee -a benchmarks/benchmark_$TIMESTAMP.txt

echo "=== Config Benchmarks ===" | tee -a benchmarks/benchmark_$TIMESTAMP.txt
go test -bench=. -benchmem -benchtime=3s ./internal/config/ | tee -a benchmarks/benchmark_$TIMESTAMP.txt
echo "" | tee -a benchmarks/benchmark_$TIMESTAMP.txt

# CPU profiling
echo "=== Generating CPU Profile ===" | tee -a benchmarks/benchmark_$TIMESTAMP.txt
go test -bench=. -cpuprofile=benchmarks/cpu_$TIMESTAMP.prof ./internal/session/
go tool pprof -text benchmarks/cpu_$TIMESTAMP.prof | head -20 | tee -a benchmarks/benchmark_$TIMESTAMP.txt
echo "" | tee -a benchmarks/benchmark_$TIMESTAMP.txt

# Memory profiling
echo "=== Generating Memory Profile ===" | tee -a benchmarks/benchmark_$TIMESTAMP.txt
go test -bench=. -memprofile=benchmarks/mem_$TIMESTAMP.prof ./internal/session/
go tool pprof -text benchmarks/mem_$TIMESTAMP.prof | head -20 | tee -a benchmarks/benchmark_$TIMESTAMP.txt
echo "" | tee -a benchmarks/benchmark_$TIMESTAMP.txt

# Compare with baseline (if exists)
if [ -f benchmarks/baseline.txt ]; then
    echo "=== Performance Comparison ===" | tee -a benchmarks/benchmark_$TIMESTAMP.txt
    benchstat benchmarks/baseline.txt benchmarks/benchmark_$TIMESTAMP.txt 2>/dev/null || \
        echo "Install benchstat for comparison: go install golang.org/x/perf/cmd/benchstat@latest"
fi

echo ""
echo "╔════════════════════════════════════════════════════════════╗"
echo "║              Benchmark Results Summary                    ║"
echo "╚════════════════════════════════════════════════════════════╝"
echo ""
echo "Files generated:"
echo "  - benchmarks/benchmark_$TIMESTAMP.txt (full results)"
echo "  - benchmarks/cpu_$TIMESTAMP.prof (CPU profile)"
echo "  - benchmarks/mem_$TIMESTAMP.prof (Memory profile)"
echo ""
echo "To set as baseline:"
echo "  cp benchmarks/benchmark_$TIMESTAMP.txt benchmarks/baseline.txt"
echo ""
echo "To analyze profiles:"
echo "  go tool pprof -http=:8080 benchmarks/cpu_$TIMESTAMP.prof"
echo "  go tool pprof -http=:8080 benchmarks/mem_$TIMESTAMP.prof"
echo ""

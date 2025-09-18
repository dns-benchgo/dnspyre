package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/tantalor93/dnspyre/v3/pkg/scoring"
)

// BatchBenchmark represents a batch benchmark configuration
type BatchBenchmark struct {
	Servers     []string
	Output      string
	Duration    time.Duration
	Concurrency int
	Workers     int
	Domains     string
}

// BatchResult represents the final result containing all server results
type BatchResult map[string]interface{}

// SetupBatchCommand sets up the batch testing command
func SetupBatchCommand(app *kingpin.Application) {
	batchCmd := app.Command("batch", "Run DNS benchmark on multiple servers")

	var batch BatchBenchmark

	batchCmd.Flag("servers", "Comma-separated list of DNS servers to test").
		Required().
		PlaceHolder("8.8.8.8,1.1.1.1,114.114.114.114").
		StringsVar(&batch.Servers)

	batchCmd.Flag("output", "Output JSON file path").
		Default(fmt.Sprintf("dnspyre_batch_result_%s.json", time.Now().Format("2006-01-02-15-04-05"))).
		StringVar(&batch.Output)

	batchCmd.Flag("duration", "Test duration for each server").
		Default("10s").
		DurationVar(&batch.Duration)

	batchCmd.Flag("concurrency", "Number of concurrent queries per server").
		Default("10").
		IntVar(&batch.Concurrency)

	batchCmd.Flag("workers", "Number of servers to test simultaneously").
		Default("5").
		IntVar(&batch.Workers)

	batchCmd.Arg("domains", "Domain names to test").
		Default("example.com").
		StringVar(&batch.Domains)

	batchCmd.Action(func(c *kingpin.ParseContext) error {
		return RunBatchBenchmark(batch)
	})
}

// RunBatchBenchmark executes batch benchmarking on multiple servers
func RunBatchBenchmark(batch BatchBenchmark) error {
	fmt.Printf("Starting batch benchmark for %d servers...\n", len(batch.Servers))

	results := make(BatchResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create a semaphore to limit concurrent workers
	semaphore := make(chan struct{}, batch.Workers)

	for _, server := range batch.Servers {
		wg.Add(1)
		semaphore <- struct{}{} // Acquire semaphore

		go func(srv string) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release semaphore

			fmt.Printf("Testing server: %s\n", srv)

			result, err := runSingleServerBenchmark(srv, batch)
			if err != nil {
				fmt.Printf("Error testing server %s: %v\n", srv, err)
				return
			}

			mu.Lock()
			results[srv] = result
			mu.Unlock()

			fmt.Printf("Completed testing server: %s\n", srv)
		}(server)
	}

	wg.Wait()

	// Write results to file
	return writeResultsToFile(results, batch.Output)
}

// runSingleServerBenchmark runs dnspyre for a single server and returns the result
func runSingleServerBenchmark(server string, batch BatchBenchmark) (map[string]interface{}, error) {
	// Build dnspyre command
	args := []string{
		"benchmark",
		"--json",
		"--server", server,
		"--duration", batch.Duration.String(),
		"--concurrency", fmt.Sprintf("%d", batch.Concurrency),
	}

	// Add domains
	domains := strings.Split(batch.Domains, ",")
	args = append(args, domains...)

	// Execute command
	cmd := exec.Command("./dnspyre", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run dnspyre for server %s: %v", server, err)
	}

	// Parse JSON result
	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON result for server %s: %v", server, err)
	}

	// Add geocode information
	result["geocode"] = getServerGeocode(server)

	// Calculate score if not present
	if _, exists := result["score"]; !exists {
		if score := calculateScoreFromResult(result); score != nil {
			result["score"] = score
		}
	}

	return result, nil
}

// calculateScoreFromResult calculates performance score from benchmark result
func calculateScoreFromResult(result map[string]interface{}) *scoring.ScoreResult {
	// Extract metrics from result
	totalRequests, ok1 := result["totalRequests"].(float64)
	totalSuccess, ok2 := result["totalSuccessResponses"].(float64)
	totalError, ok3 := result["totalErrorResponses"].(float64)
	totalIOError, ok4 := result["totalIOErrors"].(float64)
	qps, ok5 := result["queriesPerSecond"].(float64)

	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 {
		return nil
	}

	// Extract latency stats
	latencyStats, ok := result["latencyStats"].(map[string]interface{})
	if !ok {
		return nil
	}

	meanMs, ok1 := latencyStats["meanMs"].(float64)
	stdMs, ok2 := latencyStats["stdMs"].(float64)
	p95Ms, ok3 := latencyStats["p95Ms"].(float64)
	p50Ms, ok4 := latencyStats["p50Ms"].(float64)

	if !ok1 || !ok2 || !ok3 || !ok4 {
		return nil
	}

	// Create metrics structure
	metrics := scoring.BenchmarkMetrics{
		TotalRequests:         int64(totalRequests),
		TotalSuccessResponses: int64(totalSuccess),
		TotalErrorResponses:   int64(totalError),
		TotalIOErrors:         int64(totalIOError),
		QueriesPerSecond:      qps,
		LatencyStats: scoring.LatencyMetrics{
			MeanMs: int64(meanMs),
			StdMs:  int64(stdMs),
			P95Ms:  int64(p95Ms),
			P50Ms:  int64(p50Ms),
		},
	}

	// Calculate score
	scoreResult := scoring.CalculateScore(metrics)
	return &scoreResult
}

// writeResultsToFile writes the batch results to a JSON file
func writeResultsToFile(results BatchResult, outputPath string) error {
	// Ensure the output file has .json extension
	if !strings.HasSuffix(outputPath, ".json") {
		outputPath += ".json"
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(results); err != nil {
		return fmt.Errorf("failed to write JSON results: %v", err)
	}

	fmt.Printf("Batch benchmark results written to: %s\n", outputPath)
	return nil
}

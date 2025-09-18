package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/miekg/dns"
	"github.com/tantalor93/dnspyre/v3/pkg/dnsbench"
	"github.com/tantalor93/dnspyre/v3/pkg/geo"
	"github.com/tantalor93/dnspyre/v3/pkg/printutils"
	"github.com/tantalor93/dnspyre/v3/pkg/reporter"
	"github.com/tantalor93/dnspyre/v3/pkg/scoring"
)

var (
	// Version is set during release of project during build process.
	Version string

	author = "Ondrej Benkovsky <obenky@gmail.com>"
)

var (
	pApp = kingpin.New("dnspyre", "A high QPS DNS benchmark.").Author(author)

	// Main benchmark command (default)
	benchmarkCmd = pApp.Command("benchmark", "Run DNS benchmark").Default()

	// Frontend command
	frontendCmd  = pApp.Command("frontend", "Start web frontend for DNS benchmark result analysis")
	frontendPort = frontendCmd.Flag("port", "Port to run the frontend server on").
			Short('p').Default("8080").String()
	frontendHost = frontendCmd.Flag("host", "Host to bind the frontend server to").
			Default("localhost").String()
	frontendOpen = frontendCmd.Flag("open", "Automatically open browser").
			Default("true").Bool()
	frontendFile = frontendCmd.Flag("file", "Preload JSON data file").
			Short('f').String()

	benchmark = dnsbench.Benchmark{
		Writer: os.Stdout,
	}

	failConditions []string
)

const (
	ioerrorFailCondition    = "ioerror"
	negativeFailCondition   = "negative"
	errorFailCondition      = "error"
	idmismatchFailCondition = "idmismatch"
)

func init() {
	benchmarkCmd.Flag("server", "Server represents (plain DNS, DoT, DoH or DoQ) server, which will be benchmarked. "+
		"Format depends on the DNS protocol, that should be used for DNS benchmark. "+
		"For plain DNS (either over UDP or TCP) the format is <IP/host>[:port], if port is not provided then port 53 is used. "+
		"For DoT the format is <IP/host>[:port], if port is not provided then port 853 is used. "+
		"For DoH the format is https://<IP/host>[:port][/path] or http://<IP/host>[:port][/path], if port is not provided then either 443 or 80 port is used. If no path is provided, then /dns-query is used. "+
		"For DoQ the format is quic://<IP/host>[:port], if port is not provided then port 853 is used.").Short('s').Default("127.0.0.1").StringVar(&benchmark.Server)

	benchmarkCmd.Flag("type", "Query type. Repeatable flag. If multiple query types are specified then each query will be duplicated for each type.").
		Short('t').Default("A").EnumsVar(&benchmark.Types, getSupportedDNSTypes()...)

	benchmarkCmd.Flag("number", "How many times the provided queries are repeated. Note that the total number of queries issued = types*number*concurrency*len(queries).").
		Short('n').PlaceHolder("1").Int64Var(&benchmark.Count)

	benchmarkCmd.Flag("concurrency", "Number of concurrent queries to issue.").
		Short('c').Default("1").Uint32Var(&benchmark.Concurrency)

	benchmarkCmd.Flag("rate-limit", "Apply a global questions / second rate limit.").
		Short('l').Default("0").IntVar(&benchmark.Rate)

	benchmarkCmd.Flag("rate-limit-worker", "Apply a questions / second rate limit for each concurrent worker specified by --concurrency option.").
		Default("0").IntVar(&benchmark.RateLimitWorker)

	pApp.Flag("query-per-conn", "Queries on a connection before creating a new one. 0: unlimited. Applicable for plain DNS and DoT, this option is not considered for DoH or DoQ.").
		Default("0").Int64Var(&benchmark.QperConn)

	pApp.Flag("recurse", "Allow DNS recursion. Enabled by default.").
		Short('r').Default("true").BoolVar(&benchmark.Recurse)

	pApp.Flag("probability", "Each provided hostname will be used with provided probability. Value 1 and above means that each hostname will be used by each concurrent benchmark goroutine. Useful for randomizing queries across benchmark goroutines.").
		Default("1").Float64Var(&benchmark.Probability)

	pApp.Flag("ednsopt", "code[:value], Specify EDNS option with code point code and optionally payload of value as a hexadecimal string. code must be an arbitrary numeric value.").
		Default("").StringVar(&benchmark.EdnsOpt)

	pApp.Flag("dnssec", "Allow DNSSEC (sets DO bit for all DNS requests to 1)").BoolVar(&benchmark.DNSSEC)

	pApp.Flag("edns0", "Configures EDNS0 usage in DNS requests send by benchmark and configures EDNS0 buffer size to the specified value. When 0 is configured, then EDNS0 is not used.").
		Default("0").Uint16Var(&benchmark.Edns0)

	pApp.Flag("tcp", "Use TCP for DNS requests.").BoolVar(&benchmark.TCP)

	pApp.Flag("dot", "Use DoT (DNS over TLS) for DNS requests.").BoolVar(&benchmark.DOT)

	pApp.Flag("write", "write timeout.").Default(dnsbench.DefaultWriteTimeout.String()).
		DurationVar(&benchmark.WriteTimeout)

	pApp.Flag("read", "read timeout.").Default(dnsbench.DefaultReadTimeout.String()).
		DurationVar(&benchmark.ReadTimeout)

	pApp.Flag("connect", "connect timeout.").Default(dnsbench.DefaultConnectTimeout.String()).
		DurationVar(&benchmark.ConnectTimeout)

	pApp.Flag("request", "request timeout.").Default(dnsbench.DefaultRequestTimeout.String()).
		DurationVar(&benchmark.RequestTimeout)

	pApp.Flag("codes", "Enable counting DNS return codes. Enabled by default.").
		Default("true").BoolVar(&benchmark.Rcodes)

	pApp.Flag("min", "Minimum value for timing histogram.").
		Default((time.Microsecond * 400).String()).DurationVar(&benchmark.HistMin)

	pApp.Flag("max", "Maximum value for timing histogram.").DurationVar(&benchmark.HistMax)

	pApp.Flag("precision", "Significant figure for histogram precision.").
		Default("1").PlaceHolder("[1-5]").IntVar(&benchmark.HistPre)

	pApp.Flag("distribution", "Display distribution histogram of timings to stdout. Enabled by default.").
		Default("true").BoolVar(&benchmark.HistDisplay)

	pApp.Flag("csv", "Export distribution to CSV.").
		Default("").PlaceHolder("/path/to/file.csv").StringVar(&benchmark.Csv)

	pApp.Flag("json", "Report benchmark results as JSON.").BoolVar(&benchmark.JSON)

	pApp.Flag("batch-json", "Generate batch JSON output for multiple servers. Format: server1,server2,server3").
		PlaceHolder("8.8.8.8,1.1.1.1,114.114.114.114").StringVar(&benchmark.BatchJSON)

	pApp.Flag("html", "Path to create HTML report file with embedded benchmark results.").
		PlaceHolder("/path/to/report.html").StringVar(&benchmark.HTML)

	pApp.Flag("silent", "Disable stdout.").BoolVar(&benchmark.Silent)

	pApp.Flag("color", "ANSI Color output. Enabled by default.").
		Default("true").BoolVar(&benchmark.Color)

	pApp.Flag("plot", "Plot benchmark results and export them to the directory.").
		Default("").PlaceHolder("/path/to/folder").StringVar(&benchmark.PlotDir)

	pApp.Flag("plotf", "Format of graphs. Supported formats: svg, png and jpg.").
		Default(dnsbench.DefaultPlotFormat).EnumVar(&benchmark.PlotFormat, "svg", "png", "jpg")

	pApp.Flag("doh-method", "HTTP method to use for DoH requests. Supported values: get, post.").
		Default(dnsbench.PostHTTPMethod).EnumVar(&benchmark.DohMethod, dnsbench.GetHTTPMethod, dnsbench.PostHTTPMethod)

	pApp.Flag("doh-protocol", "HTTP protocol to use for DoH requests. Supported values: 1.1, 2 and 3.").
		Default(dnsbench.HTTP1Proto).EnumVar(&benchmark.DohProtocol, dnsbench.HTTP1Proto, dnsbench.HTTP2Proto, dnsbench.HTTP3Proto)

	pApp.Flag("insecure", "Disables server TLS certificate validation. Applicable for DoT, DoH and DoQ.").
		BoolVar(&benchmark.Insecure)

	pApp.Flag("duration", "Specifies for how long the benchmark should be executing, the benchmark will run for the specified time "+
		"while sending DNS requests in an infinite loop based on the data source. After running for the specified duration, the benchmark is canceled. "+
		"This option is exclusive with --number option. The duration is specified in GO duration format e.g. 10s, 15m, 1h.").
		PlaceHolder("1m").Short('d').DurationVar(&benchmark.Duration)

	pApp.Flag("progress", "Controls whether the progress bar is shown. Enabled by default.").
		Default("true").BoolVar(&benchmark.ProgressBar)

	pApp.Flag("fail", "Controls conditions upon which the dnspyre will exit with a non-zero exit code. Repeatable flag. "+
		"Supported options are 'ioerror' (fail if there is at least 1 IO error), 'negative' (fail if there is at least 1 negative DNS answer), "+
		"'error' (fail if there is at least 1 error DNS response), 'idmismatch' (fail there is at least 1 ID mismatch between DNS request and response).").
		PlaceHolder(ioerrorFailCondition).
		EnumsVar(&failConditions, ioerrorFailCondition, negativeFailCondition, errorFailCondition, idmismatchFailCondition)

	pApp.Flag("log-requests", "Controls whether the Benchmark requests are logged. Requests are logged into the file specified by --log-requests-path flag. Disabled by default.").
		BoolVar(&benchmark.RequestLogEnabled)

	pApp.Flag("log-requests-path", "Specifies path to the file, where the request logs will be logged. If the file exists, the logs will be appended to the file. "+
		"If the file does not exist, the file will be created.").
		Default(dnsbench.DefaultRequestLogPath).StringVar(&benchmark.RequestLogPath)

	pApp.Flag("separate-worker-connections", "Controls whether the concurrent workers will try to share connections to the server or not. When enabled "+
		"the workers will use separate connections. Disabled by default.").
		BoolVar(&benchmark.SeparateWorkerConnections)

	pApp.Flag("request-delay", "Configures delay to be added before each request done by worker. Delay can be either constant or randomized. "+
		"Constant delay is configured as single duration <GO duration> (e.g. 500ms, 2s, etc.). Randomized delay is configured as interval of "+
		"two durations <GO duration>-<GO duration> (e.g. 1s-2s, 500ms-2s, etc.), where the actual delay is random value from the interval that "+
		"is randomized after each request.").Default("0s").StringVar(&benchmark.RequestDelay)

	pApp.Flag("prometheus", "Enables Prometheus metrics endpoint on the specified address. For example :8080 or localhost:8080. The endpoint is available at /metrics path.").
		PlaceHolder(":8080").StringVar(&benchmark.PrometheusMetricsAddr)

	benchmarkCmd.Arg("queries", "Queries to issue. It can be a local file referenced using @<file-path>, for example @data/2-domains. "+
		"It can also be resource accessible using HTTP, like https://raw.githubusercontent.com/Tantalor93/dnspyre/master/data/1000-domains, in that "+
		"case, the file will be downloaded and saved in-memory. "+
		"These data sources can be combined, for example \"google.com @data/2-domains https://raw.githubusercontent.com/Tantalor93/dnspyre/master/data/2-domains\"").
		Required().StringsVar(&benchmark.Queries)

	info, ok := debug.ReadBuildInfo()
	if ok && len(Version) == 0 {
		Version = info.Main.Version
	}
}

// Execute starts main logic of command.
func Execute() {
	pApp.Version(Version)
	parsed := kingpin.MustParse(pApp.Parse(os.Args[1:]))

	// Handle frontend command
	if parsed == frontendCmd.FullCommand() {
		config := FrontendConfig{
			Port:        *frontendPort,
			Host:        *frontendHost,
			OpenBrowser: *frontendOpen,
			PreloadFile: *frontendFile,
		}

		if err := StartFrontendServer(config); err != nil {
			printutils.ErrFprintf(os.Stderr, "Frontend server error: %s\n", err.Error())
			os.Exit(1)
		}
		return
	}

	// Handle benchmark command (default behavior)

	// Check if batch JSON is requested
	if len(benchmark.BatchJSON) > 0 {
		if err := runBatchBenchmark(benchmark.BatchJSON); err != nil {
			printutils.ErrFprintf(os.Stderr, "Batch benchmark error: %s\n", err.Error())
			os.Exit(1)
		}
		return
	}

	sigsInt := make(chan os.Signal, 8)
	signal.Notify(sigsInt, syscall.SIGINT)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_, ok := <-sigsInt
		if !ok {
			// standard exit based on channel close
			return
		}
		cancel()

		<-sigsInt

		close(sigsInt)
		os.Exit(1)
	}()

	start := time.Now()
	res, err := benchmark.Run(ctx)
	end := time.Now()

	if err != nil {
		printutils.ErrFprintf(os.Stderr, "There was an error while starting benchmark: %s\n", err.Error())
		close(sigsInt)
		os.Exit(1)
	}

	if err := reporter.PrintReport(&benchmark, res, start, end.Sub(start), getServerGeocode(benchmark.Server)); err != nil {
		printutils.ErrFprintf(os.Stderr, "There was an error while printing report: %s\n", err.Error())
		close(sigsInt)
		os.Exit(1)
	}

	// Handle HTML output if specified
	if benchmark.HTML != "" {
		stats := reporter.Merge(&benchmark, res)
		jsonData, err := generateJSONForHTML(&stats, &benchmark, end.Sub(start))
		if err != nil {
			printutils.ErrFprintf(os.Stderr, "Failed to generate JSON for HTML output: %s\n", err.Error())
		} else if err := OutputHTML(benchmark.HTML, jsonData); err != nil {
			printutils.ErrFprintf(os.Stderr, "Failed to generate HTML output: %s\n", err.Error())
		}
	}

	close(sigsInt)

	if len(failConditions) > 0 {
		stats := reporter.Merge(&benchmark, res)
		for _, f := range failConditions {
			switch f {
			case ioerrorFailCondition:
				if stats.Counters.IOError > 0 {
					os.Exit(1)
				}
			case negativeFailCondition:
				if stats.Counters.Negative > 0 {
					os.Exit(1)
				}
			case errorFailCondition:
				if stats.Counters.Error > 0 {
					os.Exit(1)
				}
			case idmismatchFailCondition:
				if stats.Counters.IDmismatch > 0 {
					os.Exit(1)
				}
			}
		}
	}
}

func getSupportedDNSTypes() []string {
	keys := make([]string, 0, len(dns.StringToType))
	for k := range dns.StringToType {
		keys = append(keys, k)
	}
	return keys
}

// generateJSONForHTML creates JSON data suitable for HTML visualization
func generateJSONForHTML(stats *reporter.BenchmarkResultStats, b *dnsbench.Benchmark, benchDuration time.Duration) (string, error) {
	// Create a structure similar to jsonResult but with access to internal data
	codeTotalsMapped := make(map[string]int64)
	if b.Rcodes {
		for rcode, total := range stats.Codes {
			codeTotalsMapped[dns.RcodeToString[rcode]] = total
		}
	}

	// Create histogram points
	var histogramPoints []map[string]interface{}
	if b.HistDisplay {
		res := make([]map[string]interface{}, 0)
		for _, bar := range stats.Hist.Distribution() {
			if bar.Count > 0 {
				res = append(res, map[string]interface{}{
					"latencyMs": time.Duration(bar.To).Milliseconds(),
					"count":     bar.Count,
				})
			}
		}
		histogramPoints = res
	}

	// Calculate performance score
	metrics := scoring.BenchmarkMetrics{
		TotalRequests:         stats.Counters.Total,
		TotalSuccessResponses: stats.Counters.Success,
		TotalErrorResponses:   stats.Counters.Error,
		TotalIOErrors:         stats.Counters.IOError,
		QueriesPerSecond:      math.Round(float64(stats.Counters.Total)/benchDuration.Seconds()*100) / 100,
		LatencyStats: scoring.LatencyMetrics{
			MeanMs: time.Duration(stats.Hist.Mean()).Milliseconds(),
			StdMs:  time.Duration(stats.Hist.StdDev()).Milliseconds(),
			P50Ms:  time.Duration(stats.Hist.ValueAtQuantile(50)).Milliseconds(),
			P95Ms:  time.Duration(stats.Hist.ValueAtQuantile(95)).Milliseconds(),
		},
	}
	scoreResult := scoring.CalculateScore(metrics)

	serverResult := map[string]interface{}{
		"totalRequests":            stats.Counters.Total,
		"totalSuccessResponses":    stats.Counters.Success,
		"totalNegativeResponses":   stats.Counters.Negative,
		"totalErrorResponses":      stats.Counters.Error,
		"totalIOErrors":            stats.Counters.IOError,
		"totalIDmismatch":          stats.Counters.IDmismatch,
		"totalTruncatedResponses":  stats.Counters.Truncated,
		"queriesPerSecond":         math.Round(float64(stats.Counters.Total)/benchDuration.Seconds()*100) / 100,
		"benchmarkDurationSeconds": benchDuration.Seconds(),
		"responseRcodes":           codeTotalsMapped,
		"questionTypes":            stats.Qtypes,
		"score":                    scoreResult,
		"geocode":                  getServerGeocode(benchmark.Server),
		"ip":                       extractIPFromServer(benchmark.Server),
		"latencyStats": map[string]interface{}{
			"minMs":  time.Duration(stats.Hist.Min()).Milliseconds(),
			"meanMs": time.Duration(stats.Hist.Mean()).Milliseconds(),
			"stdMs":  time.Duration(stats.Hist.StdDev()).Milliseconds(),
			"maxMs":  time.Duration(stats.Hist.Max()).Milliseconds(),
			"p99Ms":  time.Duration(stats.Hist.ValueAtQuantile(99)).Milliseconds(),
			"p95Ms":  time.Duration(stats.Hist.ValueAtQuantile(95)).Milliseconds(),
			"p90Ms":  time.Duration(stats.Hist.ValueAtQuantile(90)).Milliseconds(),
			"p75Ms":  time.Duration(stats.Hist.ValueAtQuantile(75)).Milliseconds(),
			"p50Ms":  time.Duration(stats.Hist.ValueAtQuantile(50)).Milliseconds(),
		},
		"latencyDistribution":        histogramPoints,
		"dohHTTPResponseStatusCodes": stats.DoHStatusCodes,
	}

	if b.DNSSEC {
		totalDNSSECSecuredDomains := len(stats.AuthenticatedDomains)
		serverResult["totalDNSSECSecuredDomains"] = &totalDNSSECSecuredDomains
	}

	// Wrap in multi-server format
	result := map[string]interface{}{
		benchmark.Server: serverResult,
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// extractIPFromServer extracts IP address from server string
func extractIPFromServer(server string) string {
	// Simple extraction - if it's an IP address, return it
	// If it's a hostname with protocol, try to extract the hostname part
	// This is a basic implementation that could be enhanced
	if server == "" {
		return ""
	}

	// Handle DoH URLs
	if len(server) > 8 && server[:8] == "https://" {
		return server[8:] // Return everything after https://
	}
	// Handle DoT URLs
	if len(server) > 6 && server[:6] == "tls://" {
		return server[6:] // Return everything after tls://
	}
	// Handle DoQ URLs
	if len(server) > 7 && server[:7] == "quic://" {
		return server[7:] // Return everything after quic://
	}

	// For plain DNS (IP:port or just IP), return as is
	return server
}

// OutputHTML creates a standalone HTML file with benchmark results
func OutputHTML(outputPath string, resultString string) error {
	htmlFilePath := outputPath
	if !strings.HasSuffix(htmlFilePath, ".html") {
		htmlFilePath = strings.TrimSuffix(htmlFilePath, ".json") + ".html"
	}

	htmlFile, err := os.Create(htmlFilePath)
	if err != nil {
		return fmt.Errorf("failed to create HTML file: %v", err)
	}
	defer htmlFile.Close()

	// Create a single server report HTML template
	singleServerTemplate := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>DNS 服务器性能报告</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            margin: 0;
            padding: 20px;
            background-color: #f5f5f7;
            color: #1d1d1f;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background-color: white;
            padding: 30px;
            border-radius: 12px;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
        }
        h1 {
            text-align: center;
            color: #1d1d1f;
            margin-bottom: 30px;
            font-weight: 600;
        }
        .metrics-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        .metric-card {
            padding: 20px;
            border-radius: 10px;
            color: white;
            text-align: center;
        }
        .metric-title {
            font-size: 14px;
            opacity: 0.9;
            margin-bottom: 8px;
        }
        .metric-value {
            font-size: 24px;
            font-weight: bold;
            margin-bottom: 5px;
        }
        .metric-subtitle {
            font-size: 12px;
            opacity: 0.8;
        }
        .score-section {
            background: linear-gradient(135deg, #11998e 0%, #38ef7d 100%);
        }
        .latency-section {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
        }
        .success-section {
            background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%);
        }
        .qps-section {
            background: linear-gradient(135deg, #4facfe 0%, #00f2fe 100%);
        }
        .chart-container {
            margin: 30px 0;
            height: 400px;
        }
        .details-table {
            width: 100%;
            border-collapse: collapse;
            margin-top: 20px;
        }
        .details-table th,
        .details-table td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        .details-table th {
            background-color: #f8f9fa;
            font-weight: 600;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>DNS 服务器性能报告</h1>
        
        <div class="metrics-grid">
            <div class="metric-card score-section">
                <div class="metric-title">综合评分</div>
                <div class="metric-value" id="totalScore">-</div>
                <div class="metric-subtitle">总分 (满分100)</div>
            </div>
            <div class="metric-card latency-section">
                <div class="metric-title">平均延迟</div>
                <div class="metric-value" id="avgLatency">-</div>
                <div class="metric-subtitle">毫秒 (ms)</div>
            </div>
            <div class="metric-card success-section">
                <div class="metric-title">成功率</div>
                <div class="metric-value" id="successRate">-</div>
                <div class="metric-subtitle">百分比 (%)</div>
            </div>
            <div class="metric-card qps-section">
                <div class="metric-title">查询速率</div>
                <div class="metric-value" id="qpsValue">-</div>
                <div class="metric-subtitle">查询/秒 (QPS)</div>
            </div>
        </div>

        <div class="chart-container">
            <canvas id="scoreChart"></canvas>
        </div>

        <div class="chart-container">
            <canvas id="latencyChart"></canvas>
        </div>

        <table class="details-table">
            <thead>
                <tr>
                    <th>指标</th>
                    <th>数值</th>
                    <th>说明</th>
                </tr>
            </thead>
            <tbody id="detailsTableBody">
            </tbody>
        </table>
    </div>

    <script>
        const data = __JSON_DATA_PLACEHOLDER__;
        
        // Update metric cards
        document.getElementById('totalScore').textContent = data.score.total.toFixed(1);
        document.getElementById('avgLatency').textContent = data.latencyStats.meanMs;
        document.getElementById('successRate').textContent = ((data.totalSuccessResponses / data.totalRequests) * 100).toFixed(2);
        document.getElementById('qpsValue').textContent = data.queriesPerSecond.toFixed(1);

        // Create score breakdown chart
        const scoreCtx = document.getElementById('scoreChart').getContext('2d');
        new Chart(scoreCtx, {
            type: 'radar',
            data: {
                labels: ['成功率', '延迟', '错误率', 'QPS'],
                datasets: [{
                    label: '性能评分',
                    data: [data.score.successRate, data.score.latency, data.score.errorRate, data.score.qps],
                    fill: true,
                    backgroundColor: 'rgba(54, 162, 235, 0.2)',
                    borderColor: 'rgba(54, 162, 235, 1)',
                    pointBackgroundColor: 'rgba(54, 162, 235, 1)',
                    pointBorderColor: '#fff',
                    pointHoverBackgroundColor: '#fff',
                    pointHoverBorderColor: 'rgba(54, 162, 235, 1)'
                }]
            },
            options: {
                responsive: true,
                scales: {
                    r: {
                        beginAtZero: true,
                        max: 100,
                        ticks: {
                            stepSize: 20
                        }
                    }
                }
            }
        });

        // Create latency distribution chart if data is available
        if (data.latencyDistribution && data.latencyDistribution.length > 0) {
            const latencyCtx = document.getElementById('latencyChart').getContext('2d');
            const latencyLabels = data.latencyDistribution.map(item => item.latencyMs + 'ms');
            const latencyCounts = data.latencyDistribution.map(item => item.count);
            
            new Chart(latencyCtx, {
                type: 'bar',
                data: {
                    labels: latencyLabels,
                    datasets: [{
                        label: '请求数量',
                        data: latencyCounts,
                        backgroundColor: 'rgba(75, 192, 192, 0.6)',
                        borderColor: 'rgba(75, 192, 192, 1)',
                        borderWidth: 1
                    }]
                },
                options: {
                    responsive: true,
                    plugins: {
                        title: {
                            display: true,
                            text: '延迟分布图'
                        }
                    },
                    scales: {
                        y: {
                            beginAtZero: true,
                            title: {
                                display: true,
                                text: '请求数量'
                            }
                        },
                        x: {
                            title: {
                                display: true,
                                text: '延迟 (ms)'
                            }
                        }
                    }
                }
            });
        }

        // Populate details table
        const detailsTableBody = document.getElementById('detailsTableBody');
        const details = [
            ['总请求数', data.totalRequests, '测试期间发送的总DNS查询数'],
            ['成功响应', data.totalSuccessResponses, '成功解析的DNS查询数'],
            ['错误响应', data.totalErrorResponses, 'DNS服务器返回错误的查询数'],
            ['IO错误', data.totalIOErrors, '网络IO错误的查询数'],
            ['测试时长', data.benchmarkDurationSeconds.toFixed(2) + '秒', '基准测试持续时间'],
            ['平均延迟', data.latencyStats.meanMs + 'ms', '所有查询的平均响应时间'],
            ['P50延迟', data.latencyStats.p50Ms + 'ms', '50%查询的响应时间在此值以下'],
            ['P95延迟', data.latencyStats.p95Ms + 'ms', '95%查询的响应时间在此值以下'],
            ['P99延迟', data.latencyStats.p99Ms + 'ms', '99%查询的响应时间在此值以下']
        ];

        details.forEach(([metric, value, description]) => {
            const row = detailsTableBody.insertRow();
            row.insertCell(0).textContent = metric;
            row.insertCell(1).textContent = value;
            row.insertCell(2).textContent = description;
        });
    </script>
</body>
</html>`

	htmlTemplate := strings.Replace(singleServerTemplate, "__JSON_DATA_PLACEHOLDER__", resultString, 1)

	_, err = htmlFile.WriteString(htmlTemplate)
	if err != nil {
		return fmt.Errorf("failed to write HTML file: %v", err)
	}

	log.Printf("HTML output written to: %s", htmlFilePath)
	return nil
}

// getServerGeocode returns the geocode for a DNS server based on IP address
func getServerGeocode(server string) string {
	// Try to use geo service first
	geoService, err := geo.NewGeoService()
	if err == nil && geoService != nil {
		defer geoService.Close()
		_, geoCode, err := geoService.CheckGeo(server, true)
		if err == nil {
			return geoCode
		}
	}

	// Fallback for unknown locations
	return "XX"
}

// runBatchBenchmark runs benchmark on multiple servers and generates batch JSON output
func runBatchBenchmark(serverList string) error {
	servers := strings.Split(serverList, ",")
	if len(servers) == 0 {
		return fmt.Errorf("no servers provided for batch benchmark")
	}

	// Output progress to stderr instead of stdout to avoid polluting JSON
	fmt.Fprintf(os.Stderr, "Starting batch benchmark for %d servers...\n", len(servers))

	batchResults := make(map[string]interface{})

	for _, server := range servers {
		server = strings.TrimSpace(server)
		if server == "" {
			continue
		}

		fmt.Fprintf(os.Stderr, "Testing server: %s\n", server)

		// Create a copy of the global benchmark config for this server
		serverBenchmark := benchmark
		serverBenchmark.Server = server
		serverBenchmark.JSON = true   // Force JSON output
		serverBenchmark.Silent = true // Suppress normal output

		// Run benchmark for this server
		ctx := context.Background()
		start := time.Now()
		res, err := serverBenchmark.Run(ctx)
		end := time.Now()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error testing server %s: %v\n", server, err)
			continue
		}

		// Generate JSON result for this server
		jsonData, err := generateJSONForServer(&serverBenchmark, res, start, end.Sub(start), server)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating JSON for server %s: %v\n", server, err)
			continue
		}

		// Parse the JSON - now it's already in multi-server format
		var multiServerResult map[string]interface{}
		if err := json.Unmarshal(jsonData, &multiServerResult); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing JSON for server %s: %v\n", server, err)
			continue
		}

		// Extract the server result from the multi-server format
		if serverResult, exists := multiServerResult[server]; exists {
			batchResults[server] = serverResult
		} else {
			// Fallback: take the first (and should be only) result
			for _, result := range multiServerResult {
				batchResults[server] = result
				break
			}
		}
		fmt.Fprintf(os.Stderr, "Completed testing server: %s\n", server)
	}

	// Output batch results as JSON to stdout
	batchJSON, err := json.MarshalIndent(batchResults, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal batch results: %v", err)
	}

	fmt.Println(string(batchJSON))
	return nil
}

// generateJSONForServer generates JSON output for a single server benchmark result
func generateJSONForServer(bench *dnsbench.Benchmark, res []*dnsbench.ResultStats, start time.Time, duration time.Duration, server string) ([]byte, error) {
	// Use a buffer to capture the JSON output
	var buf bytes.Buffer
	originalWriter := bench.Writer
	originalSilent := bench.Silent

	bench.Writer = &buf
	bench.Silent = false // Ensure output is generated even in batch mode

	// Generate the report which will write JSON to our buffer
	geocode := getServerGeocode(server)
	if err := reporter.PrintReport(bench, res, start, duration, geocode); err != nil {
		bench.Writer = originalWriter // Restore original writer
		bench.Silent = originalSilent // Restore original silent flag
		return nil, err
	}

	bench.Writer = originalWriter // Restore original writer
	bench.Silent = originalSilent // Restore original silent flag
	return buf.Bytes(), nil
}

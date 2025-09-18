package reporter

import (
	"encoding/json"
	"math"
	"time"

	"github.com/miekg/dns"
	"github.com/tantalor93/dnspyre/v3/pkg/scoring"
)

type jsonReporter struct{}

type latencyStats struct {
	MinMs  int64 `json:"minMs"`
	MeanMs int64 `json:"meanMs"`
	StdMs  int64 `json:"stdMs"`
	MaxMs  int64 `json:"maxMs"`
	P99Ms  int64 `json:"p99Ms"`
	P95Ms  int64 `json:"p95Ms"`
	P90Ms  int64 `json:"p90Ms"`
	P75Ms  int64 `json:"p75Ms"`
	P50Ms  int64 `json:"p50Ms"`
}

type histogramPoint struct {
	LatencyMs int64 `json:"latencyMs"`
	Count     int64 `json:"count"`
}

type jsonResult struct {
	TotalRequests              int64                `json:"totalRequests"`
	TotalSuccessResponses      int64                `json:"totalSuccessResponses"`
	TotalNegativeResponses     int64                `json:"totalNegativeResponses"`
	TotalErrorResponses        int64                `json:"totalErrorResponses"`
	TotalIOErrors              int64                `json:"totalIOErrors"`
	TotalIDmismatch            int64                `json:"totalIDmismatch"`
	TotalTruncatedResponses    int64                `json:"totalTruncatedResponses"`
	ResponseRcodes             map[string]int64     `json:"responseRcodes,omitempty"`
	QuestionTypes              map[string]int64     `json:"questionTypes"`
	QueriesPerSecond           float64              `json:"queriesPerSecond"`
	BenchmarkDurationSeconds   float64              `json:"benchmarkDurationSeconds"`
	LatencyStats               latencyStats         `json:"latencyStats"`
	LatencyDistribution        []histogramPoint     `json:"latencyDistribution,omitempty"`
	TotalDNSSECSecuredDomains  *int                 `json:"totalDNSSECSecuredDomains,omitempty"`
	DohHTTPResponseStatusCodes map[int]int64        `json:"dohHTTPResponseStatusCodes,omitempty"`
	Geocode                    string               `json:"geocode,omitempty"`
	IP                         string               `json:"ip,omitempty"`
	Score                      *scoring.ScoreResult `json:"score,omitempty"`
}

// multiServerResult wraps single server results in the format expected by frontend
type multiServerResult map[string]jsonResult

func (s *jsonReporter) print(params reportParameters) error {
	result := s.buildSingleResult(params)

	// Calculate score
	score := s.calculateScore(params)
	if score != nil {
		result.Score = score
	}

	// Extract IP from server address if possible
	result.IP = extractIPFromServer(params.benchmark.Server)

	// Wrap single server result in multi-server format expected by frontend
	multiResult := multiServerResult{
		params.benchmark.Server: result,
	}

	return json.NewEncoder(params.outputWriter).Encode(multiResult)
}

func (s *jsonReporter) buildSingleResult(params reportParameters) jsonResult {
	codeTotalsMapped := make(map[string]int64)
	if params.benchmark.Rcodes {
		for k, v := range params.codeTotals {
			codeTotalsMapped[dns.RcodeToString[k]] = v
		}
	}

	var res []histogramPoint

	if params.benchmark.HistDisplay {
		dist := params.hist.Distribution()
		for _, d := range dist {
			res = append(res, histogramPoint{
				LatencyMs: roundDuration(time.Duration(d.To/2 + d.From/2)).Milliseconds(),
				Count:     d.Count,
			})
		}

		var dedupRes []histogramPoint
		i := -1
		for _, r := range res {
			if i >= 0 {
				if dedupRes[i].LatencyMs == r.LatencyMs {
					dedupRes[i].Count += r.Count
				} else {
					dedupRes = append(dedupRes, r)
					i++
				}
			} else {
				dedupRes = append(dedupRes, r)
				i++
			}
		}
		res = dedupRes
	}

	result := jsonResult{
		TotalRequests:            params.totalCounters.Total,
		TotalSuccessResponses:    params.totalCounters.Success,
		TotalNegativeResponses:   params.totalCounters.Negative,
		TotalErrorResponses:      params.totalCounters.Error,
		TotalIOErrors:            params.totalCounters.IOError,
		TotalIDmismatch:          params.totalCounters.IDmismatch,
		TotalTruncatedResponses:  params.totalCounters.Truncated,
		QueriesPerSecond:         math.Round(float64(params.totalCounters.Total)/params.benchmarkDuration.Seconds()*100) / 100,
		BenchmarkDurationSeconds: roundDuration(params.benchmarkDuration).Seconds(),
		ResponseRcodes:           codeTotalsMapped,
		QuestionTypes:            params.qtypeTotals,
		LatencyStats: latencyStats{
			MinMs:  roundDuration(time.Duration(params.hist.Min())).Milliseconds(),
			MeanMs: roundDuration(time.Duration(params.hist.Mean())).Milliseconds(),
			StdMs:  roundDuration(time.Duration(params.hist.StdDev())).Milliseconds(),
			MaxMs:  roundDuration(time.Duration(params.hist.Max())).Milliseconds(),
			P99Ms:  roundDuration(time.Duration(params.hist.ValueAtQuantile(99))).Milliseconds(),
			P95Ms:  roundDuration(time.Duration(params.hist.ValueAtQuantile(95))).Milliseconds(),
			P90Ms:  roundDuration(time.Duration(params.hist.ValueAtQuantile(90))).Milliseconds(),
			P75Ms:  roundDuration(time.Duration(params.hist.ValueAtQuantile(75))).Milliseconds(),
			P50Ms:  roundDuration(time.Duration(params.hist.ValueAtQuantile(50))).Milliseconds(),
		},
		LatencyDistribution:        res,
		DohHTTPResponseStatusCodes: params.dohResponseStatusesTotals,
		Geocode:                    params.geocode,
	}

	if params.benchmark.DNSSEC {
		totalDNSSECSecuredDomains := len(params.authenticatedDomains)
		result.TotalDNSSECSecuredDomains = &totalDNSSECSecuredDomains
	}

	return result
}

func (s *jsonReporter) calculateScore(params reportParameters) *scoring.ScoreResult {
	// Build metrics for scoring
	metrics := scoring.BenchmarkMetrics{
		TotalRequests:         params.totalCounters.Total,
		TotalSuccessResponses: params.totalCounters.Success,
		TotalErrorResponses:   params.totalCounters.Error,
		TotalIOErrors:         params.totalCounters.IOError,
		QueriesPerSecond:      math.Round(float64(params.totalCounters.Total)/params.benchmarkDuration.Seconds()*100) / 100,
		LatencyStats: scoring.LatencyMetrics{
			MeanMs: roundDuration(time.Duration(params.hist.Mean())).Milliseconds(),
			StdMs:  roundDuration(time.Duration(params.hist.StdDev())).Milliseconds(),
			P50Ms:  roundDuration(time.Duration(params.hist.ValueAtQuantile(50))).Milliseconds(),
			P95Ms:  roundDuration(time.Duration(params.hist.ValueAtQuantile(95))).Milliseconds(),
		},
	}

	// Calculate and return score
	score := scoring.CalculateScore(metrics)
	return &score
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

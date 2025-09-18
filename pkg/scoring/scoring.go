// Package scoring provides DNS server performance scoring and ranking functionality.
package scoring

import (
	"math"
)

// ScoreResult represents the scoring breakdown for a DNS server
type ScoreResult struct {
	Total       float64 `json:"total"`
	SuccessRate float64 `json:"successRate"`
	ErrorRate   float64 `json:"errorRate"`
	Latency     float64 `json:"latency"`
	QPS         float64 `json:"qps"`
}

// Scoring configuration constants
const (
	SuccessRateScoreWeight = 35.0
	ErrorRateScoreWeight   = 10.0
	LatencyScoreWeight     = 50.0
	QPSScoreWeight         = 5.0

	LatencyRangeMax      = 1000.0 // Above this latency gets 0 points
	LatencyRangeMin      = 0.1    // Below this latency gets 0 points
	LatencyFullMarkPoint = 50.0   // Below this latency gets full points
	MaxQPS               = 100.0  // This QPS gets full points
)

// BenchmarkMetrics represents the metrics needed for scoring
type BenchmarkMetrics struct {
	TotalRequests         int64
	TotalSuccessResponses int64
	TotalErrorResponses   int64
	TotalIOErrors         int64
	QueriesPerSecond      float64
	LatencyStats          LatencyMetrics
}

// LatencyMetrics represents latency statistics
type LatencyMetrics struct {
	MeanMs int64
	StdMs  int64
	P50Ms  int64
	P95Ms  int64
}

// CalculateScore computes the performance score for a DNS server based on benchmark results
func CalculateScore(metrics BenchmarkMetrics) ScoreResult {
	// Check if there are no successful responses
	if metrics.TotalSuccessResponses == 0 {
		return ScoreResult{} // Return zero scores
	}

	// Calculate success rate: ratio of successful responses to total requests
	successRate := float64(metrics.TotalSuccessResponses) / float64(metrics.TotalRequests)
	successRateScore := successRate * 100

	// Calculate error rate: ratio of errors and IO errors to total requests
	errorRate := float64(metrics.TotalErrorResponses+metrics.TotalIOErrors) / float64(metrics.TotalRequests)
	errorRateScore := 100 * (1 - errorRate)
	errorRateScore = math.Max(0, math.Min(100, errorRateScore))

	// Calculate latency score: considers both mean and median for stability
	var latencyScore float64
	meanMs := float64((metrics.LatencyStats.MeanMs + metrics.LatencyStats.P50Ms) / 2)

	if meanMs < LatencyRangeMin {
		// Very low latency gets high score, but not perfect to account for measurement accuracy
		latencyScore = 95.0
	} else if meanMs > LatencyRangeMax {
		// Very high latency gets 0 points
		latencyScore = 0
	} else {
		// Linear scoring between min and max thresholds
		latencyScore = 100 * (1 - (meanMs-LatencyRangeMin)/(LatencyRangeMax-LatencyRangeMin))
		latencyScore = math.Max(0, math.Min(100, latencyScore))

		// Penalize for high standard deviation (instability) only if we have meaningful data
		if meanMs > 0 {
			stdPenalty := float64(metrics.LatencyStats.StdMs) / meanMs * 5 // Reduced penalty
			latencyScore = math.Max(0, latencyScore-stdPenalty)
		}
	} // Further penalize if P95 latency is very high
	if metrics.LatencyStats.P95Ms > int64(LatencyRangeMax) {
		latencyScore *= 0.7 // Reduce score for instability
	}

	// Calculate QPS score using logarithmic function
	qpsScore := 100 * math.Log(1+metrics.QueriesPerSecond) / math.Log(1+MaxQPS)
	qpsScore = math.Min(100, qpsScore)

	// Calculate total score based on weights
	totalScore := (successRateScore*SuccessRateScoreWeight +
		errorRateScore*ErrorRateScoreWeight +
		latencyScore*LatencyScoreWeight +
		qpsScore*QPSScoreWeight) / 100

	return ScoreResult{
		Total:       totalScore,
		SuccessRate: successRateScore,
		ErrorRate:   errorRateScore,
		Latency:     latencyScore,
		QPS:         qpsScore,
	}
}

// RankServers sorts DNS servers by their total score in descending order
func RankServers(servers map[string]ScoreResult) []ServerRank {
	var rankings []ServerRank

	for server, score := range servers {
		rankings = append(rankings, ServerRank{
			Server: server,
			Score:  score,
		})
	}

	// Sort by total score in descending order
	for i := 0; i < len(rankings); i++ {
		for j := i + 1; j < len(rankings); j++ {
			if rankings[i].Score.Total < rankings[j].Score.Total {
				rankings[i], rankings[j] = rankings[j], rankings[i]
			}
		}
	}

	return rankings
}

// ServerRank represents a server and its score for ranking
type ServerRank struct {
	Server string      `json:"server"`
	Score  ScoreResult `json:"score"`
}

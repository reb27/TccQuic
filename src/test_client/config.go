package test_client

import (
	"fmt"
	"os"
	"strconv"
)

const (
	defaultPipeline               = false
	defaultFOVTracePath           = "data/user_fov.csv"
	defaultFOVTraceFPS            = 30
	defaultSegmentDurationSeconds = 1
	defaultABRMode                = "bola"
)

type EnvironmentConfig struct {
	Pipeline                     bool
	FOVTracePath                 string
	FOVTraceFPS                  int
	StatisticsPath               string
	SummaryPath                  string
	FOVDeliveryPath              string
	FOVGoodputPath               string
	DeadlineLatenessPath         string
	ClientQUICUplinkLossRatePath string
	ABRMode                      string
}

func ResolveEnvironmentConfig() EnvironmentConfig {
	cfg := EnvironmentConfig{
		Pipeline:     defaultPipeline,
		FOVTracePath: getEnvOrDefault("FOV_TRACE_PATH", defaultFOVTracePath),
		FOVTraceFPS:  getEnvInt("FOV_TRACE_FPS", defaultFOVTraceFPS),
		ABRMode:      getEnvOrDefault("ABR_MODE", defaultABRMode),
	}

	pid := os.Getpid()
	cfg.StatisticsPath = fmt.Sprintf("statistics-%d.csv", pid)
	cfg.SummaryPath = fmt.Sprintf("statistics-summary-%d.csv", pid)
	cfg.FOVDeliveryPath = fmt.Sprintf("fov-delivery-%d.csv", pid)
	cfg.FOVGoodputPath = fmt.Sprintf("fov-goodput-%d.csv", pid)
	cfg.DeadlineLatenessPath = fmt.Sprintf("deadline-lateness-%d.csv", pid)
	cfg.ClientQUICUplinkLossRatePath = fmt.Sprintf("client-quic-uplink-loss-rate-%d.csv", pid)

	return cfg
}

func getEnvOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(val)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

// SegmentLimitFromEnv returns TEST_CLIENT_SEGMENT_LIMIT if set to a positive
// integer, otherwise 0 (no limit). Used to shorten proof / CI runs.
func SegmentLimitFromEnv() int {
	val := os.Getenv("TEST_CLIENT_SEGMENT_LIMIT")
	if val == "" {
		return 0
	}
	parsed, err := strconv.Atoi(val)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

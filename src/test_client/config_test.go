package test_client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveEnvironmentConfigReadsBOLAQmaxSegments(t *testing.T) {
	t.Setenv("BOLA_QMAX_SEGMENTS", "6")

	cfg := ResolveEnvironmentConfig()

	require.Equal(t, 6, cfg.BOLAQmaxSegments)
}

func TestResolveEnvironmentConfigInvalidBOLAQmaxSegmentsUsesDefault(t *testing.T) {
	t.Setenv("BOLA_QMAX_SEGMENTS", "0")

	cfg := ResolveEnvironmentConfig()

	require.Equal(t, defaultBOLAQmaxSegments, cfg.BOLAQmaxSegments)
}

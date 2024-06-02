package api

import (
	"github.com/HyperGAI/serving-agent/platform"
	"github.com/HyperGAI/serving-agent/utils"
	"github.com/HyperGAI/serving-agent/worker"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func newTestServer(
	t *testing.T,
	platform platform.Platform,
	distributor worker.TaskDistributor,
	webhook platform.Webhook,
) *Server {
	config := utils.Config{MaxQueueSize: 300}
	server, err := NewServer(config, platform, distributor, webhook)
	require.NoError(t, err)
	return server
}

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

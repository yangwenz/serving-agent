// https://github.com/hibiken/asynq/wiki

package worker

import (
	"context"
	"fmt"
	"github.com/HyperGAI/serving-agent/platform"
	"github.com/HyperGAI/serving-agent/utils"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"time"
)

const (
	QueueCritical = "critical"
	QueueDefault  = "default"
)

type RedisTaskProcessor struct {
	config   utils.Config
	server   *asynq.Server
	platform platform.Platform
	webhook  platform.Webhook
}

func NewRedisTaskProcessor(config utils.Config, platform platform.Platform, webhook platform.Webhook) *RedisTaskProcessor {
	var redisOpt asynq.RedisConnOpt
	if config.RedisClusterMode {
		// Support redis cluster (https://github.com/hibiken/asynq/wiki/Redis-Cluster)
		redisOpt = asynq.RedisClusterClientOpt{
			Addrs: []string{config.RedisAddress},
		}
	} else {
		redisOpt = asynq.RedisClientOpt{
			Addr: config.RedisAddress,
		}
	}
	logger := NewLogger()
	redis.SetLogger(logger)

	server := asynq.NewServer(
		redisOpt,
		asynq.Config{
			Concurrency: config.WorkerConcurrency,
			Queues: map[string]int{
				QueueCritical: 10,
				QueueDefault:  5,
			},
			ShutdownTimeout: time.Duration(config.TaskTimeout) * time.Second,
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				log.Error().Err(err).Str("type", task.Type()).
					Bytes("payload", task.Payload()).Msg("process task failed")
			}),
			Logger: logger,
		},
	)

	return &RedisTaskProcessor{
		config:   config,
		server:   server,
		platform: platform,
		webhook:  webhook,
	}
}

func (processor *RedisTaskProcessor) Start() error {
	mux := asynq.NewServeMux()
	mux.HandleFunc(fmt.Sprintf("task:%s", processor.config.TaskTypeName),
		processor.ProcessTaskRunPrediction)
	return processor.server.Start(mux)
}

func (processor *RedisTaskProcessor) Stop() {
	processor.server.Stop()
}

func (processor *RedisTaskProcessor) Shutdown() {
	processor.server.Shutdown()
}

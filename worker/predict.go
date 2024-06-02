package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/HyperGAI/serving-agent/platform"
	"github.com/hibiken/asynq"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"
	"strconv"
	"strings"
)

type PayloadRunPrediction struct {
	platform.InferRequest
	ID         string `json:"id"`
	APIVersion string `json:"api_version" default:"v1"`
}

var predictFailureCounts = promauto.NewCounter(prometheus.CounterOpts{
	Name: "async_predict_failure_total",
	Help: "Number of async prediction failures",
})

var predictTime = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "async_predict_time",
	Help: "Prediction running time",
})

func (distributor *RedisTaskDistributor) GetTaskQueueInfo(queue string) (*asynq.QueueInfo, error) {
	return distributor.inspector.GetQueueInfo(queue)
}

func (distributor *RedisTaskDistributor) DistributeTaskRunPrediction(
	ctx context.Context,
	payload *PayloadRunPrediction,
	opts ...asynq.Option,
) (string, error) {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal task payload: %w", err)
	}

	task := asynq.NewTask(fmt.Sprintf("task:%s", distributor.config.TaskTypeName),
		jsonPayload, opts...)
	info, err := distributor.client.EnqueueContext(ctx, task)
	if err != nil {
		return "", fmt.Errorf("failed to enqueue task: %w", err)
	}

	log.Info().Str("type", task.Type()).Str("queue", info.Queue).
		Int("max_retry", info.MaxRetry).Msg("enqueued task")
	return info.ID, nil
}

func (processor *RedisTaskProcessor) ProcessTaskRunPrediction(
	ctx context.Context,
	task *asynq.Task,
) error {
	var payload PayloadRunPrediction
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		log.Error().Msgf("failed to unmarshal payload")
		return fmt.Errorf("failed to unmarshal payload: %w", asynq.SkipRetry)
	}

	info := platform.UpdateRequest{ID: payload.ID}
	info.Status = "running"
	if err := processor.webhook.UpdateTaskInfo(&info); err != nil {
		log.Error().Msgf("failed to update task info: %v", err)
		return fmt.Errorf("failed to update task info")
	}

	response, err := processor.platform.Predict(&payload.InferRequest, payload.APIVersion)
	if err != nil {
		log.Error().Msgf("failed to run prediction: %v", err)
		info.Status = "failed"
		info.ErrorInfo = err.Error()
		if err := processor.webhook.UpdateTaskInfo(&info); err != nil {
			log.Error().Msgf("failed to update task info: %v", err)
			return fmt.Errorf("failed to update task info")
		}
		predictFailureCounts.Inc()
		return nil
	}

	info.Status = "succeeded"
	info.Outputs = response.Outputs
	runningTime, ok := response.Outputs["running_time"]
	if ok {
		info.RunningTime = fmt.Sprintf("%v", runningTime)
		delete(response.Outputs, "running_time")
		if info.RunningTime != "" {
			f := strings.Replace(info.RunningTime, "s", "", -1)
			if s, err := strconv.ParseFloat(f, 64); err == nil {
				predictTime.Set(s)
			}
		}
	}
	if err := processor.webhook.UpdateTaskInfo(&info); err != nil {
		log.Error().Msgf("failed to update task info: %v", err)
		return fmt.Errorf("failed to update task info")
	}
	return nil
}

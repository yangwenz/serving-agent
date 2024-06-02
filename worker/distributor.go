package worker

import (
	"context"
	"encoding/json"
	"github.com/HyperGAI/serving-agent/platform"
	"github.com/HyperGAI/serving-agent/utils"
	"github.com/hibiken/asynq"
	"github.com/rs/zerolog/log"
	"slices"
	"time"
)

type TaskDistributor interface {
	DistributeTaskRunPrediction(
		ctx context.Context,
		payload *PayloadRunPrediction,
		opts ...asynq.Option,
	) (string, error)

	GetTaskQueueInfo(
		queue string,
	) (*asynq.QueueInfo, error)

	DeleteTask(
		queue string,
		id string,
	) error

	ListArchivedTasks(
		queue string,
	) ([]*asynq.TaskInfo, error)

	ListScheduledTasks(
		queue string,
	) ([]*asynq.TaskInfo, error)

	ListPendingTasks(
		queue string,
	) ([]*asynq.TaskInfo, error)

	ListRetryTasks(
		queue string,
	) ([]*asynq.TaskInfo, error)

	ListActiveTasks(
		queue string,
	) ([]*asynq.TaskInfo, error)

	ListUnfinishedTasks(
		queue string,
	) ([]*asynq.TaskInfo, error)

	PauseQueue(
		queue string,
	) error

	UnpauseQueue(
		queue string,
	) error
}

type RedisTaskDistributor struct {
	client    *asynq.Client
	inspector *asynq.Inspector
	config    utils.Config
}

func NewRedisTaskDistributor(config utils.Config) TaskDistributor {
	if config.RedisAddress == "" {
		return nil
	}
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
	client := asynq.NewClient(redisOpt)
	inspector := asynq.NewInspector(redisOpt)
	return &RedisTaskDistributor{
		client:    client,
		inspector: inspector,
		config:    config,
	}
}

func (distributor *RedisTaskDistributor) ListArchivedTasks(queue string) ([]*asynq.TaskInfo, error) {
	return distributor.inspector.ListArchivedTasks(queue, asynq.PageSize(distributor.config.MaxQueueSize))
}

func (distributor *RedisTaskDistributor) ListScheduledTasks(queue string) ([]*asynq.TaskInfo, error) {
	return distributor.inspector.ListScheduledTasks(queue, asynq.PageSize(distributor.config.MaxQueueSize))
}

func (distributor *RedisTaskDistributor) ListPendingTasks(queue string) ([]*asynq.TaskInfo, error) {
	return distributor.inspector.ListPendingTasks(queue, asynq.PageSize(distributor.config.MaxQueueSize))
}

func (distributor *RedisTaskDistributor) ListRetryTasks(queue string) ([]*asynq.TaskInfo, error) {
	return distributor.inspector.ListRetryTasks(queue, asynq.PageSize(distributor.config.MaxQueueSize))
}

func (distributor *RedisTaskDistributor) ListActiveTasks(queue string) ([]*asynq.TaskInfo, error) {
	return distributor.inspector.ListActiveTasks(queue, asynq.PageSize(distributor.config.MaxQueueSize))
}

func (distributor *RedisTaskDistributor) ListUnfinishedTasks(queue string) ([]*asynq.TaskInfo, error) {
	tasks := make([]*asynq.TaskInfo, 0)
	scheduledTasks, err := distributor.ListScheduledTasks(queue)
	if err != nil {
		log.Error().Msgf("failed to list scheduled tasks: %v", err)
	} else {
		tasks = append(tasks, scheduledTasks...)
	}
	pendingTasks, err := distributor.ListPendingTasks(queue)
	if err != nil {
		log.Error().Msgf("failed to list pending tasks: %v", err)
	} else {
		tasks = append(tasks, pendingTasks...)
	}
	retryTasks, err := distributor.ListRetryTasks(queue)
	if err != nil {
		log.Error().Msgf("failed to list retry tasks: %v", err)
	} else {
		tasks = append(tasks, retryTasks...)
	}
	activeTasks, err := distributor.ListActiveTasks(queue)
	if err != nil {
		log.Error().Msgf("failed to list active tasks: %v", err)
	} else {
		tasks = append(tasks, activeTasks...)
	}
	return tasks, nil
}

func (distributor *RedisTaskDistributor) DeleteTask(queue string, id string) error {
	return distributor.inspector.DeleteTask(queue, id)
}

func (distributor *RedisTaskDistributor) PauseQueue(queue string) error {
	return distributor.inspector.PauseQueue(queue)
}

func (distributor *RedisTaskDistributor) UnpauseQueue(queue string) error {
	return distributor.inspector.UnpauseQueue(queue)
}

// CheckArchivedTasks periodically checks archived tasks, i.e., for each archived task,
// set its status to `failed` and then remove it from the archive queue.
// Tasks will be archived only when the redis cluster fails for a while.
// https://github.com/hibiken/asynq/blob/v0.24.1/inspector.go#L457
// https://github.com/hibiken/asynq/blob/v0.24.1/inspector.go#L578
func CheckArchivedTasks(distributor TaskDistributor, webhook platform.Webhook) {
	tasks, err := distributor.ListArchivedTasks(QueueCritical)
	if err != nil {
		log.Error().Msgf("failed to list archived tasks: %v", err)
	} else {
		for i := 0; i < len(tasks); i++ {
			// Parse the task payload
			var payload PayloadRunPrediction
			if err := json.Unmarshal(tasks[i].Payload, &payload); err != nil {
				log.Error().Msgf("failed to unmarshal payload from archived task: %v", err)
				continue
			}
			// Update the task status
			info := platform.UpdateRequest{
				ID:        payload.ID,
				Status:    "failed",
				ErrorInfo: "failed due to system errors",
			}
			if err := webhook.UpdateTaskInfo(&info); err != nil {
				// Two cases:
				// 1. The task record has been deleted by redis automatically due to key expiration.
				// 2. The redis fails during the call (happens rarely).
				log.Error().Msgf("failed to update archived task info: %v", err)
				currentTime := time.Now()
				expiredTime := tasks[i].LastFailedAt.Add(12 * time.Hour)
				if currentTime.Before(expiredTime) {
					continue
				}
			}
			// Delete the processed task from the archived queue if the status was updated or the task is expired
			err := distributor.DeleteTask(tasks[i].Queue, tasks[i].ID)
			if err != nil {
				log.Error().Msgf("failed to delete archived task %s: %v", tasks[i].ID, err)
			} else {
				log.Info().Msgf("deleted archived task %s", tasks[i].ID)
			}
		}
	}
}

// ShutdownDistributor (only use it when redis is in local memory):
// It will set the status of all the scheduled, pending and retry tasks to `failed`.
func ShutdownDistributor(distributor TaskDistributor, webhook platform.Webhook) {
	tasks := make([]*asynq.TaskInfo, 0)
	scheduledTasks, err := distributor.ListScheduledTasks(QueueCritical)
	if err != nil {
		log.Error().Msgf("failed to list scheduled tasks: %v", err)
	} else {
		tasks = append(tasks, scheduledTasks...)
	}
	pendingTasks, err := distributor.ListPendingTasks(QueueCritical)
	if err != nil {
		log.Error().Msgf("failed to list pending tasks: %v", err)
	} else {
		tasks = append(tasks, pendingTasks...)
	}
	retryTasks, err := distributor.ListRetryTasks(QueueCritical)
	if err != nil {
		log.Error().Msgf("failed to list retry tasks: %v", err)
	} else {
		tasks = append(tasks, retryTasks...)
	}

	for i := 0; i < len(tasks); i++ {
		// Parse the task payload
		var payload PayloadRunPrediction
		if err := json.Unmarshal(tasks[i].Payload, &payload); err != nil {
			log.Error().Msgf("failed to unmarshal payload from archived task: %v", err)
			continue
		}
		// Update the task status
		info := platform.UpdateRequest{
			ID:        payload.ID,
			Status:    "failed",
			ErrorInfo: "the task queue was closed",
		}
		if err := webhook.UpdateTaskInfo(&info); err != nil {
			log.Error().Msgf("failed to update archived task info: %v", err)
		}
		// Delete the processed task from the archived queue if the status was updated or the task is expired
		err := distributor.DeleteTask(tasks[i].Queue, tasks[i].ID)
		if err != nil {
			log.Error().Msgf("failed to delete task %s: %v", tasks[i].ID, err)
		} else {
			log.Info().Msgf("deleted task %s", tasks[i].ID)
		}
	}
}

// CheckTaskStatus checks if there are tasks remaining in pending/running status in database due to service failures:
// 1. List all the task records with `pending` and `running` states for a specific model.
// 2. Wait for `TaskTimeout` seconds.
// 2. Check if the task ids are not in the task queue and the status isn't changed.
// 3. If yes, then set the status to `failed`.
// Note that this only works when there is exactly one task queue.
// If the same agent has multiple queues (e.g., local redis), please make sure that the maximum pending
// time is less than `TaskTimeout`.
func CheckTaskStatus(config utils.Config, distributor TaskDistributor, webhook platform.Webhook) int {
	if config.ModelName == "" {
		log.Warn().Msg("MODEL_NAME is not set for this serving agent")
		return 0
	}

	pendingTaskIDs, err := webhook.GetTaskIDByModelStatus(config.ModelName, "pending")
	if err != nil {
		log.Error().Msgf("task status check: GetTaskIDByModelStatus failed: %v", err)
		pendingTaskIDs = make([]string, 0)
	}
	log.Info().Msgf("number of task records with `pending` status: %d", len(pendingTaskIDs))

	runningTaskIDs, err := webhook.GetTaskIDByModelStatus(config.ModelName, "running")
	if err != nil {
		log.Error().Msgf("task status check: GetTaskIDByModelStatus failed: %v", err)
		runningTaskIDs = make([]string, 0)
	}
	log.Info().Msgf("number of task records with `running` status: %d", len(runningTaskIDs))

	// Wait for a while
	if len(pendingTaskIDs) > 0 || len(runningTaskIDs) > 0 {
		log.Info().Msgf("wait for pending/running tasks to be finished ...")
		time.Sleep(time.Duration(config.TaskTimeout) * time.Second)
	}

	unfinishedTasks, _ := distributor.ListUnfinishedTasks(QueueCritical)
	log.Info().Msgf("task status check: number of unfinished tasks: %d", len(unfinishedTasks))
	unfinishedQueueIDs := make([]string, 0)
	for _, taskInfo := range unfinishedTasks {
		unfinishedQueueIDs = append(unfinishedQueueIDs, taskInfo.ID)
	}

	// Handle pending tasks
	numFailedTasks := checkTaskStatus(pendingTaskIDs, unfinishedQueueIDs, "pending", webhook)
	// Handle running tasks
	numFailedTasks += checkTaskStatus(runningTaskIDs, unfinishedQueueIDs, "running", webhook)
	return numFailedTasks
}

func checkTaskStatus(taskIDs []string, unfinishedQueueIDs []string, status string, webhook platform.Webhook) int {
	numFailedTasks := 0
	for _, taskID := range taskIDs {
		task, e := webhook.GetTaskInfoObject(taskID)
		if e != nil {
			// The redis record has expired, set `failed` in the database for this record
			log.Error().Msgf("task status check: GetTaskInfoObject failed: %s, %v", taskID, e)
			info := platform.UpdateRequest{
				ID: taskID, Status: "failed", ErrorInfo: "Unknown failure", DatabaseOnly: true,
			}
			if e := webhook.UpdateTaskInfo(&info); e != nil {
				log.Error().Msgf("task status check: failed to update task info: %s, %v", taskID, e)
			}
			numFailedTasks += 1
			continue
		}
		if !slices.Contains(unfinishedQueueIDs, task.QueueID) && task.Status == status {
			info := platform.UpdateRequest{
				ID: taskID, Status: "failed", ErrorInfo: "Unknown failure",
			}
			if e := webhook.UpdateTaskInfo(&info); e != nil {
				log.Error().Msgf("task status check: failed to update task info: %s, %v", taskID, e)
			}
			log.Info().Msgf("task status check: set task %s to `failed`", taskID)
			numFailedTasks += 1
		}
	}
	return numFailedTasks
}

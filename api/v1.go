package api

import (
	"encoding/json"
	"fmt"
	"github.com/HyperGAI/serving-agent/platform"
	"github.com/HyperGAI/serving-agent/worker"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/rs/zerolog/log"
	"net/http"
	"time"
)

func (server *Server) predict(ctx *gin.Context) {
	var req platform.InferRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	if server.config.MLPlatform == "kserve" ||
		server.config.MLPlatform == "k8s" ||
		server.config.MLPlatform == "k8s-plugin" {
		// Append uploading webhook
		uploadURL := fmt.Sprintf("http://%s/upload", server.config.UploadWebhookAddress)
		req.Inputs["upload_webhook"] = uploadURL
	}

	// Add a prediction task record
	userID := ctx.Request.Header.Get("UID")
	id := uuid.New().String()
	_, err := server.webhook.CreateNewTask(id, userID, req.ModelName, "running", 0)
	if err != nil {
		log.Error().Msgf("failed to create new task info: %v", err)
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	info := platform.UpdateRequest{ID: id}

	// Run prediction
	response, e := server.platform.Predict(&req, "v1")
	if e != nil {
		log.Error().Msgf("failed to run prediction: %v", e)
		info.Status = "failed"
		info.ErrorInfo = e.Error()
		if err := server.webhook.UpdateTaskInfo(&info); err != nil {
			log.Error().Msgf("failed to update task info: %v", err)
		}
		server.convertErrorCode(e, ctx)
		return
	}

	// Update task status
	info.Status = "succeeded"
	info.Outputs = response.Outputs
	runningTime, ok := response.Outputs["running_time"]
	if ok {
		info.RunningTime = fmt.Sprintf("%v", runningTime)
		delete(response.Outputs, "running_time")
	}
	if err := server.webhook.UpdateTaskInfo(&info); err != nil {
		log.Error().Msgf("failed to update task info: %v", err)
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Get results
	outputs, err := server.webhook.GetTaskInfo(id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	ctx.JSON(http.StatusOK, outputs)
}

func (server *Server) asyncPredict(ctx *gin.Context) {
	var req platform.InferRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	if server.config.MLPlatform == "kserve" ||
		server.config.MLPlatform == "k8s" ||
		server.config.MLPlatform == "k8s-plugin" {
		// Append uploading webhook
		uploadURL := fmt.Sprintf("http://%s/upload", server.config.UploadWebhookAddress)
		req.Inputs["upload_webhook"] = uploadURL
	}

	id := uuid.New().String()
	opts := []asynq.Option{
		asynq.MaxRetry(1),
		asynq.Queue(worker.QueueCritical),
		asynq.Timeout(time.Duration(server.config.TaskTimeout) * time.Second),
	}
	payload := &worker.PayloadRunPrediction{
		InferRequest: req,
		ID:           id,
		APIVersion:   "v1",
	}

	// Get task queue info
	var queueSize = 0
	queueInfo, err := server.distributor.GetTaskQueueInfo(worker.QueueCritical)
	if err == nil {
		queueSize = queueInfo.Scheduled + queueInfo.Pending + queueInfo.Retry
		log.Info().Msgf("task queue current size: %d", queueSize)
		if queueSize >= server.config.MaxQueueSize {
			log.Error().Msg("the task queue is full, cannot add more tasks")
			ctx.JSON(http.StatusTooManyRequests,
				errorResponse(fmt.Errorf(
					"the prediction task queue is full, please wait for a while")))
			return
		}
	}

	// Add a prediction task record
	userID := ctx.Request.Header.Get("UID")
	output := map[string]string{}
	res, err := server.webhook.CreateNewTask(id, userID, req.ModelName, "", queueSize)
	if err != nil {
		log.Error().Msgf("failed to create new task info: %v", err)
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	if err := json.Unmarshal([]byte(res), &output); err != nil {
		log.Error().Msgf("failed to unmarshal output: %v", err)
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	// Submit a new prediction task
	taskID, err := server.distributor.DistributeTaskRunPrediction(ctx, payload, opts...)
	if err != nil {
		log.Error().Msgf("failed to distribute prediction task %s: %v", payload.ID, err)
		info := platform.UpdateRequest{ID: payload.ID, Status: "failed", ErrorInfo: "task queue failed"}
		if e := server.webhook.UpdateTaskInfo(&info); e != nil {
			log.Error().Msgf("failed to update task info: %v", err)
			ctx.JSON(http.StatusInternalServerError, errorResponse(e))
			return
		}
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	} else {
		info := platform.UpdateRequest{ID: payload.ID, QueueID: taskID}
		if e := server.webhook.UpdateTaskInfo(&info); e != nil {
			log.Error().Msgf("failed to update task info: %v", err)
			if e := server.distributor.DeleteTask(worker.QueueCritical, taskID); e != nil {
				log.Error().Msgf("failed to delete task from queue: %v", e)
			}
			ctx.JSON(http.StatusInternalServerError, errorResponse(e))
			return
		}
	}
	// url := fmt.Sprintf("%s/task/%s", server.config.PublicURL, output["id"])
	// ctx.JSON(http.StatusOK, gin.H{"url": url})
	ctx.JSON(http.StatusOK, gin.H{"id": output["id"]})
}

func (server *Server) docs(ctx *gin.Context) {
	var req platform.DocsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	response, err := server.platform.Docs(&req)
	if err != nil {
		server.convertErrorCode(err, ctx)
		return
	}
	ctx.JSON(http.StatusOK, response)
}

func (server *Server) convertErrorCode(err *platform.RequestError, ctx *gin.Context) {
	switch err.StatusCode {
	case platform.MarshalError:
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
	case platform.BuildRequestError:
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
	case platform.SendRequestError:
		ctx.JSON(http.StatusForbidden, errorResponse(err))
	case platform.InvalidInputError:
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
	default:
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
	}
}

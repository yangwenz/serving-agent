package api

import (
	"fmt"
	"github.com/HyperGAI/serving-agent/platform"
	"github.com/HyperGAI/serving-agent/utils"
	"github.com/HyperGAI/serving-agent/worker"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"strings"
	"time"
)

type Server struct {
	config      utils.Config
	router      *gin.Engine
	platform    platform.Platform
	distributor worker.TaskDistributor
	webhook     platform.Webhook
}

func NewServer(
	config utils.Config,
	platform platform.Platform,
	distributor worker.TaskDistributor,
	webhook platform.Webhook,
) (*Server, error) {
	server := Server{
		config:      config,
		router:      nil,
		platform:    platform,
		distributor: distributor,
		webhook:     webhook,
	}
	server.setupRouter()
	return &server, nil
}

func (server *Server) setupRouter() {
	router := gin.Default()

	router.GET("/live", server.checkLiveness)
	router.GET("/ready", server.checkReadiness)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))
	router.POST("/pause", server.pauseQueue)
	router.POST("/unpause", server.unpauseQueue)
	router.POST("/delete_pending", server.deleteAllPendingTasks)
	router.GET("/unfinished", server.listUnfinishedTasks)

	v1Routes := router.Group("/v1")
	v1Routes.Use(prometheusMiddleware())
	v1Routes.POST("/predict", server.predict)
	v1Routes.POST("/generate", server.generate)
	v1Routes.GET("/docs", server.docs)
	v1Routes.GET("/queue_size", server.getQueueSize)

	asyncV1Routes := router.Group("/async/v1")
	asyncV1Routes.Use(prometheusMiddleware())
	asyncV1Routes.POST("/predict", server.asyncPredict)

	taskRoutes := router.Group("/task")
	taskRoutes.GET("/:id", server.getTask)

	cancelRoutes := router.Group("/cancel")
	cancelRoutes.POST("/:id", server.cancelTask)

	server.router = router
}

func (server *Server) Start(address string) error {
	return server.router.Run(address)
}

func (server *Server) Handler() http.Handler {
	return server.router.Handler()
}

func (server *Server) checkLiveness(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"message": "API OK"})
}

func (server *Server) checkReadiness(ctx *gin.Context) {
	// TODO: Return False if the task queue is full
	ctx.JSON(http.StatusOK, gin.H{"message": "API OK"})
}

type TaskID struct {
	ID string `uri:"id" binding:"required"`
}

func (server *Server) getTask(ctx *gin.Context) {
	var taskID TaskID
	if err := ctx.ShouldBindUri(&taskID); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	outputs, err := server.webhook.GetTaskInfo(taskID.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	ctx.JSON(http.StatusOK, outputs)
}

func (server *Server) cancelTask(ctx *gin.Context) {
	var taskID TaskID
	if err := ctx.ShouldBindUri(&taskID); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	outputs, err := server.webhook.GetTaskInfoObject(taskID.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	if err := server.distributor.DeleteTask(worker.QueueCritical, outputs.QueueID); err != nil {
		ctx.JSON(http.StatusForbidden, errorResponse(err))
	} else {
		info := platform.UpdateRequest{
			ID:     outputs.ID,
			Status: "canceled",
		}
		if err := server.webhook.UpdateTaskInfo(&info); err != nil {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"id": outputs.ID})
	}
}

func (server *Server) getQueueSize(ctx *gin.Context) {
	var queueSize = 0
	queueInfo, err := server.distributor.GetTaskQueueInfo(worker.QueueCritical)
	if err == nil {
		queueSize = queueInfo.Scheduled + queueInfo.Pending + queueInfo.Retry
	} else if strings.Contains(err.Error(), "NOT_FOUND") {
		queueSize = 0
	} else {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"queue_size": queueSize})
}

func (server *Server) pauseQueue(ctx *gin.Context) {
	err := server.distributor.PauseQueue(worker.QueueCritical)
	if err != nil {
		ctx.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"info": fmt.Sprintf("queue %s paused", worker.QueueCritical)})
}

func (server *Server) unpauseQueue(ctx *gin.Context) {
	err := server.distributor.UnpauseQueue(worker.QueueCritical)
	if err != nil {
		ctx.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"info": fmt.Sprintf("Unpaused queue %s paused", worker.QueueCritical)})
}

func (server *Server) deleteAllPendingTasks(ctx *gin.Context) {
	pendingTaskIDs, err := server.webhook.GetTaskIDByModelStatus(server.config.ModelName, "pending")
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	numPendingTasks := len(pendingTaskIDs)

	numDeleteTasks := 0
	for _, taskID := range pendingTaskIDs {
		task, e := server.webhook.GetTaskInfoObject(taskID)
		if e != nil {
			continue
		}
		if err := server.distributor.DeleteTask(worker.QueueCritical, task.QueueID); err != nil {
			continue
		}
		numDeleteTasks += 1
	}
	ctx.JSON(http.StatusOK, gin.H{
		"num_of_pending_tasks": numPendingTasks,
		"num_of_deleted_tasks": numDeleteTasks,
	})
}

func (server *Server) listUnfinishedTasks(ctx *gin.Context) {
	unfinishedTasks, err := server.distributor.ListUnfinishedTasks(worker.QueueCritical)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	} else {
		ctx.JSON(http.StatusOK, gin.H{"num of unfinished tasks": len(unfinishedTasks)})
	}
}

func errorResponse(err error) gin.H {
	return gin.H{"error": err.Error()}
}

func (server *Server) CheckQueueSize() {
	for {
		var queueSize = 0
		queueInfo, err := server.distributor.GetTaskQueueInfo(worker.QueueCritical)
		if err == nil {
			queueSize = queueInfo.Scheduled + queueInfo.Pending + queueInfo.Retry
		}
		queueSizeGauge.Set(float64(queueSize))
		queueSizeRatioGauge.Set(float64(queueSize) / float64(server.config.MaxQueueSize))
		time.Sleep(30 * time.Second)
	}
}

func PeriodicCheck(config utils.Config, distributor worker.TaskDistributor, webhook platform.Webhook) {
	worker.CheckArchivedTasks(distributor, webhook)
	numFailedTasks := worker.CheckTaskStatus(config, distributor, webhook)
	taskStatusSetToFailedGauge.Set(float64(numFailedTasks))
}

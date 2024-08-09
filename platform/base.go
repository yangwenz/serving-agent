package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type InferRequest struct {
	ModelName string                 `json:"model_name" binding:"required"`
	Inputs    map[string]interface{} `json:"inputs" binding:"required"`
}

type InferResponse struct {
	Outputs map[string]interface{}
}

type DocsRequest struct {
	ModelName string `json:"model_name" binding:"required"`
}

type TaskInfo struct {
	ID          string      `json:"id"`
	Status      string      `json:"status"`
	RunningTime string      `json:"running_time"`
	Outputs     interface{} `json:"outputs"`
	CreatedAt   time.Time   `json:"created_at"`
	ErrorInfo   string      `json:"error_info"`
	QueueID     string      `json:"queue_id"`
}

type Platform interface {
	Predict(request *InferRequest, version string) (*InferResponse, *RequestError)
	Generate(request *InferRequest, version string, ctx context.Context, encoder *json.Encoder, flusher http.Flusher) *RequestError
	Docs(request *DocsRequest) (interface{}, *RequestError)
}

type UpdateRequest struct {
	ID           string      `json:"id" binding:"required"`
	Status       string      `json:"status"`
	RunningTime  string      `json:"running_time"`
	Outputs      interface{} `json:"outputs"`
	ErrorInfo    string      `json:"error_info"`
	QueueID      string      `json:"queue_id"`
	DatabaseOnly bool        `json:"database_only"`
}

type Webhook interface {
	CreateNewTask(taskID, userID, modelName, status string, queueNum int) (string, error)
	UpdateTaskInfo(info *UpdateRequest) error
	GetTaskInfo(taskID string) (interface{}, error)
	GetTaskInfoObject(taskID string) (*TaskInfo, error)
	GetTaskIDByModelStatus(modelName, status string) ([]string, error)
}

type Fetcher interface {
	SendRequest(req *http.Request, timeout time.Duration, retries int) (*http.Response, error)
}

type StreamingMessage struct {
	Id   int    `json:"id"`
	Data string `json:"data"`
}

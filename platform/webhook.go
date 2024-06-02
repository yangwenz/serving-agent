package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HyperGAI/serving-agent/utils"
	"github.com/avast/retry-go/v4"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"time"
)

type InternalWebhook struct {
	Config  utils.Config
	Url     string
	Fetcher Fetcher
}

type WebhookFetcher struct {
}

func NewInternalWebhook(config utils.Config) Webhook {
	webhook := InternalWebhook{
		Config:  config,
		Url:     fmt.Sprintf("http://%s/task", config.WebhookServerAddress),
		Fetcher: &WebhookFetcher{},
	}
	return &webhook
}

func (fetcher *WebhookFetcher) SendRequest(req *http.Request, timeout time.Duration, retries int) (*http.Response, error) {
	// https://github.com/avast/retry-go
	configs := []retry.Option{
		retry.Attempts(uint(retries)),
		retry.OnRetry(func(n uint, err error) {
			log.Warn().Msgf("Retry request %d to and get error: %v", n+1, err)
		}),
		retry.Delay(2 * time.Second),
	}

	var res *http.Response
	err := retry.Do(
		func() error {
			var e error
			client := http.Client{Timeout: timeout}
			res, e = client.Do(req)
			return e
		},
		configs...,
	)
	if err != nil {
		return nil, err
	}
	return res, err
}

func (webhook *InternalWebhook) CreateNewTask(
	taskID string,
	userID string,
	modelName string,
	status string,
	queueNum int,
) (string, error) {
	info := map[string]interface{}{
		"id":         taskID,
		"model_name": modelName,
		"queue_num":  queueNum,
		"status":     status,
	}
	data, err := json.Marshal(info)
	if err != nil {
		return "", errors.New("failed to marshal task info")
	}
	req, err := http.NewRequest("POST", webhook.Url, bytes.NewReader(data))
	if err != nil {
		return "", errors.New("failed to build request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", webhook.Config.WebhookAPIKey)
	req.Header.Set("UID", userID)

	res, err := webhook.Fetcher.SendRequest(req, 10*time.Second, 3)
	if err != nil {
		return "", fmt.Errorf("http post request /task failed: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		message := readErrorMessage(res)
		return "", fmt.Errorf("http post request /task failed, status code: %d, error: %v",
			res.StatusCode, message)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	return string(body), nil
}

func (webhook *InternalWebhook) UpdateTaskInfo(info *UpdateRequest) error {
	data, err := json.Marshal(info)
	if err != nil {
		return errors.New("failed to marshal task info")
	}
	req, err := http.NewRequest("PUT", webhook.Url, bytes.NewReader(data))
	if err != nil {
		return errors.New("failed to build request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", webhook.Config.WebhookAPIKey)

	res, err := webhook.Fetcher.SendRequest(req, 10*time.Second, 3)
	if err != nil {
		return fmt.Errorf("failed to update task info: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		message := readErrorMessage(res)
		return fmt.Errorf("failed to update task info, status code: %d, error: %v",
			res.StatusCode, message)
	}
	return nil
}

func (webhook *InternalWebhook) GetTaskInfo(taskID string) (interface{}, error) {
	url := fmt.Sprintf("%s/%s", webhook.Url, taskID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, errors.New("failed to build request")
	}
	req.Header.Set("apikey", webhook.Config.WebhookAPIKey)

	res, err := webhook.Fetcher.SendRequest(req, 10*time.Second, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to get task info: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		message := readErrorMessage(res)
		return nil, fmt.Errorf("failed to get task info, status code: %d, error: %v",
			res.StatusCode, message)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	var outputs interface{}
	err = json.Unmarshal(body, &outputs)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}
	return outputs, nil
}

func (webhook *InternalWebhook) GetTaskInfoObject(taskID string) (*TaskInfo, error) {
	info, err := webhook.GetTaskInfo(taskID)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(info)
	if err != nil {
		return nil, err
	}
	var outputs TaskInfo
	err = json.Unmarshal(data, &outputs)
	if err != nil {
		return nil, err
	}
	return &outputs, nil
}

func (webhook *InternalWebhook) GetTaskIDByModelStatus(modelName, status string) ([]string, error) {
	inputs := map[string]string{
		"model_name": modelName,
		"status":     status,
	}
	data, err := json.Marshal(inputs)
	if err != nil {
		return nil, errors.New("failed to marshal model_name and status")
	}

	url := fmt.Sprintf("%s/modelstatus", webhook.Url)
	req, err := http.NewRequest("GET", url, bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("failed to build request")
	}
	req.Header.Set("apikey", webhook.Config.WebhookAPIKey)

	res, err := webhook.Fetcher.SendRequest(req, 10*time.Second, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to get task by model status: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		log.Warn().Msgf("no tasks were found by model status: %s and %s", modelName, status)
		return make([]string, 0), nil
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var taskIDs = make([]string, 0)
	var outputs []map[string]interface{}
	err = json.Unmarshal(body, &outputs)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}
	for _, task := range outputs {
		id := fmt.Sprint(task["task_id"])
		taskIDs = append(taskIDs, id)
	}
	return taskIDs, nil
}

func readErrorMessage(res *http.Response) interface{} {
	var errorMessage interface{}
	data, e := io.ReadAll(res.Body)
	if e == nil {
		_ = json.Unmarshal(data, &errorMessage)
	} else {
		log.Error().Msgf("failed to read error message: %v", e)
	}
	return errorMessage
}

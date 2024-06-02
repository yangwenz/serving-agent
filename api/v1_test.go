package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HyperGAI/serving-agent/platform"
	mockplatform "github.com/HyperGAI/serving-agent/platform/mock"
	"github.com/HyperGAI/serving-agent/utils"
	"github.com/HyperGAI/serving-agent/worker"
	mockwk "github.com/HyperGAI/serving-agent/worker/mock"
	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPredictV1(t *testing.T) {
	userID := "12345"
	testCases := []struct {
		name          string
		body          gin.H
		buildStubs    func(platform *mockplatform.MockPlatform, webhook *mockplatform.MockWebhook)
		checkResponse func(recorder *httptest.ResponseRecorder)
	}{
		{
			name: "OK",
			body: gin.H{
				"model_name":    "test_model",
				"model_version": "v1",
				"inputs": map[string][][]float32{
					"instances": {
						[]float32{6.8, 2.8, 4.8, 1.4},
						[]float32{6.0, 3.4, 4.5, 1.6},
					},
				},
			},
			buildStubs: func(x *mockplatform.MockPlatform, webhook *mockplatform.MockWebhook) {
				x.EXPECT().
					Predict(gomock.Any(), gomock.Any()).
					Times(1).
					Return(&platform.InferResponse{}, nil)
				webhook.EXPECT().
					CreateNewTask(gomock.Any(), gomock.Eq(userID), gomock.Any(), gomock.Eq("running"), 0).
					Times(1).
					Return("", nil)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(nil)
				webhook.EXPECT().
					GetTaskInfo(gomock.Any()).
					Times(1).
					Return(nil, nil)
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, recorder.Code)
			},
		},
		{
			name: "Predict failed",
			body: gin.H{
				"model_name":    "test_model",
				"model_version": "v1",
				"inputs": map[string][][]float32{
					"instances": nil,
				},
			},
			buildStubs: func(x *mockplatform.MockPlatform, webhook *mockplatform.MockWebhook) {
				x.EXPECT().
					Predict(gomock.Any(), gomock.Any()).
					Times(1).
					Return(nil, platform.NewRequestError(20007, errors.New("invalid inputs")))
				webhook.EXPECT().
					CreateNewTask(gomock.Any(), gomock.Eq(userID), gomock.Eq("test_model"), gomock.Eq("running"), 0).
					Times(1).
					Return("", nil)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(nil)
				webhook.EXPECT().
					GetTaskInfo(gomock.Any()).
					Times(0)
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusBadRequest, recorder.Code)
			},
		},
		{
			name: "Internal error",
			body: gin.H{
				"model_name":    "test_model",
				"model_version": "v1",
				"inputs": map[string][][]float32{
					"instances": nil,
				},
			},
			buildStubs: func(x *mockplatform.MockPlatform, webhook *mockplatform.MockWebhook) {
				x.EXPECT().
					Predict(gomock.Any(), gomock.Any()).
					Times(1).
					Return(nil, platform.NewRequestError(20000, errors.New("internal error")))
				webhook.EXPECT().
					CreateNewTask(gomock.Any(), gomock.Eq(userID), gomock.Eq("test_model"), gomock.Eq("running"), 0).
					Times(1).
					Return("", nil)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(nil)
				webhook.EXPECT().
					GetTaskInfo(gomock.Any()).
					Times(0)
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusInternalServerError, recorder.Code)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			p := mockplatform.NewMockPlatform(ctrl)
			distributor := mockwk.NewMockTaskDistributor(ctrl)
			webhook := mockplatform.NewMockWebhook(ctrl)
			tc.buildStubs(p, webhook)

			server := newTestServer(t, p, distributor, webhook)
			recorder := httptest.NewRecorder()

			data, err := json.Marshal(tc.body)
			require.NoError(t, err)

			request, err := http.NewRequest(
				http.MethodPost, "/v1/predict", bytes.NewReader(data))
			require.NoError(t, err)
			request.Header.Set("UID", userID)

			server.router.ServeHTTP(recorder, request)
			tc.checkResponse(recorder)
		})
	}
}

func TestAsyncPredictV1(t *testing.T) {
	userID := "12345"
	testCases := []struct {
		name          string
		body          gin.H
		buildStubs    func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook)
		checkResponse func(recorder *httptest.ResponseRecorder)
	}{
		{
			name: "OK",
			body: gin.H{
				"model_name": "test_model",
				"inputs": map[string][][]float32{
					"instances": {
						[]float32{6.8, 2.8, 4.8, 1.4},
						[]float32{6.0, 3.4, 4.5, 1.6},
					},
				},
			},
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				webhook.EXPECT().CreateNewTask(gomock.Any(), gomock.Eq(userID), gomock.Eq("test_model"), "", 0).
					Times(1).
					Return("{\"id\": \"test-id\"}", nil)
				distributor.EXPECT().
					GetTaskQueueInfo(gomock.Any()).
					Times(1).
					Return(&asynq.QueueInfo{Size: 1}, nil)
				distributor.EXPECT().
					DistributeTaskRunPrediction(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(1).
					Return("123", nil)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(nil)
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, recorder.Code)
			},
		},
		{
			name: "Redis failed",
			body: gin.H{
				"model_name": "test_model",
				"inputs": map[string][][]float32{
					"instances": {
						[]float32{6.8, 2.8, 4.8, 1.4},
						[]float32{6.0, 3.4, 4.5, 1.6},
					},
				},
			},
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				webhook.EXPECT().CreateNewTask(gomock.Any(), gomock.Eq(userID), gomock.Eq("test_model"), "", 0).
					Times(1).
					Return("", fmt.Errorf("redis error"))
				distributor.EXPECT().
					GetTaskQueueInfo(gomock.Any()).
					Times(1).
					Return(&asynq.QueueInfo{Size: 1}, nil)
				distributor.EXPECT().
					DistributeTaskRunPrediction(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusInternalServerError, recorder.Code)
			},
		},
		{
			name: "Failed to distribute task",
			body: gin.H{
				"model_name": "test_model",
				"inputs": map[string][][]float32{
					"instances": {
						[]float32{6.8, 2.8, 4.8, 1.4},
						[]float32{6.0, 3.4, 4.5, 1.6},
					},
				},
			},
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				webhook.EXPECT().CreateNewTask(gomock.Any(), gomock.Eq(userID), gomock.Eq("test_model"), "", 0).
					Times(1).
					Return("{\"id\": \"test-id\"}", nil)
				distributor.EXPECT().
					GetTaskQueueInfo(gomock.Any()).
					Times(1).
					Return(&asynq.QueueInfo{Size: 1}, nil)
				distributor.EXPECT().
					DistributeTaskRunPrediction(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(1).
					Return("", fmt.Errorf("task delivered failed"))
				webhook.EXPECT().UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(nil)
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusInternalServerError, recorder.Code)
			},
		},
		{
			name: "Failed to update task",
			body: gin.H{
				"model_name": "test_model",
				"inputs": map[string][][]float32{
					"instances": {
						[]float32{6.8, 2.8, 4.8, 1.4},
						[]float32{6.0, 3.4, 4.5, 1.6},
					},
				},
			},
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				webhook.EXPECT().CreateNewTask(gomock.Any(), gomock.Eq(userID), gomock.Eq("test_model"), "", 0).
					Times(1).
					Return("{\"id\": \"test-id\"}", nil)
				distributor.EXPECT().
					GetTaskQueueInfo(gomock.Any()).
					Times(1).
					Return(&asynq.QueueInfo{Size: 1}, nil)
				distributor.EXPECT().
					DistributeTaskRunPrediction(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(1).
					Return("", fmt.Errorf("task delivered failed"))
				webhook.EXPECT().UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(fmt.Errorf("task update failed"))
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusInternalServerError, recorder.Code)
			},
		},
		{
			name: "Queue is full",
			body: gin.H{
				"model_name": "test_model",
				"inputs": map[string][][]float32{
					"instances": {
						[]float32{6.8, 2.8, 4.8, 1.4},
						[]float32{6.0, 3.4, 4.5, 1.6},
					},
				},
			},
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				distributor.EXPECT().
					GetTaskQueueInfo(gomock.Any()).
					Times(1).
					Return(&asynq.QueueInfo{Size: 100000, Pending: 50000}, nil)
				webhook.EXPECT().CreateNewTask(gomock.Any(), gomock.Eq(userID), gomock.Any(), "", 0).
					Times(0)
				distributor.EXPECT().
					DistributeTaskRunPrediction(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(0)
				webhook.EXPECT().UpdateTaskInfo(gomock.Any()).
					Times(0)
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusTooManyRequests, recorder.Code)
			},
		},
		{
			name: "Failed to distribute task 2",
			body: gin.H{
				"model_name": "test_model",
				"inputs": map[string][][]float32{
					"instances": {
						[]float32{6.8, 2.8, 4.8, 1.4},
						[]float32{6.0, 3.4, 4.5, 1.6},
					},
				},
			},
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				webhook.EXPECT().CreateNewTask(gomock.Any(), gomock.Eq(userID), gomock.Eq("test_model"), "", 0).
					Times(1).
					Return("{\"id\": \"test-id\"}", nil)
				distributor.EXPECT().
					GetTaskQueueInfo(gomock.Any()).
					Times(1).
					Return(&asynq.QueueInfo{Size: 1}, nil)
				distributor.EXPECT().
					DistributeTaskRunPrediction(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Times(1).
					Return("123", nil)
				webhook.EXPECT().UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(errors.New("update failed"))
				distributor.EXPECT().
					DeleteTask(gomock.Eq(worker.QueueCritical), gomock.Eq("123")).
					Times(1).
					Return(nil)
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusInternalServerError, recorder.Code)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			p := mockplatform.NewMockPlatform(ctrl)
			distributor := mockwk.NewMockTaskDistributor(ctrl)
			webhook := mockplatform.NewMockWebhook(ctrl)
			tc.buildStubs(distributor, webhook)

			server := newTestServer(t, p, distributor, webhook)
			recorder := httptest.NewRecorder()

			data, err := json.Marshal(tc.body)
			require.NoError(t, err)

			request, err := http.NewRequest(
				http.MethodPost, "/async/v1/predict", bytes.NewReader(data))
			require.NoError(t, err)
			request.Header.Set("UID", userID)

			server.router.ServeHTTP(recorder, request)
			tc.checkResponse(recorder)
		})
	}
}

func TestGetTask(t *testing.T) {
	testCases := []struct {
		name          string
		body          string
		buildStubs    func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook)
		checkResponse func(recorder *httptest.ResponseRecorder)
	}{
		{
			name: "OK",
			body: "12345",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				webhook.EXPECT().
					GetTaskInfo(gomock.Eq("12345")).
					Times(1).
					Return(
						&platform.TaskInfo{
							ID:          "12345",
							Status:      "pending",
							RunningTime: "",
							Outputs:     nil,
							ErrorInfo:   "",
							QueueID:     "12-23",
						}, nil)
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, recorder.Code)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			p := mockplatform.NewMockPlatform(ctrl)
			distributor := mockwk.NewMockTaskDistributor(ctrl)
			webhook := mockplatform.NewMockWebhook(ctrl)
			tc.buildStubs(distributor, webhook)

			server := newTestServer(t, p, distributor, webhook)
			recorder := httptest.NewRecorder()

			url := fmt.Sprintf("/task/%s", tc.body)
			request, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)

			server.router.ServeHTTP(recorder, request)
			tc.checkResponse(recorder)
		})
	}
}

func TestCancelTask(t *testing.T) {
	testCases := []struct {
		name          string
		body          string
		buildStubs    func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook)
		checkResponse func(recorder *httptest.ResponseRecorder)
	}{
		{
			name: "OK",
			body: "12345",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				webhook.EXPECT().
					GetTaskInfoObject(gomock.Eq("12345")).
					Times(1).
					Return(
						&platform.TaskInfo{
							ID:          "12345",
							Status:      "pending",
							RunningTime: "",
							Outputs:     nil,
							ErrorInfo:   "",
							QueueID:     "12-23",
						}, nil)
				distributor.EXPECT().
					DeleteTask(gomock.Eq(worker.QueueCritical), gomock.Eq("12-23")).
					Times(1).
					Return(nil)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(nil)
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, recorder.Code)
			},
		},
		{
			name: "Failed to get task info",
			body: "12345",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				webhook.EXPECT().
					GetTaskInfoObject(gomock.Eq("12345")).
					Times(1).
					Return(nil, errors.New("failed"))
				distributor.EXPECT().
					DeleteTask(gomock.Eq(worker.QueueCritical), gomock.Eq("12-23")).
					Times(0)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(0)
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusInternalServerError, recorder.Code)
			},
		},
		{
			name: "Delete failed",
			body: "12345",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				webhook.EXPECT().
					GetTaskInfoObject(gomock.Eq("12345")).
					Times(1).
					Return(
						&platform.TaskInfo{
							ID:          "12345",
							Status:      "pending",
							RunningTime: "",
							Outputs:     nil,
							ErrorInfo:   "",
							QueueID:     "12-23",
						}, nil)
				distributor.EXPECT().
					DeleteTask(gomock.Eq(worker.QueueCritical), gomock.Eq("12-23")).
					Times(1).
					Return(errors.New("failed"))
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(0)
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusForbidden, recorder.Code)
			},
		},
		{
			name: "Update failed",
			body: "12345",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				webhook.EXPECT().
					GetTaskInfoObject(gomock.Eq("12345")).
					Times(1).
					Return(
						&platform.TaskInfo{
							ID:          "12345",
							Status:      "pending",
							RunningTime: "",
							Outputs:     nil,
							ErrorInfo:   "",
							QueueID:     "12-23",
						}, nil)
				distributor.EXPECT().
					DeleteTask(gomock.Eq(worker.QueueCritical), gomock.Eq("12-23")).
					Times(1).
					Return(nil)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(errors.New("failed"))
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusInternalServerError, recorder.Code)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			p := mockplatform.NewMockPlatform(ctrl)
			distributor := mockwk.NewMockTaskDistributor(ctrl)
			webhook := mockplatform.NewMockWebhook(ctrl)
			tc.buildStubs(distributor, webhook)

			server := newTestServer(t, p, distributor, webhook)
			recorder := httptest.NewRecorder()

			url := fmt.Sprintf("/cancel/%s", tc.body)
			request, err := http.NewRequest(http.MethodPost, url, nil)
			require.NoError(t, err)

			server.router.ServeHTTP(recorder, request)
			tc.checkResponse(recorder)
		})
	}
}

func TestCheckArchivedTasks(t *testing.T) {
	currentTime := time.Now()
	payload := worker.PayloadRunPrediction{
		ID: "123456",
	}
	data, _ := json.Marshal(payload)
	taskA := asynq.TaskInfo{
		ID:           "abcdefg",
		Queue:        worker.QueueCritical,
		Payload:      data,
		LastFailedAt: currentTime,
	}
	tasksA := []*asynq.TaskInfo{&taskA}
	taskB := asynq.TaskInfo{
		ID:           "abcdefg",
		Queue:        worker.QueueCritical,
		Payload:      data,
		LastFailedAt: currentTime.Add(-13 * time.Hour),
	}
	tasksB := []*asynq.TaskInfo{&taskB}

	testCases := []struct {
		name       string
		buildStubs func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook)
	}{
		{
			name: "No archived tasks",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				distributor.EXPECT().
					ListArchivedTasks(gomock.Any()).
					Times(1).
					Return(make([]*asynq.TaskInfo, 0), nil)
			},
		},
		{
			name: "Failed to update task info",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				distributor.EXPECT().
					ListArchivedTasks(gomock.Any()).
					Times(1).
					Return(tasksA, nil)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(errors.New("failed"))
				distributor.EXPECT().
					DeleteTask(gomock.Any(), gomock.Any()).
					Times(0)
			},
		},
		{
			name: "Delete task",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				distributor.EXPECT().
					ListArchivedTasks(gomock.Any()).
					Times(1).
					Return(tasksA, nil)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(nil)
				distributor.EXPECT().
					DeleteTask(gomock.Any(), gomock.Any()).
					Times(1).
					Return(nil)
			},
		},
		{
			name: "failed to delete task",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				distributor.EXPECT().
					ListArchivedTasks(gomock.Any()).
					Times(1).
					Return(tasksA, nil)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(nil)
				distributor.EXPECT().
					DeleteTask(gomock.Any(), gomock.Any()).
					Times(1).
					Return(errors.New("failed"))
			},
		},
		{
			name: "Failed to update task info and task expires",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				distributor.EXPECT().
					ListArchivedTasks(gomock.Any()).
					Times(1).
					Return(tasksB, nil)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(errors.New("failed"))
				distributor.EXPECT().
					DeleteTask(gomock.Any(), gomock.Any()).
					Times(1)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			distributor := mockwk.NewMockTaskDistributor(ctrl)
			webhook := mockplatform.NewMockWebhook(ctrl)
			tc.buildStubs(distributor, webhook)
			worker.CheckArchivedTasks(distributor, webhook)
		})
	}
}

func TestShutdownDistributor(t *testing.T) {
	currentTime := time.Now()
	payload := worker.PayloadRunPrediction{
		ID: "123456",
	}
	data, _ := json.Marshal(payload)
	taskA := asynq.TaskInfo{
		ID:           "abcdefg",
		Queue:        worker.QueueCritical,
		Payload:      data,
		LastFailedAt: currentTime,
	}
	tasksA := []*asynq.TaskInfo{&taskA}
	taskB := asynq.TaskInfo{
		ID:           "abcdefg",
		Queue:        worker.QueueCritical,
		Payload:      data,
		LastFailedAt: currentTime.Add(-13 * time.Hour),
	}
	tasksB := []*asynq.TaskInfo{&taskB}

	testCases := []struct {
		name       string
		buildStubs func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook)
	}{
		{
			name: "Shutdown successfully",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				distributor.EXPECT().
					ListScheduledTasks(gomock.Any()).
					Times(1).
					Return(tasksA, nil)
				distributor.EXPECT().
					ListPendingTasks(gomock.Any()).
					Times(1).
					Return(tasksB, nil)
				distributor.EXPECT().
					ListRetryTasks(gomock.Any()).
					Times(1).
					Return(make([]*asynq.TaskInfo, 0), nil)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(2).
					Return(nil)
				distributor.EXPECT().
					DeleteTask(gomock.Any(), gomock.Any()).
					Times(2)
			},
		},
		{
			name: "Failed to list tasks",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				distributor.EXPECT().
					ListScheduledTasks(gomock.Any()).
					Times(1).
					Return(nil, errors.New("failed"))
				distributor.EXPECT().
					ListPendingTasks(gomock.Any()).
					Times(1).
					Return(nil, errors.New("failed"))
				distributor.EXPECT().
					ListRetryTasks(gomock.Any()).
					Times(1).
					Return(nil, errors.New("failed"))
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(0)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			distributor := mockwk.NewMockTaskDistributor(ctrl)
			webhook := mockplatform.NewMockWebhook(ctrl)
			tc.buildStubs(distributor, webhook)
			worker.ShutdownDistributor(distributor, webhook)
		})
	}
}

func TestInitCheck(t *testing.T) {
	config := utils.Config{ModelName: "test", TaskTimeout: 2}
	unfinishedTasks := []*asynq.TaskInfo{
		{
			ID: "123",
		},
		{
			ID: "456",
		},
	}

	testCases := []struct {
		name       string
		buildStubs func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook)
	}{
		{
			name: "OK",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				distributor.EXPECT().
					ListUnfinishedTasks(gomock.Any()).
					Times(1).
					Return(unfinishedTasks, nil)
				// Pending
				webhook.EXPECT().
					GetTaskIDByModelStatus(gomock.Eq("test"), gomock.Eq("pending")).
					Times(1).
					Return([]string{"abc"}, nil)
				webhook.EXPECT().
					GetTaskInfoObject(gomock.Eq("abc")).
					Times(1).
					Return(&platform.TaskInfo{
						ID:      "abc",
						Status:  "pending",
						QueueID: "0",
					}, nil)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(nil)
				// Running
				webhook.EXPECT().
					GetTaskIDByModelStatus(gomock.Eq("test"), gomock.Eq("running")).
					Times(1).
					Return([]string{"abc"}, nil)
				webhook.EXPECT().
					GetTaskInfoObject(gomock.Eq("abc")).
					Times(1).
					Return(&platform.TaskInfo{
						ID:      "abc",
						Status:  "running",
						QueueID: "0",
					}, nil)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(nil)
			},
		},
		{
			name: "GetTaskIDByModelStatus failed",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				distributor.EXPECT().
					ListUnfinishedTasks(gomock.Any()).
					Times(1).
					Return(unfinishedTasks, nil)
				// Pending
				webhook.EXPECT().
					GetTaskIDByModelStatus(gomock.Eq("test"), gomock.Eq("pending")).
					Times(1).
					Return(nil, errors.New("failed"))
				webhook.EXPECT().
					GetTaskInfoObject(gomock.Eq("abc")).
					Times(0)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(0)
				// Running
				webhook.EXPECT().
					GetTaskIDByModelStatus(gomock.Eq("test"), gomock.Eq("running")).
					Times(1).
					Return([]string{}, nil)
				webhook.EXPECT().
					GetTaskInfoObject(gomock.Eq("abc")).
					Times(0)
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(0)
			},
		},
		{
			name: "GetTaskInfoObject failed",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				distributor.EXPECT().
					ListUnfinishedTasks(gomock.Any()).
					Times(1).
					Return(unfinishedTasks, nil)
				// Pending
				webhook.EXPECT().
					GetTaskIDByModelStatus(gomock.Eq("test"), gomock.Eq("pending")).
					Times(1).
					Return([]string{"abc"}, nil)
				webhook.EXPECT().
					GetTaskInfoObject(gomock.Eq("abc")).
					Times(1).
					Return(nil, errors.New("failed"))
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(nil)
				// Running
				webhook.EXPECT().
					GetTaskIDByModelStatus(gomock.Eq("test"), gomock.Eq("running")).
					Times(1).
					Return([]string{"abc"}, nil)
				webhook.EXPECT().
					GetTaskInfoObject(gomock.Eq("abc")).
					Times(1).
					Return(nil, errors.New("failed"))
				webhook.EXPECT().
					UpdateTaskInfo(gomock.Any()).
					Times(1).
					Return(errors.New("update failed"))
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			distributor := mockwk.NewMockTaskDistributor(ctrl)
			webhook := mockplatform.NewMockWebhook(ctrl)
			tc.buildStubs(distributor, webhook)
			worker.CheckTaskStatus(config, distributor, webhook)
		})
	}
}

func TestGetQueueSize(t *testing.T) {
	testCases := []struct {
		name          string
		buildStubs    func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook)
		checkResponse func(recorder *httptest.ResponseRecorder)
	}{
		{
			name: "OK",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				distributor.EXPECT().GetTaskQueueInfo(gomock.Any()).Times(1).Return(&asynq.QueueInfo{
					Pending:   1,
					Active:    0,
					Scheduled: 2,
					Retry:     3,
					Archived:  4,
				}, nil)
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, recorder.Code)
				data, err := io.ReadAll(recorder.Body)
				require.NoError(t, err)

				var output map[string]int
				err = json.Unmarshal(data, &output)
				require.NoError(t, err)
				require.Equal(t, output["queue_size"], 6)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			p := mockplatform.NewMockPlatform(ctrl)
			distributor := mockwk.NewMockTaskDistributor(ctrl)
			webhook := mockplatform.NewMockWebhook(ctrl)
			tc.buildStubs(distributor, webhook)

			server := newTestServer(t, p, distributor, webhook)
			recorder := httptest.NewRecorder()

			url := fmt.Sprintf("/v1/queue_size")
			request, err := http.NewRequest(http.MethodGet, url, nil)
			require.NoError(t, err)

			server.router.ServeHTTP(recorder, request)
			tc.checkResponse(recorder)
		})
	}
}

func TestDeleteAllPendingTasks(t *testing.T) {
	testCases := []struct {
		name          string
		buildStubs    func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook)
		checkResponse func(recorder *httptest.ResponseRecorder)
	}{
		{
			name: "OK",
			buildStubs: func(distributor *mockwk.MockTaskDistributor, webhook *mockplatform.MockWebhook) {
				webhook.EXPECT().
					GetTaskIDByModelStatus(gomock.Any(), gomock.Eq("pending")).
					Times(1).
					Return([]string{"1", "2", "3"}, nil)
				webhook.EXPECT().
					GetTaskInfoObject(gomock.Any()).
					Times(3).
					Return(&platform.TaskInfo{
						ID:          "123",
						Status:      "pending",
						RunningTime: "",
						Outputs:     nil,
						CreatedAt:   time.Time{},
						ErrorInfo:   "",
						QueueID:     "12345",
					}, nil)
				distributor.EXPECT().
					DeleteTask(gomock.Any(), gomock.Eq("12345")).
					Times(3).
					Return(nil)
			},
			checkResponse: func(recorder *httptest.ResponseRecorder) {
				require.Equal(t, http.StatusOK, recorder.Code)
				fmt.Println(recorder.Body)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			p := mockplatform.NewMockPlatform(ctrl)
			distributor := mockwk.NewMockTaskDistributor(ctrl)
			webhook := mockplatform.NewMockWebhook(ctrl)
			tc.buildStubs(distributor, webhook)

			server := newTestServer(t, p, distributor, webhook)
			recorder := httptest.NewRecorder()

			url := fmt.Sprintf("/delete_pending")
			request, err := http.NewRequest(http.MethodPost, url, nil)
			require.NoError(t, err)

			server.router.ServeHTTP(recorder, request)
			tc.checkResponse(recorder)
		})
	}
}

package platform_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HyperGAI/serving-agent/platform"
	mockplatform "github.com/HyperGAI/serving-agent/platform/mock"
	"github.com/HyperGAI/serving-agent/utils"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"io"
	"net/http"
	"testing"
)

func newInternalWebhook(config utils.Config, fetcher platform.Fetcher) platform.Webhook {
	webhook := platform.InternalWebhook{
		Config:  config,
		Url:     fmt.Sprintf("http://%s/task", config.WebhookServerAddress),
		Fetcher: fetcher,
	}
	return &webhook
}

func TestCreateNewTask(t *testing.T) {
	inputs := map[string]string{
		"id": "12345",
	}
	data, _ := json.Marshal(inputs)
	readCloser := io.NopCloser(bytes.NewReader(data))

	testCases := []struct {
		name          string
		taskID        string
		userID        string
		modelName     string
		queueNum      int
		buildStubs    func(fetcher *mockplatform.MockFetcher)
		checkResponse func(result string, err error)
	}{
		{
			name:      "OK",
			taskID:    "12345",
			userID:    "abcde",
			modelName: "test",
			queueNum:  1,
			buildStubs: func(fetcher *mockplatform.MockFetcher) {
				fetcher.EXPECT().
					SendRequest(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(1).
					Return(&http.Response{
						StatusCode: 200,
						Body:       readCloser,
					}, nil)
			},
			checkResponse: func(result string, err error) {
				require.NoError(t, err)
				fmt.Println(result)
			},
		},
		{
			name:      "Failed to send request",
			taskID:    "12345",
			userID:    "abcde",
			modelName: "test",
			queueNum:  1,
			buildStubs: func(fetcher *mockplatform.MockFetcher) {
				fetcher.EXPECT().
					SendRequest(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(1).
					Return(nil, errors.New("failed"))
			},
			checkResponse: func(result string, err error) {
				require.NotNil(t, err)
			},
		},
		{
			name:      "Status code not 200",
			taskID:    "12345",
			userID:    "abcde",
			modelName: "test",
			queueNum:  1,
			buildStubs: func(fetcher *mockplatform.MockFetcher) {
				fetcher.EXPECT().
					SendRequest(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(1).
					Return(&http.Response{
						StatusCode: 400,
						Body:       readCloser,
					}, nil)
			},
			checkResponse: func(result string, err error) {
				require.NotNil(t, err)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			fetcher := mockplatform.NewMockFetcher(ctrl)
			tc.buildStubs(fetcher)

			webhook := newInternalWebhook(utils.Config{}, fetcher)
			result, err := webhook.CreateNewTask(tc.taskID, tc.userID, tc.modelName, "", tc.queueNum)
			tc.checkResponse(result, err)
		})
	}
}

func TestUpdateTaskInfo(t *testing.T) {
	inputs := map[string]string{
		"id": "12345",
	}
	data, _ := json.Marshal(inputs)
	readCloser := io.NopCloser(bytes.NewReader(data))

	info := &platform.UpdateRequest{
		ID:          "",
		Status:      "",
		RunningTime: "",
	}
	testCases := []struct {
		name          string
		info          *platform.UpdateRequest
		buildStubs    func(fetcher *mockplatform.MockFetcher)
		checkResponse func(err error)
	}{
		{
			name: "OK",
			info: info,
			buildStubs: func(fetcher *mockplatform.MockFetcher) {
				fetcher.EXPECT().
					SendRequest(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(1).
					Return(&http.Response{
						StatusCode: 200,
						Body:       readCloser,
					}, nil)
			},
			checkResponse: func(err error) {
				require.NoError(t, err)
			},
		},
		{
			name: "Failed",
			info: info,
			buildStubs: func(fetcher *mockplatform.MockFetcher) {
				fetcher.EXPECT().
					SendRequest(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(1).
					Return(nil, errors.New("failed"))
			},
			checkResponse: func(err error) {
				require.NotNil(t, err)
			},
		},
		{
			name: "Status code not 200",
			info: info,
			buildStubs: func(fetcher *mockplatform.MockFetcher) {
				fetcher.EXPECT().
					SendRequest(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(1).
					Return(&http.Response{
						StatusCode: 400,
						Body:       readCloser,
					}, nil)
			},
			checkResponse: func(err error) {
				require.NotNil(t, err)
				fmt.Println(err)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			fetcher := mockplatform.NewMockFetcher(ctrl)
			tc.buildStubs(fetcher)

			webhook := newInternalWebhook(utils.Config{}, fetcher)
			err := webhook.UpdateTaskInfo(info)
			tc.checkResponse(err)
		})
	}
}

func TestGetTaskIDByModelStatus(t *testing.T) {
	inputs := []map[string]string{
		{"task_id": "12345", "model_name": "test"},
	}
	data, _ := json.Marshal(inputs)
	readCloser := io.NopCloser(bytes.NewReader(data))

	testCases := []struct {
		name          string
		modelName     string
		status        string
		buildStubs    func(fetcher *mockplatform.MockFetcher)
		checkResponse func(taskIDs []string, err error)
	}{
		{
			name:      "OK",
			modelName: "test",
			status:    "pending",
			buildStubs: func(fetcher *mockplatform.MockFetcher) {
				fetcher.EXPECT().
					SendRequest(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(1).
					Return(&http.Response{
						StatusCode: 200,
						Body:       readCloser,
					}, nil)
			},
			checkResponse: func(taskIDs []string, err error) {
				require.NoError(t, err)
				require.Equal(t, len(taskIDs), 1)
				require.Equal(t, taskIDs[0], "12345")
			},
		},
		{
			name:      "Not found",
			modelName: "test",
			status:    "pending",
			buildStubs: func(fetcher *mockplatform.MockFetcher) {
				fetcher.EXPECT().
					SendRequest(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(1).
					Return(&http.Response{
						StatusCode: 400,
						Body:       readCloser,
					}, nil)
			},
			checkResponse: func(taskIDs []string, err error) {
				require.NoError(t, err)
				require.Equal(t, len(taskIDs), 0)
			},
		},
		{
			name:      "Failed",
			modelName: "test",
			status:    "pending",
			buildStubs: func(fetcher *mockplatform.MockFetcher) {
				fetcher.EXPECT().
					SendRequest(gomock.Any(), gomock.Any(), gomock.Any()).
					Times(1).
					Return(nil, errors.New("failed"))
			},
			checkResponse: func(taskIDs []string, err error) {
				require.NotNil(t, err)
			},
		},
	}

	for i := range testCases {
		tc := testCases[i]

		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			fetcher := mockplatform.NewMockFetcher(ctrl)
			tc.buildStubs(fetcher)

			webhook := newInternalWebhook(utils.Config{}, fetcher)
			taskIDs, err := webhook.GetTaskIDByModelStatus(tc.modelName, tc.status)
			tc.checkResponse(taskIDs, err)
		})
	}
}

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HyperGAI/serving-agent/utils"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"time"
)

type K8sPlugin struct {
	address string
	timeout int
}

func NewK8sPlugin(config utils.Config) Platform {
	return &K8sPlugin{
		address: config.K8sPluginAddress,
		timeout: config.K8sPluginRequestTimeout,
	}
}

func (service *K8sPlugin) sendRequest(
	method string,
	url string,
	body io.Reader,
	timeout time.Duration,
) (*http.Response, *RequestError) {
	// Build a new prediction request
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, NewRequestError(BuildRequestError,
			errors.New("failed to build request"))
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the prediction request
	client := http.Client{Timeout: timeout}
	res, err := client.Do(req)
	if err != nil {
		return nil, NewRequestError(SendRequestError,
			fmt.Errorf("url: %s, failed to send request, model not ready", url))
	}
	if res.StatusCode != 200 {
		var errorMessage interface{}
		data, e := io.ReadAll(res.Body)
		if e == nil {
			_ = json.Unmarshal(data, &errorMessage)
		} else {
			log.Error().Msgf("url: %s, failed to read error message: %v", url, e)
		}
		res.Body.Close()
		return nil, NewRequestError(InvalidInputError,
			fmt.Errorf("url: %s, status-code: %d, invalid inputs: %v",
				url, res.StatusCode, errorMessage))
	}
	return res, nil
}

func (service *K8sPlugin) Predict(request *InferRequest, version string) (*InferResponse, *RequestError) {
	if version == "v1" {
		return service.predictV1(request)
	}
	return nil, NewRequestError(UnknownAPIVersion,
		errors.New("prediction API version is not supported"))
}

func (service *K8sPlugin) Generate(
	request *InferRequest,
	version string,
	ctx context.Context,
	encoder *json.Encoder,
	flusher http.Flusher,
) *RequestError {
	return NewRequestError(UnknownAPIVersion,
		errors.New("generation API for k8s is not supported"))
}

func (service *K8sPlugin) predictV1(request *InferRequest) (*InferResponse, *RequestError) {
	// Marshal the input data
	data, err := json.Marshal(request)
	if err != nil {
		return nil, NewRequestError(MarshalError,
			errors.New("failed to marshal request"))
	}
	// Send a new prediction request
	url := fmt.Sprintf("http://%s/v1/predict", service.address)
	res, e := service.sendRequest(
		"POST", url, bytes.NewReader(data),
		time.Duration(service.timeout)*time.Second,
	)
	if e != nil {
		return nil, e
	}

	// Parse the response
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, NewRequestError(ReadResponseError,
			errors.New("failed to read response body"))
	}
	var outputs map[string]interface{}
	err = json.Unmarshal(body, &outputs)
	if err != nil {
		return nil, NewRequestError(UnmarshalResponseError,
			errors.New("failed to unmarshal response body"))
	}
	response := InferResponse{Outputs: outputs}
	return &response, nil
}

func (service *K8sPlugin) Docs(request *DocsRequest) (interface{}, *RequestError) {
	data, err := json.Marshal(request)
	if err != nil {
		return nil, NewRequestError(MarshalError,
			errors.New("failed to marshal request"))
	}
	url := fmt.Sprintf("http://%s/v1/docs", service.address)
	res, e := service.sendRequest("GET", url, bytes.NewReader(data), 10*time.Second)
	if e != nil {
		return nil, e
	}

	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, NewRequestError(ReadResponseError,
			errors.New("failed to read response body"))
	}
	var outputs interface{}
	err = json.Unmarshal(body, &outputs)
	if err != nil {
		return nil, NewRequestError(UnmarshalResponseError,
			errors.New("failed to unmarshal response body"))
	}
	return outputs, nil
}

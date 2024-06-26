package platform

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/HyperGAI/serving-agent/utils"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"time"
)

type KServe struct {
	address      string
	customDomain string
	namespace    string
	timeout      int
}

func NewKServe(config utils.Config) Platform {
	return &KServe{
		address:      config.KServeAddress,
		customDomain: config.KServeCustomDomain,
		namespace:    config.KServeNamespace,
		timeout:      config.KServeRequestTimeout,
	}
}

func (service *KServe) sendRequest(
	modelName string,
	method string,
	url string,
	data []byte,
	timeout time.Duration,
) (*http.Response, *RequestError) {
	numRetries := 5
	for i := 0; i < numRetries; i++ {
		// Build a new prediction request
		req, err := http.NewRequest(method, url, bytes.NewReader(data))
		if err != nil {
			return nil, NewRequestError(BuildRequestError,
				errors.New("failed to build request"))
		}
		req.Header.Set("Content-Type", "application/json")
		req.Host = fmt.Sprintf("%s.%s.%s",
			modelName, service.namespace, service.customDomain)

		// Send the prediction request
		client := http.Client{Timeout: timeout}
		res, err := client.Do(req)
		if err != nil {
			if i < numRetries-1 {
				log.Warn().Msgf("model-name: %s, failed to send request: %v, retry: %d", modelName, err, i+1)
				time.Sleep(time.Duration((i+1)*2) * time.Second)
				continue
			}
			return nil, NewRequestError(SendRequestError,
				fmt.Errorf("model-name: %s, failed to send request: %v", modelName, err))
		}
		if res.StatusCode != 200 {
			if (res.StatusCode >= 502 && res.StatusCode <= 504) && i < numRetries-1 {
				res.Body.Close()
				log.Warn().Msgf("model-name: %s, status-code: %d, retry: %d", modelName, res.StatusCode, i+1)
				time.Sleep(time.Duration((i+1)*2) * time.Second)
				continue
			}
			var errorMessage interface{}
			data, e := io.ReadAll(res.Body)
			if e == nil {
				_ = json.Unmarshal(data, &errorMessage)
			} else {
				log.Error().Msgf("model-name: %s, failed to read error message: %v", modelName, e)
			}
			res.Body.Close()
			return nil, NewRequestError(InvalidInputError,
				fmt.Errorf("model-name: %s, status-code: %d, error: %v, retries: %d",
					modelName, res.StatusCode, errorMessage, i))
		}
		return res, nil
	}
	return nil, NewRequestError(InternalError, fmt.Errorf("shouldn't be here"))
}

func (service *KServe) Predict(request *InferRequest, version string) (*InferResponse, *RequestError) {
	if version == "v1" {
		return service.predictV1(request)
	}
	return nil, NewRequestError(UnknownAPIVersion,
		errors.New("prediction API version is not supported"))
}

func (service *KServe) predictV1(request *InferRequest) (*InferResponse, *RequestError) {
	modelName := request.ModelName
	inputs := request.Inputs

	// Marshal the input data
	data, err := json.Marshal(inputs)
	if err != nil {
		return nil, NewRequestError(MarshalError,
			errors.New("failed to marshal request"))
	}
	// Send a new prediction request
	url := fmt.Sprintf("http://%s/v1/models/%s:predict", service.address, modelName)
	res, e := service.sendRequest(
		modelName, "POST", url, data,
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

func (service *KServe) Docs(request *DocsRequest) (interface{}, *RequestError) {
	modelName := request.ModelName
	url := fmt.Sprintf("http://%s/v1/docs/%s", service.address, modelName)
	res, e := service.sendRequest(modelName, "GET", url, nil, 10*time.Second)
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

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

type Replicate struct {
	address string
	apikey  string
	modelID string
	timeout int
}

func NewReplicate(config utils.Config) Platform {
	return &Replicate{
		address: config.ReplicateAddress,
		apikey:  config.ReplicateAPIKey,
		modelID: config.ReplicateModelID,
		timeout: config.ReplicateRequestTimeout,
	}
}

func (service *Replicate) sendRequest(
	method string,
	address string,
	body io.Reader,
	timeout time.Duration,
) (*http.Response, *RequestError) {
	// Build a new prediction request
	req, err := http.NewRequest(method, address, body)
	if err != nil {
		return nil, NewRequestError(BuildRequestError,
			errors.New("failed to build request"))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Token %s", service.apikey))

	// Send the prediction request
	client := http.Client{Timeout: timeout}
	res, err := client.Do(req)
	if err != nil {
		return nil, NewRequestError(SendRequestError,
			errors.New("failed to send request, model not ready"))
	}
	if res.StatusCode > 300 {
		var errorMessage interface{}
		data, e := io.ReadAll(res.Body)
		if e == nil {
			_ = json.Unmarshal(data, &errorMessage)
		} else {
			log.Error().Msgf("failed to read error message: %v", e)
		}
		res.Body.Close()
		return nil, NewRequestError(InvalidInputError,
			fmt.Errorf("invalid inputs: %v", errorMessage))
	}
	return res, nil
}

func (service *Replicate) Predict(request *InferRequest, version string) (*InferResponse, *RequestError) {
	inputs := request.Inputs
	delete(inputs, "upload_webhook")
	replicateInput := map[string]interface{}{
		"version": service.modelID,
		"input":   inputs,
	}

	// Marshal the input data
	data, err := json.Marshal(replicateInput)
	if err != nil {
		return nil, NewRequestError(MarshalError,
			errors.New("failed to marshal request"))
	}

	// Send a new prediction request
	res, e := service.sendRequest(
		"POST", service.address, bytes.NewReader(data),
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
	if res.StatusCode >= 300 {
		return nil, NewRequestError(InternalError,
			fmt.Errorf("predict failed: %s", outputs))
	}

	val, ok := outputs["urls"]
	if !ok {
		return nil, NewRequestError(ReadResponseError,
			errors.New("failed to read webhook urls"))
	}
	urls := val.(map[string]interface{})
	url, ok := urls["get"]
	if !ok {
		return nil, NewRequestError(ReadResponseError,
			errors.New("failed to read webhook 'get' url"))
	}
	getURL := fmt.Sprintf("%s", url)

	// Get prediction status
	for i := 0; i < service.timeout; i++ {
		statusResponse, e := service.sendRequest(
			"GET", getURL, nil,
			time.Duration(service.timeout)*time.Second,
		)
		if e != nil {
			return nil, e
		}

		// Parse the prediction status
		defer statusResponse.Body.Close()
		statusBody, err := io.ReadAll(statusResponse.Body)
		if err != nil {
			return nil, NewRequestError(ReadResponseError,
				errors.New("failed to read response body"))
		}
		err = json.Unmarshal(statusBody, &outputs)
		if err != nil {
			return nil, NewRequestError(UnmarshalResponseError,
				errors.New("failed to unmarshal response body"))
		}

		if statusResponse.StatusCode < 400 {
			val, ok := outputs["status"]
			if !ok {
				return nil, NewRequestError(ReadResponseError,
					errors.New("failed to read get prediction status"))
			}
			status := fmt.Sprintf("%s", val)
			if status == "succeeded" {
				metrics := outputs["metrics"].(map[string]interface{})
				req := InferResponse{
					Outputs: map[string]interface{}{
						"output":       outputs["output"],
						"running_time": fmt.Sprintf("%fs", metrics["predict_time"]),
					},
				}
				return &req, nil
			} else if status == "failed" {
				return nil, NewRequestError(InternalError,
					fmt.Errorf("predict failed: %s", outputs))
			}
			time.Sleep(time.Second)

		} else if statusResponse.StatusCode == 429 {
			// Rate limit: Request was throttled
			time.Sleep(2 * time.Second)
		} else {
			return nil, NewRequestError(InternalError,
				fmt.Errorf("predict failed: %s", outputs))
		}
	}
	return nil, NewRequestError(InternalError, errors.New("predict timeout"))
}

func (service *Replicate) Docs(request *DocsRequest) (interface{}, *RequestError) {
	return "", nil
}

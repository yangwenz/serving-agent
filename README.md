# serving-agent

## Setup

Install dependencies:

```shell
go mod tidy
```

Install mockgen:

```shell
go install go.uber.org/mock/mockgen@v0.2.0
go get go.uber.org/mock/mockgen/model@v0.2.0
```

Run tests:

```shell
make test
```

Run on local machine:

```shell
make server
```

## Overview

This is the key component of our ML service. The system for model serving has two layers:

1. The serving platform, e.g., KServe, Replicate, RunPod or Kubernetes deployment.
2. The serving agent (this repo) on GKE or EKS.

The role of a serving agent is to offer sync/async prediction APIs, redirect the requests to the underlying
ML platforms (either KServe or Replicate), and provide task queues for long-running predictions.
The agent requires a Redis or Redis cluster for the task queue, and the serving webhook implemented in this
[repo](https://github.com/yangwenz/serving-webhook) for updating prediction status and results.

### Sync API

The serving agent provides both Sync and Async APIs. For the Sync prediction API, when the agent receives
the request, it will do the followings:

1. Check if the request format is valid.
2. Create a new task record via the serving webhook.
3. Send the request to the underlying ML platform and wait for the prediction results.
4. If the prediction succeeded, update the task record via the serving webhook, and return the results.
5. If the prediction failed, update the task status to `failed`.
6. If any webhook call failed, return the error.

### Async API

For the Async prediction API, we utilize the asynq lib. For the asynq client:

1. Check if the request format is valid.
2. Check if the task queue is full. If the queue is full, return the error.
3. Create a new task record via the serving webhook.
4. Submit the prediction task to the asynq task queue.
5. Return the task ID if all these steps succeeded.
6. If any step failed, it will update the task record status to `failed` and return the error.

For the asynq server:

1. Pull a task from the task queue and send it to one available worker.
2. The worker verifies the payload format.
3. The worker changes the task status to `running`.
4. The worker sends the request to the underlying ML platform and waits for the prediction results.
5. If the prediction succeeded, update the task record via the serving webhook.
6. If the prediction failed, update the task status to `failed`.
7. If any webhook call failed, raise an error and retry the task in the future.

### Graceful Termination 

The asynq queue can either use a redis in local memory or a redis cluster on GCP, which depends on
whether high availability is required. When an agent is terminated by k8s (either doing upgrading or rescheduling),
we should shut down the agent service in different ways according to the redis type:

1. If we use a redis cluster on GCP, we just need to call `Shutdown` of `RedisTaskProcessor`. This method
   will wait for the active running task to be finished and shut down the asynq server. If the running task
   doesn't finish in `TASK_TIMEOUT` seconds, it will be put back to the queue.
2. If we use a local redis, we call `ShutdownDistributor`.
   `ShutdownDistributor` will wait for `SHUTDOWN_DELAY` seconds first, and then wait for the active running task 
   to be finished and set the other tasks (scheduled, pending or retry) in the queue to `failed`.

Because of this termination issue, we need to set `terminationGracePeriodSeconds` to a proper value, i.e., 
greater than `SHUTDOWN_DELAY`.

### Node Failure and Unknown Errors

If the agent or local redis fails, it is possible that some tasks remain "pending/running" stored in the database. 
This can happen when task creation succeeds but the other steps are not executed
due to node failure or other unknown errors. This happens rarely, but it's still an issue needed to be
resolved. To handle this issue, we do the following checks:
1. The service will check if there exists archived tasks in the queue periodically (every 10mins). 
   If some tasks are archived, their status will be set to `failed`.
2. Before starting the service, we will check if there are `pending` or `running` tasks recorded in the database
   by calling `InitCheck` in the `worker` package. This function will handle remaining `pending` or `running` tasks.

## API Definition

The key APIs:

|        API        |       Description        | Method |              Input data (JSON format)               |
:-----------------:|:------------------------:|:------:|:---------------------------------------------------:
|    /v1/predict    | The sync prediction API  |  POST  | {"model_name": "model", "inputs": {<MODEL_INPUTS>}} |
| /async/v1/predict | The async prediction API |  POST  | {"model_name": "model", "inputs": {<MODEL_INPUTS>}} |
|    /task/{ID}     | Get the task information |  GET   |                         NA                          |
|   /cancel/{ID}    |  Cancel a pending task   |  POST  |                         NA                          |

## Parameter Settings

Here are the key parameters:

|       Parameter        |                               Description                               |          Sample value          |
:----------------------:|:-----------------------------------------------------------------------:|:------------------------------:
|  HTTP_SERVER_ADDRESS   |               The TCP address for the server to listen on               |          0.0.0.0:8000          |
|     REDIS_ADDRESS      |                   The redis server address for Asynq                    |          0.0.0.0:6379          |
|   REDIS_CLUSTER_MODE   |                      Whether it is a redis cluster                      |             False              |
|   WORKER_CONCURRENCY   |                     The number of workers for Asynq                     |          8,64 or more          |
|     MAX_QUEUE_SIZE     |        The maximum number of scheduled, pending and retry tasks         |               10               |
|      ML_PLATFORM       |                        Which ML platform to use                         | kserve, k8s, replicate, runpod |
| WEBHOOK_SERVER_ADDRESS |                       The serving webhook address                       |         0.0.0.0:12000          |
| UPLOAD_WEBHOOK_ADDRESS |                The webhook for uploading images or files                |         0.0.0.0:12000          |
|     SHUTDOWN_DELAY     | The server will wait for SHUTDOWN_DELAY seconds after receiving SIGTERM |              340               |
|      TASK_TIMEOUT      |                    The timeout of a prediction task                     |              320               |

The followings are the other parameters depending on which ML platform to use. For KServe:

|       Parameter        |                Description                | Sample value |
:----------------------:|:-----------------------------------------:|:------------:
|     KSERVE_ADDRESS     |            The KServe address             | 0.0.0.0:8080 |
|  KSERVE_CUSTOM_DOMAIN  |             The custom domain             | example.com  |
|    KSERVE_NAMESPACE    | The namespace where the model is deployed |   default    |
| KSERVE_REQUEST_TIMEOUT |   The timeout for a prediction request    |     180      |

For Replicate:

|         Parameter         |             Description              |               Sample value               |
:-------------------------:|:------------------------------------:|:----------------------------------------:
|     REPLICATE_ADDRESS     |      The Replicate API address       | https://api.replicate.com/v1/predictions |
|     REPLICATE_APIKEY      |        The Replicate API key         |                  xxxxx                   |
|    REPLICATE_MODEL_ID     |             The model ID             |                  xxxxx                   |
| REPLICATE_REQUEST_TIMEOUT | The timeout for a prediction request |                   180                    |

For RunPod:

|       Parameter        |             Description              |       Sample value       |
:----------------------:|:------------------------------------:|:------------------------:
|     RUNPOD_ADDRESS     |        The RunPod API address        | https://api.runpod.ai/v2 |
|     RUNPOD_APIKEY      |          The RunPod API key          |          xxxxx           |
|    RUNPOD_MODEL_ID     |             The model ID             |          xxxxx           |
| RUNPOD_REQUEST_TIMEOUT | The timeout for a prediction request |           180            |

For K8S deployment:

|         Parameter         |             Description              | Sample value |
:-------------------------:|:------------------------------------:|:------------:
|     K8SPLUGIN_ADDRESS     |    The k8s serving plugin address    | 0.0.0.0:8002 |
| K8SPLUGIN_REQUEST_TIMEOUT | The timeout for a prediction request |     180      |

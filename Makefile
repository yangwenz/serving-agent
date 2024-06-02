
server:
	go run main.go

test:
	go test -v -cover -short ./...

mock:
	mockgen -package mockplatform -destination platform/mock/platform.go github.com/HyperGAI/serving-agent/platform Platform
	mockgen -package mockwk -destination worker/mock/distributor.go github.com/HyperGAI/serving-agent/worker TaskDistributor
	mockgen -package mockplatform -destination platform/mock/webhook.go github.com/HyperGAI/serving-agent/platform Webhook,Fetcher

docker:
	docker build --platform=linux/amd64 -t yangwenz/serving-agent:latest .
	docker push yangwenz/serving-agent:latest

redis:
	service redis stop
	docker run --name redis -p 6379:6379 -d redis:7.2-alpine

dropredis:
	docker stop redis
	docker container rm redis

.PHONY: server test mock docker redis dropredis

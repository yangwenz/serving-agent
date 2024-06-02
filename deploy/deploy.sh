export IMAGE_VERSION="v1.4.5"
export PLATFORM=kserve
export MODEL_NAME=hyperbooth-prod-slow

export REDIS_ADDRESS="0.0.0.0:6379"
export USE_LOCAL_REDIS=true
export SHUTDOWN_DELAY=2
export WORKER_CONCURRENCY=4
export MAX_QUEUE_SIZE=50
export WEBHOOK_SERVER_ADDRESS="serving-webhook-service.default.svc.cluster.local:12000"

# For the PoC cluster
export KSERVE_ADDRESS="103.179.204.64"
export KSERVE_NAMESPACE="wzyang-restricted"
export UPLOAD_WEBHOOK_ADDRESS="serving-webhook-service.wzyang-restricted.svc.cluster.local:12000"

# For the GKE cluster
# export KSERVE_ADDRESS="istio-ingressgateway-internal.istio-system.svc.cluster.local"
# export KSERVE_NAMESPACE="default"
# export UPLOAD_WEBHOOK_ADDRESS="serving-webhook-service.default.svc.cluster.local:12000"

# For the H100 cluster
# export KSERVE_ADDRESS="210.87.104.32:8001"
# export KSERVE_NAMESPACE="prod"
# export UPLOAD_WEBHOOK_ADDRESS="serving-webhook-service.prod.svc.cluster.local:12000"

export REPLICATE_APIKEY=""
export REPLICATE_MODEL_ID=""

cat deployment.template.yaml | \
  sed -e "s/{{IMAGE_VERSION}}/${IMAGE_VERSION}/g" | \
  sed -e "s/{{PLATFORM}}/${PLATFORM}/g" | \
  sed -e "s/{{REDIS_ADDRESS}}/${REDIS_ADDRESS}/g" | \
  sed -e "s/{{USE_LOCAL_REDIS}}/'${USE_LOCAL_REDIS}'/g" | \
  sed -e "s/{{MODEL_NAME}}/${MODEL_NAME}/g" | \
  sed -e "s/{{SHUTDOWN_DELAY}}/'${SHUTDOWN_DELAY}'/g" | \
  sed -e "s/{{WORKER_CONCURRENCY}}/'${WORKER_CONCURRENCY}'/g" | \
  sed -e "s/{{MAX_QUEUE_SIZE}}/'${MAX_QUEUE_SIZE}'/g" | \
  sed -e "s/{{WEBHOOK_SERVER_ADDRESS}}/'${WEBHOOK_SERVER_ADDRESS}'/g" | \
  sed -e "s/{{KSERVE_ADDRESS}}/'${KSERVE_ADDRESS}'/g" | \
  sed -e "s/{{KSERVE_NAMESPACE}}/'${KSERVE_NAMESPACE}'/g" | \
  sed -e "s/{{UPLOAD_WEBHOOK_ADDRESS}}/'${UPLOAD_WEBHOOK_ADDRESS}'/g" | \
  sed -e "s/{{REPLICATE_APIKEY}}/'${REPLICATE_APIKEY}'/g" | \
  sed -e "s/{{REPLICATE_MODEL_ID}}/'${REPLICATE_MODEL_ID}'/g" > deployment.yaml

cat service.template.yaml | \
  sed -e "s/{{MODEL_NAME}}/${MODEL_NAME}/g" > service.yaml

cat monitoring.template.yaml | \
  sed -e "s/{{PLATFORM}}/${PLATFORM}/g" | \
  sed -e "s/{{MODEL_NAME}}/${MODEL_NAME}/g" > monitoring.yaml

# kubectl apply -f deployment.yaml
# kubectl apply -f service.yaml
# kubectl apply -f monitoring.yaml
# rm -rf deployment.yaml
# rm -rf service.yaml
# rm -rf monitoring.yaml

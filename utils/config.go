package utils

import (
	"github.com/spf13/viper"
)

// Config stores all configuration of the application.
// The values are read by viper from a config file or environment variable.
type Config struct {
	Environment          string `mapstructure:"ENVIRONMENT"`
	HTTPServerAddress    string `mapstructure:"HTTP_SERVER_ADDRESS"`
	RedisAddress         string `mapstructure:"REDIS_ADDRESS"`
	RedisClusterMode     bool   `mapstructure:"REDIS_CLUSTER_MODE"`
	UseLocalRedis        bool   `mapstructure:"USE_LOCAL_REDIS"`
	ShutdownDelay        int    `mapstructure:"SHUTDOWN_DELAY"`
	WorkerConcurrency    int    `mapstructure:"WORKER_CONCURRENCY"`
	MaxQueueSize         int    `mapstructure:"MAX_QUEUE_SIZE"`
	TaskTypeName         string `mapstructure:"TASK_TYPE_NAME"`
	TaskTimeout          int    `mapstructure:"TASK_TIMEOUT"`
	ModelName            string `mapstructure:"MODEL_NAME"`
	WebhookServerAddress string `mapstructure:"WEBHOOK_SERVER_ADDRESS"`
	WebhookAPIKey        string `mapstructure:"WEBHOOK_APIKEY"`
	MLPlatform           string `mapstructure:"ML_PLATFORM"`
	UploadWebhookAddress string `mapstructure:"UPLOAD_WEBHOOK_ADDRESS"`
	EnablePeriodicCheck  bool   `mapstructure:"ENABLE_PERIODIC_CHECK"`
	// KServe
	KServeAddress        string `mapstructure:"KSERVE_ADDRESS"`
	KServeCustomDomain   string `mapstructure:"KSERVE_CUSTOM_DOMAIN"`
	KServeNamespace      string `mapstructure:"KSERVE_NAMESPACE"`
	KServeRequestTimeout int    `mapstructure:"KSERVE_REQUEST_TIMEOUT"`
	// Replicate
	ReplicateAddress        string `mapstructure:"REPLICATE_ADDRESS"`
	ReplicateAPIKey         string `mapstructure:"REPLICATE_APIKEY"`
	ReplicateModelID        string `mapstructure:"REPLICATE_MODEL_ID"`
	ReplicateRequestTimeout int    `mapstructure:"REPLICATE_REQUEST_TIMEOUT"`
	// RunPod
	RunPodAddress        string `mapstructure:"RUNPOD_ADDRESS"`
	RunPodAPIKey         string `mapstructure:"RUNPOD_APIKEY"`
	RunPodModelID        string `mapstructure:"RUNPOD_MODEL_ID"`
	RunPodRequestTimeout int    `mapstructure:"RUNPOD_REQUEST_TIMEOUT"`
	// K8s deployment
	K8sPluginAddress        string `mapstructure:"K8SPLUGIN_ADDRESS"`
	K8sPluginRequestTimeout int    `mapstructure:"K8SPLUGIN_REQUEST_TIMEOUT"`
}

// LoadConfigs reads configuration from file or environment variables.
func LoadConfigs(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("app")
	viper.SetConfigType("env")

	viper.AutomaticEnv()
	err = viper.ReadInConfig()
	if err != nil {
		return
	}

	err = viper.Unmarshal(&config)
	return
}

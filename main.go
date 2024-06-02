package main

import (
	"context"
	"fmt"
	"github.com/HyperGAI/serving-agent/api"
	"github.com/HyperGAI/serving-agent/platform"
	"github.com/HyperGAI/serving-agent/utils"
	"github.com/HyperGAI/serving-agent/worker"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/sys/unix"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	utils.InitZerolog()
	config, err := utils.LoadConfigs(".")
	if err != nil {
		log.Fatal().Err(err).Msg("cannot load config")
	}
	if config.Environment == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
	PreCheck(config)

	// Initialize ML platform service
	var service platform.Platform
	if config.MLPlatform == "kserve" {
		log.Info().Msg(fmt.Sprintf("using KServe platform: %s", config.KServeAddress))
		service = platform.NewKServe(config)
	} else if config.MLPlatform == "replicate" {
		log.Info().Msg(fmt.Sprintf("using Replicate platform: %s, %s",
			config.ReplicateAddress, config.ReplicateModelID))
		service = platform.NewReplicate(config)
	} else if config.MLPlatform == "runpod" {
		log.Info().Msg(fmt.Sprintf("using RunPod platform: %s, %s",
			config.RunPodAddress, config.RunPodModelID))
		service = platform.NewRunPod(config)
	} else if config.MLPlatform == "k8s" || config.MLPlatform == "k8s-plugin" {
		log.Info().Msg(fmt.Sprintf("using k8s deployment: %s", config.K8sPluginAddress))
		service = platform.NewK8sPlugin(config)
	} else {
		log.Fatal().Msg("ML platform is not set")
	}

	webhook := platform.NewInternalWebhook(config)
	distributor := worker.NewRedisTaskDistributor(config)
	/*
		// Start task processor
		go runTaskProcessor(config, service, webhook)
		// Start model API server
		runGinServer(config, service, distributor, webhook)
	*/
	runServer(config, service, distributor, webhook)
}

func PreCheck(config utils.Config) {
	if config.MaxQueueSize < 1 {
		log.Fatal().Msg("MaxQueueSize must be > 0")
	}
	if config.TaskTimeout < config.KServeRequestTimeout ||
		config.TaskTimeout < config.K8sPluginRequestTimeout ||
		config.TaskTimeout < config.ReplicateRequestTimeout ||
		config.TaskTimeout < config.RunPodRequestTimeout {
		log.Fatal().Msg("timeout setting error: TaskTimeout must be >= [Platform]RequestTimeout")
	}
}

func runGinServer(
	config utils.Config,
	platform platform.Platform,
	distributor worker.TaskDistributor,
	webhook platform.Webhook,
) {
	server, err := api.NewServer(config, platform, distributor, webhook)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot create server")
	}
	err = server.Start(config.HTTPServerAddress)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot start server")
	}
}

func runTaskProcessor(config utils.Config, platform platform.Platform, webhook platform.Webhook) {
	if config.RedisAddress == "" {
		log.Fatal().Msg("redis address is not set")
	}
	taskProcessor := worker.NewRedisTaskProcessor(config, platform, webhook)
	log.Info().Msg("start task processor")
	err := taskProcessor.Start()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to start task processor")
	}
}

func runServer(
	config utils.Config,
	platform platform.Platform,
	distributor worker.TaskDistributor,
	webhook platform.Webhook,
) {
	// Start the Gin server
	server, err := api.NewServer(config, platform, distributor, webhook)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot create server")
	}
	httpServer := &http.Server{
		Addr:    config.HTTPServerAddress,
		Handler: server.Handler(),
	}
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("cannot start server")
		}
	}()

	// Check task queue size
	go func() {
		server.CheckQueueSize()
	}()

	// Start the Asynq server
	if config.RedisAddress == "" {
		log.Fatal().Msg("redis address is not set")
	}
	taskProcessor := worker.NewRedisTaskProcessor(config, platform, webhook)
	log.Info().Msg("start task processor")
	go func() {
		if err := taskProcessor.Start(); err != nil {
			log.Fatal().Err(err).Msg("failed to start task processor")
		}
	}()

	// Start checking the archived tasks
	go func() {
		for {
			if config.EnablePeriodicCheck {
				api.PeriodicCheck(config, distributor, webhook)
			}
			time.Sleep(30 * time.Minute)
		}
	}()

	// Shutdown the Asynq server
	// https://pkg.go.dev/github.com/hibiken/asynq#example-Server.Shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, unix.SIGTERM, unix.SIGINT, unix.SIGTSTP)
	// Handle SIGTERM, SIGINT to exit the program.
	// Handle SIGTSTP to stop processing new tasks.
	for {
		s := <-sigs
		if s == unix.SIGTSTP {
			taskProcessor.Stop() // stop processing new tasks
			continue
		}
		break // received SIGTERM or SIGINT signal
	}
	// If not in redis cluster mode and use local redis, run the full shutdown
	if !config.RedisClusterMode && config.UseLocalRedis {
		if config.ShutdownDelay > 0 {
			// Wait for `ShutdownDelay` seconds
			log.Info().Msgf("waiting for %d seconds", config.ShutdownDelay)
			time.Sleep(time.Duration(config.ShutdownDelay) * time.Second)
		}
		worker.ShutdownDistributor(distributor, webhook)
	}
	taskProcessor.Shutdown()

	// Shutdown the Gin server
	// https://gin-gonic.com/docs/examples/graceful-restart-or-stop/
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("server shutdown")
	}
	// catching ctx.Done(). timeout of 5 seconds.
	select {
	case <-ctx.Done():
		log.Info().Msg("timeout of 5 seconds")
	}
	log.Info().Msg("server exiting")
}

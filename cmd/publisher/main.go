package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"live-1/internal/backend"
	"live-1/internal/config"
	"live-1/internal/device"
	sessions "live-1/internal/session"
	"live-1/internal/whip"
)

func main() {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})

	cfg, err := config.Load()
	if err != nil {
		logger.WithError(err).Fatal("load configuration")
	}

	logger.WithFields(logrus.Fields{
		"client_id":     cfg.ClientID,
		"backend_url":   cfg.BackendURL,
		"whip_base":     cfg.WhipBaseURL,
		"poll_interval": cfg.PollInterval,
	}).Info("starting publisher client")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	httpClient := &http.Client{Timeout: cfg.HTTPTimeout}

	backendClient, err := backend.NewClient(cfg.BackendURL, httpClient)
	if err != nil {
		logger.WithError(err).Fatal("create backend client")
	}

	manifest := os.Getenv("DEVICES_MANIFEST")
	includeDesktop := !strings.EqualFold(os.Getenv("DISABLE_DESKTOP"), "1") &&
		!strings.EqualFold(os.Getenv("DISABLE_DESKTOP"), "true")

	devices, err := device.Discover(ctx, manifest, includeDesktop)
	if err != nil {
		logger.WithError(err).Fatal("enumerate devices")
	}

	if len(devices) == 0 {
		logger.Fatal("no devices available to publish")
	}

	logger.WithField("count", len(devices)).Info("registering devices")
	if err := backendClient.Register(ctx, backend.RegisterRequest{ClientID: cfg.ClientID, Devices: devices}); err != nil {
		logger.WithError(err).Fatal("register client")
	}

	whipManager := whip.NewManager(cfg.WhipBaseURL, cfg.WhipSecret, cfg.StreamKeys, httpClient, logger)
	sessionManager := sessions.NewManager(whipManager, logger)

	run(ctx, logger, cfg, backendClient, sessionManager)
}

func run(ctx context.Context, logger *logrus.Logger, cfg config.Config, backendClient *backend.Client, sessionManager *sessions.Manager) {
	logger.Info("entering command polling loop")
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutdown requested; stopping active tracks")
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			sessionManager.StopAll(stopCtx)
			cancel()
			return
		case <-ticker.C:
			pollCtx, cancel := backend.WithLongPollTimeout(ctx, cfg.PollInterval)
			command, err := backendClient.PollCommand(pollCtx, cfg.ClientID)
			cancel()
			if err != nil {
				if errors.Is(err, context.Canceled) {
					continue
				}
				logger.WithError(err).Warn("poll command failed")
				continue
			}

			if command == nil {
				continue
			}

			logger.WithField("command", command.Command).Info("received command")
			switch command.Command {
			case "start":
				sessionManager.StartTracks(ctx, command.Tracks)
			case "stop":
				stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				sessionManager.StopAll(stopCtx)
				cancel()
			default:
				logger.WithField("command", command.Command).Warn("unknown command received")
			}
		}
	}
}

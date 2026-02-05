package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"majmun/internal/config"
	"majmun/internal/logging"
	"majmun/internal/server"
)

func main() {
	ctx := context.Background()
	configPath := flag.String("config", "config.yaml", "path to configuration (file or dir)")
	flag.Parse()

	logging.Info(ctx, "starting majmun", "config_path", *configPath)

	c, err := config.Load(*configPath)
	if err != nil {
		logging.Error(ctx, err, "failed to load config")
		os.Exit(1)
	}

	logging.SetLevelAndFormat(c.Logs.Level, c.Logs.Format)

	s, err := server.NewServer(c)
	if err != nil {
		logging.Error(ctx, err, "failed to create server")
		os.Exit(1)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := s.Start(); err != nil {
			logging.Error(ctx, err, "server error")
			os.Exit(1)
		}
	}()

	<-stop
	if err := s.Stop(); err != nil {
		logging.Error(ctx, err, "error during shutdown")
		os.Exit(1)
	}
}

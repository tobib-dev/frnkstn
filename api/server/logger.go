package main

import (
	"log/slog"
	"os"
)

func initializeLogger(_ string) (*slog.Logger, error) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	return logger, nil 
} 

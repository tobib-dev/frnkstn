package main

import (
	"io"
	"log/slog"
	"os"
)

func initializeLogger(logFile string) (*slog.Logger, *os.File, error) {
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return &slog.Logger{}, &os.File{}, err
	}
	mw := io.MultiWriter(os.Stdout, file)
	logger := slog.New(slog.NewJSONHandler(mw, nil))
	slog.SetDefault(logger)

	return logger, file, nil
}

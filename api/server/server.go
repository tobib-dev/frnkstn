package main

import (
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"strconv"

	sessionsV1 "github.com/tobib-dev/frnkstn/api/proto/sessions/v1"
	usersV1 "github.com/tobib-dev/frnkstn/api/proto/users/v1"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

type Config struct {
	server         GRPCServerConfig
	db             DBConfig
	logger         *slog.Logger
	userService    UserConfig
	sessionService SessionConfig
}

type GRPCServerConfig struct {
	port int
}

type DBConfig struct {
	dbURL string
}

func main() {
	fmt.Println("Testing the grpc server")

	godotenv.Load("../../.env")
	logFile := os.Getenv("LOGFILE")
	dbPort := os.Getenv("DB_PORT")
	apiPortString := os.Getenv("API_PORT")

	apiPort, err := strconv.Atoi(apiPortString)
	if err != nil {
		log.Fatalf("failed to parse API Port: %v", err)
	}

	logger, err := initializeLogger(logFile)
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}

	cfg := Config{
		server: GRPCServerConfig{
			port: apiPort,
		},
		db: DBConfig{
			dbURL: "127.0.0.1" + dbPort,
		},
		logger: logger,
	}
	lis, err := net.Listen("tcp", fmt.Sprintf(":%v", cfg.server.port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	s := grpc.NewServer()
	usersV1.RegisterUserServiceServer(s, &UserConfig{})
	sessionsV1.RegisterSessionServiceServer(s, &SessionConfig{})
	logger.Info("frnkstn started", "port", cfg.server.port, "environment", "dev")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

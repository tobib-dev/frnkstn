package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"strconv"

	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
	usersV1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	"github.com/tobib-dev/frnkstn/api/db"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

type Config struct {
	server GRPCServerConfig
	db     DBConfig
	logger *slog.Logger
}

type GRPCServerConfig struct {
	port int
}

type DBConfig struct {
	dbURL   string
	session db.DB
}

func main() {
	godotenv.Load("../.env")
	logFile := os.Getenv("LOGFILE")
	dbPort := os.Getenv("DB_PORT")
	apiPortString := os.Getenv("API_PORT")

	apiPort, err := strconv.Atoi(apiPortString)
	if err != nil {
		log.Fatalf("failed to parse API Port: %v", err)
	}

	cfg := Config{
		server: GRPCServerConfig{
			port: apiPort,
		},
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%v", cfg.server.port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	// Initialize logger
	logger, err := initializeLogger(logFile)
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	cfg.logger = logger

	// Start DB connection pool
	dbUrl := "localhost:" + dbPort
	db, err := db.New(dbUrl)
	if err != nil {
		logger.Error("failed to start Scylla DB session", "error", err)
	}
	defer db.Close()

	// Run database migrations
	if err := db.Migrate(
		context.Background(),
	); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}
	cfg.db = DBConfig{
		dbURL:   dbUrl,
		session: *db,
	}

	server := grpc.NewServer()

	// Inject DB into services and register services
	userService := NewUserService(&cfg)
	sessionService := NewSessionService(&cfg)

	usersV1.RegisterUserServiceServer(server, userService)
	sessionsV1.RegisterSessionServiceServer(server, sessionService)

	logger.Info("frnkstn started", "port", cfg.server.port, "environment", "dev")
	if err := server.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

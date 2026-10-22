package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"strconv"

	dmsV1 "github.com/tobib-dev/frnkstn-proto/dms/v1"
	friendsV1 "github.com/tobib-dev/frnkstn-proto/friends/v1"
	ntfnV1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
	usersV1 "github.com/tobib-dev/frnkstn-proto/users/v1"

	"github.com/tobib-dev/frnkstn/api/db"
	"github.com/tobib-dev/frnkstn/internal/rpclog"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

type Config struct {
	server     GRPCServerConfig
	db         DBConfig
	brokerURI  string
	signingKey string
	logger     *slog.Logger
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
	jwtKey := os.Getenv("SIGNING_KEY")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	apiPortString := os.Getenv("API_PORT")
	queueBrokerURI := os.Getenv("RABBITMQ_BROKER_URI")

	apiPort, err := strconv.Atoi(apiPortString)
	if err != nil {
		log.Fatalf("failed to parse API Port: %v", err)
	}

	cfg := Config{
		server: GRPCServerConfig{
			port: apiPort,
		},
		brokerURI:  queueBrokerURI,
		signingKey: jwtKey,
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%v", cfg.server.port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	// Initialize logger
	logger, logfile, err := initializeLogger(logFile)
	if err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
	}
	defer logfile.Close()
	cfg.logger = logger

	// Start DB connection pool
	dbUrl := "localhost:" + dbPort
	db, err := db.New(dbUrl, dbUser, dbPassword)
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

	server := grpc.NewServer(grpc.StatsHandler(rpclog.Handler{Message: "api request completed", Log: logger.Info}))

	// Inject DB into services and register services
	userService := NewUserService(&cfg)
	sessionService := NewSessionService(&cfg)
	friendshipService := NewFriendService(&cfg)
	notificationService := NewNotificationService(&cfg)
	if err := notificationService.StartNotificationConsumer(context.Background()); err != nil {
		logger.Error("failed to subscribe to message notifications", "error", err)
	}
	directMessageService := NewDMService(&cfg)

	usersV1.RegisterUserServiceServer(server, userService)
	sessionsV1.RegisterSessionServiceServer(server, sessionService)
	friendsV1.RegisterFriendServiceServer(server, friendshipService)
	dmsV1.RegisterDMServiceServer(server, directMessageService)
	ntfnV1.RegisterN10NServiceServer(server, notificationService)

	logger.Info("frnkstn started", "port", cfg.server.port, "environment", "dev")
	if err := server.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

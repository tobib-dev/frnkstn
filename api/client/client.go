package main

import (
	"context"
	"flag"
	"log"
	"time"

	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
	usersV1 "github.com/tobib-dev/frnkstn-proto/users/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	tls                = flag.Bool("tls", false, "Connection uses TLS if true, else plain TCP")
	caFile             = flag.String("ca_file", "", "The file containing the CA root cert file")
	serverAddr         = flag.String("addr", "localhost:7789", "The server address in the format of host:port")
	serverHostOverride = flag.String("server_host_override", "x.test.example.com", "The server name used to verify the hostname returned by the TLS handshake")
)

func createUser(client usersV1.UserServiceClient, guest *usersV1.CreateUserRequest) {
	log.Printf("Creating user {%s}...\n", guest.Name)
	user, err := client.CreateUser(context.Background(), guest)
	if err != nil {
		log.Fatalf("client.createUser failed: %v", err)
	}
	log.Println(user)
}

func createSession(client sessionsV1.SessionServiceClient, session *sessionsV1.CreateSessionRequest) {
	log.Printf("Creating session {%s}...\n", session.AccessToken)
	sess, err := client.CreateSession(context.Background(), session)
	if err != nil {
		log.Printf("client.createSession failed: %v", err)
	}
	log.Println(sess)
}

func main() {
	flag.Parse()

	conn, err := grpc.NewClient(*serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	sessClient := sessionsV1.NewSessionServiceClient(conn)
	createSession(sessClient,
		&sessionsV1.CreateSessionRequest{
			AccessToken:           "access_token",
			RefreshToken:          "refresh_token",
			RefreshTokenExpiresAt: time.Now().Add(time.Minute).String(),
		},
	)
	userClient := usersV1.NewUserServiceClient(conn)
	createUser(userClient,
		&usersV1.CreateUserRequest{
			Name:     "tobi",
			Username: "testuser",
		},
	)
}

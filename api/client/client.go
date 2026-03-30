package main

import (
	"context"
	"flag"
	"log"

	pb "frnkstn/api/proto/users/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	tls                = flag.Bool("tls", false, "Connection uses TLS if true, else plain TCP")
	caFile             = flag.String("ca_file", "", "The file containing the CA root cert file")
	serverAddr         = flag.String("addr", "localhost:7789", "The server address in the format of host:port")
	serverHostOverride = flag.String("server_host_override", "x.test.example.com", "The server name used to verify the hostname returned by the TLS handshake")
)

func createUser(client pb.UserServiceClient, guest *pb.Guest) {
	log.Printf("Creating user {%s}...\n", guest.Email)
	user, err := client.CreateUser(context.Background(), guest)
	if err != nil {
		log.Fatalf("client.createUser failed: %v", err)
	}
	log.Println(user)
}

func main() {
	flag.Parse()

	conn, err := grpc.NewClient(*serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	client := pb.NewUserServiceClient(conn)
	createUser(client, &pb.Guest{Email: "test@example.com", Name: "Test User", GitUsername: "testuser"})
}

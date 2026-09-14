package main

import (
	"fmt"
	"log"
	"net"

	pb "github.com/tobib-dev/frnkstn/api/proto/users/v1"

	//"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

type Config struct {
	Server      GRPCServerConfig
	DB          DBConfig
	UserService UserConfig
}

type GRPCServerConfig struct {
	Port int
}

type DBConfig struct {
	DBURL string
}

func main() {
	fmt.Println("Testing the grpc server")

	//port := os.Getenv("API_PORT")
	port := "7789"
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer lis.Close()

	s := grpc.NewServer()
	pb.RegisterUserServiceServer(s, &UserConfig{})
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

package main

import (
	"context"
	"fmt"
	"log"
	"net"

	pb "frnkstn/gen/proto/users/v1"

	//"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

// Sample user server
type userServer struct {
	pb.UnimplementedUserServiceServer
	savedUsers []*pb.User
}

func (u *userServer) CreateUser(ctx context.Context, guest *pb.Guest) (*pb.User, error) {
	for _, us := range u.savedUsers {
		if guest.Email == us.Email || guest.GitUsername == us.GitUsername {
			log.Printf("Email: %s or git profile: %s already exists!!!", guest.Email, guest.GitUsername)
			return &pb.User{}, nil
		}
	}

	user := &pb.User{
		Name:        guest.Name,
		Email:       guest.Email,
		GitUsername: guest.GitUsername,
	}
	u.savedUsers = append(u.savedUsers, user)
	return user, nil
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
	pb.RegisterUserServiceServer(s, &userServer{})
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

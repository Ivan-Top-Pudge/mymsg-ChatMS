package chat

import (
	chatv1 "github.com/Ivan-Top-Pudge/mymsg-protos/gen/go/chat"
	"google.golang.org/grpc"
)

// TODO: Chat interface
type Chat interface{}

type serverAPI struct {
	chatv1.UnimplementedChatServer
	chat Chat
}

func Register(gRPC *grpc.Server, chat Chat) {
	chatv1.RegisterChatServer(gRPC, &serverAPI{chat: chat})
}

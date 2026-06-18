package chat

import (
	"context"
	"errors"

	"chat/internal/delivery/grpc/interceptors"
	"chat/internal/domain/models"
	chatservice "chat/internal/services/chat"

	chatv1 "github.com/Ivan-Top-Pudge/mymsg-protos/gen/go/chat"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Это интерфейс для функций бизнес логики, который описывает контракт для gRPC
type Chat interface {
	CreateChat(ctx context.Context, members []int64) (int64, error)
	DeleteChat(ctx context.Context, chatID int64) error

	SendMessage(ctx context.Context, chatID int64, senderID int64, text string) (int64, error)
	DeleteMessage(ctx context.Context, msgID int64, chatID int64, requestorID int64) error

	GetChatHistory(ctx context.Context, chatID int64, requestorID int64, limit int64, offset int64) ([]models.Message, error)
}

type serverAPI struct {
	chatv1.UnimplementedChatServer
	chat Chat
}

func Register(gRPC *grpc.Server, chat Chat) {
	chatv1.RegisterChatServer(gRPC, &serverAPI{chat: chat})
}

// ---Handlers for grpc calls---

func (s *serverAPI) CreateChat(
	ctx context.Context,
	req *chatv1.CreateChatRequest,
) (*chatv1.CreateChatResponse, error) {
	if err := validateCreateChat(req); err != nil {
		return nil, err
	}

	chatID, err := s.chat.CreateChat(ctx, req.Members)
	if err != nil {
		if errors.Is(err, chatservice.ErrUserNotFound) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &chatv1.CreateChatResponse{ChatId: chatID}, nil
}

func (s *serverAPI) DeleteChat(ctx context.Context, req *chatv1.DeleteChatRequest) (*chatv1.DeleteChatResponse, error) {
	if err := validateDeleteChat(req); err != nil {
		return nil, err
	}

	err := s.chat.DeleteChat(ctx, req.ChatId)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &chatv1.DeleteChatResponse{DeletedChatId: req.ChatId}, nil
}

func (s *serverAPI) GetChatHistory(
	ctx context.Context,
	req *chatv1.GetChatHistoryRequest,
) (*chatv1.GetChatHistoryResponse, error) {
	requestorID, ok := interceptors.UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "failed to get user id from context")
	}

	if err := validateGetChatHistory(req); err != nil {
		return nil, err
	}

	messages, err := s.chat.GetChatHistory(ctx, req.ChatId, requestorID, req.Limit, req.Offset)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}

	// Конвертируем доменные модели в структуры Protobuf
	pbMessages := make([]*chatv1.Message, 0, len(messages))
	for _, msg := range messages {
		pbMessages = append(pbMessages, &chatv1.Message{
			Id:        msg.ID,
			ChatId:    msg.ChatID,
			SenderId:  msg.SenderID,
			Text:      msg.Text,
			CreatedAt: msg.CreatedAt.Unix(), // Здесь мы переводим time.Time обратно в int64 для передачи по сети
		})
	}

	return &chatv1.GetChatHistoryResponse{Messages: pbMessages}, nil
}

func (s *serverAPI) SendMessage(
	ctx context.Context,
	req *chatv1.SendMessageRequest,
) (*chatv1.SendMessageResponse, error) {
	senderID, ok := interceptors.UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "failed to get user id from context")
	}

	if err := validateSendMessage(req); err != nil {
		return nil, err
	}

	msgID, err := s.chat.SendMessage(ctx, req.ChatId, senderID, req.Text)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &chatv1.SendMessageResponse{MsgId: msgID}, nil
}

func (s *serverAPI) DeleteMessage(
	ctx context.Context,
	req *chatv1.DeleteMessageRequest,
) (*chatv1.DeleteMessageResponse, error) {
	requestorID, ok := interceptors.UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "failed to get user id from context")
	}

	if err := validateDeleteMessage(req); err != nil {
		return nil, err
	}

	err := s.chat.DeleteMessage(ctx, req.MsgId, req.ChatId, requestorID)
	if err != nil {
		if errors.Is(err, chatservice.ErrPermissionDenied) {
			return nil, status.Error(codes.PermissionDenied, "permission denied")
		}
		return nil, status.Error(codes.Internal, "internal error")
	}

	return &chatv1.DeleteMessageResponse{Success: true}, nil
}

// ---Validators for grpc Handlers---

func validateCreateChat(req *chatv1.CreateChatRequest) error {
	if len(req.Members) < 2 {
		return status.Error(codes.InvalidArgument, "not enough users to create chat")
	}
	return nil
}

func validateDeleteChat(req *chatv1.DeleteChatRequest) error {
	if req.ChatId <= 0 {
		return status.Error(codes.InvalidArgument, "invalid chat id")
	}
	return nil
}

func validateDeleteMessage(req *chatv1.DeleteMessageRequest) error {
	if req.MsgId <= 0 {
		return status.Error(codes.InvalidArgument, "invalid message id")
	}
	if req.ChatId <= 0 {
		return status.Error(codes.InvalidArgument, "invalid chat id")
	}
	return nil
}

func validateSendMessage(req *chatv1.SendMessageRequest) error {
	if req.ChatId <= 0 {
		return status.Error(codes.InvalidArgument, "invalid chat id")
	}
	if req.Text == "" {
		return status.Error(codes.InvalidArgument, "message text cannot be empty")
	}
	return nil
}

func validateGetChatHistory(req *chatv1.GetChatHistoryRequest) error {
	if req.ChatId <= 0 {
		return status.Error(codes.InvalidArgument, "invalid chat id")
	}
	if req.Limit <= 0 {
		return status.Error(codes.InvalidArgument, "limit must be greater than zero")
	}
	if req.Offset < 0 {
		return status.Error(codes.InvalidArgument, "offset cannot be negative")
	}
	return nil
}

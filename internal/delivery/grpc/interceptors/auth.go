package interceptors

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type contextKey string

const UserIDKey contextKey = "userID"

func AuthInterceptor(secret string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		// 1. Достаем метаданные из gRPC запроса
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "metadata is not provided")
		}

		// 2. Ищем заголовок authorization
		values := md["authorization"]
		if len(values) == 0 {
			return nil, status.Error(codes.Unauthenticated, "authorization token is not provided")
		}

		authHeader := values[0]
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			return nil, status.Error(codes.Unauthenticated, "invalid token format")
		}
		tokenString := parts[1]

		// 3. Парсим и валидируем токен
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			// Возвращаем секретный ключ для проверки подписи
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "invalid token claims")
		}

		// 4. Достаем userID (предполагается, что SSO положил его под ключом "uid" при выдаче)
		uidFloat, ok := claims["uid"].(float64)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "user id not found in token")
		}

		// 5. Кладем извлеченный ID в контекст
		newCtx := context.WithValue(ctx, UserIDKey, int64(uidFloat))

		// 6. Передаем управление дальше — твоему хендлеру (уже с новым контекстом)
		return handler(newCtx, req)
	}
}

func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(UserIDKey).(int64)
	return id, ok
}

package chat_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"chat/internal/domain/models"
	chatservice "chat/internal/services/chat" // Замени на свой реальный импорт
	"chat/internal/services/chat/mocks"       // Замени на свой реальный импорт
)

// setupTest инициализирует все моки и возвращает готовый инстанс сервиса Chat.
// Это избавляет нас от дублирования кода в каждом тесте.
func setupTest(t *testing.T) (
	*chatservice.Chat,
	*mocks.MockChatSaver,
	*mocks.MockChatProvider,
	*mocks.MockMessageSaver,
	*mocks.MockMessageProvider,
	*mocks.MockSSOProvider,
	*mocks.MockChatCache,
) {
	ctrl := gomock.NewController(t)

	chatSaver := mocks.NewMockChatSaver(ctrl)
	chatProvider := mocks.NewMockChatProvider(ctrl)
	messageSaver := mocks.NewMockMessageSaver(ctrl)
	messageProvider := mocks.NewMockMessageProvider(ctrl)
	ssoProvider := mocks.NewMockSSOProvider(ctrl)
	chatCache := mocks.NewMockChatCache(ctrl)

	// Используем логгер, который пишет "в никуда", чтобы не засорять консоль при тестах
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	chat := chatservice.New(logger,
		chatSaver,
		chatProvider,
		messageSaver,
		messageProvider,
		ssoProvider,
		chatCache,
	)

	return chat,
		chatSaver,
		chatProvider,
		messageSaver,
		messageProvider,
		ssoProvider,
		chatCache
}

// --- ТЕСТЫ ---

func TestChat_CreateChat(t *testing.T) {
	ctx := context.Background()
	errInternal := errors.New("internal db error")

	tests := []struct {
		name        string
		members     []int64
		mockSetup   func(sso *mocks.MockSSOProvider, saver *mocks.MockChatSaver)
		expectedID  int64
		expectError bool
		errTarget   error
	}{
		{
			name:    "Success",
			members: []int64{1, 2},
			mockSetup: func(sso *mocks.MockSSOProvider, saver *mocks.MockChatSaver) {
				sso.EXPECT().IsUserExists(ctx, int64(1)).Return(true, nil)
				sso.EXPECT().IsUserExists(ctx, int64(2)).Return(true, nil)
				saver.EXPECT().CreateChat(ctx, []int64{1, 2}).Return(int64(42), nil)
			},
			expectedID:  42,
			expectError: false,
		},
		{
			name:    "User Does Not Exist",
			members: []int64{1, 99},
			mockSetup: func(sso *mocks.MockSSOProvider, saver *mocks.MockChatSaver) {
				sso.EXPECT().IsUserExists(ctx, int64(1)).Return(true, nil)
				sso.EXPECT().IsUserExists(ctx, int64(99)).Return(false, nil) // 99 не существует
				// CreateChat НЕ должен быть вызван
				saver.EXPECT().CreateChat(gomock.Any(), gomock.Any()).Times(0)
			},
			expectedID:  0,
			expectError: true,
			errTarget:   chatservice.ErrUserNotFound,
		},
		{
			name:    "Chat Saver Error",
			members: []int64{1},
			mockSetup: func(sso *mocks.MockSSOProvider, saver *mocks.MockChatSaver) {
				sso.EXPECT().IsUserExists(ctx, int64(1)).Return(true, nil)
				saver.EXPECT().CreateChat(ctx, []int64{1}).Return(int64(0), errInternal)
			},
			expectedID:  0,
			expectError: true,
			errTarget:   errInternal,
		},
		{
			name:    "SSO Provider Error",
			members: []int64{1},
			mockSetup: func(sso *mocks.MockSSOProvider, saver *mocks.MockChatSaver) {
				// Имитируем падение сети или базы в SSO
				sso.EXPECT().IsUserExists(ctx, int64(1)).Return(false, errInternal)
				// До базы чатов мы дойти не должны
				saver.EXPECT().CreateChat(gomock.Any(), gomock.Any()).Times(0)
			},
			expectedID:  0,
			expectError: true,
			errTarget:   errInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chat, chatSaver, _, _, _, sso, _ := setupTest(t)
			tt.mockSetup(sso, chatSaver)

			id, err := chat.CreateChat(ctx, tt.members)

			if tt.expectError {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.errTarget)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedID, id)
			}
		})
	}
}

func TestChat_DeleteChat(t *testing.T) {
	ctx := context.Background()
	chatID := int64(1)
	errInternal := errors.New("internal error")

	chat, chatSaver, _, _, _, _, _ := setupTest(t)

	// Сценарий 1: Успех
	chatSaver.EXPECT().DeleteChat(ctx, chatID).Return(nil)
	err := chat.DeleteChat(ctx, chatID)
	require.NoError(t, err)

	// Сценарий 2: Ошибка
	chatSaver.EXPECT().DeleteChat(ctx, chatID).Return(errInternal)
	err = chat.DeleteChat(ctx, chatID)
	require.ErrorIs(t, err, errInternal)
}

func TestChat_SendMessage(t *testing.T) {
	ctx := context.Background()
	errInternal := errors.New("internal error")

	tests := []struct {
		name     string
		chatID   int64
		senderID int64
		text     string
		// Передаем нужные моки в setup
		mockSetup   func(provider *mocks.MockChatProvider, saver *mocks.MockMessageSaver, cache *mocks.MockChatCache)
		expectedID  int64
		expectError bool
		errTarget   error
	}{
		{
			name:     "Success - Cache Hit (Быстрый путь)",
			chatID:   1,
			senderID: 2,
			text:     "hello",
			mockSetup: func(provider *mocks.MockChatProvider, saver *mocks.MockMessageSaver, cache *mocks.MockChatCache) {
				// 1. Кэш сразу отвечает: "Пользователь есть в чате"
				cache.EXPECT().CheckChatMember(ctx, int64(1), int64(2)).Return(true, nil)

				// ВАЖНО: Мы НЕ ожидаем вызова БД (provider.IsChatMember) и записи в кэш!

				// 2. Сразу сохраняем сообщение
				saver.EXPECT().SaveMessage(ctx, int64(1), int64(2), "hello").Return(int64(42), nil)
			},
			expectedID:  42,
			expectError: false,
		},
		{
			name:     "Success - Cache Miss -> DB Hit (Долгий путь)",
			chatID:   1,
			senderID: 2,
			text:     "hello",
			mockSetup: func(provider *mocks.MockChatProvider, saver *mocks.MockMessageSaver, cache *mocks.MockChatCache) {
				// 1. Кэш пустой
				cache.EXPECT().CheckChatMember(ctx, int64(1), int64(2)).Return(false, chatservice.ErrCacheMiss)
				// 2. Идем в базу данных, юзер там есть
				provider.EXPECT().IsChatMember(ctx, int64(1), int64(2)).Return(true, nil)
				// 3. Сохраняем результат в кэш
				cache.EXPECT().SetChatMember(ctx, int64(1), int64(2), true).Return(nil)
				// 4. Сохраняем сообщение
				saver.EXPECT().SaveMessage(ctx, int64(1), int64(2), "hello").Return(int64(42), nil)
			},
			expectedID:  42,
			expectError: false,
		},
		{
			name:     "Permission Denied - User not in chat",
			chatID:   1,
			senderID: 3,
			text:     "hello",
			mockSetup: func(provider *mocks.MockChatProvider, saver *mocks.MockMessageSaver, cache *mocks.MockChatCache) {
				// 1. Кэш пустой
				cache.EXPECT().CheckChatMember(ctx, int64(1), int64(3)).Return(false, chatservice.ErrCacheMiss)
				// 2. Идем в базу, юзера там НЕТ
				provider.EXPECT().IsChatMember(ctx, int64(1), int64(3)).Return(false, nil)
				// 3. Сохраняем этот факт в кэш (negative cache)
				cache.EXPECT().SetChatMember(ctx, int64(1), int64(3), false).Return(nil)

				// ВАЖНО: saver.EXPECT().SaveMessage НЕ должен вызываться
			},
			expectedID:  0,
			expectError: true,
			errTarget:   chatservice.ErrPermissionDenied,
		},
		{
			name:     "Message Saver Error",
			chatID:   1,
			senderID: 2,
			text:     "hello",
			mockSetup: func(provider *mocks.MockChatProvider, saver *mocks.MockMessageSaver, cache *mocks.MockChatCache) {
				// Для разнообразия: кэш ответил успешно, но база данных сообщений упала
				cache.EXPECT().CheckChatMember(ctx, int64(1), int64(2)).Return(true, nil)
				saver.EXPECT().SaveMessage(ctx, int64(1), int64(2), "hello").Return(int64(0), errInternal)
			},
			expectedID:  0,
			expectError: true,
			errTarget:   errInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Вытаскиваем нужные моки из твоего setupTest
			// Подставь правильные индексы, если они у тебя возвращаются в другом порядке!
			chatApp, _, chatProvider, msgSaver, _, _, cache := setupTest(t)

			tt.mockSetup(chatProvider, msgSaver, cache)

			id, err := chatApp.SendMessage(ctx, tt.chatID, tt.senderID, tt.text)

			if tt.expectError {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.errTarget)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedID, id)
			}
		})
	}
}

func TestChat_DeleteMessage(t *testing.T) {
	ctx := context.Background()
	msgID := int64(10)
	chatID := int64(1)
	requestorID := int64(42)
	errInternal := errors.New("internal error")

	tests := []struct {
		name        string
		requestor   int64
		mockSetup   func(msgProv *mocks.MockMessageProvider, msgSaver *mocks.MockMessageSaver)
		expectError bool
		errTarget   error
	}{
		{
			name:      "Success",
			requestor: requestorID,
			mockSetup: func(msgProv *mocks.MockMessageProvider, msgSaver *mocks.MockMessageSaver) {
				// Владелец совпадает с тем, кто удаляет
				msg := models.Message{ID: msgID, SenderID: requestorID}
				msgProv.EXPECT().GetMessage(ctx, chatID, msgID).Return(msg, nil)
				msgSaver.EXPECT().DeleteMessage(ctx, msgID, chatID).Return(nil)
			},
			expectError: false,
		},
		{
			name:      "Permission Denied (Not Sender)",
			requestor: 99, // Другой юзер
			mockSetup: func(msgProv *mocks.MockMessageProvider, msgSaver *mocks.MockMessageSaver) {
				msg := models.Message{ID: msgID, SenderID: requestorID}
				msgProv.EXPECT().GetMessage(ctx, chatID, msgID).Return(msg, nil)
				// Saver не должен вызываться!
				msgSaver.EXPECT().DeleteMessage(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectError: true,
			errTarget:   chatservice.ErrPermissionDenied,
		},
		{
			name:      "Provider Error (GetMessage fails)",
			requestor: requestorID,
			mockSetup: func(msgProv *mocks.MockMessageProvider, msgSaver *mocks.MockMessageSaver) {
				// Ошибка при поиске сообщения
				msgProv.EXPECT().GetMessage(ctx, chatID, msgID).Return(models.Message{}, errInternal)

				// До удаления не доходим
				msgSaver.EXPECT().DeleteMessage(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expectError: true,
			errTarget:   errInternal,
		},
		{
			name:      "Saver Error (DeleteMessage fails)",
			requestor: requestorID,
			mockSetup: func(msgProv *mocks.MockMessageProvider, msgSaver *mocks.MockMessageSaver) {
				// Сообщение нашли успешно...
				msg := models.Message{ID: msgID, SenderID: requestorID}
				msgProv.EXPECT().GetMessage(ctx, chatID, msgID).Return(msg, nil)

				// ...Но удалить из базы не смогли
				msgSaver.EXPECT().DeleteMessage(ctx, msgID, chatID).Return(errInternal)
			},
			expectError: true,
			errTarget:   errInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chat, _, _, msgSaver, msgProv, _, _ := setupTest(t)
			tt.mockSetup(msgProv, msgSaver)

			err := chat.DeleteMessage(ctx, msgID, chatID, tt.requestor)

			if tt.expectError {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.errTarget)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestChat_GetChatHistory(t *testing.T) {
	ctx := context.Background()
	chatID := int64(1)
	requestorID := int64(42)
	limit := int64(10)
	offset := int64(0)
	errInternal := errors.New("internal error")

	expectedMsgs := []models.Message{{ID: 1, Text: "hi"}}

	tests := []struct {
		name        string
		mockSetup   func(chatProv *mocks.MockChatProvider, msgProv *mocks.MockMessageProvider)
		expected    []models.Message
		expectError bool
		errTarget   error
	}{
		{
			name: "Success",
			mockSetup: func(chatProv *mocks.MockChatProvider, msgProv *mocks.MockMessageProvider) {
				chatProv.EXPECT().IsChatMember(ctx, chatID, requestorID).Return(true, nil)
				msgProv.EXPECT().GetHistory(ctx, chatID, limit, offset).Return(expectedMsgs, nil)
			},
			expected:    expectedMsgs,
			expectError: false,
		},
		{
			name: "Permission Denied (Not a member)",
			mockSetup: func(chatProv *mocks.MockChatProvider, msgProv *mocks.MockMessageProvider) {
				chatProv.EXPECT().IsChatMember(ctx, chatID, requestorID).Return(false, nil)
				msgProv.EXPECT().GetHistory(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expected:    nil,
			expectError: true,
			errTarget:   chatservice.ErrPermissionDenied,
		},
		{
			name: "Chat Provider Error (IsChatMember fails)",
			mockSetup: func(chatProv *mocks.MockChatProvider, msgProv *mocks.MockMessageProvider) {
				// Ошибка базы при попытке узнать, в чате ли юзер
				chatProv.EXPECT().IsChatMember(ctx, chatID, requestorID).Return(false, errInternal)

				msgProv.EXPECT().GetHistory(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			},
			expected:    nil,
			expectError: true,
			errTarget:   errInternal,
		},
		{
			name: "Message Provider Error (GetHistory fails)",
			mockSetup: func(chatProv *mocks.MockChatProvider, msgProv *mocks.MockMessageProvider) {
				// Проверка прав прошла успешно
				chatProv.EXPECT().IsChatMember(ctx, chatID, requestorID).Return(true, nil)

				// А вот достать историю не вышло
				msgProv.EXPECT().GetHistory(ctx, chatID, limit, offset).Return(nil, errInternal)
			},
			expected:    nil,
			expectError: true,
			errTarget:   errInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chat, _, chatProv, _, msgProv, _, _ := setupTest(t)
			tt.mockSetup(chatProv, msgProv)

			msgs, err := chat.GetChatHistory(ctx, chatID, requestorID, limit, offset)

			if tt.expectError {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.errTarget)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, msgs)
			}
		})
	}
}

package interceptors

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAuth_ParsingAndValidation(t *testing.T) {
	// Общий секрет для теста (такой же, как в базе SSO и конфиге Chat)
	secret := "test-secret-key-32-characters-long!!"
	expectedUserID := int64(1337)

	// ШАГ 1: Симулируем генерацию токена в SSO
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid": float64(expectedUserID), // В JWT все числа кодируются как float64
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	t.Logf("Generated JWT Token: %s", tokenString)

	// ШАГ 2: Симулируем парсинг в Chat Service
	parsedToken, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		// Проверяем метод подписи
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			t.Errorf("unexpected signing method: %v", token.Header["alg"])
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	})

	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}

	if !parsedToken.Valid {
		t.Fatalf("token is invalid")
	}

	// ШАГ 3: Проверяем claims (полезную нагрузку)
	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("failed to cast claims to MapClaims")
	}

	uidFloat, ok := claims["uid"].(float64)
	if !ok {
		t.Fatalf("uid field not found in token or has wrong type")
	}

	actualUserID := int64(uidFloat)

	if actualUserID != expectedUserID {
		t.Errorf("expected user ID %d, but got %d", expectedUserID, actualUserID)
	} else {
		t.Logf("Success! Successfully extracted trusted User ID: %d", actualUserID)
	}
}

func TestAuth_InvalidSecret(t *testing.T) {
	// Тест на то, что система заблокирует токен, подписанный чужим секретом
	ssoSecret := "sso-secret-key"
	chatSecret := "wrong-chat-secret-key"

	// SSO подписывает токен своим ключом
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"uid": float64(1),
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenString, _ := token.SignedString([]byte(ssoSecret))

	// Chat пытается распарсить его со своим (неправильным) ключом
	_, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		return []byte(chatSecret), nil
	})

	// Ожидаем ошибку валидации подписи
	if err == nil {
		t.Error("expected error due to invalid signature, but got nil")
	} else {
		t.Logf("Success! Token blocked correctly with error: %v", err)
	}
}

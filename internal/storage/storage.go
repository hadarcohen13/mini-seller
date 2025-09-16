package storage

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hadarco13/mini-seller/internal/redis"
)

const (
	TempDataPrefix    = "temp:"
	SessionPrefix     = "session:"
	DefaultTempTTL    = 15 * time.Minute
	DefaultSessionTTL = 24 * time.Hour
)

// SetTempData stores temporary data with TTL
func SetTempData(ctx context.Context, key string, data interface{}, ttl time.Duration) error {
	if ttl == 0 {
		ttl = DefaultTempTTL
	}

	redisKey := TempDataPrefix + key
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	client := redis.GetClient()
	return client.Set(ctx, redisKey, jsonData, ttl).Err()
}

// GetTempData retrieves temporary data
func GetTempData(ctx context.Context, key string, result interface{}) error {
	redisKey := TempDataPrefix + key
	client := redis.GetClient()

	data, err := client.Get(ctx, redisKey).Result()
	if err != nil {
		return err
	}

	return json.Unmarshal([]byte(data), result)
}

// DeleteTempData removes temporary data
func DeleteTempData(ctx context.Context, key string) error {
	redisKey := TempDataPrefix + key
	client := redis.GetClient()
	return client.Del(ctx, redisKey).Err()
}

// Session represents session information
type Session struct {
	UserID    string                 `json:"user_id"`
	Data      map[string]interface{} `json:"data"`
	CreatedAt time.Time              `json:"created_at"`
	ExpiresAt time.Time              `json:"expires_at"`
}

// CreateSession creates a new session
func CreateSession(ctx context.Context, sessionID, userID string, ttl time.Duration) (*Session, error) {
	if ttl == 0 {
		ttl = DefaultSessionTTL
	}

	session := &Session{
		UserID:    userID,
		Data:      make(map[string]interface{}),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(ttl),
	}

	redisKey := SessionPrefix + sessionID
	jsonData, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}

	client := redis.GetClient()
	err = client.Set(ctx, redisKey, jsonData, ttl).Err()
	if err != nil {
		return nil, err
	}

	return session, nil
}

// GetSession retrieves session information
func GetSession(ctx context.Context, sessionID string) (*Session, error) {
	redisKey := SessionPrefix + sessionID
	client := redis.GetClient()

	data, err := client.Get(ctx, redisKey).Result()
	if err != nil {
		return nil, err
	}

	var session Session
	err = json.Unmarshal([]byte(data), &session)
	if err != nil {
		return nil, err
	}

	return &session, nil
}

// UpdateSession updates session data
func UpdateSession(ctx context.Context, sessionID string, session *Session) error {
	redisKey := SessionPrefix + sessionID
	jsonData, err := json.Marshal(session)
	if err != nil {
		return err
	}

	client := redis.GetClient()
	ttl := time.Until(session.ExpiresAt)
	return client.Set(ctx, redisKey, jsonData, ttl).Err()
}

// DeleteSession removes a session
func DeleteSession(ctx context.Context, sessionID string) error {
	redisKey := SessionPrefix + sessionID
	client := redis.GetClient()
	return client.Del(ctx, redisKey).Err()
}

// SetSessionData sets specific data in session
func SetSessionData(ctx context.Context, sessionID, key string, value interface{}) error {
	session, err := GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.Data[key] = value
	return UpdateSession(ctx, sessionID, session)
}

// GetSessionData gets specific data from session
func GetSessionData(ctx context.Context, sessionID, key string) (interface{}, error) {
	session, err := GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	value, exists := session.Data[key]
	if !exists {
		return nil, nil
	}

	return value, nil
}

// ExtendSession extends session expiration
func ExtendSession(ctx context.Context, sessionID string, additionalTime time.Duration) error {
	session, err := GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	session.ExpiresAt = session.ExpiresAt.Add(additionalTime)
	return UpdateSession(ctx, sessionID, session)
}

// IsSessionValid checks if session is still valid
func IsSessionValid(ctx context.Context, sessionID string) (bool, error) {
	session, err := GetSession(ctx, sessionID)
	if err != nil {
		return false, err
	}

	return time.Now().Before(session.ExpiresAt), nil
}

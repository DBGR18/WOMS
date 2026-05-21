package api

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/d11nn/woms/internal/auth"
)

var ErrTokenSessionNotFound = errors.New("token session not found")

type TokenSessionStore interface {
	Save(ctx context.Context, token string, claims auth.Claims) error
	Verify(ctx context.Context, token string, claims auth.Claims) error
	Revoke(ctx context.Context, token string) (bool, error)
	TracksSessions() bool
	Close() error
}

type NoopTokenSessionStore struct{}

func (NoopTokenSessionStore) Save(context.Context, string, auth.Claims) error {
	return nil
}

func (NoopTokenSessionStore) Verify(context.Context, string, auth.Claims) error {
	return nil
}

func (NoopTokenSessionStore) Revoke(context.Context, string) (bool, error) {
	return false, nil
}

func (NoopTokenSessionStore) TracksSessions() bool {
	return false
}

func (NoopTokenSessionStore) Close() error {
	return nil
}

type MemoryTokenSessionStore struct {
	mu       sync.Mutex
	sessions map[string]time.Time
}

func NewMemoryTokenSessionStore() *MemoryTokenSessionStore {
	return &MemoryTokenSessionStore{sessions: map[string]time.Time{}}
}

func (s *MemoryTokenSessionStore) Save(_ context.Context, token string, claims auth.Claims) error {
	expires := time.Unix(claims.Expires, 0)
	if token == "" || !expires.After(time.Now()) {
		return ErrTokenSessionNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[tokenSessionKey(token)] = expires
	return nil
}

func (s *MemoryTokenSessionStore) Verify(_ context.Context, token string, _ auth.Claims) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := tokenSessionKey(token)
	expires, ok := s.sessions[key]
	if !ok {
		return ErrTokenSessionNotFound
	}
	if !expires.After(time.Now()) {
		delete(s.sessions, key)
		return ErrTokenSessionNotFound
	}
	return nil
}

func (s *MemoryTokenSessionStore) Revoke(_ context.Context, token string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := tokenSessionKey(token)
	_, ok := s.sessions[key]
	delete(s.sessions, key)
	return ok, nil
}

func (s *MemoryTokenSessionStore) TracksSessions() bool {
	return true
}

func (s *MemoryTokenSessionStore) Close() error {
	return nil
}

type RedisTokenSessionStore struct {
	addr    string
	timeout time.Duration
	mu      sync.Mutex
	conn    net.Conn
	reader  *bufio.Reader
}

func NewRedisTokenSessionStore(addr string) *RedisTokenSessionStore {
	return &RedisTokenSessionStore{
		addr:    strings.TrimSpace(addr),
		timeout: 2 * time.Second,
	}
}

func (s *RedisTokenSessionStore) Ping(ctx context.Context) error {
	value, err := s.command(ctx, "PING")
	if err != nil {
		return err
	}
	if value != "PONG" {
		return fmt.Errorf("unexpected redis PING response: %s", value)
	}
	return nil
}

func (s *RedisTokenSessionStore) Save(ctx context.Context, token string, claims auth.Claims) error {
	ttl := time.Until(time.Unix(claims.Expires, 0))
	if token == "" || ttl <= 0 {
		return ErrTokenSessionNotFound
	}
	seconds := int(ttl.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	_, err := s.command(ctx, "SET", tokenSessionKey(token), "1", "EX", strconv.Itoa(seconds))
	return err
}

func (s *RedisTokenSessionStore) Verify(ctx context.Context, token string, _ auth.Claims) error {
	value, err := s.command(ctx, "GET", tokenSessionKey(token))
	if err != nil {
		if errors.Is(err, ErrTokenSessionNotFound) {
			return err
		}
		return err
	}
	if value != "1" {
		return ErrTokenSessionNotFound
	}
	return nil
}

func (s *RedisTokenSessionStore) Revoke(ctx context.Context, token string) (bool, error) {
	value, err := s.command(ctx, "DEL", tokenSessionKey(token))
	return value == "1", err
}

func (s *RedisTokenSessionStore) TracksSessions() bool {
	return true
}

func (s *RedisTokenSessionStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked()
	return nil
}

func (s *RedisTokenSessionStore) command(ctx context.Context, args ...string) (string, error) {
	if s.addr == "" {
		return "", errors.New("REDIS_ADDR 不可為空")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commandLocked(ctx, args...)
}

func (s *RedisTokenSessionStore) commandLocked(ctx context.Context, args ...string) (string, error) {
	conn, err := s.connLocked(ctx)
	if err != nil {
		return "", err
	}
	deadline := time.Now().Add(s.timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write(redisCommand(args...)); err != nil {
		s.closeLocked()
		return "", err
	}
	value, err := readRedisValue(s.reader)
	if err != nil && !errors.Is(err, ErrTokenSessionNotFound) {
		s.closeLocked()
	}
	return value, err
}

func (s *RedisTokenSessionStore) connLocked(ctx context.Context) (net.Conn, error) {
	if s.conn != nil {
		return s.conn, nil
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return nil, err
	}
	s.conn = conn
	s.reader = bufio.NewReader(conn)
	return conn, nil
}

func (s *RedisTokenSessionStore) closeLocked() {
	if s.conn != nil {
		_ = s.conn.Close()
	}
	s.conn = nil
	s.reader = nil
}

func redisCommand(args ...string) []byte {
	var b strings.Builder
	b.WriteString("*")
	b.WriteString(strconv.Itoa(len(args)))
	b.WriteString("\r\n")
	for _, arg := range args {
		b.WriteString("$")
		b.WriteString(strconv.Itoa(len(arg)))
		b.WriteString("\r\n")
		b.WriteString(arg)
		b.WriteString("\r\n")
	}
	return []byte(b.String())
}

func readRedisValue(r *bufio.Reader) (string, error) {
	prefix, err := r.ReadByte()
	if err != nil {
		return "", err
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	switch prefix {
	case '+':
		return line, nil
	case '-':
		return "", errors.New(line)
	case ':':
		return line, nil
	case '$':
		length, err := strconv.Atoi(line)
		if err != nil {
			return "", err
		}
		if length < 0 {
			return "", ErrTokenSessionNotFound
		}
		buf := make([]byte, length+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return string(buf[:length]), nil
	default:
		return "", fmt.Errorf("unsupported redis response prefix %q", prefix)
	}
}

func tokenSessionKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "woms:auth:token:" + base64.RawURLEncoding.EncodeToString(sum[:])
}

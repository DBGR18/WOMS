package lock

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

var ErrNotAcquired = errors.New("lock not acquired")

type Provider interface {
	Acquire(ctx context.Context, key string, ttl time.Duration) (Lock, error)
}

type Lock interface {
	Refresh(ctx context.Context, ttl time.Duration) error
	Release(ctx context.Context) error
}

type RedisProvider struct {
	addr    string
	timeout time.Duration
}

func NewRedisProvider(addr string) *RedisProvider {
	return &RedisProvider{addr: strings.TrimSpace(addr), timeout: 2 * time.Second}
}

func (p *RedisProvider) Ping(ctx context.Context) error {
	value, err := p.command(ctx, "PING")
	if err != nil {
		return err
	}
	if value != "PONG" {
		return fmt.Errorf("unexpected redis PING response: %s", value)
	}
	return nil
}

func (p *RedisProvider) Acquire(ctx context.Context, key string, ttl time.Duration) (Lock, error) {
	if ttl <= 0 {
		return nil, errors.New("lock ttl must be positive")
	}
	value, err := randomValue()
	if err != nil {
		return nil, err
	}
	result, err := p.command(ctx, "SET", key, value, "NX", "PX", strconv.FormatInt(ttl.Milliseconds(), 10))
	if err != nil {
		if errors.Is(err, ErrNotAcquired) {
			return nil, err
		}
		return nil, err
	}
	if result != "OK" {
		return nil, ErrNotAcquired
	}
	return &redisLock{provider: p, key: key, value: value}, nil
}

func (p *RedisProvider) command(ctx context.Context, args ...string) (string, error) {
	if p.addr == "" {
		return "", errors.New("REDIS_ADDR 不可為空")
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", p.addr)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	deadline := time.Now().Add(p.timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write(command(args...)); err != nil {
		return "", err
	}
	return readValue(bufio.NewReader(conn))
}

type redisLock struct {
	provider *RedisProvider
	key      string
	value    string
}

func (l *redisLock) Refresh(ctx context.Context, ttl time.Duration) error {
	script := `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("PEXPIRE", KEYS[1], ARGV[2]) else return 0 end`
	result, err := l.provider.command(ctx, "EVAL", script, "1", l.key, l.value, strconv.FormatInt(ttl.Milliseconds(), 10))
	if err != nil {
		return err
	}
	if result != "1" {
		return ErrNotAcquired
	}
	return nil
}

func (l *redisLock) Release(ctx context.Context) error {
	script := `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`
	_, err := l.provider.command(ctx, "EVAL", script, "1", l.key, l.value)
	return err
}

func command(args ...string) []byte {
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

func readValue(r *bufio.Reader) (string, error) {
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
			return "", ErrNotAcquired
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

func randomValue() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

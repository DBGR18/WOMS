package api

import (
	"bufio"
	"strings"
	"testing"
)

func TestRedisCommandEncodesRESPArrays(t *testing.T) {
	got := string(redisCommand("SET", "woms:auth:token:test", "1", "EX", "60"))
	want := "*5\r\n$3\r\nSET\r\n$20\r\nwoms:auth:token:test\r\n$1\r\n1\r\n$2\r\nEX\r\n$2\r\n60\r\n"
	if got != want {
		t.Fatalf("unexpected RESP command:\nwant %q\ngot  %q", want, got)
	}
}

func TestReadRedisValueMapsNilBulkToMissingSession(t *testing.T) {
	_, err := readRedisValue(bufio.NewReader(strings.NewReader("$-1\r\n")))
	if err != ErrTokenSessionNotFound {
		t.Fatalf("expected ErrTokenSessionNotFound, got %v", err)
	}
}

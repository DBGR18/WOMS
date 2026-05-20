package lock

import (
	"bufio"
	"strings"
	"testing"
)

func TestCommandEncodesRESPArrays(t *testing.T) {
	got := string(command("SET", "woms:locks:schedule-line:A", "value", "NX", "PX", "15000"))
	want := "*6\r\n$3\r\nSET\r\n$26\r\nwoms:locks:schedule-line:A\r\n$5\r\nvalue\r\n$2\r\nNX\r\n$2\r\nPX\r\n$5\r\n15000\r\n"
	if got != want {
		t.Fatalf("unexpected command:\nwant %q\ngot  %q", want, got)
	}
}

func TestReadValueMapsNilBulkToNotAcquired(t *testing.T) {
	_, err := readValue(bufio.NewReader(strings.NewReader("$-1\r\n")))
	if err != ErrNotAcquired {
		t.Fatalf("expected ErrNotAcquired, got %v", err)
	}
}

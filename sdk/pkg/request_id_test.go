package pkg

import "testing"

func TestRequestIDIsStableForOneRequest(t *testing.T) {
	c := benchContext(false)
	first := RequestID(c)
	second := RequestID(c)
	if first == "" || first != second {
		t.Fatalf("request ID changed: %q -> %q", first, second)
	}
	if got := c.Writer.Header().Get(TrafficKey); got != first {
		t.Fatalf("response header=%q, want %q", got, first)
	}
	other := benchContext(false)
	if RequestID(other) == first {
		t.Fatal("separate requests shared an ID")
	}
}

func TestRequestIDEchoesSuppliedHeader(t *testing.T) {
	c := benchContext(true)
	want := c.GetHeader(TrafficKey)
	if got := RequestID(c); got != want {
		t.Fatalf("request ID=%q, want %q", got, want)
	}
	if got := c.Writer.Header().Get(TrafficKey); got != want {
		t.Fatalf("response header=%q, want %q", got, want)
	}
}

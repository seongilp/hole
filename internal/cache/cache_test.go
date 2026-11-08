package cache

import (
	"strconv"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func answer(name string, ttl uint32) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeA)
	rr, _ := dns.NewRR(name + " " + strconv.Itoa(int(ttl)) + " IN A 1.2.3.4")
	m.Answer = []dns.RR{rr}
	return m
}

func TestCache_GetMissAndHit(t *testing.T) {
	now := time.Unix(1000, 0)
	c := New(100, func() time.Time { return now })

	if _, ok := c.Get("a.com.", dns.TypeA); ok {
		t.Fatal("expected miss on empty cache")
	}
	c.Set("a.com.", dns.TypeA, answer("a.com.", 60))

	got, ok := c.Get("a.com.", dns.TypeA)
	if !ok {
		t.Fatal("expected hit")
	}
	if len(got.Answer) != 1 {
		t.Fatalf("answer count = %d, want 1", len(got.Answer))
	}
	if _, ok := c.Get("a.com.", dns.TypeAAAA); ok {
		t.Error("different qtype must miss")
	}
}

func TestCache_ExpiresAndDecrementsTTL(t *testing.T) {
	now := time.Unix(1000, 0)
	c := New(100, func() time.Time { return now })
	c.Set("a.com.", dns.TypeA, answer("a.com.", 60))

	now = now.Add(20 * time.Second)
	got, ok := c.Get("a.com.", dns.TypeA)
	if !ok {
		t.Fatal("expected hit before expiry")
	}
	if ttl := got.Answer[0].Header().Ttl; ttl != 40 {
		t.Errorf("remaining TTL = %d, want 40", ttl)
	}

	now = now.Add(41 * time.Second)
	if _, ok := c.Get("a.com.", dns.TypeA); ok {
		t.Error("expected miss after expiry")
	}
}

func TestCache_ReturnsCopy(t *testing.T) {
	now := time.Unix(1000, 0)
	c := New(100, func() time.Time { return now })
	c.Set("a.com.", dns.TypeA, answer("a.com.", 60))

	first, _ := c.Get("a.com.", dns.TypeA)
	first.Answer = nil

	second, _ := c.Get("a.com.", dns.TypeA)
	if len(second.Answer) != 1 {
		t.Error("mutating a returned message must not affect the cache")
	}
}

func TestCache_EvictsWhenFull(t *testing.T) {
	now := time.Unix(1000, 0)
	c := New(2, func() time.Time { return now })
	c.Set("a.com.", dns.TypeA, answer("a.com.", 60))
	c.Set("b.com.", dns.TypeA, answer("b.com.", 60))
	c.Set("c.com.", dns.TypeA, answer("c.com.", 60))

	if c.Len() > 2 {
		t.Errorf("Len() = %d, want <= 2", c.Len())
	}
	if _, ok := c.Get("c.com.", dns.TypeA); !ok {
		t.Error("most recently inserted entry should be present")
	}
}

func TestCache_SkipsZeroTTLAndErrors(t *testing.T) {
	now := time.Unix(1000, 0)
	c := New(100, func() time.Time { return now })
	c.Set("a.com.", dns.TypeA, answer("a.com.", 0))

	fail := new(dns.Msg)
	fail.SetQuestion("b.com.", dns.TypeA)
	fail.Rcode = dns.RcodeServerFailure
	c.Set("b.com.", dns.TypeA, fail)

	if c.Len() != 0 {
		t.Errorf("Len() = %d, want 0", c.Len())
	}
}

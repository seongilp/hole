// Package cache is a small TTL-aware cache for DNS responses.
package cache

import (
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type key struct {
	name  string
	qtype uint16
}

type entry struct {
	msg      *dns.Msg
	storedAt time.Time
	expires  time.Time
}

// Cache stores upstream DNS responses until their minimum TTL elapses.
// It is safe for concurrent use.
type Cache struct {
	mu       sync.Mutex
	items    map[key]entry
	capacity int
	now      func() time.Time
}

// New creates a cache holding at most capacity entries. now supplies the
// current time (pass time.Now in production; inject a fake in tests).
func New(capacity int, now func() time.Time) *Cache {
	return &Cache{items: make(map[key]entry, capacity), capacity: capacity, now: now}
}

func makeKey(name string, qtype uint16) key {
	return key{name: strings.ToLower(dns.Fqdn(name)), qtype: qtype}
}

// Get returns a copy of the cached response with TTLs reduced by the time
// spent in the cache.
func (c *Cache) Get(name string, qtype uint16) (*dns.Msg, bool) {
	k := makeKey(name, qtype)
	now := c.now()

	c.mu.Lock()
	e, ok := c.items[k]
	if ok && !now.Before(e.expires) {
		delete(c.items, k)
		ok = false
	}
	c.mu.Unlock()
	if !ok {
		return nil, false
	}

	out := e.msg.Copy()
	elapsed := uint32(now.Sub(e.storedAt) / time.Second)
	for _, section := range [][]dns.RR{out.Answer, out.Ns, out.Extra} {
		for _, rr := range section {
			if rr.Header().Rrtype == dns.TypeOPT {
				continue
			}
			rr.Header().Ttl -= min(elapsed, rr.Header().Ttl)
		}
	}
	return out, true
}

// Set stores a copy of msg. Failed responses and zero-TTL answers are not
// cached.
func (c *Cache) Set(name string, qtype uint16, msg *dns.Msg) {
	if msg == nil || (msg.Rcode != dns.RcodeSuccess && msg.Rcode != dns.RcodeNameError) {
		return
	}
	ttl, ok := minTTL(msg)
	if !ok || ttl == 0 {
		return
	}
	now := c.now()
	e := entry{msg: msg.Copy(), storedAt: now, expires: now.Add(time.Duration(ttl) * time.Second)}

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= c.capacity {
		c.evictLocked(now)
	}
	c.items[makeKey(name, qtype)] = e
}

// evictLocked drops expired entries, then the entry closest to expiry if the
// cache is still full.
func (c *Cache) evictLocked(now time.Time) {
	var victim key
	var victimExp time.Time
	first := true
	for k, e := range c.items {
		if !now.Before(e.expires) {
			delete(c.items, k)
			continue
		}
		if first || e.expires.Before(victimExp) {
			victim, victimExp, first = k, e.expires, false
		}
	}
	if len(c.items) >= c.capacity && !first {
		delete(c.items, victim)
	}
}

func minTTL(msg *dns.Msg) (uint32, bool) {
	var ttl uint32
	found := false
	for _, section := range [][]dns.RR{msg.Answer, msg.Ns} {
		for _, rr := range section {
			if !found || rr.Header().Ttl < ttl {
				ttl, found = rr.Header().Ttl, true
			}
		}
	}
	return ttl, found
}

// Len returns the number of stored entries (including not-yet-evicted
// expired ones).
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

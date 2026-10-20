// Package bench measures P2PACT against HTTPS+JSON and WebSocket (JSON over TLS): bytes on
// the wire and time, for chat messages of four sizes, over emulated network profiles.
//
// Everything runs on one machine: links are emulated (latency, bandwidth, loss) so results
// are reproducible. Bytes are counted on the sockets, not computed from formulas.
package bench

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"
)

// Profile is an emulated link: one-way latency, bandwidth per direction and packet loss.
type Profile struct {
	Name  string
	Delay time.Duration
	Rate  int     // bytes per second, 0 — unlimited
	Loss  float64 // packet loss, 0..1 (P2PACT only: TCP loss cannot be emulated in user space)
}

// Profiles are the default network conditions.
var Profiles = []Profile{
	{Name: "local network", Delay: time.Millisecond},
	{Name: "bluetooth", Delay: 30 * time.Millisecond, Rate: 40 << 10},
	{Name: "internet", Delay: 40 * time.Millisecond, Loss: 0.01},
	{Name: "mobile", Delay: 100 * time.Millisecond, Rate: 500 << 10, Loss: 0.03},
}

// Size is a chat message size. Bytes is the generated size, or the median of the real
// messages loaded into the bin [Min, Max).
type Size struct {
	Name     string
	Bytes    int
	Min, Max int
	real     []string
}

// Sizes are the default message sizes.
var Sizes = []Size{
	{Name: "very short", Bytes: 20, Min: 1, Max: 40},
	{Name: "medium", Bytes: 150, Min: 40, Max: 200},
	{Name: "long", Bytes: 2 << 10, Min: 200, Max: 2 << 10},
	{Name: "very long", Bytes: 100 << 10, Min: 2 << 10, Max: 1 << 30},
}

// Message is the i-th message of a size: a real one if a corpus is loaded, else generated.
func Message(s Size, i int) string {
	if len(s.real) > 0 {
		return s.real[i%len(s.real)]
	}
	return Text(s.Bytes, int64(i))
}

// LoadCorpus puts real messages into the size bins; each bin's Bytes becomes its median.
// Bins without messages keep generated text.
func LoadCorpus(msgs []string) {
	for i := range Sizes {
		s := &Sizes[i]
		s.real = nil
		var lens []int
		for _, m := range msgs {
			if n := len(m); n >= s.Min && n < s.Max {
				s.real = append(s.real, m)
				lens = append(lens, n)
			}
		}
		if len(lens) > 0 {
			sort.Ints(lens)
			s.Bytes = lens[len(lens)/2]
		}
	}
}

// Result is one protocol over one profile.
type Result struct {
	Protocol string
	Profile  string
	// per size name
	Payload map[string]int64         // bytes above TCP/UDP per message, both directions
	Wire    map[string]int64         // ≈ bytes on the wire incl. IP and TCP/UDP headers
	RTT     map[string]time.Duration // median time to send a message and get the ack
	// Cold: a new connection (P2PACT: a new session) plus one very short message.
	ColdTime  time.Duration
	ColdBytes int64
	Note      string
}

func newResult(proto, profile string) *Result {
	return &Result{Protocol: proto, Profile: profile, Payload: map[string]int64{}, Wire: map[string]int64{},
		RTT: map[string]time.Duration{}}
}

// Text returns a deterministic chat-like text of n bytes: words drawn from a Zipf
// distribution over a generated vocabulary, so it compresses like natural text instead of
// repeating itself.
func Text(n int, seed int64) string {
	r := rand.New(rand.NewSource(seed))
	z := rand.NewZipf(r, 1.1, 2, uint64(len(vocab)-1))
	var b strings.Builder
	for b.Len() < n {
		if b.Len() > 0 {
			if r.Intn(12) == 0 {
				b.WriteString(". ")
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteString(vocab[z.Uint64()])
	}
	return b.String()[:n]
}

var vocab = func() []string {
	r := rand.New(rand.NewSource(42))
	syl := []string{"ka", "re", "to", "mi", "sa", "lo", "ne", "di", "pu", "va", "te", "ro", "chi", "an", "el", "ou",
		"ing", "er", "st", "pr", "co", "de", "fi", "ma", "ti", "on", "ex", "un", "go", "fu", "nc", "rt"}
	common := []string{"the", "a", "to", "and", "is", "of", "in", "it", "test", "file", "fix", "run", "error", "go",
		"func", "return", "chat", "agent", "ok", "done", "commit", "build", "we", "this", "that", "for", "not"}
	out := append([]string{}, common...)
	seen := map[string]bool{}
	for _, w := range common {
		seen[w] = true
	}
	for len(out) < 4000 {
		var w string
		for i := 0; i < 1+r.Intn(4); i++ {
			w += syl[r.Intn(len(syl))]
		}
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}()

func median(d []time.Duration) time.Duration {
	if len(d) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), d...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2]
}

// Markdown renders results as tables: bytes per message, time per message, cold start.
func Markdown(results []*Result) string {
	var b strings.Builder
	byProfile := map[string][]*Result{}
	var order []string
	for _, r := range results {
		if byProfile[r.Profile] == nil {
			order = append(order, r.Profile)
		}
		byProfile[r.Profile] = append(byProfile[r.Profile], r)
	}
	for _, p := range order {
		rs := byProfile[p]
		var prof Profile
		for _, x := range Profiles {
			if x.Name == p {
				prof = x
			}
		}
		fmt.Fprintf(&b, "### %s (%s)\n\n", p, describe(prof))
		header := "| Message | " + join(rs, func(r *Result) string { return r.Protocol }) + " |\n" +
			"|---" + strings.Repeat("|---:", len(rs)) + "|\n"
		b.WriteString("Bytes on the wire per message, both directions:\n\n" + header)
		for _, s := range Sizes {
			b.WriteString("| " + fmt.Sprintf("%s (%s)", s.Name, human(int64(s.Bytes))) + " | ")
			b.WriteString(join(rs, func(r *Result) string { return wire(r, s.Name) }) + " |\n")
		}
		b.WriteString("| new connection + very short | " + join(rs, func(r *Result) string { return "≈" + human(r.ColdBytes) }) + " |\n\n")
		b.WriteString("Time from sending to the ack (median):\n\n" + header)
		for _, s := range Sizes {
			b.WriteString("| " + fmt.Sprintf("%s (%s)", s.Name, human(int64(s.Bytes))) + " | ")
			b.WriteString(join(rs, func(r *Result) string { return dur(r.RTT[s.Name]) }) + " |\n")
		}
		b.WriteString("| new connection + very short | " + join(rs, func(r *Result) string { return dur(r.ColdTime) }) + " |\n\n")
		for _, r := range rs {
			if r.Note != "" {
				fmt.Fprintf(&b, "- %s: %s\n", r.Protocol, r.Note)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func describe(p Profile) string {
	parts := []string{fmt.Sprintf("%v one way", p.Delay)}
	if p.Rate > 0 {
		parts = append(parts, human(int64(p.Rate))+"/s")
	}
	if p.Loss > 0 {
		parts = append(parts, fmt.Sprintf("%g%% loss", p.Loss*100))
	}
	return strings.Join(parts, ", ")
}

func wire(r *Result, size string) string {
	return fmt.Sprintf("≈%s", human(r.Wire[size]))
}

func join(rs []*Result, f func(*Result) string) string {
	var out []string
	for _, r := range rs {
		out = append(out, f(r))
	}
	return strings.Join(out, " | ")
}

func human(n int64) string {
	switch {
	case n >= 10<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/1024)
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func dur(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2f s", d.Seconds())
	case d >= 10*time.Millisecond:
		return fmt.Sprintf("%d ms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.1f ms", float64(d.Microseconds())/1000)
	}
}

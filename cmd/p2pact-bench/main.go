// Command p2pact-bench compares P2PACT with WebSocket (JSON over TLS) and HTTPS+JSON: bytes
// on the wire and time for very short, medium, long and very long chat messages over
// emulated links — a local network, Bluetooth, the internet and mobile internet.
//
//	p2pact-bench [-n 20] [-profiles "local network,bluetooth,internet,mobile"] [-corpus msgs.json]
//
// -corpus is a JSON array of your own messages: they are sorted into the size bins instead
// of generated text (nothing leaves your machine).
//
// It prints Markdown tables.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/p2pact/p2pact/internal/bench"
)

func main() {
	n := flag.Int("n", 20, "messages per size (very long: n/10)")
	names := flag.String("profiles", "", "comma-separated profiles (default: all)")
	corpus := flag.String("corpus", "", "JSON array of real messages to use instead of generated text")
	flag.Parse()
	if *corpus != "" {
		b, err := os.ReadFile(*corpus)
		var msgs []string
		if err == nil {
			err = json.Unmarshal(b, &msgs)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "corpus:", err)
			os.Exit(1)
		}
		bench.LoadCorpus(msgs)
		for _, s := range bench.Sizes {
			fmt.Fprintf(os.Stderr, "%s: median %d B\n", s.Name, s.Bytes)
		}
	}
	profiles := bench.Profiles
	if *names != "" {
		profiles = nil
		for _, name := range strings.Split(*names, ",") {
			for _, p := range bench.Profiles {
				if p.Name == strings.TrimSpace(name) {
					profiles = append(profiles, p)
				}
			}
		}
	}
	res, err := bench.Run(profiles, *n, func(s string) { fmt.Fprintln(os.Stderr, "done:", s) })
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(bench.Markdown(res))
}

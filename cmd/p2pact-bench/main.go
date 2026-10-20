// Command p2pact-bench compares P2PACT with WebSocket (JSON over TLS) and HTTPS+JSON: bytes
// on the wire and time for very short, medium, long and very long chat messages over
// emulated links — a local network, Bluetooth, the internet and mobile internet.
//
//	p2pact-bench [-n 20] [-profiles "local network,bluetooth,internet,mobile"]
//
// It prints Markdown tables.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/p2pact/p2pact/internal/bench"
)

func main() {
	n := flag.Int("n", 20, "messages per size (very long: n/10)")
	names := flag.String("profiles", "", "comma-separated profiles (default: all)")
	flag.Parse()
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

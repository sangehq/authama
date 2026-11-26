package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// holderClient asks a taken port who holds it; a loopback answer is quick.
var holderClient = &http.Client{Timeout: 700 * time.Millisecond}

// PortHolder names what listens at addr, by its /healthz: the sange-family
// tools answer it, imprimatur and authama with {"app": "<name>"} and sange
// with plain "ok". Anything else, or nothing, is "another program". It reads
// at most 4 KiB.
func PortHolder(addr string) string {
	const other = "another program"
	resp, err := holderClient.Get("http://" + addr + "/healthz")
	if err != nil {
		return other
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return other
	}
	if strings.TrimSpace(string(body)) == "ok" {
		return "sange"
	}
	var h struct {
		App string `json:"app"`
	}
	if json.Unmarshal(body, &h) == nil {
		switch h.App {
		case "imprimatur":
			return "imprimatur"
		case "authama":
			return "another authama web UI"
		}
	}
	return other
}

// PortTakenHint says what holds the taken port at addr and how to see its
// process.
func PortTakenHint(addr string) string {
	_, port, _ := net.SplitHostPort(addr)
	return fmt.Sprintf("port %s is in use by %s (see its process with `lsof -nP -iTCP:%s -sTCP:LISTEN`)", port, PortHolder(addr), port)
}

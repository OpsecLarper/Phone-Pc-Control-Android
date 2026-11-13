// PhoneRemote PC server: lets the phone app control this PC's mouse,
// keyboard and volume over Wi-Fi.
package main

import (
	"crypto/rand"
	_ "embed"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const port = 8765

//go:embed index.html
var indexHTML []byte

type msg struct {
	T    string   `json:"t"`
	Pin  string   `json:"pin"`
	DX   float64  `json:"dx"`
	DY   float64  `json:"dy"`
	B    string   `json:"b"`
	S    string   `json:"s"`
	K    string   `json:"k"`
	Mods []string `json:"mods"`
	V    int      `json:"v"`
	On   bool     `json:"on"`
}

var (
	upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	clientsM sync.Mutex
	clients  = map[*client]bool{}
	pin      string
)

type client struct {
	conn *websocket.Conn
	wm   sync.Mutex
}

func (c *client) send(v interface{}) {
	c.wm.Lock()
	defer c.wm.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	c.conn.WriteJSON(v)
}

func main() {
	pin = loadPin()
	startVolumeWorker()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
	http.HandleFunc("/ws", handleWS)

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		fmt.Printf("\nCould not start on port %d: %v\nIs PhoneRemote already running?\n\nPress Enter to exit.", port, err)
		fmt.Scanln()
		os.Exit(1)
	}

	fmt.Println("=============================================")
	fmt.Println("  Phone Remote is running")
	fmt.Println("=============================================")
	ips := lanIPs()
	if len(ips) == 0 {
		fmt.Println("  IP address:  (no network found)")
	}
	for i, ip := range ips {
		label := "  IP address:  "
		if i > 0 {
			label = "          or:  "
		}
		fmt.Println(label + ip)
	}
	fmt.Println("  PIN:         " + pin)
	fmt.Println("=============================================")
	fmt.Println("Enter the IP and PIN in the Phone Remote app.")
	fmt.Println("Phone and PC must be on the same Wi-Fi network.")
	fmt.Println("If Windows Firewall asks, click \"Allow access\" (Private networks).")
	fmt.Println("Keep this window open. Close it to stop.")
	fmt.Println()

	go volumeWatcher()
	log.Fatal(http.Serve(ln, nil))
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &client{conn: conn}
	defer conn.Close()

	// first message must be auth
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var m msg
	if err := conn.ReadJSON(&m); err != nil || m.T != "auth" {
		return
	}
	if strings.TrimSpace(m.Pin) != pin {
		time.Sleep(700 * time.Millisecond) // slow down guessing
		c.send(map[string]string{"t": "err", "msg": "Wrong PIN. Check the PIN shown on your PC."})
		log.Printf("Rejected %s (wrong PIN)", r.RemoteAddr)
		return
	}
	conn.SetReadDeadline(time.Time{})
	host, _ := os.Hostname()
	vol, muted := getVolume()
	c.send(map[string]interface{}{"t": "ok", "name": host, "vol": vol, "muted": muted})
	log.Printf("Phone connected from %s", r.RemoteAddr)

	clientsM.Lock()
	clients[c] = true
	clientsM.Unlock()
	defer func() {
		clientsM.Lock()
		delete(clients, c)
		clientsM.Unlock()
		releaseHeld()
		log.Printf("Phone disconnected (%s)", r.RemoteAddr)
	}()

	for {
		var m msg
		if err := conn.ReadJSON(&m); err != nil {
			return
		}
		handle(m)
	}
}

var (
	heldM    sync.Mutex
	heldLeft bool
)

func releaseHeld() {
	heldM.Lock()
	defer heldM.Unlock()
	if heldLeft {
		mouseButton("left", false)
		heldLeft = false
	}
}

func handle(m msg) {
	switch m.T {
	case "move":
		mouseMove(int32(clamp(m.DX, -500, 500)), int32(clamp(m.DY, -500, 500)))
	case "scroll":
		mouseScroll(int32(clamp(m.DY, -50, 50)))
	case "click":
		mouseButton(m.B, true)
		mouseButton(m.B, false)
	case "down", "up":
		heldM.Lock()
		if m.B == "left" {
			heldLeft = m.T == "down"
		}
		heldM.Unlock()
		mouseButton(m.B, m.T == "down")
	case "text":
		if len(m.S) <= 2000 {
			typeText(m.S)
		}
	case "key":
		pressKey(m.K, m.Mods)
	case "vol":
		if m.V >= 0 && m.V <= 100 {
			setVolume(m.V)
		}
	case "mute":
		setMute(m.On)
	}
}

// volumeWatcher pushes volume changes made on the PC to connected phones.
func volumeWatcher() {
	lastV, lastM := -1, false
	for range time.Tick(700 * time.Millisecond) {
		clientsM.Lock()
		n := len(clients)
		clientsM.Unlock()
		if n == 0 {
			lastV = -1
			continue
		}
		v, mu := getVolume()
		if v != lastV || mu != lastM {
			lastV, lastM = v, mu
			clientsM.Lock()
			for c := range clients {
				go c.send(map[string]interface{}{"t": "vol", "vol": v, "muted": mu})
			}
			clientsM.Unlock()
		}
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func loadPin() string {
	dir := "."
	if exe, err := os.Executable(); err == nil {
		dir = filepath.Dir(exe)
	}
	path := filepath.Join(dir, "phoneremote-pin.txt")
	if b, err := os.ReadFile(path); err == nil {
		p := strings.TrimSpace(string(b))
		if len(p) >= 4 {
			return p
		}
	}
	n, _ := rand.Int(rand.Reader, big.NewInt(10000))
	p := fmt.Sprintf("%04d", n.Int64())
	os.WriteFile(path, []byte(p+"\n"), 0600)
	return p
}

func lanIPs() []string {
	var out []string
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.ToLower(ifc.Name)
		if strings.Contains(name, "vethernet") || strings.Contains(name, "virtualbox") || strings.Contains(name, "vmware") {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				if ip4 := ipn.IP.To4(); ip4 != nil && !ip4.IsLinkLocalUnicast() {
					out = append(out, ip4.String())
				}
			}
		}
	}
	// prefer typical home-network addresses first
	for i, ip := range out {
		if strings.HasPrefix(ip, "192.168.") && i > 0 {
			out[0], out[i] = out[i], out[0]
			break
		}
	}
	return out
}

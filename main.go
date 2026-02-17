package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

var version = "dev"

//go:embed index.html
var indexHTML []byte

const serviceUnit = `[Unit]
Description=HotSpotter WiFi Hotspot Manager
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s --addr %s --config-dir %s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`

func main() {
	addr := flag.String("addr", ":8080", "listen address (e.g. :8080 or 0.0.0.0:9090)")
	configDir := flag.String("config-dir", "/etc/hotspotter", "directory for persistent config")
	showVersion := flag.Bool("version", false, "print version and exit")
	installService := flag.Bool("install-service", false, "install and enable systemd service")
	uninstallService := flag.Bool("uninstall-service", false, "disable and remove systemd service")
	flag.Parse()

	if *showVersion {
		fmt.Println("hotspotter", version)
		os.Exit(0)
	}

	if *installService {
		doInstallService(*addr, *configDir)
		return
	}
	if *uninstallService {
		doUninstallService()
		return
	}

	config, err := NewConfigStore(*configDir)
	if err != nil {
		log.Fatalf("Failed to init config: %v", err)
	}

	wifi := NewWifiManager()
	hotspot := NewHotspotManager(wifi)
	handlers := NewHandlers(hotspot, wifi, config)

	// Auto-reconnect to a saved WiFi network on startup.
	go autoReconnect(wifi, config)

	mux := http.NewServeMux()
	handlers.RegisterRoutes(mux)

	// Serve the embedded single-page frontend.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	fmt.Printf("HotSpotter %s listening on %s\n", version, *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// autoReconnect tries to connect to a saved network if not already connected.
func autoReconnect(wifi *WifiManager, config *ConfigStore) {
	// Brief delay to let NetworkManager initialise.
	time.Sleep(3 * time.Second)

	saved := config.GetSavedNetworks()
	if len(saved) == 0 {
		return
	}

	// Check if already connected.
	ssid, _ := wifi.ConnectionStatus("")
	if ssid != "" {
		log.Printf("Already connected to %q, skipping auto-reconnect", ssid)
		return
	}

	log.Printf("Attempting auto-reconnect to %d saved network(s)...", len(saved))
	for _, net := range saved {
		log.Printf("  Trying %q...", net.SSID)
		if err := wifi.Connect("", net.SSID, net.Password); err == nil {
			log.Printf("  Connected to %q", net.SSID)
			return
		}
	}
	log.Printf("Auto-reconnect: no saved networks reachable")
}

func doInstallService(addr, configDir string) {
	exePath, err := os.Executable()
	if err != nil {
		log.Fatalf("Cannot determine executable path: %v", err)
	}

	unit := fmt.Sprintf(serviceUnit,
		exePath,
		addr,
		configDir,
	)

	const servicePath = "/etc/systemd/system/hotspotter.service"
	if err := os.WriteFile(servicePath, []byte(unit), 0644); err != nil {
		log.Fatalf("Failed to write service file: %v", err)
	}

	cmds := [][]string{
		{"systemctl", "daemon-reload"},
		{"systemctl", "enable", "hotspotter.service"},
		{"systemctl", "start", "hotspotter.service"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatalf("%s failed: %v", strings.Join(args, " "), err)
		}
	}

	fmt.Println("HotSpotter service installed and started.")
	fmt.Printf("  Config dir: %s\n", configDir)
	fmt.Printf("  Listening:  %s\n", addr)
	fmt.Println("  Manage with: systemctl {start|stop|restart|status} hotspotter")
}

func doUninstallService() {
	cmds := [][]string{
		{"systemctl", "stop", "hotspotter.service"},
		{"systemctl", "disable", "hotspotter.service"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run() // ignore errors if not installed
	}

	const servicePath = "/etc/systemd/system/hotspotter.service"
	_ = os.Remove(servicePath)

	cmd := exec.Command("systemctl", "daemon-reload")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()

	fmt.Println("HotSpotter service removed.")
}

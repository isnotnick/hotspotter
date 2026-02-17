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

	// On first run (no config, no saved networks), start a default open
	// hotspot so the user can connect and configure via the web UI.
	go autoStart(hotspot, wifi, config)

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

// autoStart handles startup behaviour: if a config exists it tries to
// reconnect to a saved upstream network; on first run (no config at all) it
// launches a default open hotspot so the device is reachable over WiFi.
func autoStart(hotspot *HotspotManager, wifi *WifiManager, config *ConfigStore) {
	// Brief delay to let NetworkManager initialise.
	time.Sleep(3 * time.Second)

	// If there are saved networks, try to reconnect first.
	saved := config.GetSavedNetworks()
	if len(saved) > 0 {
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

	// First run: no saved hotspot config and no saved networks — start a
	// default open hotspot so the user can connect and reach the web UI.
	if config.GetHotspotConfig() != nil || len(saved) > 0 {
		return
	}

	log.Println("First run detected — starting default setup hotspot (open, no password)")
	cfg := HotspotConfig{
		SSID:       "HotSpotter-Setup",
		Gateway:    "192.168.12.1",
		NoInternet: true,
	}
	if err := hotspot.Start(cfg); err != nil {
		log.Printf("Failed to start default setup hotspot: %v", err)
		return
	}
	hotspot.SetSetupMode(true)
	log.Println("Setup hotspot running — connect to \"HotSpotter-Setup\" and open http://192.168.12.1:8080")
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

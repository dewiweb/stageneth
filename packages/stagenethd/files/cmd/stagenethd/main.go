package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"stageneth/internal/engine"
	"stageneth/internal/render"
	"stageneth/internal/stcfg"
	"stageneth/internal/ucifile"
)

// run executes a command with a hard timeout. init.d paths are remapped
// under STAGENETH_INIT_DIR when set, so tests can stub service scripts.
func run(name string, args ...string) error {
	if d := os.Getenv("STAGENETH_INIT_DIR"); d != "" && strings.HasPrefix(name, "/etc/init.d/") {
		name = filepath.Join(d, filepath.Base(name))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Run()
}

// runBounded kills the command after timeout — a stuck ethtool/init
// script must not hold the apply lock forever.
func runBounded(timeout time.Duration, name string, args ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	exec.CommandContext(ctx, name, args...).Run()
}

// tuneNics applies AV-friendly NIC settings on physical interfaces.
func tuneNics(avail map[string]bool) {
	for iface := range avail {
		if strings.HasPrefix(iface, "lo") ||
			strings.HasPrefix(iface, "br-") ||
			strings.HasPrefix(iface, "bond") ||
			strings.HasPrefix(iface, "docker") ||
			strings.HasPrefix(iface, "veth") ||
			strings.HasPrefix(iface, "wg") ||
			strings.HasPrefix(iface, "tun") ||
			strings.Contains(iface, ".") {
			continue
		}
		runBounded(2*time.Second, "ethtool", "--set-eee", iface, "eee", "disabled")
		runBounded(2*time.Second, "ethtool", "-G", iface, "rx", "4096", "tx", "4096")
		runBounded(2*time.Second, "ethtool", "-C", iface, "rx-usecs", "0", "tx-usecs", "0")
		runBounded(2*time.Second, "ethtool", "-K", iface, "gro", "off", "tso", "off", "lro", "off", "gso", "off")
	}
}

// reloadServices restarts only the services whose config changed.
// The first failing reload aborts and reports an error.
func reloadServices(changed []string, avail map[string]bool) error {
	for _, cfg := range changed {
		var err error
		switch cfg {
		case "network":
			err = run("/etc/init.d/network", "reload")
			tuneNics(avail)
			run("/etc/init.d/stageneth-ptp", "enable")
			run("/etc/init.d/stageneth-ptp", "restart")
		case "firewall":
			err = run("/etc/init.d/firewall", "reload")
		case "dhcp":
			err = run("/etc/init.d/dnsmasq", "restart")
		case "rsyslog":
			run("/etc/init.d/rsyslog", "enable")
			err = run("/etc/init.d/rsyslog", "restart")
		case "umdns":
			run("/etc/init.d/umdns", "enable")
			err = run("/etc/init.d/umdns", "restart")
		case "mdns_repeater":
			run("/etc/init.d/mdns-repeater", "enable")
			err = run("/etc/init.d/mdns-repeater", "restart")
		}
		if err != nil {
			return fmt.Errorf("%s reload failed: %w", cfg, err)
		}
	}
	return nil
}

func printPlan(p *engine.Plan) {
	empty := true
	for _, fc := range p.Changes {
		if !fc.Changed {
			continue
		}
		fmt.Printf("== %s ==\n", fc.Config)
		empty = false
		for _, s := range fc.Added {
			fmt.Printf("  + section %s\n", s)
		}
		for _, s := range fc.Removed {
			fmt.Printf("  - section %s\n", s)
		}
		for _, s := range fc.Modified {
			fmt.Printf("  ~ section %s\n", s)
		}
	}
	if p.RsyslogConfDiff {
		fmt.Println("== /etc/rsyslog.conf == (collector block changed)")
		empty = false
	}
	if empty {
		fmt.Println("No changes.")
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: stagenethd apply [-noreload]|preview|rollback")
		os.Exit(1)
	}
	st, err := stcfg.Load(filepath.Join(ucifile.ConfigDir, "stageneth"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load stageneth config:", err)
		os.Exit(1)
	}
	e := engine.New()
	cmd := os.Args[1]
	noReload := len(os.Args) > 2 && os.Args[2] == "-noreload"
	switch cmd {
	case "preview":
		plan, err := e.Plan(st)
		if err != nil {
			fmt.Fprintln(os.Stderr, "plan failed:", err)
			os.Exit(1)
		}
		printPlan(plan)
	case "apply":
		if st.Globals["show_lock"] == "1" {
			fmt.Fprintln(os.Stderr, "show lock is active — refusing to apply (set stageneth.globals.show_lock=0 to unlock)")
			os.Exit(3)
		}
		changed, snap, err := e.Apply(st)
		if err != nil {
			fmt.Fprintln(os.Stderr, "apply failed:", err)
			os.Exit(1)
		}
		if len(changed) == 0 {
			fmt.Println("No changes.")
			return
		}
		fmt.Println("Changed configs:", strings.Join(changed, ", "))
		if noReload {
			// boot path: services read the generated files at their own
			// start — reloading now could race ubus, and a failure would
			// wrongly trigger rollback. The files are the success path.
			if err := e.SaveLastGood(snap); err != nil {
				fmt.Fprintln(os.Stderr, "warning: last-good snapshot not saved:", err)
			}
			fmt.Println("Files written; service reload deferred to boot order.")
			return
		}
		if err := reloadServices(changed, render.AvailableInterfaces()); err != nil {
			fmt.Fprintln(os.Stderr, "reload failed:", err, "— rolling back")
			if rerr := e.Restore(snap); rerr != nil {
				fmt.Fprintln(os.Stderr, "rollback failed:", rerr)
			} else {
				// re-reload the restored config so runtime matches files
				reloadServices(changed, render.AvailableInterfaces())
			}
			os.Exit(1)
		}
		// apply succeeded: persist the pre-apply state as last-good so
		// `stagenethd rollback` can recover even after losing mgmt access
		if err := e.SaveLastGood(snap); err != nil {
			fmt.Fprintln(os.Stderr, "warning: last-good snapshot not saved:", err)
		}
		fmt.Println("StageNeth configuration applied.")
	case "rollback":
		restored, err := e.Rollback()
		if err != nil {
			fmt.Fprintln(os.Stderr, "rollback failed:", err)
			os.Exit(1)
		}
		// reload every service the restored files could affect
		var cfgs []string
		for _, p := range restored {
			base := filepath.Base(p)
			if base == "rsyslog.conf" {
				base = "rsyslog"
			}
			cfgs = append(cfgs, base)
		}
		if err := reloadServices(cfgs, render.AvailableInterfaces()); err != nil {
			fmt.Fprintln(os.Stderr, "files restored but reload failed:", err)
			os.Exit(1)
		}
		fmt.Println("Rolled back to last-good configuration:", strings.Join(restored, ", "))
	default:
		fmt.Println("Usage: stagenethd apply [-noreload]|preview|rollback")
		os.Exit(1)
	}
}

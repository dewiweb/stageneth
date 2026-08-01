package main

import (
	json "encoding/json"
	fmt "fmt"
	http "net/http"
	exec "os/exec"
	strings "strings"
)

type presetService struct {
	Name, VlanID, Priority, DSCP, PTP, Multicast, Untagged, MTU, IPAddr, Netmask, Description string
}
type presetForwarding struct {
	Name, Src, Dest string
}
type preset struct {
	Name, Label, Description, Category string
	Services                           []presetService
	Forwardings                        []presetForwarding
}

var stagenethPresets = []preset{
	{
		Name:        "base",
		Label:       "Base management + PTP",
		Description: "VLANs d'administration (mgmt) et de timing PTP",
		Category:    "base",
		Services: []presetService{
			{Name: "mgmt", VlanID: "10", Priority: "0", DSCP: "0", PTP: "0", Multicast: "0", MTU: "1500", Description: "Administration routeur"},
			{Name: "ptp", VlanID: "50", Priority: "7", DSCP: "46", PTP: "1", Multicast: "1", MTU: "1500", Description: "Precision Time Protocol"},
		},
		Forwardings: []presetForwarding{},
	},
	{
		Name:        "audio",
		Label:       "Audio (Dante / AES67 / AVB)",
		Description: "VLANs pour les flux audio professionnels",
		Category:    "audio",
		Services: []presetService{
			{Name: "mgmt", VlanID: "10", Priority: "0", DSCP: "0", PTP: "0", Multicast: "0", MTU: "1500", Description: "Administration routeur"},
			{Name: "ptp", VlanID: "50", Priority: "7", DSCP: "46", PTP: "1", Multicast: "1", MTU: "1500", Description: "Precision Time Protocol"},
			{Name: "dante", VlanID: "20", Priority: "7", DSCP: "46", PTP: "1", Multicast: "1", MTU: "1500", Description: "Dante audio"},
			{Name: "aes67", VlanID: "21", Priority: "6", DSCP: "34", PTP: "0", Multicast: "1", MTU: "1500", Description: "AES67 / RAVENNA"},
			{Name: "avb", VlanID: "22", Priority: "0", DSCP: "0", PTP: "1", Multicast: "0", MTU: "1500", Description: "AVB / Milan"},
		},
		Forwardings: []presetForwarding{
			{Name: "dante_to_mgmt", Src: "dante", Dest: "mgmt"},
			{Name: "aes67_to_mgmt", Src: "aes67", Dest: "mgmt"},
			{Name: "avb_to_mgmt", Src: "avb", Dest: "mgmt"},
		},
	},
	{
		Name:        "video",
		Label:       "Vidéo (NDI / ST 2110)",
		Description: "VLANs pour les flux vidéo sur IP",
		Category:    "video",
		Services: []presetService{
			{Name: "mgmt", VlanID: "10", Priority: "0", DSCP: "0", PTP: "0", Multicast: "0", MTU: "1500", Description: "Administration routeur"},
			{Name: "ptp", VlanID: "50", Priority: "7", DSCP: "46", PTP: "1", Multicast: "1", MTU: "1500", Description: "Precision Time Protocol"},
			{Name: "ndihx", VlanID: "30", Priority: "0", DSCP: "0", PTP: "0", Multicast: "1", MTU: "1500", Description: "NDI / NDI|HX"},
			{Name: "st2110", VlanID: "31", Priority: "6", DSCP: "34", PTP: "0", Multicast: "1", MTU: "1500", Description: "SMPTE ST 2110"},
		},
		Forwardings: []presetForwarding{
			{Name: "ndihx_to_mgmt", Src: "ndihx", Dest: "mgmt"},
			{Name: "st2110_to_mgmt", Src: "st2110", Dest: "mgmt"},
		},
	},
	{
		Name:        "light",
		Label:       "Lumière (Art-Net / sACN)",
		Description: "VLANs pour le contrôle lumière",
		Category:    "light",
		Services: []presetService{
			{Name: "mgmt", VlanID: "10", Priority: "0", DSCP: "0", PTP: "0", Multicast: "0", MTU: "1500", Description: "Administration routeur"},
			{Name: "ptp", VlanID: "50", Priority: "7", DSCP: "46", PTP: "1", Multicast: "1", MTU: "1500", Description: "Precision Time Protocol"},
			{Name: "artnet", VlanID: "40", Priority: "5", DSCP: "0", PTP: "0", Multicast: "0", MTU: "1500", IPAddr: "2.0.0.1", Netmask: "255.0.0.0", Description: "Art-Net"},
			{Name: "sacn", VlanID: "41", Priority: "5", DSCP: "0", PTP: "0", Multicast: "1", MTU: "1500", Description: "sACN E1.31"},
			{Name: "proprietary", VlanID: "42", Priority: "5", DSCP: "0", PTP: "0", Multicast: "1", MTU: "1500", Description: "MA-Net / autre"},
		},
		Forwardings: []presetForwarding{
			{Name: "artnet_to_mgmt", Src: "artnet", Dest: "mgmt"},
			{Name: "sacn_to_mgmt", Src: "sacn", Dest: "mgmt"},
			{Name: "proprietary_to_mgmt", Src: "proprietary", Dest: "mgmt"},
		},
	},
	{
		Name:        "full-show",
		Label:       "Spectacle complet",
		Description: "Tous les VLANs audio, vidéo, lumière, management et PTP",
		Category:    "show",
		Services: []presetService{
			{Name: "mgmt", VlanID: "10", Priority: "0", DSCP: "0", PTP: "0", Multicast: "0", MTU: "1500", Description: "Administration routeur"},
			{Name: "ptp", VlanID: "50", Priority: "7", DSCP: "46", PTP: "1", Multicast: "1", MTU: "1500", Description: "Precision Time Protocol"},
			{Name: "guest", VlanID: "99", Priority: "0", DSCP: "0", PTP: "0", Multicast: "0", MTU: "1500", Description: "Internet / backoffice"},
			{Name: "dante", VlanID: "20", Priority: "7", DSCP: "46", PTP: "1", Multicast: "1", MTU: "1500", Description: "Dante audio"},
			{Name: "aes67", VlanID: "21", Priority: "6", DSCP: "34", PTP: "0", Multicast: "1", MTU: "1500", Description: "AES67 / RAVENNA"},
			{Name: "avb", VlanID: "22", Priority: "0", DSCP: "0", PTP: "1", Multicast: "0", MTU: "1500", Description: "AVB / Milan"},
			{Name: "ndihx", VlanID: "30", Priority: "0", DSCP: "0", PTP: "0", Multicast: "1", MTU: "1500", Description: "NDI / NDI|HX"},
			{Name: "st2110", VlanID: "31", Priority: "6", DSCP: "34", PTP: "0", Multicast: "1", MTU: "9000", Description: "SMPTE ST 2110"},
			{Name: "artnet", VlanID: "40", Priority: "5", DSCP: "0", PTP: "0", Multicast: "0", MTU: "1500", IPAddr: "2.0.0.1", Netmask: "255.0.0.0", Description: "Art-Net"},
			{Name: "sacn", VlanID: "41", Priority: "5", DSCP: "0", PTP: "0", Multicast: "1", MTU: "1500", Description: "sACN E1.31"},
			{Name: "proprietary", VlanID: "42", Priority: "5", DSCP: "0", PTP: "0", Multicast: "1", MTU: "1500", Description: "MA-Net / autre"},
		},
		Forwardings: []presetForwarding{
			{Name: "mgmt_to_dante", Src: "mgmt", Dest: "dante"},
			{Name: "mgmt_to_aes67", Src: "mgmt", Dest: "aes67"},
			{Name: "mgmt_to_avb", Src: "mgmt", Dest: "avb"},
			{Name: "mgmt_to_ndihx", Src: "mgmt", Dest: "ndihx"},
			{Name: "mgmt_to_st2110", Src: "mgmt", Dest: "st2110"},
			{Name: "mgmt_to_artnet", Src: "mgmt", Dest: "artnet"},
			{Name: "mgmt_to_sacn", Src: "mgmt", Dest: "sacn"},
			{Name: "mgmt_to_proprietary", Src: "mgmt", Dest: "proprietary"},
			{Name: "dante_to_mgmt", Src: "dante", Dest: "mgmt"},
			{Name: "aes67_to_mgmt", Src: "aes67", Dest: "mgmt"},
			{Name: "avb_to_mgmt", Src: "avb", Dest: "mgmt"},
			{Name: "ndihx_to_mgmt", Src: "ndihx", Dest: "mgmt"},
			{Name: "st2110_to_mgmt", Src: "st2110", Dest: "mgmt"},
			{Name: "artnet_to_mgmt", Src: "artnet", Dest: "mgmt"},
			{Name: "sacn_to_mgmt", Src: "sacn", Dest: "mgmt"},
			{Name: "proprietary_to_mgmt", Src: "proprietary", Dest: "mgmt"},
		},
	},
}

func applyPresetToUci(name string, services []string) (string, error) {
	selectedSet := map[string]bool{}
	for _, s := range services {
		selectedSet[s] = true
	}
	var selected *preset
	for i := range stagenethPresets {
		if stagenethPresets[i].Name == name {
			selected = &stagenethPresets[i]
			break
		}
	}
	if selected == nil {
		return "", fmt.Errorf("preset not found")
	}
	exec.Command("rm", "-f", "/etc/config/stageneth").Run()
	exec.Command("touch", "/etc/config/stageneth").Run()
	commands := []string{}
	for _, s := range selected.Services {
		if len(services) > 0 && !selectedSet[s.Name] {
			continue
		}
		untagged := s.Untagged
		if untagged == "" {
			untagged = "0"
		}
		commands = append(commands, fmt.Sprintf("set stageneth.%s=service", s.Name))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.vlan_id='%s'", s.Name, s.VlanID))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.priority='%s'", s.Name, s.Priority))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.dscp='%s'", s.Name, s.DSCP))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.ptp='%s'", s.Name, s.PTP))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.multicast='%s'", s.Name, s.Multicast))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.untagged='%s'", s.Name, untagged))
		mtu := s.MTU
		if mtu == "" {
			mtu = "1500"
			if s.Name == "st2110" {
				mtu = "9000"
			}
		}
		commands = append(commands, fmt.Sprintf("set stageneth.%s.mtu='%s'", s.Name, mtu))
		if s.IPAddr != "" {
			commands = append(commands, fmt.Sprintf("set stageneth.%s.ipaddr='%s'", s.Name, s.IPAddr))
		}
		if s.Netmask != "" {
			commands = append(commands, fmt.Sprintf("set stageneth.%s.netmask='%s'", s.Name, s.Netmask))
		}
		commands = append(commands, fmt.Sprintf("set stageneth.%s.description='%s'", s.Name, s.Description))
	}
	for _, f := range selected.Forwardings {
		if len(services) > 0 && (!selectedSet[f.Src] || !selectedSet[f.Dest]) {
			continue
		}
		commands = append(commands, fmt.Sprintf("set stageneth.%s=forwarding", f.Name))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.src='%s'", f.Name, f.Src))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.dest='%s'", f.Name, f.Dest))
		commands = append(commands, fmt.Sprintf("set stageneth.%s.enabled='1'", f.Name))
	}
	in := strings.Join(commands, "\n") + "\n"
	cmd := exec.Command("uci", "-q", "batch")
	cmd.Stdin = strings.NewReader(in)
	if err := cmd.Run(); err != nil {
		return "", err
	}
	exec.Command("uci", "-q", "commit", "stageneth").Run()
	out, err := exec.Command("/usr/sbin/stageneth-network", "apply").CombinedOutput()
	return string(out), err
}

func presetsList(w http.ResponseWriter, r *http.Request) {
	out := []map[string]interface{}{}
	for _, p := range stagenethPresets {
		svcs := []string{}
		for _, s := range p.Services {
			svcs = append(svcs, s.Name)
		}
		out = append(out, map[string]interface{}{
			"name":        p.Name,
			"label":       p.Label,
			"description": p.Description,
			"category":    p.Category,
			"services":    svcs,
		})
	}
	respond(w, 200, out, "presets list")
}
func presetApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string   `json:"name"`
		Services []string `json:"services"`
	}
	if json.NewDecoder(r.Body).Decode(&req) != nil || req.Name == "" {
		respond(w, 400, nil, "invalid preset name")
		return
	}
	out, err := applyPresetToUci(req.Name, req.Services)
	if err != nil {
		respond(w, 500, map[string]string{"log": out}, "preset apply failed")
		return
	}
	respond(w, 200, map[string]string{"log": out}, "preset applied")
}

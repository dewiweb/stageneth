package api

import (
	context "context"
	json "encoding/json"
	fmt "fmt"
	http "net/http"
	exec "os/exec"
	"stageneth/internal/ucifile"
	time "time"
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

// errPresetValidation marks request-validation failures (bad name,
// unknown service) so the handler can return 400 instead of 500.
type errPresetValidation struct{ msg string }

func (e *errPresetValidation) Error() string { return e.msg }

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

// applyPresetToUci rewrites the stageneth config with the preset's
// service topology, preserving globals and logging sections (previous
// implementation rm'd the file and destroyed global settings).
// Returns the stagenethd apply output.
func applyPresetToUci(name string, services []string) (string, error) {
	selectedSet := map[string]bool{}
	for _, s := range services {
		selectedSet[s] = true
	}
	// mgmt can never be deselected — dropping it deletes the management
	// VLAN on apply and locks the operator out of the appliance
	if len(services) > 0 {
		selectedSet["mgmt"] = true
	}
	var selected *preset
	for i := range stagenethPresets {
		if stagenethPresets[i].Name == name {
			selected = &stagenethPresets[i]
			break
		}
	}
	if selected == nil {
		return "", &errPresetValidation{"preset not found"}
	}
	// the services filter must name preset services — a typo silently
	// producing a mgmt-only config is worse than an error
	if len(services) > 0 {
		valid := map[string]bool{}
		for _, s := range selected.Services {
			valid[s.Name] = true
		}
		for _, s := range services {
			if !valid[s] {
				return "", &errPresetValidation{fmt.Sprintf("service %q not in preset %q", s, name)}
			}
		}
	}
	f, _, err := loadStageneth()
	if err != nil {
		return "", err
	}
	// keep globals + logging; drop service/binding/forwarding (the
	// topology is fully replaced by the preset)
	var out []ucifile.Section
	for _, s := range f.Sections {
		if s.Type == "stageneth" || s.Type == "logging" {
			out = append(out, s)
		}
	}
	for _, s := range selected.Services {
		if len(services) > 0 && !selectedSet[s.Name] {
			continue
		}
		mtu := s.MTU
		if mtu == "" {
			mtu = "1500"
			if s.Name == "st2110" {
				mtu = "9000"
			}
		}
		untagged := s.Untagged
		if untagged == "" {
			untagged = "0"
		}
		opts := []ucifile.Option{
			{Name: "vlan_id", Value: s.VlanID},
			{Name: "priority", Value: s.Priority},
			{Name: "dscp", Value: s.DSCP},
			{Name: "ptp", Value: s.PTP},
			{Name: "multicast", Value: s.Multicast},
			{Name: "untagged", Value: untagged},
			{Name: "mtu", Value: mtu},
		}
		if s.IPAddr != "" {
			opts = append(opts, ucifile.Option{Name: "ipaddr", Value: s.IPAddr})
		}
		if s.Netmask != "" {
			opts = append(opts, ucifile.Option{Name: "netmask", Value: s.Netmask})
		}
		opts = append(opts, ucifile.Option{Name: "description", Value: s.Description})
		out = append(out, ucifile.Section{Type: "service", Name: s.Name, Options: opts})
	}
	for _, fw := range selected.Forwardings {
		if len(services) > 0 && (!selectedSet[fw.Src] || !selectedSet[fw.Dest]) {
			continue
		}
		out = append(out, ucifile.Section{Type: "forwarding", Name: fw.Name, Options: []ucifile.Option{
			{Name: "src", Value: fw.Src},
			{Name: "dest", Value: fw.Dest},
			{Name: "enabled", Value: "1"},
		}})
	}
	f.Sections = out
	if err := saveStageneth(f); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out2, err := exec.CommandContext(ctx, stagenethdBin, "apply").CombinedOutput()
	return string(out2), err
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
	name := r.PathValue("name")
	var req struct {
		Services []string `json:"services"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if name == "" {
		respond(w, 400, nil, "invalid preset name")
		return
	}
	out, err := applyPresetToUci(name, req.Services)
	if err != nil {
		code := 500
		if _, ok := err.(*errPresetValidation); ok {
			code = 400
		}
		respond(w, code, map[string]string{"log": out}, "preset apply failed")
		return
	}
	audit("preset_apply", name, r)
	respond(w, 200, map[string]string{"log": out}, "preset applied")
}

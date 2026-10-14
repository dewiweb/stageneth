// LAN/WAN and interface listing.
var netMethods = {
    async loadLan() {
      try {
        const res = await this.uciGet('network');
        const lan = (res.data?.values || {}).lan || {};
        this.lan.ipaddr = lan.ipaddr || '';
        this.lan.netmask = lan.netmask || '';
        this.lan.gateway = lan.gateway || '';
        this.lan.proto = lan.proto || 'static';
        this.lan.device = lan.device || '';
      } catch (e) {
        this.toast('Failed to load LAN settings: ' + e.message, 'error');
      }
    },
    async loadWan() {
      try {
        const res = await this.uciGet('network');
        const wan = (res.data?.values || {}).wan || {};
        this.wan.ipaddr = wan.ipaddr || '';
        this.wan.netmask = wan.netmask || '';
        this.wan.gateway = wan.gateway || '';
        this.wan.proto = wan.proto || 'dhcp';
        this.wan.device = wan.device || '';
        this.wan.macaddr = wan.macaddr || '';
      } catch (e) {
        this.toast('Failed to load WAN settings: ' + e.message, 'error');
      }
    },
    async saveLan() {
      this.isLoading = true;
      try {
        await api('/api/v1/raw/network', 'PUT', {
          section: 'lan',
          type: 'interface',
          values: {
            proto: this.lan.proto || 'static',
            ipaddr: this.lan.ipaddr,
            netmask: this.lan.netmask,
            gateway: this.lan.gateway,
            device: this.lan.device
          }
        });
        await api('/api/v1/network/reload', 'POST', {});
        this.toast('LAN settings saved and network reloaded', 'success');
      } catch (e) {
        this.toast('LAN save failed: ' + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async saveWan() {
      this.isLoading = true;
      try {
        await api('/api/v1/raw/network', 'PUT', {
          section: 'wan',
          type: 'interface',
          values: {
            proto: this.wan.proto || 'dhcp',
            ipaddr: this.wan.ipaddr,
            netmask: this.wan.netmask,
            gateway: this.wan.gateway,
            device: this.wan.device,
            macaddr: this.wan.macaddr
          }
        });
        await api('/api/v1/network/reload', 'POST', {});
        this.toast('WAN settings saved and network reloaded', 'success');
      } catch (e) {
        this.toast('WAN save failed: ' + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async loadNetworkInterfaces() {
      try {
        const res = await api('/api/v1/interfaces', 'GET');
        this.availableInterfaces = (res.data && res.data.interfaces) ? res.data.interfaces : [];
      } catch (e) {
        this.toast('Failed to load network interfaces: ' + e.message, 'error');
      }
    },
    async ensureDefaultBinding() {
      const bindings = this.bindings || {};
      if (Object.keys(bindings).length > 0) return;
      const services = this.services || {};
      const serviceNames = Object.keys(services);
      if (serviceNames.length === 0) return;
      const firstService = serviceNames[0];
      const defaultIface = this.physicalInterfaces.find(i => i.startsWith('eth')) || this.physicalInterfaces[0];
      if (!defaultIface) return;
      try {
        await this.addUci('stageneth', 'default_binding', 'binding', { interface: defaultIface, service: firstService },
          { newBinding: { name: '', interface: '', service: '' }, editing: { type: '', name: '' } });
        this.toast('Created default binding: ' + defaultIface + ' → ' + firstService, 'success');
      } catch (e) {
        this.toast('Failed to create default binding: ' + e.message, 'error');
      }
    },
    async pingService(name) {
      const ip = (this.serviceNetworks[name] || {}).ip;
      if (!ip) return;
      this.pingResult[name] = { loading: true };
      try {
        const res = await api('/api/v1/ping?ip=' + encodeURIComponent(ip), 'GET');
        this.pingResult[name] = { ok: res.data?.ok, output: res.data?.output, loading: false };
      } catch (e) {
        this.pingResult[name] = { ok: false, output: e.message, loading: false };
      }
    }
};

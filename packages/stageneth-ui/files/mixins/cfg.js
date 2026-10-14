// StageNeth config CRUD: typed v1 writes, raw expert UCI, apply/preview, backup/restore.
var cfgMethods = {
    async uciSet(config, section, type, values) {
      return api('/api/v1/raw/' + config, 'PUT', { section, type, values });
    },
    async uciGet(config) {
      return api('/api/v1/raw/' + config, 'GET');
    },
    async uciDelete(config, section) {
      return api('/api/v1/raw/' + config + '?section=' + encodeURIComponent(section), 'DELETE');
    },
    async refresh() {
      this.isLoading = true;
      try {
        {
          const res = await api('/api/v1/state', 'GET');
          this.uci.stageneth = { values: this.stateToUci(res.data || {}) };
        }
        for (const config of ['network', 'dhcp', 'firewall', 'system']) {
          const res = await this.uciGet(config);
          this.uci[config] = res.data || {};
        }
        const values = this.uci.system?.values || {};
        const sys = values['@system[0]'] || {};
        this.system.hostname = sys.hostname || 'stageneth';
        const remote = (this.uci.stageneth.values || {}).remote || {};
        this.logging.enabled = remote.enabled || '0';
        this.logging.host = remote.host || '';
        this.logging.port = remote.port || '514';
        this.logging.protocol = remote.protocol || 'udp';
        const collector = (this.uci.stageneth.values || {}).collector || {};
        this.collector.enabled = collector.enabled || '0';
        this.collector.port = collector.port || '514';
        this.collector.protocol = collector.protocol || 'udp';
        this.collector.dir = collector.dir || '/var/log/remote';
        this.toast('Données actualisées', 'success');
      } catch (e) {
        this.toast("Échec de l'actualisation : " + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async saveLogging() {
      this.isLoading = true;
      try {
        await api('/api/v1/logging/remote', 'PUT', {
          enabled: this.logging.enabled,
          host: this.logging.host,
          port: this.logging.port,
          protocol: this.logging.protocol
        });
        await api('/api/v1/logging/collector', 'PUT', {
          enabled: this.collector.enabled,
          port: this.collector.port,
          protocol: this.collector.protocol,
          dir: this.collector.dir
        });
        this.hasPendingChanges = true;
        this.toast('Configuration syslog enregistrée — appliquez pour activer', 'success');
        await this.refresh();
      } catch (e) {
        this.toast("Échec de l'enregistrement : " + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async saveSystem() {
      this.isLoading = true;
      try {
        await this.uciSet('system', '@system[0]', 'system', { hostname: this.system.hostname });
        this.toast('Paramètres système enregistrés', 'success');
        await this.refresh();
      } catch (e) {
        this.toast("Échec de l'enregistrement : " + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async removeUci(config, section) {
      if (!confirm('Confirmer la suppression de ' + section + ' ?')) return;
      this.isLoading = true;
      try {
        if (config === 'stageneth') {
          const type = this.uci.stageneth?.values?.[section]?.['.type'];
          const path = this.v1PathForType(type);
          if (path) {
            await api('/api/v1/' + path + '/' + section, 'DELETE');
          } else {
            throw new Error('Type de section inconnu : ' + type);
          }
        } else {
          await this.uciDelete(config, section);
        }
        this.hasPendingChanges = true;
        await this.refresh();
      } catch (e) {
        this.toast('Remove failed: ' + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async addUci(config, section, type, values, reset) {
      this.isLoading = true;
      try {
        if (config === 'stageneth') {
          const path = this.v1PathForType(type);
          if (!path) throw new Error('Type de section inconnu : ' + type);
          await api('/api/v1/' + path + '/' + section, 'PUT', values);
        } else {
          await this.uciSet(config, section, type, values);
        }
        this.hasPendingChanges = true;
        Object.assign(this, reset);
        await this.refresh();
      } catch (e) {
        this.toast('Add failed: ' + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    startEdit(type, name, data) {
      this.editing = { type, name };
      if (type === 'service') Object.assign(this.newService, { name, vlan_id: data.vlan_id || '', dscp: data.dscp || '', priority: data.priority || '', mtu: data.mtu || '', ipaddr: data.ipaddr || '', netmask: data.netmask || '', ptp: data.ptp || '0', multicast: data.multicast || '0', untagged: data.untagged || '0' });
      else if (type === 'binding') Object.assign(this.newBinding, { name, interface: data.interface || '', service: data.service || '' });
      else if (type === 'forwarding') Object.assign(this.newForwarding, { name, src: data.src || '', dest: data.dest || '', enabled: data.enabled || '1' });
      else if (type === 'interface') Object.assign(this.newInterface, { name, proto: data.proto || 'static', ipaddr: data.ipaddr || '', netmask: data.netmask || '', device: data.device || '' });
      else if (type === 'device') Object.assign(this.newDevice, { name, type: data.type || '8021q', vid: data.vid || '', ifname: data.ifname || data.ports || '' });
      else if (type === 'dhcp') Object.assign(this.newDhcp, { name, interface: data.interface || '', start: data.start || '100', limit: data.limit || '50', leasetime: data.leasetime || '12h' });
      else if (type === 'dns') Object.assign(this.newDns, { name, domain: data.domain || '', local: data.local || '', server: data.server || '' });
      else if (type === 'zone') Object.assign(this.newZone, { name, network: data.network || '', input: data.input || 'REJECT', output: data.output || 'ACCEPT', forward: data.forward || 'REJECT' });
      else if (type === 'fwforwarding') Object.assign(this.newFwForwarding, { name, src: data.src || '', dest: data.dest || '', enabled: data.enabled || '1' });
      else if (type === 'rule') Object.assign(this.newRule, { name: data.name || name, src: data.src || '', proto: data.proto || 'tcp udp', dest_port: data.dest_port || '', target: data.target || 'ACCEPT' });
      else if (type === 'host') Object.assign(this.newHost, { name: data.name || '', mac: data.mac || '', ip: data.ip || '', dns: data.dns || '1' });
    },
    async addService() {
      const s = this.newService;
      const section = this.editing.type === 'service' ? this.editing.name : s.name;
      await this.addUci('stageneth', section, 'service', {
        vlan_id: s.vlan_id, dscp: s.dscp, priority: s.priority, mtu: s.mtu, ipaddr: s.ipaddr, netmask: s.netmask, ptp: s.ptp, multicast: s.multicast, untagged: s.untagged
      }, { newService: { name: '', vlan_id: '20', dscp: '0', priority: '0', mtu: '', ipaddr: '', netmask: '', ptp: '0', multicast: '0', untagged: '0' }, editing: { type: '', name: '' } });
    },
    async addBinding() {
      const b = this.newBinding;
      const section = this.editing.type === 'binding' ? this.editing.name : b.name;
      await this.addUci('stageneth', section, 'binding', { interface: b.interface, service: b.service },
        { newBinding: { name: '', interface: '', service: '' }, editing: { type: '', name: '' } });
    },
    async addForwarding() {
      const f = this.newForwarding;
      const section = this.editing.type === 'forwarding' ? this.editing.name : f.name;
      await this.addUci('stageneth', section, 'forwarding', { src: f.src, dest: f.dest, enabled: f.enabled },
        { newForwarding: { name: '', src: '', dest: '', enabled: '1' }, editing: { type: '', name: '' } });
    },
    async addInterface() {
      const i = this.newInterface;
      const section = this.editing.type === 'interface' ? this.editing.name : i.name;
      await this.addUci('network', section, 'interface', { proto: i.proto, ipaddr: i.ipaddr, netmask: i.netmask, device: i.device },
        { newInterface: { name: '', proto: 'static', ipaddr: '', netmask: '', device: '' }, editing: { type: '', name: '' } });
    },
    async addDevice() {
      const d = this.newDevice;
      const section = this.editing.type === 'device' ? this.editing.name : d.name;
      const values = { type: d.type };
      if (d.type === '8021q') { values.vid = d.vid; values.ifname = d.ifname; }
      if (d.type === 'bridge') { values.ports = d.ifname; }
      await this.addUci('network', section, 'device', values,
        { newDevice: { name: '', type: '8021q', vid: '', ifname: '' }, editing: { type: '', name: '' } });
    },
    async addDhcp() {
      const d = this.newDhcp;
      const section = this.editing.type === 'dhcp' ? this.editing.name : d.name;
      await this.addUci('dhcp', section, 'dhcp', { interface: d.interface, start: d.start, limit: d.limit, leasetime: d.leasetime },
        { newDhcp: { name: '', interface: '', start: '100', limit: '50', leasetime: '12h' }, editing: { type: '', name: '' } });
    },
    async addDns() {
      const d = this.newDns;
      const section = this.editing.type === 'dns' ? this.editing.name : d.name;
      await this.addUci('dhcp', section, 'dnsmasq', { domain: d.domain, local: d.local, server: d.server },
        { newDns: { name: '', domain: 'lan', local: '/lan/', server: '' }, editing: { type: '', name: '' } });
    },
    async addZone() {
      const z = this.newZone;
      const section = this.editing.type === 'zone' ? this.editing.name : z.name;
      await this.addUci('firewall', section, 'zone', { name: z.name, network: z.network, input: z.input, output: z.output, forward: z.forward },
        { newZone: { name: '', network: '', input: 'REJECT', output: 'ACCEPT', forward: 'REJECT' }, editing: { type: '', name: '' } });
    },
    async addFwForwarding() {
      const f = this.newFwForwarding;
      const section = this.editing.type === 'fwforwarding' ? this.editing.name : f.name;
      await this.addUci('firewall', section, 'forwarding', { src: f.src, dest: f.dest, enabled: f.enabled },
        { newFwForwarding: { name: '', src: '', dest: '', enabled: '1' }, editing: { type: '', name: '' } });
    },
    async addRule() {
      const r = this.newRule;
      const section = this.editing.type === 'rule' ? this.editing.name : r.name;
      await this.addUci('firewall', section, 'rule', { name: r.name, src: r.src, proto: r.proto, dest_port: r.dest_port, target: r.target },
        { newRule: { name: '', src: '', proto: 'tcp udp', dest_port: '', target: 'ACCEPT' }, editing: { type: '', name: '' } });
    },
    async addHost() {
      const h = this.newHost;
      const section = this.editing.type === 'host' ? this.editing.name : 'host_' + (h.name || 'unknown').replace(/[^a-zA-Z0-9_-]/g, '_');
      await this.addUci('dhcp', section, 'host', { name: h.name, mac: h.mac, ip: h.ip, dns: h.dns },
        { newHost: { name: '', mac: '', ip: '', dns: '1' }, editing: { type: '', name: '' } });
    },
    async addStaticFromLease(l) {
      const name = l.hostname || ('host_' + l.ip);
      const section = 'host_' + name.replace(/[^a-zA-Z0-9_-]/g, '_');
      await this.addUci('dhcp', section, 'host', { name, mac: l.mac, ip: l.ip, dns: '1' },
        { newHost: { name: '', mac: '', ip: '', dns: '1' }, editing: { type: '', name: '' } });
    },
    async saveGlobals() {
      this.isLoading = true;
      try {
        await api('/api/v1/globals', 'PUT', { switch_vendor: this.switchVendor, trunk: this.trunk, ptp_timestamping: this.ptpTimestamping });
        this.hasPendingChanges = true;
        this.toast('Paramètres globaux enregistrés', 'success');
      } catch (e) {
        this.toast("Échec de l'enregistrement : " + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async toggleShowLock() {
      const next = this.showLock ? '0' : '1';
      try {
        await api('/api/v1/globals', 'PUT', { show_lock: next });
        await this.refresh();
        this.toast(next === '1' ? 'Show lock activé — apply bloqué' : 'Show lock désactivé', next === '1' ? 'info' : 'success');
      } catch (e) {
        this.toast('Échec du show lock : ' + e.message, 'error');
      }
    },
    async apply() {
      this.hasPendingChanges = false;
      this.isLoading = true;
      try {
        const res = await api('/api/v1/apply', 'POST', {});
        this.toast(res.message + ' ' + (res.data?.log || ''), 'success');
      } catch (e) {
        this.toast("Échec de l'application : " + e.message, 'error');
        this.hasPendingChanges = true;
      } finally {
        this.isLoading = false;
      }
    },
    async reload() {
      this.isLoading = true;
      try {
        const res = await api('/api/v1/network/reload', 'POST', {});
        this.toast(res.message, 'success');
      } catch (e) {
        this.toast('Reload failed: ' + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async backupConfig() {
      try {
        const res = await api('/api/v1/backup', 'GET');
        const text = res.data?.data || '';
        const blob = new Blob([text], { type: 'text/plain' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = 'stageneth-config.txt';
        a.click();
        URL.revokeObjectURL(url);
        this.toast('Sauvegarde téléchargée', 'success');
      } catch (e) { this.toast('Sauvegarde échouée : ' + e.message, 'error'); }
    },
    async restoreConfig() {
      try {
        const text = this.$refs.restoreText?.value || '';
        if (!text) return this.toast('Collez une sauvegarde avant', 'warning');
        const res = await api('/api/v1/restore', 'POST', { data: text });
        this.toast(res.message, 'success');
        await this.refresh();
      } catch (e) { this.toast('Restauration échouée : ' + e.message, 'error'); }
    }
};

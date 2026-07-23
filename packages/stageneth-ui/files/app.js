const { createApp } = Vue;


createApp({
  data() {
    return {
      token: localStorage.getItem('stageneth_token_v2') || '',
      username: 'root',
      password: '',
      error: '',
      message: '',
      messageType: 'info',
      isLoading: false,
      currentTab: localStorage.getItem('stageneth_tab') || 'simple',
      simpleMode: (localStorage.getItem('stageneth_simple') !== null ? localStorage.getItem('stageneth_simple') === 'true' : true),
      mobileMenuOpen: false,
      messageTimeout: null,
      tabGroups: [
        { label: 'Vue d\'ensemble', tabs: [
          { key: 'simple', label: 'Tableau de bord', icon: 'fas fa-home' }
        ]},
        { label: 'Services & flux', tabs: [
          { key: 'services', label: 'Services', icon: 'fas fa-layer-group' },
          { key: 'monitoring', label: 'Monitoring', icon: 'fas fa-chart-line' },
          { key: 'snmp', label: 'SNMP', icon: 'fas fa-satellite-dish' },
          { key: 'discovery', label: 'Discovery', icon: 'fas fa-search' },
          { key: 'logs', label: 'Logs', icon: 'fas fa-scroll' }
        ]},
        { label: 'Infrastructure', tabs: [
          { key: 'network', label: 'Network', icon: 'fas fa-network-wired' },
          { key: 'dhcpdns', label: 'DHCP & DNS', icon: 'fas fa-server' },
          { key: 'firewall', label: 'Firewall', icon: 'fas fa-shield-alt' },
          { key: 'ntp', label: 'NTP', icon: 'fas fa-clock' }
        ]},
        { label: 'Paramètres', tabs: [
          { key: 'credentials', label: 'Credentials', icon: 'fas fa-user-lock' },
          { key: 'lan', label: 'LAN/WAN', icon: 'fas fa-ethernet' }
        ]}
      ],
      isDark: true,
      uci: { stageneth: {}, network: {}, dhcp: {}, firewall: {} },
      newService: { name: '', vlan_id: '20', dscp: '0', priority: '0', mtu: '', ptp: '0', multicast: '0', untagged: '0' },
      newBinding: { name: '', interface: '', service: '' },
      newForwarding: { name: '', src: '', dest: '', enabled: '1' },
      newInterface: { name: '', proto: 'static', ipaddr: '', netmask: '', device: '' },
      newDevice: { name: '', type: '8021q', vid: '', ifname: '' },
      newDhcp: { name: '', interface: '', start: '100', limit: '50', leasetime: '12h' },
      newDns: { name: '', domain: 'lan', local: '/lan/', server: '' },
      newZone: { name: '', network: '', input: 'REJECT', output: 'ACCEPT', forward: 'REJECT' },
      newFwForwarding: { name: '', src: '', dest: '', enabled: '1' },
      newRule: { name: '', src: '', proto: 'tcp udp', dest_port: '', target: 'ACCEPT' },
      newHost: { name: '', mac: '', ip: '', dns: '1' },
      monitoring: {},
      monitoringHistory: { cpu: [], memory: [] },
      monitoringTimeout: null,
      snmpQuery: { host: '', port: 161, community: 'public', oid: '1.3.6.1.2.1.1.1.0' },
      snmpResult: [],
      mdnsService: '_services._dns-sd._udp',
      mdnsDuration: 3,
      mdnsResults: [],
      ntp: { enabled: false, enable_server: false, servers: [], newServer: '', timezone: 'UTC0' },
      ntpTime: { date: '-', time: '-', timezone: '', source: '', stratum: '', offset: '' },
      ntpTimeInterval: null,
      timezones: [
        { label: 'UTC', value: 'UTC0' },
        { label: 'Europe/Paris', value: 'CET-1CEST,M3.5.0,M10.5.0/3' },
        { label: 'Europe/London', value: 'GMT0BST,M3.5.0/1,M10.5.0/2' },
        { label: 'Europe/Berlin', value: 'CET-1CEST,M3.5.0,M10.5.0/3' },
        { label: 'America/New York', value: 'EST5EDT,M3.2.0,M11.1.0' },
        { label: 'America/Chicago', value: 'CST6CDT,M3.2.0,M11.1.0' },
        { label: 'America/Los Angeles', value: 'PST8PDT,M3.2.0,M11.1.0' },
        { label: 'America/Sao Paulo', value: 'BRT3BRST,M10.3.0/0,M2.3.0/0' },
        { label: 'Asia/Dubai', value: 'GST-4' },
        { label: 'Asia/Shanghai', value: 'CST-8' },
        { label: 'Asia/Tokyo', value: 'JST-9' },
        { label: 'Asia/Singapore', value: 'SGT-8' },
        { label: 'Australia/Sydney', value: 'AEST-10AEDT,M10.1.0,M4.1.0/3' },
        { label: 'Pacific/Auckland', value: 'NZST-12NZDT,M9.5.0/2,M4.1.0/3' }
      ],
      logLines: [],
      logsInterval: null,
      logsPaused: false,
      logsLimit: 200,
      passwordChange: { current_password: '', new_password: '', confirm_password: '' },
      lan: { ipaddr: '', netmask: '', gateway: '', proto: 'static', device: '' },
      wan: { ipaddr: '', netmask: '', gateway: '', proto: 'dhcp', device: '', macaddr: '' },
      availableInterfaces: [],
      presets: [],
      selectedPreset: '',
      selectedPresetServices: [],
      switchVendor: 'generic',
      trunk: 'eth1',
      editing: { type: '', name: '' },
      hasPendingChanges: false,
      showWizard: false,
      wizardStep: 1,
      wizardPassword: '',
      wizardConfirmPassword: '',
      wizardServices: {}
    };
  },
  computed: {
    currentPreset() { return this.presets.find(p => p.name === this.selectedPreset) || {}; },
    services() { return this.filterType('stageneth', 'service'); },
    bindings() { return this.filterType('stageneth', 'binding'); },
    forwardings() { return this.filterType('stageneth', 'forwarding'); },
    networkInterfaces() { return this.filterType('network', 'interface'); },
    bridges() { return Object.fromEntries(Object.entries(this.filterType('network', 'device') || {}).filter(([,s]) => s.type === 'bridge')); },
    vlans() { return Object.fromEntries(Object.entries(this.filterType('network', 'device') || {}).filter(([,s]) => s.type === '8021q')); },
    networkDevices() { return this.filterType('network', 'device'); },
    serviceNetworks() {
      const services = this.services || {};
      const interfaces = this.networkInterfaces || {};
      const devices = this.networkDevices || {};
      const result = {};
      for (const [name, svc] of Object.entries(services)) {
        const bridge = devices[`br_${name}`] || {};
        const vlan = Object.entries(devices).find(([,d]) => d.type === '8021q' && d.name && bridge.ports && bridge.ports.includes(d.name));
        const iface = interfaces[`svc_${name}`] || {};
        result[name] = {
          vlan_id: svc.vlan_id,
          bridge: `br_${name}`,
          bridgePorts: bridge.ports || '-',
          vlan: vlan ? vlan[1].name : '-',
          vlanParent: vlan ? vlan[1].ifname : '-',
          interface: `svc_${name}`,
          ip: iface.ipaddr || '-',
          netmask: iface.netmask || '-'
        };
      }
      return result;
    },
    physicalInterfaces() {
      return (this.availableInterfaces || [])
        .map(i => (typeof i === 'string' ? i : i.name))
        .filter(i => i && !i.startsWith('br_') && !i.startsWith('lo') && !i.includes('.'));
    },
    dhcpPools() { return this.filterType('dhcp', 'dhcp'); },
    dnsSections() { return this.filterType('dhcp', 'dnsmasq'); },
    staticLeases() { return this.filterType('dhcp', 'host'); },
    firewallZones() { return this.filterType('firewall', 'zone'); },
    firewallForwardings() { return this.filterType('firewall', 'forwarding'); },
    firewallRules() { return this.filterType('firewall', 'rule'); },
    servicesCount() { return Object.keys(this.services || {}).length; },
    leasesCount() { return (this.monitoring.dhcp_leases || []).length; },
    vendorTip() {
      const tips = {
        generic: 'Schéma standard 802.1Q. Choisissez un constructeur pour des conseils spécifiques.',
        luminex: 'GigaCore gère PTP, IGMP et les profils AV. Laissez le multicast/PTP au switch.',
        cisco: 'Activez IGMP snooping, IGMP querier, jumbo frames et trust DSCP sur le port trunk.',
        netgear: 'Activez IGMP snooping, IGMP querier, jumbo frames et classofservice trust DSCP.'
      };
      return tips[this.switchVendor] || tips.generic;
    },
    visibleTabGroups() {
      if (!this.simpleMode) return this.tabGroups;
      return [
        { label: 'Vue d\'ensemble', tabs: [
          { key: 'simple', label: 'Tableau de bord', icon: 'fas fa-home' }
        ]},
        { label: 'Supervision', tabs: [
          { key: 'monitoring', label: 'Monitoring', icon: 'fas fa-chart-line' },
          { key: 'logs', label: 'Logs', icon: 'fas fa-scroll' }
        ]}
      ];
    },
    visibleTabs() { return this.visibleTabGroups.flatMap(g => g.tabs).map(t => t.key); }
  },
  watch: {
    currentTab(tab) {
      localStorage.setItem('stageneth_tab', tab);
      this.stopLogs();
      this.stopNtpTime();
      if (tab === 'monitoring') { this.fetchMonitoring(); }
      else if (tab === 'logs') { this.startLogs(); }
      else if (tab === 'ntp') { this.loadNtp(); this.fetchNtpTime(); this.startNtpTime(); }
      else { this.refresh(); }
      this.mobileMenuOpen = false;
    },
    message(v) {
      if (this.messageTimeout) clearTimeout(this.messageTimeout);
      if (v) this.messageTimeout = setTimeout(() => { this.message = ''; }, 4000);
    },
    'wan.device'(newVal) {
      const mac = this.interfaceMac(newVal);
      if (mac && !this.wan.macaddr) {
        this.wan.macaddr = mac;
      }
    }
  },
  async mounted() {
    const saved = localStorage.getItem('stageneth_theme');
    this.isDark = saved ? saved === 'dark' : true;
    if (this.isDark) document.documentElement.classList.add('dark');
    else document.documentElement.classList.remove('dark');
    await this.checkFirstboot();
    if (this.showWizard) return;
    if (this.token) {
      await this.refresh();
      this.switchVendor = (this.uci.stageneth?.values?.globals?.switch_vendor) || 'generic';
      this.trunk = (this.uci.stageneth?.values?.globals?.trunk) || 'eth1';
      this.simpleMode = (localStorage.getItem('stageneth_simple') !== null ? localStorage.getItem('stageneth_simple') === 'true' : true);
      if (!this.visibleTabs.includes(this.currentTab)) this.currentTab = 'simple';
      this.startMonitoring();
      await this.loadPresets();
      await this.loadLan();
      await this.loadWan();
      await this.loadNetworkInterfaces();
      await this.ensureDefaultBinding();
      await this.loadNtp();
    }
  },
  beforeUnmount() {
    this.stopMonitoring();
    this.stopLogs();
    this.stopNtpTime();
  },
  methods: {
    ...utils,
    toast(msg, type='info') {
      this.message = msg;
      this.messageType = type;
    },
    filterType(config, type) {
      const values = this.uci[config]?.values || {};
      return Object.fromEntries(Object.entries(values).filter(([_, v]) => v && v['.type'] === type));
    },
    async login() {
      try {
        const res = await api('/api/login', 'POST', { username: this.username, password: this.password });
        this.token = res.data.token;
        localStorage.setItem('stageneth_token_v2', this.token);
        this.error = '';
        await this.refresh();
        this.startMonitoring();
        await this.loadPresets();
      } catch (e) {
        this.error = e.message;
      }
    },
    logout() {
      this.stopMonitoring();
      this.token = '';
      localStorage.removeItem('stageneth_token_v2');
    },
    async checkFirstboot() {
      try {
        const res = await api('/api/firstboot', 'GET');
        if (res.data && !res.data.firstboot_done) {
          this.showWizard = true;
          this.wizardServices = Object.fromEntries((res.data.services || []).map(s => [s, true]));
        }
      } catch (e) {}
    },
    nextWizardStep() {
      this.error = '';
      if (this.wizardPassword.length < 4) { this.error = 'Mot de passe trop court'; return; }
      if (this.wizardPassword !== this.wizardConfirmPassword) { this.error = 'Les mots de passe ne correspondent pas'; return; }
      this.wizardStep = 2;
    },
    async submitWizard() {
      this.error = '';
      const services = Object.keys(this.wizardServices).filter(k => this.wizardServices[k]);
      try {
        this.isLoading = true;
        const res = await api('/api/wizard', 'POST', { password: this.wizardPassword, services });
        this.token = res.data.token;
        localStorage.setItem('stageneth_token_v2', this.token);
        this.showWizard = false;
        this.wizardStep = 1;
        this.wizardPassword = '';
        this.wizardConfirmPassword = '';
        await this.refresh();
        this.startMonitoring();
        this.toast('Configuration initiale terminée', 'success');
      } catch (e) {
        this.error = e.message;
      } finally {
        this.isLoading = false;
      }
    },
    async resetWizard() {
      if (!confirm('Relancer le wizard ? Cela déconnectera et réinitialisera le flag firstboot.')) return;
      try {
        await this.uciSet('stageneth', 'globals', 'stageneth', { firstboot_done: '0' });
        await this.uciCommit('stageneth');
        this.logout();
        this.showWizard = true;
        this.wizardStep = 1;
      } catch (e) {
        this.toast('Reset wizard failed: ' + e.message, 'error');
      }
    },
    async ubus(path, method, payload = {}) {
      return api('/api/ubus/call', 'POST', { path, method, payload });
    },
    async uciSet(config, section, type, values) {
      return api('/api/uci/set', 'POST', { config, section, type, values });
    },
    async uciCommit(config) {
      return api('/api/uci/commit', 'POST', { config });
    },
    async refresh() {
      this.isLoading = true;
      try {
        for (const config of ['stageneth', 'network', 'dhcp', 'firewall']) {
          const res = await api('/api/uci/get', 'POST', { config });
          this.uci[config] = res.data || {};
        }
        this.toast('Refreshed', 'success');
      } catch (e) {
        this.toast('Refresh failed: ' + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async removeUci(config, section) {
      if (!confirm('Confirmer la suppression de ' + section + ' ?')) return;
      this.isLoading = true;
      try {
        await this.uciSet(config, section, '', {});
        await this.uciCommit(config);
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
        await this.uciSet(config, section, type, values);
        await this.uciCommit(config);
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
      if (type === 'service') Object.assign(this.newService, { name, vlan_id: data.vlan_id || '', dscp: data.dscp || '', priority: data.priority || '', mtu: data.mtu || '', ptp: data.ptp || '0', multicast: data.multicast || '0', untagged: data.untagged || '0' });
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
        vlan_id: s.vlan_id, dscp: s.dscp, priority: s.priority, mtu: s.mtu, ptp: s.ptp, multicast: s.multicast, untagged: s.untagged
      }, { newService: { name: '', vlan_id: '20', dscp: '0', priority: '0', mtu: '', ptp: '0', multicast: '0', untagged: '0' }, editing: { type: '', name: '' } });
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
    startMonitoring() {
      this.fetchMonitoring();
    },
    stopMonitoring() {
      if (this.monitoringTimeout) { clearTimeout(this.monitoringTimeout); this.monitoringTimeout = null; }
    },
    async fetchMonitoring() {
      try {
        const res = await api('/api/monitoring/summary', 'GET');
        this.monitoring = res.data || {};
        const max = 20;
        this.monitoringHistory.cpu.push(this.monitoring.cpu_percent || 0);
        this.monitoringHistory.memory.push(this.monitoring.memory?.used_kb || 0);
        for (const k of ['cpu', 'memory']) {
          if (this.monitoringHistory[k].length > max) this.monitoringHistory[k].shift();
        }
      } catch (e) {
        this.toast('Monitoring failed: ' + e.message, 'error');
      } finally {
        this.monitoringTimeout = setTimeout(() => this.fetchMonitoring(), 5000);
      }
    },
    async loadPresets() {
      try {
        const res = await api('/api/stageneth/presets', 'GET');
        this.presets = res.data || [];
      } catch (e) {
        this.toast('Presets load failed: ' + e.message, 'error');
      }
    },
    onPresetChange() {
      const p = this.presets.find(p => p.name === this.selectedPreset);
      this.selectedPresetServices = p && p.services ? [...p.services] : [];
    },
    async applyPreset() {
      if (!this.selectedPreset) return;
      this.hasPendingChanges = false;
      this.isLoading = true;
      try {
        const res = await api('/api/stageneth/preset-apply', 'POST', { name: this.selectedPreset, services: this.selectedPresetServices });
        this.toast(res.message + ' ' + (res.data?.log || ''), 'success');
        await this.refresh();
      } catch (e) {
        this.toast('Preset apply failed: ' + e.message, 'error');
        this.hasPendingChanges = true;
      } finally {
        this.isLoading = false;
      }
    },
    toggleTheme() {
      this.isDark = !this.isDark;
      document.documentElement.classList.toggle('dark', this.isDark);
      localStorage.setItem('stageneth_theme', this.isDark ? 'dark' : 'light');
    },
    toggleSimpleMode() {
      this.simpleMode = !this.simpleMode;
      localStorage.setItem('stageneth_simple', this.simpleMode);
      if (!this.visibleTabs.includes(this.currentTab)) this.currentTab = 'simple';
      this.mobileMenuOpen = false;
    },
    async apply() {
      this.hasPendingChanges = false;
      this.isLoading = true;
      try {
        const res = await api('/api/stageneth/apply', 'POST', {});
        this.toast(res.message + ' ' + (res.data?.log || ''), 'success');
      } catch (e) {
        this.toast('Apply failed: ' + e.message, 'error');
        this.hasPendingChanges = true;
      } finally {
        this.isLoading = false;
      }
    },
    async saveGlobals() {
      this.isLoading = true;
      try {
        await this.uciSet('stageneth', 'globals', 'stageneth', { switch_vendor: this.switchVendor, trunk: this.trunk });
        await this.uciCommit('stageneth');
        this.hasPendingChanges = true;
        this.toast('Globals saved', 'success');
      } catch (e) {
        this.toast('Save globals failed: ' + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async reload() {
      this.isLoading = true;
      try {
        const res = await api('/api/network/reload', 'POST', {});
        this.toast(res.message, 'success');
      } catch (e) {
        this.toast('Reload failed: ' + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async runSnmpWalk() {
      this.isLoading = true;
      this.snmpResult = [];
      try {
        const res = await api('/api/snmp/walk', 'POST', this.snmpQuery);
        this.snmpResult = res.data || [];
        this.toast(res.message + ' (' + this.snmpResult.length + ' results)', 'success');
      } catch (e) {
        this.toast('SNMP walk failed: ' + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async runMdns() {
      this.isLoading = true;
      this.mdnsResults = [];
      try {
        const res = await api('/api/mdns/discover', 'POST', { service: this.mdnsService, duration: parseInt(this.mdnsDuration) });
        this.mdnsResults = res.data || [];
        this.toast(res.message + ' (' + this.mdnsResults.length + ' found)', 'success');
      } catch (e) {
        this.toast('mDNS discovery failed: ' + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async changePassword() {
      if (this.passwordChange.new_password !== this.passwordChange.confirm_password) {
        this.toast('New passwords do not match', 'error');
        return;
      }
      this.isLoading = true;
      try {
        const res = await api('/api/change-password', 'POST', this.passwordChange);
        this.toast(res.message, 'success');
        this.passwordChange = { current_password: '', new_password: '', confirm_password: '' };
      } catch (e) {
        this.toast('Password change failed: ' + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async loadLan() {
      try {
        const res = await api('/api/uci/get', 'POST', { config: 'network' });
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
    async loadNetworkInterfaces() {
      try {
        const res = await api('/api/network/interfaces', 'GET');
        console.log('network interfaces raw', res);
        this.availableInterfaces = (res.data && res.data.interfaces) ? res.data.interfaces : [];
        console.log('availableInterfaces set', this.availableInterfaces, 'physicalInterfaces', this.physicalInterfaces);
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
    async loadWan() {
      try {
        const res = await api('/api/uci/get', 'POST', { config: 'network' });
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
        await api('/api/uci/set', 'POST', {
          config: 'network',
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
        await api('/api/uci/commit', 'POST', { config: 'network' });
        await api('/api/network/reload', 'POST', {});
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
        await api('/api/uci/set', 'POST', {
          config: 'network',
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
        await api('/api/uci/commit', 'POST', { config: 'network' });
        await api('/api/network/reload', 'POST', {});
        this.toast('WAN settings saved and network reloaded', 'success');
      } catch (e) {
        this.toast('WAN save failed: ' + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async loadNtp() {
      try {
        const res = await api('/api/ntp', 'GET');
        this.ntp = { ...this.ntp, ...res.data };
      } catch (e) {
        this.toast('NTP load failed: ' + e.message, 'error');
      }
    },
    async saveNtp() {
      this.isLoading = true;
      try {
        const res = await api('/api/ntp/set', 'POST', {
          enabled: this.ntp.enabled,
          enable_server: this.ntp.enable_server,
          servers: this.ntp.servers,
          timezone: this.ntp.timezone
        });
        this.ntp = { ...this.ntp, ...res.data };
        this.toast('NTP settings saved', 'success');
      } catch (e) {
        this.toast('NTP save failed: ' + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async fetchNtpTime() {
      try {
        const tz = encodeURIComponent(this.ntp.timezone || 'UTC0');
        const res = await api('/api/time?tz=' + tz, 'GET');
        this.ntpTime = { ...this.ntpTime, ...res.data };
      } catch (e) {}
    },
    startNtpTime() {
      this.stopNtpTime();
      this.ntpTimeInterval = setInterval(() => this.fetchNtpTime(), 1000);
    },
    stopNtpTime() {
      if (this.ntpTimeInterval) { clearInterval(this.ntpTimeInterval); this.ntpTimeInterval = null; }
    },
    addNtpServer() {
      const s = (this.ntp.newServer || '').trim();
      if (!s) return;
      if (!this.ntp.servers.includes(s)) {
        this.ntp.servers.push(s);
      }
      this.ntp.newServer = '';
    },
    removeNtpServer(s) {
      this.ntp.servers = this.ntp.servers.filter(x => x !== s);
    },
    toggleDark() {
      this.isDark = !this.isDark;
      localStorage.setItem('stageneth_theme', this.isDark ? 'dark' : 'light');
      if (this.isDark) {
        document.documentElement.classList.add('dark');
      } else {
        document.documentElement.classList.remove('dark');
      }
    },
    async fetchLogs() {
      if (this.logsPaused) return;
      try {
        const res = await api(`/api/logs?limit=${this.logsLimit}`, 'GET');
        this.logLines = res.data || [];
        this.$nextTick(() => {
          const el = this.$refs.logContainer;
          if (el) el.scrollTop = el.scrollHeight;
        });
      } catch (e) {
        // silently ignore on background poll
      }
    },
    startLogs() {
      this.fetchLogs();
      this.logsInterval = setInterval(() => this.fetchLogs(), 2000);
    },
    stopLogs() {
      if (this.logsInterval) { clearInterval(this.logsInterval); this.logsInterval = null; }
    }
  }
}).mount('#app');

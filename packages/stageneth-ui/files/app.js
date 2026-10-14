const { createApp } = Vue;


createApp({
  data() {
    return {
      token: '', // truthy marker for "session active" — auth is a cookie now
      sessionUser: '',
      sessionRole: '',
      username: 'admin',
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
        { label: 'Dashboard', tabs: [
          { key: 'simple', label: 'Tableau de bord', icon: 'fas fa-tachometer-alt' }
        ]},
        { label: 'Services AV', tabs: [
          { key: 'services', label: 'Services', icon: 'fas fa-stream' }
        ]},
        { label: 'Réseau', tabs: [
          { key: 'network', label: 'Réseau', icon: 'fas fa-network-wired' },
          { key: 'lan', label: 'LAN/WAN', icon: 'fas fa-ethernet' }
        ]},
        { label: 'DHCP & DNS', tabs: [
          { key: 'dhcpdns', label: 'DHCP & DNS', icon: 'fas fa-server' }
        ]},
        { label: 'Pare-feu', tabs: [
          { key: 'firewall', label: 'Pare-feu', icon: 'fas fa-shield-alt' }
        ]},
        { label: 'Système', tabs: [
          { key: 'ntp', label: 'NTP', icon: 'fas fa-clock' },
          { key: 'system', label: 'Système', icon: 'fas fa-cog' },
          { key: 'credentials', label: 'Identifiants', icon: 'fas fa-user-lock' },
          { key: 'backup', label: 'Sauvegarde', icon: 'fas fa-save' }
        ]},
        { label: 'Supervision', tabs: [
          { key: 'monitoring', label: 'Supervision', icon: 'fas fa-heartbeat' },
          { key: 'logs', label: 'Journaux', icon: 'fas fa-scroll' }
        ]},
        { label: 'Outils', tabs: [
          { key: 'snmp', label: 'SNMP', icon: 'fas fa-sitemap' },
          { key: 'discovery', label: 'Découverte', icon: 'fas fa-search' }
        ]}
      ],
      isDark: true,
      uci: { stageneth: {}, network: {}, dhcp: {}, firewall: {}, system: {} },
      newService: { name: '', vlan_id: '20', dscp: '0', priority: '0', mtu: '', ipaddr: '', netmask: '', ptp: '0', multicast: '0', untagged: '0' },
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
      monitoringPolling: false,
      evtSource: null,
      alerts: [],
      alertsTimeout: null,
      snmpQuery: { host: '', port: 161, community: 'public', oid: '1.3.6.1.2.1.1.1.0', version: 'v2c', username: '', authProtocol: 'MD5', authPass: '', privProtocol: 'DES', privPass: '', securityLevel: 'AuthNoPriv' },
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
      logFilter: '',
      logsInterval: null,
      logsPaused: false,
      logsLimit: 200,
      logLevelFilter: 'all',
      logCategoryFilter: 'all',
      pingResult: {},
      nicErrorKeys: ['rx_errors','rx_dropped','rx_crc_errors','rx_length_errors','rx_over_errors','rx_frame_errors','rx_missed_errors','rx_no_buffer_count','rx_align_errors','rx_short_length_errors','rx_long_length_errors','tx_errors','tx_dropped','tx_missed_errors','tx_carrier_errors','tx_aborted_errors'],
      nicErrorLabels: {
        rx_errors: 'Erreurs RX', rx_dropped: 'RX dropped', rx_crc_errors: 'CRC', rx_length_errors: 'Longueur', rx_over_errors: 'Overrun',
        rx_frame_errors: 'Frame', rx_missed_errors: 'Manqués', rx_no_buffer_count: 'Buffer plein', rx_align_errors: 'Alignement',
        rx_short_length_errors: 'Trop court', rx_long_length_errors: 'Trop long',
        tx_errors: 'Erreurs TX', tx_dropped: 'TX dropped', tx_missed_errors: 'Manqués TX', tx_carrier_errors: 'Porteuse', tx_aborted_errors: 'Abandonnés'
      },
      passwordChange: { current_password: '', new_password: '', confirm_password: '' },
      users: [],
      newUser: { username: '', role: 'operator', password: '' },
      lan: { ipaddr: '', netmask: '', gateway: '', proto: 'static', device: '' },
      wan: { ipaddr: '', netmask: '', gateway: '', proto: 'dhcp', device: '', macaddr: '' },
      system: { hostname: '' },
      logging: { enabled: '0', host: '', port: '514', protocol: 'udp' },
      collector: { enabled: '0', port: '514', protocol: 'udp', dir: '/var/log/remote' },
      availableInterfaces: [],
      presets: [],
      selectedPreset: '',
      selectedPresetServices: [],
      switchVendor: 'generic',
      trunk: 'eth1',
      ptpTimestamping: 'software',
      editing: { type: '', name: '' },
      hasPendingChanges: false,
      showWizard: false,
      wizardStep: 1,
      wizardPassword: '',
      wizardConfirmPassword: '',
      wizardPresets: [],
      wizardPreset: ''
    };
  },
  computed: {
    currentPreset() { return this.presets.find(p => p.name === this.selectedPreset) || {}; },
    presetsByCategory() {
      const groups = {};
      const labels = { base: 'Base', audio: 'Audio', video: 'Vidéo', light: 'Lumière', show: 'Spectacle complet' };
      for (const p of this.presets) {
        const cat = p.category || 'autre';
        if (!groups[cat]) groups[cat] = { label: labels[cat] || cat, items: [] };
        groups[cat].items.push(p);
      }
      return groups;
    },
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
        // bridge.ports is a space-joined UCI option — split before
        // matching, 'eth1.10'.includes('eth1.1') is a false positive
        const portSet = (bridge.ports || '').split(/\s+/);
        const vlan = Object.entries(devices).find(([,d]) => d.type === '8021q' && d.name && portSet.includes(d.name));
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
    showLock() { return (this.uci.stageneth?.values?.globals?.show_lock) === '1'; },
    servicesCount() { return Object.keys(this.services || {}).length; },
    leasesCount() { return (this.monitoring.dhcp_leases || []).length; },
    activePresetCategory() {
      const svcs = Object.keys(this.services || {});
      if (svcs.some(s => ['dante','aes67','avb'].includes(s))) return 'audio';
      if (svcs.some(s => ['ndihx','st2110'].includes(s))) return 'video';
      if (svcs.some(s => ['artnet','sacn','proprietary'].includes(s))) return 'light';
      return '';
    },
    presetTip() {
      const cat = this.activePresetCategory;
      const tips = {
        audio: 'Audio : activer QoS voice, DSCP 46/34, priorité 6/7. Activer IGMP snooping sans filtrer PTP et Dante multicast.',
        video: 'Vidéo : MTU 9000 obligatoire pour ST 2110. Activer IGMP querier, jumbo frames et trust DSCP. NDI : rester en 1500.',
        light: 'Lumière : Art-Net utilise 2.x.x.x/8. sACN/MA-Net multicast : isoler les VLANs lumière. Désactiver EEE.'
      };
      return tips[cat] || '';
    },
    vendorTip() {
      const tips = {
        generic: 'Schéma standard 802.1Q. Le routeur agit comme IGMP querier sur les bridges multicast.',
        luminex: 'GigaCore gère IGMP et PTP. Le routeur active IGMP snooping mais laisse le rôle querier au switch.',
        cisco: 'Activez IGMP snooping, IGMP querier, jumbo frames et trust DSCP sur le port trunk. Le routeur laisse le rôle querier au switch.',
        netgear: 'Activez IGMP snooping, IGMP querier, jumbo frames et classofservice trust DSCP. Le routeur laisse le rôle querier au switch.'
      };
      return tips[this.switchVendor] || tips.generic;
    },
    visibleTabGroups() {
      // operator role: read + apply only — the server enforces this too,
      // so hiding tabs is UX, not a security boundary.
      const operatorTabs = ['simple', 'services', 'ntp', 'credentials', 'monitoring', 'logs'];
      const groups = this.simpleMode ? [
        { label: 'Dashboard', tabs: [
          { key: 'simple', label: 'Tableau de bord', icon: 'fas fa-tachometer-alt' }
        ]},
        { label: 'Supervision', tabs: [
          { key: 'monitoring', label: 'Supervision', icon: 'fas fa-chart-line' },
          { key: 'logs', label: 'Journaux', icon: 'fas fa-scroll' }
        ]}
      ] : this.tabGroups;
      if (this.sessionRole === 'operator') {
        return groups
          .map(g => ({ ...g, tabs: g.tabs.filter(t => operatorTabs.includes(t.key)) }))
          .filter(g => g.tabs.length > 0);
      }
      return groups;
    },
    visibleTabs() { return this.visibleTabGroups.flatMap(g => g.tabs).map(t => t.key); },
    ptpStateClass() {
      const state = this.monitoring.ptp_status?.state || 'unknown';
      if (state === 'SLAVE' || state === 'MASTER') return 'text-green-600 dark:text-green-400';
      if (state === 'LISTENING' || state === 'UNCALIBRATED') return 'text-yellow-600 dark:text-yellow-400';
      return 'text-red-600 dark:text-red-400';
    },
    mdbBridges() { return Object.keys(this.monitoring.bridge_mdb || {}); },
    clockDomains() {
      const cd = this.monitoring.clock_domains || {};
      const labels = { ptpv1: 'PTPv1 (Dante)', ptpv2: 'PTPv2 (AES67/2110)', ptpv2_l2: 'PTPv2 L2', gptp: 'gPTP (AVB/MILAN)' };
      return Object.keys(cd).sort().map(iface => ({
        iface,
        protos: Object.keys(cd[iface]).filter(k => k !== 'interface').sort()
          .map(k => Object.assign({ name: labels[k] || k, key: k }, cd[iface][k]))
      }));
    },
    filteredLogLines() {
      let lines = this.logLines || [];
      const f = this.logFilter.toLowerCase();
      if (f) lines = lines.filter(l => l.toLowerCase().includes(f));
      if (this.logLevelFilter !== 'all') {
        lines = lines.filter(l => this.logLineLevel(l) === this.logLevelFilter);
      }
      if (this.logCategoryFilter !== 'all') {
        lines = lines.filter(l => this.logLineCategory(l) === this.logCategoryFilter);
      }
      return lines;
    },
    logCategories() {
      const set = new Set((this.logLines || []).map(l => this.logLineCategory(l)).filter(Boolean));
      return Array.from(set).sort();
    },
    mdnsByType() {
      const map = {};
      for (const r of (this.mdnsResults || [])) {
        const t = r.type || 'unknown';
        (map[t] ||= []).push(r);
      }
      return Object.entries(map).sort(([a], [b]) => a.localeCompare(b));
    },
    totalNicErrors() {
      const nicStats = this.monitoring.nic_stats || {};
      return Object.entries(nicStats).reduce((total, [, stats]) => total + this.nicErrorTotal(stats), 0);
    },
    multicastByProtocol() {
      const summary = {};
      const groups = this.monitoring.igmp_groups || {};
      for (const [br, list] of Object.entries(groups)) {
        for (const g of list) {
          const name = this.igmpGroupName(g.ip);
          const key = name.replace(/^Multicast .*/, 'Autre multicast');
          if (!summary[key]) summary[key] = 0;
          summary[key]++;
        }
      }
      return summary;
    },
    healthAlerts() {
      const alerts = [];
      const ptp = this.monitoring.ptp_status || {};
      if (ptp.state && ptp.state !== 'SLAVE' && ptp.state !== 'MASTER') {
        alerts.push({ type: 'warning', message: 'PTP non synchronisé : ' + ptp.state, icon: 'fas fa-clock' });
      } else if (ptp.state === 'SLAVE' && Math.abs(ptp.offset_ns) > 1000) {
        alerts.push({ type: 'warning', message: 'PTP offset élevé : ' + ptp.offset_ns.toFixed(1) + ' ns', icon: 'fas fa-clock' });
      }
      if (this.hasPendingChanges) {
        alerts.push({ type: 'warning', message: 'Modifications en attente : cliquez sur Enregistrer & Appliquer pour les appliquer.', icon: 'fas fa-exclamation-circle' });
      }
      const ntpRunning = this.ntp?.running === true;
      const ntpServer = this.ntp?.enable_server === true;
      if (ntpServer && !ntpRunning) {
        alerts.push({ type: 'error', message: 'Le serveur NTP est activé mais n\'est pas en cours d\'exécution.', icon: 'fas fa-clock' });
      }
      const cpu = Number(this.monitoring?.cpu_percent || 0);
      if (cpu > 80) alerts.push({ type: 'error', message: `CPU très élevé : ${cpu.toFixed(1)}%`, icon: 'fas fa-microchip' });
      else if (cpu > 60) alerts.push({ type: 'warning', message: `CPU élevé : ${cpu.toFixed(1)}%`, icon: 'fas fa-microchip' });
      const mem = Number(this.monitoring?.memory?.used_percent || 0);
      if (mem > 85) alerts.push({ type: 'warning', message: `Mémoire élevée : ${mem.toFixed(1)}%`, icon: 'fas fa-memory' });
      const nicStats = this.monitoring.nic_stats || {};
      for (const [iface, stats] of Object.entries(nicStats)) {
        const details = this.nicErrorList(stats);
        if (details.length > 0) {
          const total = details.reduce((s, e) => s + Number(e.value), 0);
          const detailStr = details.map(e => `${e.label}: ${e.value}`).join(', ');
          alerts.push({ type: 'warning', message: `NIC ${iface} : ${total} (${detailStr})`, icon: 'fas fa-ethernet' });
        }
      }
      const svcStatus = this.monitoring?.services || {};
      for (const [svc, ok] of Object.entries(svcStatus)) {
        if (!ok) alerts.push({ type: 'warning', message: `Service ${svc} ne répond pas.`, icon: 'fas fa-network-wired' });
      }
      return alerts;
    }
  },
  watch: {
    currentTab(tab) {
      localStorage.setItem('stageneth_tab', tab);
      this.stopLogs();
      this.stopNtpTime();
      if (tab === 'monitoring') { this.fetchMonitoring(); }
      else if (tab === 'network') { this.loadNetworkInterfaces(); }
      else if (tab === 'logs') { this.startLogs(); }
      else if (tab === 'ntp') { this.loadNtp(); this.fetchNtpTime(); this.startNtpTime(); }
      else if (tab === 'credentials') { this.loadUsers(); }
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
    await this.checkSession();
    if (this.token) {
      await this.refresh();
      this.switchVendor = (this.uci.stageneth?.values?.globals?.switch_vendor) || 'generic';
      this.trunk = (this.uci.stageneth?.values?.globals?.trunk) || 'eth1';
      this.ptpTimestamping = (this.uci.stageneth?.values?.globals?.ptp_timestamping) || 'software';
      this.simpleMode = (localStorage.getItem('stageneth_simple') !== null ? localStorage.getItem('stageneth_simple') === 'true' : true);
      if (!this.visibleTabs.includes(this.currentTab)) this.currentTab = 'simple';
      this.startMonitoring();
      if (this.currentTab === 'logs') this.startLogs();
      if (this.currentTab === 'ntp') { this.startNtpTime(); this.fetchNtpTime(); }
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
    ...sessionMethods,
    ...cfgMethods,
    ...monMethods,
    ...netMethods,
    ...toolsMethods,
    toast(msg, type='info') {
      this.message = msg;
      this.messageType = type;
    },
    filterType(config, type) {
      const values = this.uci[config]?.values || {};
      return Object.fromEntries(Object.entries(values).filter(([_, v]) => v && v['.type'] === type));
    },
    v1PathForType(type) {
      return { service: 'services', binding: 'bindings', forwarding: 'forwardings' }[type] || null;
    },
  }
}).mount('#app');

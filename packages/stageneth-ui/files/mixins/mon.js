// Monitoring (SSE + polling fallback), logs, NIC/mcast helpers.
var monMethods = {
    startMonitoring() {
      if (this.monitoringPolling) return;
      this.monitoringPolling = true;
      this.fetchMonitoring();
      this.startEvents();
      this.pollAlerts();
    },
    stopMonitoring() {
      this.monitoringPolling = false;
      if (this.monitoringTimeout) { clearTimeout(this.monitoringTimeout); this.monitoringTimeout = null; }
      if (this.alertsTimeout) { clearTimeout(this.alertsTimeout); this.alertsTimeout = null; }
      if (this.evtSource) { this.evtSource.close(); this.evtSource = null; }
    },
    startEvents() {
      if (this.evtSource) return;
      if (!window.EventSource) { this.pollMonitoring(); return; }
      const es = new EventSource('/api/v1/events');
      this.evtSource = es;
      es.onmessage = (e) => {
        try {
          this.monitoring = JSON.parse(e.data) || {};
          this.pushMonitoringHistory();
        } catch (err) {}
      };
      es.onerror = () => {
        es.close();
        if (this.evtSource === es) this.evtSource = null;
        // fall back to polling if the stream drops
        if (this.monitoringPolling && !this.monitoringTimeout) {
          this.monitoringTimeout = setTimeout(() => this.pollMonitoring(), 5000);
        }
      };
    },
    async pollMonitoring() {
      if (!this.monitoringPolling) return;
      this.monitoringTimeout = null;
      await this.fetchMonitoring();
      if (!this.monitoringPolling) return;
      // try to get back on SSE — if the stream reconnects it owns the
      // updates and polling stops; otherwise keep the 5s fallback cycle
      if (!this.evtSource && window.EventSource) this.startEvents();
      if (!this.evtSource) this.monitoringTimeout = setTimeout(() => this.pollMonitoring(), 5000);
    },
    async pollAlerts() {
      if (!this.monitoringPolling) return;
      await this.fetchAlerts();
      if (this.monitoringPolling) this.alertsTimeout = setTimeout(() => this.pollAlerts(), 10000);
    },
    pushMonitoringHistory() {
      const max = 20;
      this.monitoringHistory.cpu.push(this.monitoring.cpu_percent || 0);
      this.monitoringHistory.memory.push(this.monitoring.memory?.used_kb || 0);
      for (const k of ['cpu', 'memory']) {
        if (this.monitoringHistory[k].length > max) this.monitoringHistory[k].shift();
      }
    },
    async fetchMonitoring() {
      try {
        const res = await api('/api/v1/monitoring', 'GET');
        this.monitoring = res.data || {};
        this.pushMonitoringHistory();
      } catch (e) {
        // silently retry next cycle
      }
    },
    async fetchAlerts() {
      try {
        const res = await api('/api/v1/alerts?limit=50', 'GET');
        this.alerts = res.data || [];
      } catch (e) {
        // silently retry next cycle
      }
    },
    async fetchLogs() {
      if (this.logsPaused) return;
      try {
        const res = await api(`/api/v1/logs?limit=${this.logsLimit}`, 'GET');
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
    },
    logLineLevel(line) {
      const m = line.match(/\b(?:emerg|alert|crit|err|warn|notice|info|debug)\b/);
      if (m) {
        const p = m[0];
        if (['emerg','alert','crit','err'].includes(p)) return 'err';
        if (['warn','warning'].includes(p)) return 'warn';
        if (p === 'notice') return 'notice';
        if (p === 'debug') return 'debug';
        return 'info';
      }
      const lower = line.toLowerCase();
      if (lower.includes('error') || lower.includes('failed') || lower.includes('failure')) return 'err';
      if (lower.includes('warn')) return 'warn';
      return 'info';
    },
    logLineCategory(line) {
      const logread = line.match(/^\w{3}\s+\w{3}\s+\d+\s+\d+:\d+:\d+\s+\d+\s+([\w.-]+)\.(?:\w+)\s+([\w_.-]+)(?:\[\d+\])?:/);
      if (logread) return logread[2].toLowerCase();
      const rsyslog = line.match(/^\w{3}\s+\d+\s+\d+:\d+:\d+\s+\S+\s+([\w_.-]+)(?:\[\d+\])?:/);
      if (rsyslog) return rsyslog[1].toLowerCase();
      const m = line.match(/\b(?:kern|user|mail|daemon|auth|syslog|lpr|news|uucp|cron|authpriv|ftp|ntp|logaudit|local[0-7])\.\w+\b/);
      if (m) return m[0].split('.')[0];
      const proc = line.match(/\s([a-zA-Z0-9_.-]+)\[\d+\]:/);
      if (proc) return proc[1].split('.')[0].toLowerCase();
      const nginx = line.match(/^\d{4}\/\d{2}\/\d{2}\s+\d+:\d+:\d+\s+\[(\w+)\]/);
      if (nginx) return 'nginx';
      const tag = line.match(/\s([a-zA-Z0-9_.-]+):\s/);
      if (tag) return tag[1].toLowerCase();
      return 'other';
    },
    logLineClass(line) {
      const level = this.logLineLevel(line);
      if (level === 'err') return 'text-red-500';
      if (level === 'warn') return 'text-yellow-500';
      if (level === 'notice') return 'text-green-500';
      if (level === 'debug') return 'text-slate-500';
      return 'text-slate-300';
    },
    igmpGroupName(ip) {
      const names = {
        '224.0.0.1': 'All hosts',
        '224.0.0.2': 'All routers',
        '224.0.0.107': 'PTP peer delay',
        '224.0.1.129': 'PTP announce',
        '224.0.0.251': 'mDNS',
      };
      return names[ip] || 'Multicast ' + ip;
    },
    nicErrorClass(stats) {
      const total = (this.nicErrorKeys || []).reduce((sum, k) => sum + Number((stats || {})[k] || 0), 0);
      if (total > 100) return 'bg-red-100 dark:bg-red-900 text-red-800 dark:text-red-100';
      if (total > 0) return 'bg-yellow-100 dark:bg-yellow-900 text-yellow-800 dark:text-yellow-100';
      return 'bg-green-100 dark:bg-green-900 text-green-800 dark:text-green-100';
    },
    nicErrorTotal(stats) {
      return (this.nicErrorKeys || []).reduce((sum, k) => sum + Number((stats || {})[k] || 0), 0);
    },
    nicErrorList(stats) {
      return (this.nicErrorKeys || []).filter(k => (stats || {})[k] > 0).map(k => ({ key: k, label: this.nicErrorLabels[k] || k, value: stats[k] }));
    }
};

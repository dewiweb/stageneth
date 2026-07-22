var utils = {
  formatPercent(v) {
    return (v === undefined || v === null || isNaN(v)) ? '-' : v + '%';
  },
  formatBytes(bytes) {
    if (bytes === undefined || bytes === null || isNaN(bytes) || bytes < 0) return '-';
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B','KB','MB','GB','TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  },
  formatUptime(s) {
    if (!s && s !== 0) return '-';
    const d = Math.floor(s / 86400);
    const h = Math.floor((s % 86400) / 3600);
    const m = Math.floor((s % 3600) / 60);
    return `${d}d ${h}h ${m}m`;
  },
  sparklinePoints(values) {
    if (!values || values.length < 2) return '0,30 100,30';
    const h = 30, w = 100;
    return values.map((v, i) => {
      const x = (i / (values.length - 1)) * w;
      const y = h - (Math.max(0, Math.min(v, 100)) / 100) * h;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    }).join(' ');
  },
  sparklineAuto(values) {
    if (!values || values.length < 2) return '0,30 100,30';
    const h = 30, w = 100;
    const min = Math.min(...values);
    const range = Math.max(...values) - min || 1;
    return values.map((v, i) => {
      const x = (i / (values.length - 1)) * w;
      const y = h - ((v - min) / range) * h;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    }).join(' ');
  },
  serviceColor(name) {
    const map = {
      mgmt: 'bg-slate-500',
      dante: 'bg-blue-500',
      aes67: 'bg-cyan-500',
      ptp: 'bg-yellow-500',
      artnet: 'bg-red-500',
      sacn: 'bg-purple-500',
      ndi: 'bg-pink-500',
      st2110: 'bg-orange-500',
      sdi: 'bg-indigo-500',
      proprietary: 'bg-slate-600'
    };
    const n = (name || '').toLowerCase();
    for (const k in map) if (n.includes(k)) return map[k];
    return 'bg-emerald-500';
  },
  interfaceMac(name) {
    const iface = (this.availableInterfaces || []).find(i => (typeof i === 'string' ? i : i.name) === name);
    return iface && typeof iface === 'object' ? iface.mac : '';
  },
  mdnsFriendlyName(r) {
    const keys = ['fn=', 'name=', 'friendlyname=', 'friendly_name=', 'devicename=', 'device_name=', 'model='];
    const txts = r.txt || [];
    for (const k of keys) {
      const t = txts.find(x => x.toLowerCase().startsWith(k));
      if (t) {
        const v = t.substring(k.length);
        if (v) return v;
      }
    }
    return r.name || '-';
  },
  exploreMdns(name) {
    const hasTransport = /\._(tcp|udp)$/.test(name);
    this.mdnsService = hasTransport ? name : name + '._tcp';
    this.runMdns();
  }
};

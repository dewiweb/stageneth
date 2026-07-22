const { createApp } = Vue;

const api = async (path, method, body) => {
  const token = localStorage.getItem('stageneth_token');
  const opts = {
    method,
    headers: { 'Content-Type': 'application/json' }
  };
  if (token) opts.headers.Authorization = `Bearer ${token}`;
  if (body) opts.body = JSON.stringify(body);
  const res = await fetch(path, opts);
  const data = await res.json().catch(() => ({}));
  if (res.status >= 400) throw new Error(data.message || res.statusText);
  return data;
};

createApp({
  data() {
    return {
      token: localStorage.getItem('stageneth_token') || '',
      username: 'root',
      password: '',
      error: '',
      message: '',
      services: {},
      bindings: {},
      newService: { name: '', vlan_id: '20', dscp: '0', priority: '0', ptp: '0', multicast: '0' },
      newBinding: { name: '', interface: '', service: '' }
    };
  },
  async mounted() {
    if (this.token) await this.refresh();
  },
  methods: {
    async login() {
      try {
        const res = await api('/api/login', 'POST', { username: this.username, password: this.password });
        this.token = res.data.token;
        localStorage.setItem('stageneth_token', this.token);
        this.error = '';
        await this.refresh();
      } catch (e) {
        this.error = e.message;
      }
    },
    logout() {
      this.token = '';
      localStorage.removeItem('stageneth_token');
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
      try {
        const st = await this.ubus('uci', 'get', { config: 'stageneth' });
        const sections = st.data?.values || {};
        this.services = Object.fromEntries(
          Object.entries(sections).filter(([_, v]) => v && v['.type'] === 'service')
        );
        this.bindings = Object.fromEntries(
          Object.entries(sections).filter(([_, v]) => v && v['.type'] === 'binding')
        );
      } catch (e) {
        this.message = 'Refresh failed: ' + e.message;
      }
    },
    async addService() {
      const s = this.newService;
      await this.uciSet('stageneth', s.name, 'service', {
        vlan_id: s.vlan_id,
        dscp: s.dscp,
        priority: s.priority,
        ptp: s.ptp,
        multicast: s.multicast
      });
      await this.uciCommit('stageneth');
      this.newService = { name: '', vlan_id: '20', dscp: '0', priority: '0', ptp: '0', multicast: '0' };
      await this.refresh();
    },
    async removeService(name) {
      await this.uciSet('stageneth', name, '', {});
      await this.uciCommit('stageneth');
      await this.refresh();
    },
    async addBinding() {
      const b = this.newBinding;
      await this.uciSet('stageneth', b.name, 'binding', { interface: b.interface, service: b.service });
      await this.uciCommit('stageneth');
      this.newBinding = { name: '', interface: '', service: '' };
      await this.refresh();
    },
    async removeBinding(name) {
      await this.uciSet('stageneth', name, '', {});
      await this.uciCommit('stageneth');
      await this.refresh();
    },
    async apply() {
      try {
        const res = await api('/api/stageneth/apply', 'POST', {});
        this.message = res.message + ' ' + (res.data?.log || '');
      } catch (e) {
        this.message = 'Apply failed: ' + e.message;
      }
    }
  }
}).mount('#app');

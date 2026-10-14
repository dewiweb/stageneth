// Session lifecycle + first-boot wizard methods.
var sessionMethods = {
    async checkSession() {
      try {
        const res = await api('/api/v1/session', 'GET');
        if (res.data && res.data.authenticated) {
          this.token = 'session';
          this.sessionUser = res.data.user || '';
          this.sessionRole = res.data.role || '';
        }
      } catch (e) {}
    },
    async login() {
      try {
        const res = await api('/api/v1/session', 'POST', { username: this.username, password: this.password });
        this.token = 'session';
        this.sessionUser = res.data.user || this.username;
        this.sessionRole = res.data.role || '';
        this.error = '';
        await this.refresh();
        this.startMonitoring();
        await this.loadPresets();
      } catch (e) {
        this.error = e.message;
      }
    },
    async logout() {
      try { await api('/api/v1/session', 'DELETE'); } catch (e) {}
      this.stopMonitoring();
      this.stopLogs();
      this.stopNtpTime();
      this.monitoring = {};
      this.monitoringHistory = { cpu: [], memory: [] };
      this.alerts = [];
      this.pingResult = {};
      this.hasPendingChanges = false;
      this.token = '';
      this.sessionUser = '';
      this.sessionRole = '';
    },
    async checkFirstboot() {
      try {
        const res = await api('/api/v1/firstboot', 'GET');
        if (res.data && !res.data.firstboot_done) {
          this.showWizard = true;
          this.wizardPresets = res.data.presets || [];
          this.wizardPreset = this.wizardPresets[0]?.name || '';
        }
      } catch (e) {}
    },
    nextWizardStep() {
      this.error = '';
      if (this.wizardPassword.length < 8) { this.error = 'Mot de passe trop court'; return; }
      if (this.wizardPassword !== this.wizardConfirmPassword) { this.error = 'Les mots de passe ne correspondent pas'; return; }
      this.wizardStep = 2;
    },
    async submitWizard() {
      this.error = '';
      try {
        this.isLoading = true;
        const res = await api('/api/v1/wizard', 'POST', { password: this.wizardPassword, preset: this.wizardPreset });
        this.token = 'session';
        this.sessionUser = res.data.user || 'admin';
        this.sessionRole = res.data.role || 'tech';
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
    async skipWizard() {
      if (this.wizardPassword.length < 8) { this.error = 'Mot de passe trop court'; return; }
      if (this.wizardPassword !== this.wizardConfirmPassword) { this.error = 'Les mots de passe ne correspondent pas'; return; }
      if (!confirm('Ignorer le wizard ? Aucun preset ne sera appliqué.')) return;
      try {
        this.isLoading = true;
        const res = await api('/api/v1/wizard-skip', 'POST', { password: this.wizardPassword });
        this.token = 'session';
        this.sessionUser = res.data.user || 'admin';
        this.sessionRole = res.data.role || 'tech';
        this.showWizard = false;
        await this.refresh();
        this.startMonitoring();
        this.toast('Wizard ignoré', 'info');
      } catch (e) {
        this.error = e.message;
      } finally {
        this.isLoading = false;
      }
    },
    stateToUci(st) {
      const out = {};
      const put = (name, type, opts) => { out[name] = { ...(opts || {}), '.type': type }; };
      put('globals', 'stageneth', st.globals);
      for (const [n, o] of Object.entries(st.services || {})) put(n, 'service', o);
      for (const [n, o] of Object.entries(st.bindings || {})) put(n, 'binding', o);
      for (const [n, o] of Object.entries(st.forwardings || {})) put(n, 'forwarding', o);
      for (const [n, o] of Object.entries(st.logging || {})) put(n, 'logging', o);
      return out;
    }
};

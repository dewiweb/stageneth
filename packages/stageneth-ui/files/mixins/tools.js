// NTP, SNMP, mDNS, users, presets, theme.
var toolsMethods = {
    async loadNtp() {
      try {
        const res = await api('/api/v1/ntp', 'GET');
        this.ntp = { ...this.ntp, ...res.data };
      } catch (e) {
        this.toast('NTP load failed: ' + e.message, 'error');
      }
    },
    async saveNtp() {
      this.isLoading = true;
      try {
        const res = await api('/api/v1/ntp', 'PUT', {
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
        const res = await api('/api/v1/time', 'GET');
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
    async runSnmpWalk() {
      this.isLoading = true;
      this.snmpResult = [];
      try {
        const res = await api('/api/v1/snmp/walk', 'POST', this.snmpQuery);
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
        const res = await api('/api/v1/mdns/discover', 'POST', { service: this.mdnsService, duration: parseInt(this.mdnsDuration) });
        this.mdnsResults = res.data || [];
        this.toast(res.message + ' (' + this.mdnsResults.length + ' found)', 'success');
      } catch (e) {
        this.toast('mDNS discovery failed: ' + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async changePassword() {
      if (this.passwordChange.new_password.length < 8) {
        this.toast('New password must be at least 8 characters', 'error');
        return;
      }
      if (this.passwordChange.new_password !== this.passwordChange.confirm_password) {
        this.toast('New passwords do not match', 'error');
        return;
      }
      this.isLoading = true;
      try {
        const res = await api('/api/v1/password', 'POST', this.passwordChange);
        this.toast(res.message, 'success');
        this.passwordChange = { current_password: '', new_password: '', confirm_password: '' };
      } catch (e) {
        this.toast('Password change failed: ' + e.message, 'error');
      } finally {
        this.isLoading = false;
      }
    },
    async loadUsers() {
      try {
        const res = await api('/api/v1/users', 'GET');
        this.users = res.data || [];
      } catch (e) {
        this.users = [];
      }
    },
    async addUser() {
      try {
        await api('/api/v1/users', 'POST', this.newUser);
        this.toast('Utilisateur ' + this.newUser.username + ' créé', 'success');
        this.newUser = { username: '', role: 'operator', password: '' };
        await this.loadUsers();
      } catch (e) {
        this.toast('Échec : ' + e.message, 'error');
      }
    },
    async deleteUser(name) {
      if (!confirm('Supprimer le compte ' + name + ' ?')) return;
      try {
        await api('/api/v1/users', 'DELETE', { username: name });
        this.toast('Compte supprimé', 'success');
        await this.loadUsers();
      } catch (e) {
        this.toast('Échec : ' + e.message, 'error');
      }
    },
    async loadPresets() {
      try {
        const res = await api('/api/v1/presets', 'GET');
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
        const res = await api('/api/v1/presets/' + encodeURIComponent(this.selectedPreset), 'POST', { services: this.selectedPresetServices });
        this.toast(res.message + ' ' + (res.data?.log || ''), 'success');
        await this.refresh();
      } catch (e) {
        this.toast("Échec de l'application du preset : " + e.message, 'error');
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
    }
};

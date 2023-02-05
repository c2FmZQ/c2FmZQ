/*
 * Copyright 2021-2023 TTBT Enterprises LLC
 *
 * This file is part of c2FmZQ (https://c2FmZQ.org/).
 *
 * c2FmZQ is free software: you can redistribute it and/or modify it under the
 * terms of the GNU General Public License as published by the Free Software
 * Foundation, either version 3 of the License, or (at your option) any later
 * version.
 *
 * c2FmZQ is distributed in the hope that it will be useful, but WITHOUT ANY
 * WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR
 * A PARTICULAR PURPOSE. See the GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License along with
 * c2FmZQ. If not, see <https://www.gnu.org/licenses/>.
 */

/* jshint -W083 */
/* jshint -W126 */
'use strict';

let _T;

const SHOW_ITEMS_INCREMENT = 25;

class UI {
  #main;
  constructor(main) {
    this.#main = main;
    this.uiStarted_ = false;
    this.promptingForPassphrase_ = false;
    this.addingFiles_ = false;
    this.popupZindex_ = 1000;
    this.galleryState_ = {
      collection: this.#main.getHash('collection', 'gallery'),
      files: [],
      lastDate: '',
      shown: SHOW_ITEMS_INCREMENT,
      EL: new EventListeners(),
    };
    if (localStorage.getItem('_')) {
      this.skippedPassphrase_ = true;
    }
    this.enableNotifications = localStorage.getItem('enableNotifications') === 'yes';

    _T = Lang.text;

    const langSelect = document.querySelector('#language');
    langSelect.setAttribute('aria-label', _T('language'));
    UI.clearElement_(langSelect);
    const languages = Lang.languages();
    for (let l of Object.keys(languages)) {
      UI.create('option', {value:l, text: languages[l], selected: l === Lang.current, parent: langSelect});
    }
    langSelect.addEventListener('change', () => {
      localStorage.setItem('lang', langSelect.options[langSelect.options.selectedIndex].value);
      window.location.reload();
    });

    this.title_ = document.querySelector('title');
    this.passphraseInput_ = document.querySelector('#passphrase-input');
    this.passphraseInputLabel_ = document.querySelector('#passphrase-input-label');
    this.passphraseInput2_ = document.querySelector('#passphrase-input2');
    this.passphraseInput2Label_ = document.querySelector('#passphrase-input2-label');
    this.setPassphraseButton_ = document.querySelector('#set-passphrase-button');
    this.showPassphraseButton_ = document.querySelector('#show-passphrase-button');
    this.showPassphraseButton_.textContent = _T('show');
    this.skipPassphraseButton_ = document.querySelector('#skip-passphrase-button');
    this.skipPassphraseButton_.textContent = _T('skip-passphrase');
    this.resetDbButton_ = document.querySelector('#resetdb-button');

    this.emailInput_ = document.querySelector('#email-input');
    this.passwordInput_ = document.querySelector('#password-input');
    this.passwordInputLabel_ = document.querySelector('#password-input-label');
    this.passwordInput2_ = document.querySelector('#password-input2');
    this.passwordInput2Label_ = document.querySelector('#password-input2-label');
    this.backupPhraseInputLabel_ = document.querySelector('#backup-phrase-input-label');
    this.backupPhraseInput_ = document.querySelector('#backup-phrase-input');
    this.backupKeysCheckbox_ = document.querySelector('#backup-keys-checkbox');
    this.backupKeysCheckboxLabel_ = document.querySelector('#backup-keys-checkbox-label');
    this.serverInput_ = document.querySelector('#server-input');
    this.serverInput_.placeholder = _T('server-placeholder');
    this.loginButton_ = document.querySelector('#login-button');
    this.refreshButton_ = document.querySelector('#refresh-button');
    this.trashButton_ = document.querySelector('#trash-button');
    this.accountButton_ = document.querySelector('#account-button');

    document.querySelectorAll('form').forEach(e => {
      e.addEventListener('submit', event => {
        event.preventDefault();
      });
    });
    document.querySelector('title').textContent = _T('login');
    document.querySelector('#login-tab').textContent = _T('login');
    document.querySelector('#register-tab').textContent = _T('register');
    document.querySelector('#recover-tab').textContent = _T('recover');
    document.querySelector('label[for=email]').textContent = _T('form-email');
    document.querySelector('label[for=password]').textContent = _T('form-password');
    document.querySelector('label[for=password2]').textContent = _T('form-confirm-password');
    document.querySelector('label[for=backup-phrase]').textContent = _T('form-backup-phrase');
    document.querySelector('label[for=backup-keys-checkbox]').textContent = _T('form-backup-keys?');
    document.querySelector('label[for=server]').textContent = _T('form-server');
    document.querySelector('#login-button').textContent = _T('login');

    this.setPassphraseButton_.addEventListener('click', () => {
      if (this.passphraseInput_.value !== this.passphraseInput2_.value && this.passphraseInput2_.style.display !== 'none') {
        if (this.passphraseInput2_.value !== '') {
          this.popupMessage(_T('new-pass-doesnt-match'), 'error', {role:'alert'});
        }
        return;
      }
      this.setPassphrase_();
    });
    this.showPassphraseButton_.addEventListener('click', () => {
      if (this.passphraseInput_.type === 'text') {
        this.passphraseInput_.type = 'password';
        this.passphraseInput2_.type = 'password';
        this.showPassphraseButton_.textContent = _T('show');
      } else {
        this.passphraseInput_.type = 'text';
        this.passphraseInput2_.type = 'text';
        this.showPassphraseButton_.textContent = _T('hide');
      }
    });
    this.skipPassphraseButton_.addEventListener('click', () => {
      this.prompt({message: _T('skip-passphrase-warning')})
      .then(() => {
        const passphrase = btoa(String.fromCharCode(...window.crypto.getRandomValues(new Uint8Array(64))));
        localStorage.setItem('_', passphrase);
        this.skippedPassphrase_ = true;
        this.passphraseInput_.value = passphrase;
        this.setPassphrase_();
      })
      .catch(err => {
        console.log(err);
      });
    });
    this.resetDbButton_.addEventListener('click', () => {
      this.#main.resetPassphrase();
      this.resetDbButton_.className = 'hidden';
      window.location.reload();
    });
  }

  async loadFilerobot_() {
    if (!this.fieLoaded_) {
      this.fieLoaded_ = new Promise((resolve, reject) => {
        console.log('Loading filerobot image editor...');
        const e = UI.create('script');
        e.addEventListener('load', () => {
          console.log('filerobot image editor loaded');
          resolve();
        }, {once: true});
        e.addEventListener('error', err => {
          console.log('filerobot image editor failed to load', err);
          reject();
        }, {once: true});
        e.src = 'thirdparty/filerobot-image-editor.min.js';
        e.type = 'module';
        e.async = true;
        document.body.appendChild(e);
      });
    }
    return this.fieLoaded_;
  }

  promptForPassphrase(reset) {
    const p = localStorage.getItem('_');
    if (p) {
      this.passphraseInput_.value = p;
      this.passphraseInput2_.value = p;
      this.setPassphrase_();
      return;
    }
    document.querySelector('#passphrase-text').innerHTML = reset ? _T('enter-passphrase-text') : _T('reenter-passphrase-text');
    this.promptingForPassphrase_ = true;
    this.setPassphraseButton_.textContent = _T('set-passphrase');
    this.setPassphraseButton_.disabled = false;
    this.passphraseInput_.disabled = false;

    this.passphraseInput2_.disabled = false;
    this.passphraseInput2_.style.display = reset ? 'block' : 'none';
    this.passphraseInput2Label_.style.display = reset ? 'block' : 'none';
    this.skipPassphraseButton_.style.display = reset ? 'inline-block' : 'none';
    this.passphraseInput_.focus();
    this.showPassphraseBox_();
  }

  promptForBackupPhrase_() {
    return this.prompt({
      message: _T('enter-backup-phrase'),
      getValue: true,
    })
    .then(v => {
      return this.#main.sendRPC('restoreSecretKey', v);
    })
    .then(() => {
      this.refresh_();
    })
    .catch(err => {
      console.log('restoreSecretKey failed', err);
      this.popupMessage(err);
      window.setTimeout(this.promptForBackupPhrase_.bind(this), 2000);
    });
  }

  setPassphrase_(reset) {
    if (!this.passphraseInput_.value) {
      return;
    }
    this.promptingForPassphrase_ = false;
    this.setPassphraseButton_.textContent = _T('setting-passphrase');
    this.setPassphraseButton_.disabled = true;
    this.passphraseInput_.disabled = true;
    this.passphraseInput2_.disabled = true;
    this.resetDbButton_.className = 'hidden';

    this.#main.setPassphrase(this.passphraseInput_.value)
      .finally(() => {
        this.passphraseInput_.value = '';
        this.passphraseInput2_.value = '';
        this.setPassphraseButton_.textContent = _T('set-passphrase');
        this.setPassphraseButton_.disabled = false;
        this.passphraseInput_.disabled = false;
        this.passphraseInput2_.disabled = false;
      });

    setTimeout(() => {
      if (!this.uiStarted_ && !this.promptingForPassphrase_) {
        window.location.reload();
      }
    }, 3000);
  }

  wrongPassphrase(err) {
    console.log('Received wrong passphrase error');
    localStorage.removeItem('_');
    this.resetDbButton_.className = 'resetdb-button';
    this.popupMessage(err);
  }

  async serverHash_(n, commit) {
    let e = document.querySelector('#server-fingerprint');
    if (!e) {
      e = UI.create('div', {id:'server-fingerprint', role:'none'});
      document.body.appendChild(e);
    }
    return this.#main.calcServerFingerPrint(n, '#server-fingerprint', commit);
  }

  startUI() {
    console.log('Start UI');
    if (this.uiStarted_) {
      return;
    }
    this.uiStarted_ = true;

    if (SAMEORIGIN) {
      this.serverUrl_ = window.location.href.replace(/^(.*\/)[^\/]*/, '$1');
    } else {
      document.querySelector('label[for=server]').style.display = '';
      this.serverInput_.style.display = '';
      this.serverUrl_ = this.serverInput_.value;
    }
    this.serverHash_(this.serverUrl_, false);

    window.addEventListener('scroll', this.onScroll_.bind(this));
    window.addEventListener('resize', this.onScroll_.bind(this));
    window.addEventListener('hashchange', () => {
      const c = this.#main.getHash('collection');
      if (c && this.galleryState_.collection !== c) {
        this.switchView({collection: c});
      }
    });
    const html = document.querySelector('html');
    html.addEventListener('dragenter', event => {
      event.preventDefault();
    });
    html.addEventListener('dragover', event => {
      event.preventDefault();
    });
    html.addEventListener('drop', event => {
      event.preventDefault();
      event.stopPropagation();
      if (this.accountEmail_) {
        this.handleCollectionDropEvent_(this.galleryState_.collection, event);
      }
    });

    this.trashButton_.title = _T('trash-title');
    this.trashButton_.setAttribute('aria-label', _T('trash-title'));
    this.trashButton_.addEventListener('click', () => {
      this.switchView({collection: 'trash'});
    });
    this.trashButton_.addEventListener('dragover', event => {
      event.dataTransfer.dropEffect = 'move';
      event.preventDefault();
    });
    this.trashButton_.addEventListener('drop', event => {
      event.preventDefault();
      event.stopPropagation();
      this.handleCollectionDropEvent_('trash', event);
    });
    this.loginButton_.addEventListener('click', this.login_.bind(this));
    this.refreshButton_.title = _T('refresh-title');
    this.refreshButton_.setAttribute('aria-label', _T('refresh-title'));
    this.refreshButton_.addEventListener('click', this.refresh_.bind(this));
    this.emailInput_.addEventListener('keydown', e => {
      if (e.key === 'Enter') {
        this.passwordInput_.focus();
      }
    });
    this.passwordInput_.addEventListener('keydown', e => {
      if (e.key === 'Enter') {
        switch(this.selectedTab_) {
          case 'login':
            this.login_();
            break;
          case 'register':
          case 'recover':
            this.passwordInput2_.focus();
            break;
        }
      }
    });
    this.passwordInput2_.addEventListener('keydown', e => {
      if (e.key === 'Enter') {
        switch(this.selectedTab_) {
          case 'login':
            break;
          case 'register':
            this.login_();
            break;
          case 'recover':
            this.backupPhraseInput_.focus();
            break;
        }
      }
    });

    this.serverInput_.addEventListener('keydown', () => {
      this.serverHash_(this.serverInput_.value, false);
    });
    this.serverInput_.addEventListener('change', () => {
      this.serverHash_(this.serverInput_.value, false);
    });
    this.accountButton_.addEventListener('click', this.showAccountMenu_.bind(this));
    this.accountButton_.addEventListener('contextmenu', this.showAccountMenu_.bind(this));
    this.accountButton_.textContent = _T('account');
    this.accountButton_.title = _T('account-title');
    this.accountButton_.setAttribute('aria-label', _T('account-title'));

    const tabClick = event => {
      for (let tab of Object.values(this.tabs_)) {
        if (event.target === tab.elem) {
          this.selectedTab_ = tab.name;
          tab.elem.classList.add('select');
          tab.click();
        } else {
          tab.elem.classList.remove('select');
        }
      }
    };
    this.tabs_ = {
      login: {
        elem: document.querySelector('#login-tab'),
        message: _T('logging-in'),
        rpc: 'login',
        click: () => {
          this.passwordInputLabel_.textContent = _T('form-password');
          this.passwordInput2Label_.style.display = 'none';
          this.passwordInput2_.style.display = 'none';
          this.backupPhraseInputLabel_.style.display = 'none';
          this.backupPhraseInput_.style.display = 'none';
          this.backupKeysCheckbox_.style.display = 'none';
          this.backupKeysCheckboxLabel_.style.display = 'none';
          this.loginButton_.textContent = _T('login');
          this.title_.textContent = _T('login');
        },
      },
      register: {
        elem: document.querySelector('#register-tab'),
        message: _T('creating-account'),
        rpc: 'createAccount',
        click: () => {
          this.passwordInputLabel_.textContent = _T('form-new-password');
          this.passwordInput2Label_.style.display = '';
          this.passwordInput2_.style.display = '';
          this.backupPhraseInputLabel_.style.display = 'none';
          this.backupPhraseInput_.style.display = 'none';
          this.backupKeysCheckbox_.style.display = '';
          this.backupKeysCheckboxLabel_.style.display = '';
          this.loginButton_.textContent = _T('create-account');
          this.title_.textContent = _T('register');
        },
      },
      recover: {
        elem: document.querySelector('#recover-tab'),
        message: _T('recovering-account'),
        rpc: 'recoverAccount',
        click: () => {
          this.passwordInputLabel_.textContent = _T('form-new-password');
          this.passwordInput2Label_.style.display = '';
          this.passwordInput2_.style.display = '';
          this.backupPhraseInputLabel_.style.display = '';
          this.backupPhraseInput_.style.display = '';
          this.backupKeysCheckbox_.style.display = '';
          this.backupKeysCheckboxLabel_.style.display = '';
          this.loginButton_.textContent = _T('recover-account');
          this.title_.textContent = _T('recover-account');
        },
      },
    };
    for (let tab of Object.keys(this.tabs_)) {
      this.tabs_[tab].name = tab;
      this.tabs_[tab].elem.addEventListener('click', tabClick);
      this.tabs_[tab].elem.addEventListener('keydown', event => {
        if (event.key === 'Enter') {
          tabClick(event);
        }
      });
    }

    this.#main.sendRPC('isLoggedIn')
    .then(({account, isAdmin, needKey}) => {
      if (account !== '') {
        this.accountEmail_ = account;
        this.isAdmin_ = isAdmin;
        this.showLoggedIn_();
        this.#main.setServerFingerPrint('#server-fingerprint');
        if (needKey) {
          return this.promptForBackupPhrase_();
        }
        return this.getUpdates_()
          .then(() => {
            this.showQuota_();
          })
          .catch(this.showError_.bind(this))
          .finally(this.refreshGallery_.bind(this, true));
      } else {
        this.showLoggedOut_();
      }
    })
    .catch(this.showLoggedOut_.bind(this));
  }

  showQuota_() {
    this.#main.sendRPC('quota')
    .then(({usage, quota}) => {
      const pct = Math.floor(100 * usage / quota) + '%';
      document.querySelector('#quota').textContent = this.formatSizeMB_(usage) + ' / ' + this.formatSizeMB_(quota) + ' (' + pct + ')';
    });
  }

  showAccountMenu_(event) {
    event.preventDefault();
    const params = {
      target: event.target,
      x: event.x,
      y: event.y,
      items: [
        {
          text: this.accountEmail_,
        },
        {},
        {
          text: _T('profile'),
          onclick: this.showProfile_.bind(this),
          id: 'account-menu-profile',
        },
        {
          text: _T('key-backup'),
          onclick: this.showBackupPhrase_.bind(this),
          id: 'account-menu-key-backup',
        },
        {
          text: _T('prefs'),
          onclick: this.showPreferences_.bind(this),
          id: 'account-menu-prefs',
        },
      ],
    };
    if (this.isAdmin_) {
      params.items.push({});
      params.items.push({
        text: _T('admin-console'),
        onclick: this.showAdminConsole_.bind(this),
        id: 'account-menu-admin',
      });
    }
    if (!this.skippedPassphrase_) {
      params.items.push({});
      params.items.push({
        text: _T('lock'),
        onclick: () => this.#main.lock(),
        id: 'account-menu-lock',
      });
    }
    params.items.push({});
    params.items.push({
      text: _T('logout'),
      onclick: this.logout_.bind(this),
      id: 'account-menu-logout',
    });
    this.contextMenu_(params);
  }

  popupMessage(message, className, opt) {
    opt = opt || {};
    const div = UI.create('div', {className:className || 'error', role:opt.role || 'status'});
    div.style.position = 'relative';
    div.style.zIndex = this.popupZindex_++;
    const v = UI.create('span', {text:'✖',parent:div});
    v.style = 'float: right;';
    const m = UI.create('div', {className:'popup-message',parent:div});
    if (message instanceof Element) {
      m.appendChild(message);
    } else {
      m.textContent = message;
    }

    const LE = new EventListeners();

    const container = document.querySelector('#popup-messages');
    const remove = () => {
      LE.clear();
      div.style.animation = '1s linear slideOut';
      div.addEventListener('animationend', () => {
        try {
          UI.removeChild_(container, div);
        } catch(e) {}
      }, {once: true});
    };
    LE.add(div, 'click', remove);
    container.appendChild(div);

    if (!opt.sticky) {
      setTimeout(remove, 5000);
    }
    return remove;
  }

  showError_(e) {
    console.log('Show Error', e);
    console.trace();
    this.popupMessage(e.toString());
  }

  static create(type, opt) {
    opt = opt || {};
    const elem = document.createElement(type);
    if (['input','select','textarea','button'].includes(type)) {
      opt.tabindex = '0';
    }
    if (opt.title && !opt['aria-label']) {
      opt['aria-label'] = opt.title;
    }
    for (const attr of Object.keys(opt)) {
      if (attr === 'text') {
        elem.textContent = opt.text;
      } else if (attr === 'html') {
        elem.innerHTML = opt.html;
      } else if (['list','tabindex','role'].includes(attr) || attr.startsWith('aria')) {
        elem.setAttribute(attr, opt[attr]);
      } else if (attr === 'parent') {
        opt.parent.appendChild(elem);
      } else {
        elem[attr] = opt[attr];
      }
    }
    return elem;
  }

  static clearElement_(elem) {
    if (!(elem instanceof Node)) return;
    while (elem && elem.firstChild) {
      UI.clearElement_(elem.firstChild);
      elem.removeChild(elem.firstChild);
    }
  }

  static clearElementById_(id) {
    UI.clearElement_(document.getElementById(id));
  }

  static removeChild_(parent, child) {
    try {
      UI.clearElement_(child);
      parent.removeChild(child);
    } catch(err) {}
  }

  static clearView_() {
    UI.clearElementById_('gallery');
    UI.clearElementById_('popup-name');
    UI.clearElementById_('popup-content');
  }

  showPassphraseBox_() {
    UI.clearView_();
    this.title_.textContent = 'c2FmZQ';
    document.querySelector('#loggedout-div').className = 'hidden';
    document.querySelector('#loggedin-div').className = 'hidden';
    document.querySelector('#passphrase-div').className = '';
    this.passphraseInput_.focus();
  }

  showLoggedIn_() {
    this.title_.textContent = 'Gallery';
    document.querySelector('#loggedout-div').className = 'hidden';
    document.querySelector('#passphrase-div').className = 'hidden';
    document.querySelector('#loggedin-div').className = '';
    UI.clearView_();
  }

  showLoggedOut_() {
    this.title_.textContent = _T('login');
    UI.clearView_();
    this.selectedTab_ = 'login';
    this.tabs_[this.selectedTab_].click();

    document.querySelector('#password-input2-label').style.display = 'none';
    this.passwordInput2_.style.display = 'none';
    document.querySelector('#loggedin-div').className = 'hidden';
    document.querySelector('#passphrase-div').className = 'hidden';
    document.querySelector('#loggedout-div').className = '';
    this.emailInput_.focus();
  }

  bitScroll_() {
    const body = document.body;
    const div = UI.create('div', {id:'bit-scroller',role:'none',parent:body});
    const a = UI.create('div', {parent:div});
    const b = UI.create('div', {parent:div});
    const c = UI.create('div', {parent:div});

    const spin = ['⨁','⨂'];
    const cr = ['▀','▄'];
    const r = () => Math.floor(Math.random()*2);
    let count = 0;
    for (let i = 0; i < 16; i++) {
      a.textContent += cr[r()];
    }
    const id = window.setInterval(() => {
      a.textContent = cr[r()] + a.textContent.substring(0, 15);
      b.textContent = spin[count++ % spin.length];
      c.textContent = cr[r()] + c.textContent.substring(0, 15);
    }, 100);
    return () => {
      window.clearInterval(id);
      UI.removeChild_(body, div);
    };
  }

  async login_() {
    if (this.selectedTab_ !== 'login' && this.passwordInput_.value !== this.passwordInput2_.value) {
      this.popupMessage(_T('new-pass-doesnt-match'), 'error', {role:'alert'});
      return;
    }
    if (!SAMEORIGIN) {
      try {
        this.serverInput_.value = new URL(this.serverInput_.value).toString();
      } catch (err) {
        return Promise.reject(err);
      }
      this.serverUrl_ = this.serverInput_.value;
    }
    await this.serverHash_(this.serverUrl_, true);
    let old = this.loginButton_.textContent;
    this.loginButton_.textContent = this.tabs_[this.selectedTab_].message;
    this.loginButton_.disabled = true;
    this.emailInput_.disabled = true;
    this.passwordInput_.disabled = true;
    this.passwordInput2_.disabled = true;
    this.backupPhraseInput_.value = this.backupPhraseInput_.value
      .replaceAll(/[\t\r\n ]+/g, ' ')
      .replace(/^ */, '')
      .replace(/ *$/, '');
    this.backupPhraseInput_.disabled = true;
    this.backupKeysCheckbox_.disabled = true;
    this.serverInput_.disabled = true;
    const args = {
      email: this.emailInput_.value,
      password: this.passwordInput_.value,
      enableBackup: this.backupKeysCheckbox_.checked,
      backupPhrase: this.backupPhraseInput_.value,
      server: SAMEORIGIN ? undefined : this.serverInput_.value,
      enableNotifications: this.enableNotifications,
    };
    this.backupPhraseInput_.value = this.backupPhraseInput_.value.replaceAll(/./g, 'X');
    const done = this.bitScroll_();
    return this.#main.sendRPC(this.tabs_[this.selectedTab_].rpc, args).finally(done)
    .then(({isAdmin, needKey}) => {
      this.accountEmail_ = this.emailInput_.value;
      this.isAdmin_ = isAdmin;
      this.passwordInput_.value = '';
      this.passwordInput2_.value = '';
      this.backupPhraseInput_.value = '';
      this.showLoggedIn_();
      if (needKey) {
        return this.promptForBackupPhrase_();
      }
      const done = this.bitScroll_();
      return this.getUpdates_().finally(done)
        .then(() => {
          this.showQuota_();
          this.refreshGallery_(true);
        });
    })
    .catch(e => {
      this.backupPhraseInput_.value = args.backupPhrase;
      if (e === 'nok') {
        switch(this.selectedTab_) {
          case 'login':
            this.showError_(_T('login-failed'));
            break;
          case 'register':
            this.showError_(_T('create-account-failed'));
            break;
          case 'recover':
            this.showError_(_T('recover-account-failed'));
            break;
        }
      } else {
        this.showError_(e);
      }
    })
    .finally(() => {
      this.loginButton_.textContent = old;
      this.loginButton_.disabled = false;
      this.emailInput_.disabled = false;
      this.passwordInput_.disabled = false;
      this.passwordInput2_.disabled = false;
      this.backupPhraseInput_.disabled = false;
      this.backupKeysCheckbox_.disabled = false;
      this.serverInput_.disabled = false;
    });
  }

  async logout_() {
    return this.#main.sendRPC('logout')
    .finally(() => {
      window.localStorage.removeItem('_');
      window.localStorage.removeItem('salt');
      this.#main.lock();
    });
  }

  async getUpdates_() {
    return this.#main.sendRPC('getUpdates')
      .catch(err => {
        this.showError_(err);
      })
      .then(() => this.#main.sendRPC('getCollections'))
      .then(v => {
        this.collections_ = v;
      });
  }

  async refresh_() {
    this.refreshButton_.disabled = true;
    return this.getUpdates_()
      .then(this.refreshGallery_.bind(this, false))
      .catch(this.showError_.bind(this))
      .finally(() => {
        this.refreshButton_.disabled = false;
        this.showQuota_();
      });
  }

  onScroll_() {
    const distanceToBottom = Math.floor(document.documentElement.scrollHeight - document.documentElement.scrollTop - window.innerHeight);
    if (distanceToBottom < 200 && !this.addingFiles_) {
      this.addingFiles_ = true;
      window.requestAnimationFrame(() => {
        this.showMoreFiles_(SHOW_ITEMS_INCREMENT)
        .finally(() => {
          this.addingFiles_ = false;
        });
      });
    }
  }

  switchView(c) {
    if (this.galleryState_.collection !== c.collection) {
      this.galleryState_.collection = c.collection;
      this.galleryState_.shown = SHOW_ITEMS_INCREMENT;
      this.#main.setHash('collection', c.collection);
      this.refreshGallery_(true);
    } else {
      this.refreshGallery_(false);
    }
  }

  showAddMenu_(event) {
    event.preventDefault();
    const params = {
      target: event.target,
      x: event.x,
      y: event.y,
      items: [
        {
          text: _T('upload-files'),
          onclick: this.showUploadView_.bind(this),
          id: 'menu-upload-files',
        },
        {
          text: _T('create-collection'),
          onclick: () => this.collectionProperties_(),
          id: 'menu-create-collection',
        },
      ],
    };
    this.contextMenu_(params);
  }

  async refreshGallery_(scrollToTop) {
    const EL = this.galleryState_.EL;
    const cachePref = await this.#main.sendRPC('cachePreference');
    if (!this.galleryState_.format) {
      this.galleryState_.format = 'grid';
    }
    if (this.galleryState_.content && this.galleryState_.content.files) {
      for (const it of this.galleryState_.content.files) {
        delete it.elem;
      }
    }
    this.galleryState_.content = await this.#main.sendRPC('getFiles', this.galleryState_.collection);
    if (!this.galleryState_.content) {
      this.galleryState_.content = {'total': 0, 'files': []};
    }
    const oldScrollLeft = document.querySelector('#collections')?.scrollLeft;
    const oldScrollTop = scrollToTop ? 0 : document.documentElement.scrollTop;
    if (scrollToTop) {
      document.documentElement.scrollTo(0, 0);
    }

    EL.clear();
    const cd = document.querySelector('#collections');
    if (this.collections_.length && !this.collections_[0].obj) {
      UI.clearElement_(cd);
    }
    let g = document.querySelector('#gallery');
    UI.clearElement_(g);

    let collectionName = '';
    let members = [];
    let scrollTo = null;
    let isOwner = false;
    let canAdd = false;

    const showContextMenu = (event, c) => {
      let params = {
        target: event.target,
        x: event.x,
        y: event.y,
        items: [],
      };
      if (this.galleryState_.collection !== c.collection) {
        params.items.push({
          text: _T('open'),
          id: "context-menu-open",
          onclick: () => this.switchView({collection: c.collection}),
        });
      }
      if (c.collection !== 'trash' && c.collection !== 'gallery') {
        params.items.push({
          text: _T('properties'),
          id: "context-menu-properties",
          onclick: () => this.collectionProperties_(c),
        });
      }
      if (this.galleryState_.collection !== c.collection) {
        if ((this.galleryState_.collection !== 'trash' || c.collection === 'gallery') && this.galleryState_.content.files.some(f => f.selected)) {
          params.items.push({});
          const f = this.galleryState_.content.files.find(f => f.selected);
          if (this.galleryState_.collection !== 'trash') {
            params.items.push({
              text: _T('copy-selected'),
              id: "context-menu-copy",
              onclick: () => this.moveFiles_({file: f.file, collection: f.collection, move: false}, c.collection),
            });
          }
          params.items.push({
            text: _T('move-selected'),
            id: "context-menu-move",
            onclick: () => this.moveFiles_({file: f.file, collection: f.collection, move: true}, c.collection),
          });
        }
      }
      if (cachePref.mode === 'encrypted') {
        params.items.push({});
        params.items.push({
          text: c.isOffline ? _T('offline-on') : _T('offline-off'),
          onclick: () => {
             this.#main.sendRPC('setCollectionOffline', c.collection, !c.isOffline)
             .then(() => {
               this.refresh_();
             });
          },
        });
      }
      if (params.items.length > 0) {
        this.contextMenu_(params);
      }
    };

    let currentCollection;
    for (let i in this.collections_) {
      if (!this.collections_.hasOwnProperty(i)) {
        continue;
      }
      const c = this.collections_[i];
      const isCurrent = this.galleryState_.collection === c.collection;
      if (!currentCollection || isCurrent) {
        currentCollection = c;
      }
      if (c.name === 'gallery' || c.name === 'trash') {
        c.name = _T(c.name);
      }
      if (isCurrent) {
        this.title_.textContent = c.name;
        this.galleryState_.canDrag = c.isOwner || c.canCopy;
        this.galleryState_.isOwner = c.isOwner;
      }
      if (c.collection === 'trash') {
        continue;
      }
      if (!c.obj) {
        c.obj = new CollectionThumb(c);
        cd.appendChild(c.obj.div);
      }
      c.obj.setCurrent(isCurrent);
      c.obj.setCacheMode(cachePref.mode);
      const div = c.obj.div;

      if (isCurrent) {
        scrollTo = div;
      }
      if (c.isOwner || c.canAdd) {
        EL.add(div, 'dragover', event => {
          event.preventDefault();
        });
        EL.add(div, 'drop', event => {
          event.preventDefault();
          event.stopPropagation();
          this.handleCollectionDropEvent_(c.collection, event);
        });
      }
      EL.add(div, 'contextmenu', event => {
        event.preventDefault();
        showContextMenu(event, c);
      });
      EL.add(div, 'click', () => {
        this.switchView(c);
      });
      EL.add(div, 'keydown', e => {
        if (e.key === 'Enter') {
          e.preventDefault();
          e.stopPropagation();
          showContextMenu(e, c);
          //this.switchView(c);
        }
      }, true);
    }

    const collButtons = UI.create('div', {id:'collection-buttons'});
    const formatButton = UI.create('button', {
      id: 'format-button',
      text: this.galleryState_.format === 'list' ? _T('grid') : _T('list'),
      title: this.galleryState_.format === 'list' ? _T('grid-title') : _T('list-title'),
      parent: collButtons,
    });
    EL.add(formatButton, 'click', () => {
      this.galleryState_.format = this.galleryState_.format === 'list' ? 'grid' : 'list';
      this.refreshGallery_(true);
    });
    if (this.galleryState_.collection === 'trash') {
      const emptyButton = UI.create('button', {className:'empty-trash',text:_T('empty'),parent:collButtons});
      EL.add(emptyButton, 'click', e => {
        this.emptyTrash_(e.target);
      });
    }
    if (this.galleryState_.collection !== 'gallery' && this.galleryState_.collection !== 'trash') {
      const settingsButton = UI.create('button', {
        id: 'settings-button',
        text: '⚙',
        title: _T('settings-title'),
      });
      EL.add(settingsButton, 'click', () => {
        this.collectionProperties_(currentCollection);
      });
      collButtons.appendChild(settingsButton);
    }
    g.appendChild(collButtons);

    if (currentCollection.isOwner || currentCollection.canAdd) {
      const addDiv = UI.create('div', {id:'add-button', text: '＋', tabindex:'0', role:'link', title: _T('add-button-title'), parent:g});
      EL.add(addDiv, 'keydown', e => {
        if (e.key === 'Enter') {
          this.showAddMenu_(e);
        }
      });
      EL.add(addDiv, 'click', this.showAddMenu_.bind(this));
      EL.add(addDiv, 'contextmenu', this.showAddMenu_.bind(this));
    }

    UI.create('br', {clear:'all',parent:g});
    UI.create('h1', {text:_T('collection:', currentCollection.name), parent:g});
    if (currentCollection?.members?.length > 0) {
      UI.sortBy(currentCollection.members, 'email');
      UI.create('div', {text:_T('shared-with', currentCollection.members.map(m => m.email).join(', ')), parent:g});
    }

    this.galleryState_.lastDate = '';
    const n = Math.max(this.galleryState_.shown, SHOW_ITEMS_INCREMENT);
    this.galleryState_.shown = 0;
    const dist = () => document.documentElement.scrollHeight - document.documentElement.scrollTop - window.innerHeight;
    do {
      await this.showMoreFiles_(n, oldScrollTop);
    } while (dist() <= 0 && this.galleryState_.shown < this.galleryState_.content.total);

    if (scrollTo) {
      setTimeout(() => {
        const left = Math.max(scrollTo.offsetLeft - (cd.offsetWidth - scrollTo.offsetWidth)/2, 0);
        cd.scrollTo({behavior: 'smooth', left: left});
        scrollTo.focus();
      }, 10);
    }
  }

  static sortBy(arr, field) {
    return arr.sort((a, b) => {
      if (a[field] < b[field]) return -1;
      if (a[field] > b[field]) return 1;
      return 0;
    });
  }

  static px_(n) {
    return ''+Math.floor(n / window.devicePixelRatio)+'px';
  }

  async maybeGetMoreFiles_(count) {
    const max = Math.min(count, this.galleryState_.content.total);
    if (max <= this.galleryState_.content.files.length) {
      return this.galleryState_.content.files.length;
    }
    while (this.galleryState_.content.files.length < max) {
      let ff = await this.#main.sendRPC('getFiles', this.galleryState_.collection, this.galleryState_.content.files.length);
      this.galleryState_.content.files.push(...ff.files);
    }
    return this.galleryState_.content.files.length;
  }

  async showMoreFiles_(n, opt_scrollTop) {
    const EL = this.galleryState_.EL;
    if (!this.galleryState_.content) {
      return;
    }
    const max = await this.maybeGetMoreFiles_(this.galleryState_.shown + n);
    const g = document.querySelector('#gallery');

    const showContextMenu = (event, i) => {
      const f = this.galleryState_.content.files[i];
      let params = {
        target: event.target,
        x: event.x,
        y: event.y,
        items: [],
      };
      params.items.push({
        text: _T('open'),
        onclick: () => this.setUpPopup_(i),
      });
      if (f.isImage && (this.galleryState_.isOwner || this.galleryState_.canAdd)) {
        params.items.push({});
        params.items.push({
          text: _T('edit'),
          id: 'context-menu-edit',
          onclick: () => this.showEdit_(f),
        });
      }
      params.items.push({
        text: f.selected ? _T('unselect') : _T('select'),
        id: 'context-menu-select',
        onclick: f.select,
      });
      if (this.galleryState_.isOwner) {
        if (f.collection !== 'trash' && f.collection !== 'gallery') {
          params.items.push({
            text: _T('use-as-cover'),
            onclick: () => this.changeCover_(f.collection, f.file),
          });
        }
        if (this.galleryState_.collection !== 'trash') {
          params.items.push({});
          params.items.push({
            text: _T('move-to-trash'),
            onclick: () => this.prompt({message: _T('confirm-move-to-trash')}).then(() => this.moveFiles_({file: f.file, collection: f.collection, move: true}, 'trash')),
          });
        } else {
          params.items.push({
            text: _T('move-to-gallery'),
            onclick: () => this.prompt({message: _T('confirm-move-to-gallery')}).then(() => this.moveFiles_({file: f.file, collection: f.collection, move: true}, 'gallery')),
          });
          params.items.push({});
          params.items.push({
            text: _T('delete-perm'),
            onclick: () => this.prompt({message: _T('confirm-delete-perm')}).then(() => this.deleteFiles_({file: f.file, collection: f.collection})),
          });
        }
      }
      if (this.galleryState_.content.files.filter(f => f.selected).length > 1) {
        params.items.push({
          text: _T('unselect-all'),
          onclick: () => {
            this.galleryState_.content.files.forEach(f => {
              if (f.selected) f.select();
            });
          },
        });
      }
      this.contextMenu_(params);
    };

    const selectItem = i => {
      const item = this.galleryState_.content.files[i];
      item.selected = !item.selected;
      item.elem.classList.toggle('selected');
    };

    const dragStart = (f, event, img) => {
      const move = event.shiftKey === false;
      event.dataTransfer.setData('application/json', JSON.stringify({collection: f.collection, file: f.file, move: move}));
      event.dataTransfer.effectAllowed = move ? 'move' : 'copy';
      event.dataTransfer.setDragImage(img, img.width/2, -20);

      if (move) {
        f.elem.classList.add('dragging');
      }
      if (document.documentElement.scrollTop > 50) {
        document.querySelector('#collections').classList.add('fixed');
      }
    };
    const dragEnd = (f, event) => {
      f.elem.classList.remove('dragging');
      document.querySelector('#collections').classList.remove('fixed');
    };
    const click = (i, event) => {
      if (event.shiftKey || this.galleryState_.content.files.some(f => f.selected)) {
        return this.galleryState_.content.files[i].select();
      }
      this.setUpPopup_(i);
    };
    const contextMenu = (i, event) => {
      event.preventDefault();
      showContextMenu(event, i);
      // chrome bug
      const f = this.galleryState_.content.files[i];
      f.elem.classList.remove('dragging');
    };

    let listDiv;
    if (this.galleryState_.format === 'list') {
      listDiv = document.querySelector('#gallery-list-div');
      if (!listDiv) {
        listDiv = UI.create('div', {id:'gallery-list-div', html: '<div class="row"><div>&nbsp;</div><div><span>'+_T('filename')+'</span></div><div class="size"><span>'+_T('filesize')+'</span></div><div class="time"><span>'+_T('creation-time')+'</span></div><div class="content-type"><span>'+_T('filetype')+'</span></div><div class="duration"><span>'+_T('duration')+'</span></div></div>', parent: g});
      }
    }

    const scroll = event => {
      event.target.style.width = '';
      if (opt_scrollTop && opt_scrollTop !== document.documentElement.scrollTop) {
        document.documentElement.scrollTo(0, opt_scrollTop);
      }
    };

    const year = new Date().getFullYear();

    const last = Math.min(this.galleryState_.shown + n, max);
    for (let i = this.galleryState_.shown; i < last; i++) {
      this.galleryState_.shown++;
      const f = this.galleryState_.content.files[i];
      if (this.galleryState_.format === 'grid') {
        const created = new Date(f.dateCreated);
        //const date = created.toLocaleDateString(undefined, {weekday: 'long', year: 'numeric', month: 'long', day: 'numeric'});
        const dateFormat = year === created.getFullYear() ? {month: 'long'} : {month: 'long', year: 'numeric'};
        const date = created.toLocaleDateString(undefined, dateFormat);
        if (date !== this.galleryState_.lastDate) {
          this.galleryState_.lastDate = date;
          const dateDiv = UI.create('div', {className:'date', html:'<br clear="all" />'+date+'<br clear="all" />', parent:g});
        }

        const d = UI.create('div', {className:'thumbdiv', draggable:true, tabindex:'0', role:'link', title:_T('file-title', f.fileName)});
        f.elem = d;
        f.select = () => selectItem(i);

        const img = new Image();
        EL.add(img, 'load', scroll);
        img.alt = f.fileName;
        img.src = f.thumbUrl;
        img.style.height = UI.px_(320);
        img.style.width = UI.px_(240);
        d.appendChild(img);
        if (f.isVideo) {
          const div = UI.create('div',{className:'duration', parent:d});
          UI.create('span', {className:'duration', text: this.formatDuration_(f.duration), parent: div});
        }

        EL.add(d, 'click', e => click(i, e));
        EL.add(d, 'keydown', e => {
          if (e.key === 'Enter') {
            e.stopPropagation();
            contextMenu(i, e);
          }
        }, true);
        EL.add(d, 'dragstart', e => dragStart(f, e, img));
        EL.add(d, 'dragend', e => dragEnd(f, e));
        EL.add(d, 'contextmenu', e => contextMenu(i, e));

        g.appendChild(d);
      } else {
        const row = UI.create('div', {className:'row', parent:listDiv});

        const imgDiv = UI.create('div', {tabindex:'0', role:'link', title:_T('file-title', f.fileName), parent:row});
        imgDiv.draggable = true;
        EL.add(imgDiv, 'dragstart', e => dragStart(f, e, img));
        EL.add(imgDiv, 'dragend', e => dragEnd(f, e));

        const img = new Image();
        EL.add(img, 'load', scroll);
        img.draggable = false;
        img.alt = f.fileName;
        img.src = f.thumbUrl;
        imgDiv.appendChild(img);

        EL.add(row, 'click', e => click(i, e));
        EL.add(row, 'contextmenu', e => contextMenu(i, e));
        EL.add(row, 'keydown', e => {
          if (e.key === 'Enter') {
            e.stopPropagation();
            contextMenu(i, e);
          }
        }, true);
        f.elem = row;
        f.select = () => selectItem(i);

        const fname = UI.create('div', {className:'filename',parent:row});
        UI.create('span', {text:f.fileName, parent:fname});

        const size = UI.create('div', {className:'size', parent:row});
        UI.create('span', {text: this.formatSize_(f.size), parent: size});

        const ts = UI.create('div', {className:'time', parent:row});
        const tsSpan = UI.create('span', {text: (new Date(f.dateCreated)).toLocaleString(), parent:ts});

        const ctype = UI.create('div', {className:'content-type', parent:row});
        UI.create('span', {text:f.contentType, parent:ctype});

        const durDiv = UI.create('div', {className:'duration', parent:row});
        if (f.isVideo) {
          UI.create('span', {className:'duration', text: this.formatDuration_(f.duration), parent:durDiv});
        }
      }
    }
  }

  async handleCollectionDropEvent_(collection, event) {
    const moveData = event.dataTransfer.getData('application/json');
    let files = [];
    if (!moveData && collection !== 'trash') {
      if (event.dataTransfer.items) {
        for (let i = 0; i < event.dataTransfer.items.length; i++) {
          if (event.dataTransfer.items[i].kind === 'file') {
            files.push(event.dataTransfer.items[i].getAsFile());
          }
        }
      } else {
        for (let i = 0; i < event.dataTransfer.files.length; i++) {
          files.push(event.dataTransfer.files[i]);
        }
      }
    }
    if (moveData) {
      return this.moveFiles_(JSON.parse(moveData), collection);
    }
    return this.handleDropUpload_(collection, files);
  }

  async cancelDropUploads_() {
    this.cancelQueuedDropUploads_ = true;
    this.cancelQueuedThumbnailRequests_();
  }

  async handleDropUpload_(collection, files) {
    const MAX = 10;
    const up = [];
    this.cancelQueuedDropUploads_ = false;
    let abort = null;
    this.popupMessage(_T('drop-received'), 'progress');
    for (let i = 0; i < files.length; i += MAX) {
      const toUpload = [];
      const tnp = [];
      for (let n = 0; n < MAX && i+n < files.length; n++) {
        const off = i+n;
        tnp.push(this.makeThumbnail_(files[off]).then(([data, duration]) => {
          toUpload.push({
            file: files[off],
            thumbnail: data,
            duration: duration,
          });
        }));
      }
      up.push(Promise.all(tnp).then(() => {
        if (this.cancelQueuedDropUploads_) {
          abort = 'canceled';
        }
        if (abort) {
          return Promise.reject(abort);
        }
        return this.#main.sendRPC('upload', collection, toUpload)
          .catch(err => {
            abort = err;
            this.cancelQueuedThumbnailRequests_();
            return Promise.reject(abort);
          });
      }));
    }
    return Promise.all(up)
      .then(() => {
        this.refresh_();
      })
      .catch(e => {
        this.showError_(e);
      });
  }

  moveFiles_(file, collection) {
    let files = [file.file];
    let useSelected = false;
    const selected = [];
    for (let i = 0; i < this.galleryState_.content.files.length; i++) {
      if (this.galleryState_.content.files[i].selected === true) {
        selected.push(this.galleryState_.content.files[i].file);
        if (this.galleryState_.content.files[i].file === file.file) {
          useSelected = true;
        }
      }
    }
    if (useSelected) {
      files = selected;
    }
    if (file.collection === collection) {
      return false;
    }
    if (file.collection === 'trash' && collection !== 'gallery') {
      this.popupMessage(_T('move-to-gallery-only'), 'info');
      return false;
    }
    return this.#main.sendRPC('moveFiles', file.collection, collection, files, file.move)
      .then(() => {
        if (file.move) {
          this.popupMessage(files.length === 1 ? _T('moved-one-file') : _T('moved-many-files', files.length), 'info');
        } else {
          this.popupMessage(files.length === 1 ? _T('copied-one-file') : _T('copied-many-files', files.length), 'info');
        }
        this.refresh_();
      })
      .catch(e => {
        this.showError_(e);
      });
  }

  deleteFiles_(file) {
    if (this.galleryState_.collection !== 'trash') {
      return;
    }
    let files = [file.file];
    let useSelected = false;
    const selected = [];
    for (let i = 0; i < this.galleryState_.content.files.length; i++) {
      if (this.galleryState_.content.files[i].selected === true) {
        selected.push(this.galleryState_.content.files[i].file);
        if (this.galleryState_.content.files[i].file === file.file) {
          useSelected = true;
        }
      }
    }
    if (useSelected) {
      files = selected;
    }
    return this.#main.sendRPC('deleteFiles', files)
      .then(() => {
        this.popupMessage(files.length === 1 ? _T('deleted-one-file') : _T('deleted-many-files', files.length), 'info');
        this.refresh_();
      })
      .catch(e => {
        this.showError_(e);
      });
  }

  async emptyTrash_(b) {
    b.disabled = true;
    this.#main.sendRPC('emptyTrash')
    .then(() => {
      this.refresh_();
    })
    .catch(e => {
      this.showError_(e);
    })
    .finally(() => {
      b.disabled = false;
    });
  }

  async changeCover_(collection, cover) {
    return this.#main.sendRPC('changeCover', collection, cover)
      .then(() => {
        this.refresh_();
      })
      .catch(e => {
        this.showError_(e);
      });
  }

  async leaveCollection_(collection) {
    return this.prompt({message: _T('confirm-leave')})
    .then(() => this.#main.sendRPC('leaveCollection', collection))
    .then(() => {
      if (this.galleryState_.collection === collection) {
        this.switchView({collection: 'gallery'});
      }
      this.refresh_();
    });
  }

  async deleteCollection_(collection) {
    return this.prompt({message: _T('confirm-delete')})
    .then(() => this.#main.sendRPC('deleteCollection', collection))
    .then(() => {
      if (this.galleryState_.collection === collection) {
        this.switchView({collection: 'gallery'});
      }
      this.refresh_();
    });
  }

  contextMenu_(params) {
    if (this.closeContextMenu_) {
      this.closeContextMenu_();
    }
    const EL = new EventListeners();
    const menu = UI.create('div', {className: params.className || 'context-menu'});
    let x = params.x;
    let y = params.y;
    if (!x) {
      let t = params.target;
      x = t.offsetLeft + t.offsetWidth / 2;
      y = t.offsetTop + t.offsetHeight / 2;
      t = t.offsetParent;
      while (t) {
        x += t.offsetLeft;
        y += t.offsetTop;
        t = t.offsetParent;
      }
      t = params.target.parentElement;
      while (t) {
        x -= t.scrollLeft;
        y -= t.scrollTop;
        t = t.parentElement;
      }
    }
    EL.add(menu, 'contextmenu', event => {
      event.preventDefault();
    });
    const point = UI.create('img', {parent:menu});
    point.style.position = 'absolute';

    let closeMenu;
    const handleEscape = e => {
      if (e.key === 'Escape') {
        closeMenu();
      }
    };
    const handleClickOutside = e => {
      if (!e.composedPath().includes(menu)) {
        e.stopPropagation();
        closeMenu();
      }
    };
    setTimeout(() => {
      this.setGlobalContext([
        ['keydown', handleEscape, true],
        ['click', handleClickOutside, true],
      ], menu);
    });

    const body = document.body;
    const collections = document.querySelector('#collections');
    body.classList.add('noscroll');
    collections.classList.add('noscroll');
    let once = false;
    closeMenu = () => {
      if (once) return;
      once = true;
      this.closeContextMenu_ = null;
      body.classList.remove('noscroll');
      collections.classList.remove('noscroll');
      // Remove handlers.
      this.setGlobalContext(null);
      EL.clear();
      menu.style.animation = 'fadeOut 0.25s';
      menu.addEventListener('animationend', () => {
        try {
          UI.removeChild_(body, menu);
        } catch (e) {}
      }, {once: true});
      closeMenu = () => null;
      params.target.focus();
    };

    for (let i = 0; i < params.items.length; i++) {
      if (!params.items[i].text) {
        if (i !== 0 && i !== params.items.length - 1) {
          UI.create('hr', {className:'context-menu-space',parent:menu});
        }
        continue;
      }
      if (params.items[i].onclick) {
        const item = UI.create('button', {
          id: params.items[i].id,
          className: 'context-menu-item',
          text: params.items[i].text,
          tabindex: '0',
          role: 'menuitem',
          parent: menu,
        });
        EL.add(item, 'click', e => {
          e.preventDefault();
          e.stopPropagation();
          e.stopImmediatePropagation();
          closeMenu();
          params.items[i].onclick(e);
        });
      } else {
        UI.create('div', {
          className: 'context-menu-div',
          text: params.items[i].text,
          parent: menu,
        });
      }
    }
    body.appendChild(menu);

    let up, left;
    if (x > window.innerWidth / 2) {
      left = true;
      x -= menu.offsetWidth + 30;
    } else {
      x += 30;
    }
    if (y > window.innerHeight / 2) {
      up = true;
      y -= menu.offsetHeight + 10;
    } else {
      y += 10;
    }
    if (up && !left) {
      point.src = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABgAAAAYCAYAAADgdz34AAAABmJLR0QA4ADgAODYf054AAAAg0lEQVRIx7XVQRKAIAxD0eT+h647HAeQpjSs9X1GxgJ4V9CJRwToxAFYAgN3BD54d2DCOwNLvCuwxTsCv/ht4IjfBFJ4NZDGKwEJVwMyrgRKeDZQxjOB+s7JY0A/UHLaPK9+IvL4RSjPlsUulTNYj1wR3T38XnNk10gZL4bpnhh4wLQeewpX1wE0L8gAAAAASUVORK5CYII=';
      point.style.left = '-24px';
      point.style.top = (menu.offsetHeight-26) + 'px';
    } else if (up && left) {
      point.src = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABgAAAAYCAYAAADgdz34AAAABmJLR0QA4ADgAODYf054AAAAj0lEQVRIx7XSSw6AIAwE0Onc/851I8YgYL9dKYy+pgAAisaiqqITIQB0IhwPXQjfLx0I54VqhKvFSoS7jSqEp80KhH+BLEJLKIPQGowi9IQjCL0deRFG5upBGL0dVkRG6P7AXSIy/rMFnqam7kqQnayLcYQQsY7cgq0QiZzvCZsRQa6Wo3wjWeAIApBq4INdkLZW6dJ0VgoAAAAASUVORK5CYII=';
      point.style.left = (menu.offsetWidth-2) + 'px';
      point.style.top = (menu.offsetHeight-26) + 'px';
    } else if (!up && left) {
      point.src = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABgAAAAYCAYAAADgdz34AAAABmJLR0QA4ADgAODYf054AAAAh0lEQVRIx7XWMRLAIAhE0d1M7n9l0kQnJkoAgZLifewk6kYAgBXoY8hMUERulf147qANHE8eXWagK1wLTJ+uzQx/L8QDWvC2lAhqwQHgjMIWHACOSjwcsOKhgAd3B7y4KxDBzYEobgrs4L+BXVwNZODLQBY+DWTin0A2PgQq8B6owlc/gdS5AJl7HmCcZQWCAAAAAElFTkSuQmCC';
      point.style.left = (menu.offsetWidth-2) + 'px';
      point.style.top = '-2px';
    } else {
      point.src = 'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABgAAAAYCAYAAADgdz34AAAABmJLR0QA4ADgAODYf054AAAAlElEQVRIx7XV3QqAIAwF4LP3f+h1kQTTpvs5ChHE6nOYRwDQcV0ZMu7qPKcBAKCqryMiYIHzix9iiiwoHcBFqphXsEUc7Pd7uxmEkFN3pxbTyIxFFqyMZP6IeieJ2hKS3UD5hS90nUKqERBGOqEW24zNsDwijFjeZxcp9l2EdrC4UU8+IReEDSzIDcAev7g3WikcRh7lLVfXPzJ76wAAAABJRU5ErkJggg==';
      point.style.left = '-24px';
      point.style.top = '-2px';
    }

    menu.style.left = x + 'px';
    menu.style.top = y + 'px';
    window.setTimeout(closeMenu, 10000);
    this.closeContextMenu_ = closeMenu;
    return menu;
  }

  static hasAncestor(node, parent) {
    while (node) {
      if (node === parent) return true;
      node = node.parentElement;
    }
    return false;
  }

  setGlobalContext(handlers, parent) {
    if (!this.windowHandlers) {
      this.windowHandlers = [];
    }
    const add = h => {
      h.handlers.forEach(args => document.addEventListener(...args));
      h.tabs.forEach(e => e.setAttribute('tabindex', '-1'));
    };
    const remove = h => {
      if (!h) {
        console.log('setGlobalContext nothing to remove');
        return;
      }
      h.handlers.forEach(args => document.removeEventListener(...args));
      h.tabs.forEach(e => e.setAttribute('tabindex', '0'));
    };
    if (handlers) {
      // Add events.
      if (this.windowHandlers.length) {
        remove(this.windowHandlers[0]);
      }
      const tabs = Array.from(document.querySelectorAll('[tabindex="0"],button,select,a')).filter(node => !UI.hasAncestor(node, parent));
      this.windowHandlers.unshift({handlers, tabs});
      add(this.windowHandlers[0]);
    } else {
      // Remove events.
      remove(this.windowHandlers.shift());
      if (this.windowHandlers.length) {
        add(this.windowHandlers[0]);
      }
    }
  }

  async prompt(params) {
    const body = document.body;
    
    const bg = UI.create('div', {className: 'prompt-bg', role:'none', parent: body});
    const win = UI.create('div', {className: 'prompt-div', role:'alertdialog', 'aria-labelledby':'prompt-text'});
    const text = UI.create('div', {className: 'prompt-text', id:'prompt-text', text: params.message, parent: win});

    let input;
    if (params.getValue) {
      if (params.password) {
        input = UI.create('input', {type:'password'});
      } else {
        input = UI.create('textarea');
      }
      input.className = 'prompt-input';
      if (params.defaultValue) {
        input.value = params.defaultValue;
      }
      win.appendChild(input);
    }

    const buttons = UI.create('div', {className:'prompt-button-row button-group',parent:win});
    const canc = UI.create('button', {className:'prompt-cancel-button', text:params.cancelText || _T('cancel'), tabindex:'0', parent:buttons});
    const conf = UI.create('button', {className:'prompt-confirm-button', text:params.confirmText || _T('confirm'), tabindex:'0', parent:buttons});

    body.appendChild(win);
    if (input) input.focus();
    else canc.focus();

    const waiting = body.classList.contains('waiting');
    if (waiting) {
      body.classList.remove('waiting');
    }
    const p = new Promise((resolve, reject) => {
      const EL = new EventListeners();
      const handleEscape = e => {
        if (e.key === 'Escape') {
          e.stopPropagation();
          close();
          reject(_T('canceled'));
        }
      };
      this.setGlobalContext([
        ['keydown', handleEscape, true],
      ], win);
      const close = () => {
        this.setGlobalContext(null);
        EL.clear();
        UI.removeChild_(body, bg);
        UI.removeChild_(body, win);
        if (waiting) {
          body.classList.add('waiting');
        }
      };
      EL.add(conf, 'click', () => {
        close();
        resolve(params.getValue ? input.value.trim() : true);
      });
      EL.add(canc, 'click', () => {
        close();
        reject(_T('canceled'));
      });
    });
    return p;
  }

  freeze(params) {
    const body = document.body;
    const bg = UI.create('div', {className:'prompt-bg', role:'none', parent:body});
    const win = UI.create('div', {className: 'prompt-div', role:'alertdialog', 'aria-labelledby':'prompt-text', parent:body});
    const text = UI.create('div', {className:'prompt-text', id:'prompt-text', text:params.message, parent:win});

    const waiting = body.classList.contains('waiting');
    if (waiting) {
      body.classList.remove('waiting');
    }
    this.setGlobalContext([], win);
    return () => {
      this.setGlobalContext(null);
      UI.removeChild_(body, bg);
      UI.removeChild_(body, win);
      if (waiting) {
        body.classList.add('waiting');
      }
    };
  }

  async getCurrentPassword() {
    return this.prompt({message: _T('enter-current-password'), getValue: true, password: true});
  }

  commonPopup_(params) {
    const EL = params.EL || new EventListeners();
    const popup = UI.create('div', {className:params.className || 'popup'});
    const popupHeader = UI.create('div', {className:'popup-header', parent:popup});
    const popupName = UI.create('div', {className:'popup-name', text:params.title || 'Title', parent:popupHeader});
    const popupInfo = UI.create('div', {className:'popup-info', text:'ⓘ', tabindex:'0', role:'button', title:_T('info')});
    const popupClose = UI.create('div', {className:'popup-close', text:'✖', tabindex:'0', role:'button', title:_T('close')});
    const popupContent = UI.create('div', {className:'popup-content', parent:popup});

    const body = document.body;

    if (!this.popupBlur_ || this.popupBlur_.depth <= 0) {
      this.popupBlur_ = {
        depth: 0,
        elem: UI.create('div', {className:'blur', parent:body}),
      };
    }
    this.popupBlur_.depth++;

    if (params.content) {
      popupContent.appendChild(params.content);
    }
    if (params.showInfo) {
      popupHeader.appendChild(popupInfo);
    }
    popupHeader.appendChild(popupClose);

    const handleClickClose = () => {
      closePopup();
    };
    const handleEscape = e => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        closePopup();
      }
    };
    const handleClickOutside = e => {
      if (params.disableClickOutsideClose !== true && !e.composedPath().includes(popup)) {
        e.stopPropagation();
        closePopup();
      }
    };
    // Add handlers.
    EL.add(popupClose, 'click', handleClickClose);
    EL.add(popupClose, 'keydown', e => {
      if (e.key === 'Enter') {
        handleClickClose(e);
      }
    });
    setTimeout(() => {
      const handlers = params.handlers || [];
      handlers.push(['keydown', handleEscape, true]);
      handlers.push(['click', handleClickOutside, true]);
      this.setGlobalContext(handlers, popup);
    });

    let closed = false;
    const closePopup = opt_slide => {
      if (closed) return;
      closed = true;
      // Remove handlers.
      this.setGlobalContext(null);
      popup.style.width = '' + popup.offsetWidth + 'px';
      switch (opt_slide) {
        case 'left':
          popup.style.animation = '0.5s linear slideOutLeft';
          break;
        case 'right':
          popup.style.animation = '0.5s linear slideOutRight';
          break;
        default:
          popup.style.animation = '0.25s linear fadeOut';
          break;
      }
      popup.addEventListener('animationend', () => {
        UI.removeChild_(body, popup);
        if (--this.popupBlur_.depth <= 0) {
          const elem = this.popupBlur_.elem;
          this.popupBlur_.elem = null;
          elem.style.animation = 'fadeOut 0.25s';
          elem.addEventListener('animationend', () => {
            UI.removeChild_(body, elem);
          }, {once: true});
        }
      }, {once: true});
      EL.clear();
      body.classList.remove('noscroll');
      if (params.onclose) {
        params.onclose();
      }
    };
    body.classList.add('noscroll');
    body.appendChild(popup);
    popup.style.width = '' + popup.offsetWidth + 'px';
    switch (params.slide) {
      case 'left':
        popup.style.animation = '0.5s linear slideInLeft';
        break;
      case 'right':
        popup.style.animation = '0.5s linear slideInRight';
        break;
      default:
        popup.style.animation = '0.25s linear fadeIn';
        break;
    }
    popup.addEventListener('animationend', () => {
      popup.style.width = '';
      popup.style.animation = '';
    }, {once: true});
    return {popup: popup, content: popupContent, close: closePopup, info: popupInfo, EL: EL};
  }

  preload_(url) {
    if (!this.recentImages_) {
      this.recentImages_ = {
        byUrl: {},
        byTime: [],
      };
    }
    if (this.recentImages_.byUrl[url]) {
      if (this.recentImages_.byTime.length > 1) {
        const i = this.recentImages_.byTime.indexOf(url);
        this.recentImages_.byTime.splice(i, 1);
        this.recentImages_.byTime.push(url);
      }
      return this.recentImages_.byUrl[url];
    }
    const img = new Image();
    img.decoding = 'async';
    img.src = url;
    img.readyp = img.decode()
      .then(() => true)
      .catch(() => {
        delete this.recentImages_.byUrl[url];
        return false;
      });
    if (this.recentImages_.byTime.length >= 10) {
      const u = this.recentImages_.byTime.shift();
      delete this.recentImages_.byUrl[u];
    }
    this.recentImages_.byTime.push(url);
    this.recentImages_.byUrl[url] = img;
    return img;
  }

  async setUpPopup_(i, opt_slide) {
    const f = this.galleryState_.content.files[i];
    const max = await this.maybeGetMoreFiles_(i+2);
    const goLeft = () => {
      close('right');
      if (i > 0) {
        this.setUpPopup_(i-1, 'right');
      }
    };
    const goRight = () => {
     close('left');
     if (i+1 < max) {
        this.setUpPopup_(i+1, 'left');
      }
    };
    const onkeydown = e => {
      if (e.key === 'F' || e.key === 'f') {
        e.stopPropagation();
        if (img) img.requestFullscreen();
      } else if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
        e.stopPropagation();
        goLeft();
      } else if (e.key === 'ArrowRight' || e.key === 'ArrowDown') {
        e.stopPropagation();
        goRight();
      }
    };
    let touchX;
    let touchY;
    const ontouchstart = event => {
      if (!event.changedTouches.length) {
        return;
      }
      touchX = event.changedTouches[0].clientX;
      touchY = event.changedTouches[0].clientY;
    };
    const ontouchmove = event => {
      if (!touchX || !touchY || !event.changedTouches.length) {
        return;
      }
      const dx = event.changedTouches[0].clientX - touchX;
      const dy = event.changedTouches[0].clientY - touchY;
      if (Math.abs(dx) > Math.abs(dy)) {
        event.stopPropagation();
        event.preventDefault();
        if (Math.abs(dx) > Math.min(content.offsetWidth / 4, 200)) {
          touchX = null;
          touchY = null;
          if (dx < 0) goRight();
          if (dx > 0) goLeft();
        }
      }
    };
    const params = {
      title: f.fileName,
      showInfo: true,
      slide: opt_slide,
      handlers: [
        ['keydown', onkeydown, true],
        ['touchstart', ontouchstart, {capture:true,passive:false}],
        ['touchmove', ontouchmove, {capture:true,passive:false}],
      ],
      onclose: () => {
        f.elem.focus();
      },
    };
    for (let j = i - 1; j <= i + 1; j++) {
      if (j >= 0 && j < max && this.galleryState_.content.files[j].isImage) {
        this.preload_(this.galleryState_.content.files[j].url);
      }
    }
    let img;
    if (f.isImage) {
      img = this.preload_(f.url);
      img.readyp.then(ok => {
        if (ok) return;
        img.onload = () => {
          const ratio = img.naturalHeight / img.naturalWidth;
          if (ratio > 1.0) {
            const h = Math.floor(0.75 * Math.min(window.innerWidth * ratio, window.innerHeight));
            img.style.height = `${h}px`;
          } else if (ratio !== 0) {
            const w = Math.floor(0.75 * Math.min(window.innerHeight / ratio, window.innerWidth));
            img.style.width = `${w}px`;
          }
        };
        img.src = f.thumbUrl;
        UI.create('div', {className:'offline-overlay',text:_T('network-error'),parent:content});
      });
      img.className = 'popup-media';
      img.alt = f.fileName;
      params.content = img;
    } else if (f.isVideo) {
      params.content = UI.create('video', {
        className: 'popup-media',
        src: f.url,
        poster: f.thumbUrl,
        controls: 'controls'
      });
    } else if (f.contentType.startsWith('audio/')) {
      params.content = UI.create('audio', {
        className: 'popup-media',
        src: f.url,
        controls: 'controls',
      });
    } else if (f.contentType.startsWith('application/')) {
      params.content = UI.create('a', {
        href: f.url,
        text: _T('download-doc'),
      });
    } else {
      params.content = UI.create('iframe', {
        src: f.url,
        title: f.fileName,
        sandbox: 'allow-downloads allow-same-origin',
      });
    }
    const {EL, content, close, info} = this.commonPopup_(params);
    content.draggable = false;
    EL.add(content, 'dragstart', event => {
      event.stopPropagation();
      event.preventDefault();
    }, true);
    if (i > 0) {
      const leftButton = UI.create('div', {className:'arrow left',text:'⬅️',tabindex:"0",title:_T('previous'),role:'button',parent:content});
      EL.add(leftButton, 'click', goLeft);
      EL.add(leftButton, 'keydown', e => {
        if (e.key === 'Enter') {
          goLeft(event);
        }
      });
    }
    if (i+1 < max) {
      const rightButton = UI.create('div', {className:'arrow right',text:'➡️',tabindex:"0",title:_T('next'),role:'button',parent:content});
      EL.add(rightButton, 'click', goRight);
      EL.add(rightButton, 'keydown', e => {
        if (e.key === 'Enter') {
          goRight(event);
        }
      });
    }
    if (f.isImage) {
      content.classList.add('image-popup');
      let exifData;
      const onclick = () => {
        if (exifData === undefined) {
          EXIF.load(f.url, {length: 128 * 1024, includeUnknown: true})
          .then(v => {
            exifData = v;
            const div = UI.create('div', {className:'exif-data', parent:content});
            div.style.maxHeight = '' + Math.floor(0.9*content.offsetHeight) + 'px';
            div.style.maxWidth = '' + Math.floor(0.9*content.offsetWidth) + 'px';
            this.formatExif_(div, exifData, EL);
          });
        } else {
          const e = content.querySelector('.exif-data');
          if (e) e.classList.toggle('hidden');
        }
      };
      EL.add(info, 'click', onclick);
      EL.add(info, 'keydown', e => {
        if (e.key === 'Enter') {
          onclick();
        }
      });
    }
    if (!f.isImage && !f.isVideo) {
      content.classList.add('popup-download');
    }
  }

  formatExif_(div, data, EL) {
    delete data.MakerNote;
    delete data.Thumbnail;
    if (Object.keys(data).length === 0) {
      div.textContent = '∅';
      return;
    }
    const out = [];
    for (let key of Object.keys(data).sort()) {
      out.push(`${key}: ${data[key].description} (${JSON.stringify(data[key].value)})`);
    }
    UI.create('div', {text: data.Make ? `${data.Make.value} ${data.Model.value}` : '', parent: div});
    const pos = UI.create('div', {parent:div});
    if (data.GPSLatitudeRef) {
      const lat = `${data.GPSLatitudeRef.value} ${data.GPSLatitude.description}°`;
      const lon = `${data.GPSLongitudeRef.value} ${data.GPSLongitude.description}°`;
      pos.textContent = `${lat} ${lon}`;
    }
    const more = UI.create('div', {className:'exif-more-details',text: '➕', tabindex:'0', role:'button', title:_T('expand'), parent:div});
    let expanded = false;
    const onclick = () => {
      details.classList.toggle('hidden');
      expanded = !expanded;
      more.textContent = expanded ? '➖' : '➕';
    };
    EL.add(more, 'click', onclick);
    EL.add(more, 'keydown', e => {
      if (e.key === 'Enter') {
        onclick();
      }
    });
    const details = UI.create('div', {className:'exif-details hidden', text: out.join('\n'), parent: div});
  }

  async showEdit_(f) {
    if (!f.isImage) {
      return;
    }
    await this.loadFilerobot_();
    const {content, close} = this.commonPopup_({
      title: _T('edit:', f.fileName),
      className: 'popup photo-editor-popup',
      disableClickOutsideClose: true,
      onclose: () => editor.terminate(),
    });

    const editor = new FilerobotImageEditor(content, {
      source: f.url,
      onSave: (img, state) => {
        console.log('saving', img.fullName);
        const binary = this.#main.base64DecodeToBytes(img.imageBase64.split(',')[1]);
        const blob = new Blob([binary], { type: img.mimeType });
        this.makeThumbnail_(blob)
        .then(([data, duration]) => {
          const up = [{
            file: blob,
            thumbnail: data,
            duration: duration,
            name: img.fullName,
            dateCreated: f.dateCreated,
            dateModified: Date.now(),
          }];
          return this.#main.sendRPC('upload', f.collection, up);
        })
        .then(() => {
          close();
          this.refresh_();
        })
        .catch(e => {
          this.showError_(e);
        });
      },
      tabsIds: ['Adjust', 'Annotate', 'Filters', 'Finetune', 'Resize'],
      defaultTabId: 'Adjust',
      defaultToolId: 'Crop',
      useBackendTranslations: false,
    });
    editor.render();
  }

  async collectionProperties_(c) {
    if (!c) {
      c = {
        create: true,
        name: '',
        members: [],
        isOwner: true,
        isShared: false,
      };
    }
    let cover = null;
    if (c.collection) {
      cover = await this.#main.sendRPC('getCover', c.collection);
    }
    const contacts = await this.#main.sendRPC('getContacts');
    const {EL, content, close} = this.commonPopup_({
      title: _T('properties:', c.name !== '' ? c.name : _T('new-collection')),
    });
    content.id = 'collection-properties';

    const origMembers = c.members.filter(m => !m.myself);
    UI.sortBy(origMembers, 'email');
    let members = c.members.filter(m => !m.myself);

    const getChanges = () => {
      const changes = {};
      if (c.isOwner) {
        const currCode = cover ? cover.code : '';
        const v = coverSelect.options[coverSelect.options.selectedIndex].value;
        if (currCode !== v) {
          changes.coverCode = v;
        }
        const newName = name.value;
        if (c.name !== newName) {
          changes.name = newName;
        }
        const newShared = shared.checked;
        if (c.isShared !== newShared) {
          changes.shared = newShared;
        }
        if (!newShared) {
          return changes;
        }
        const newCanAdd = permAdd.checked;
        if (c.canAdd !== newCanAdd) {
          changes.canAdd = newCanAdd;
        }
        const newCanCopy = permCopy.checked;
        if (c.canCopy !== newCanCopy) {
          changes.canCopy = newCanCopy;
        }
        const newCanShare = permShare.checked;
        if (c.canShare !== newCanShare) {
          changes.canShare = newCanShare;
        }
      }
      if (c.isOwner || c.canShare) {
        const a = new Set(origMembers.map(m => m.userId));
        const b = new Set(members.map(m => m.userId));
        changes.remove = [...a].filter(m => !b.has(m));
        changes.add = [...b].filter(m => !a.has(m));
        if (changes.remove.length === 0) delete changes.remove;
        if (changes.add.length === 0) delete changes.add;
      }
      return changes;
    };

    const onChange = () => {
      if (shared) {
        content.querySelectorAll('.sharing-setting').forEach(elem => {
          if (shared.checked) {
            elem.style.display = '';
          } else {
            elem.style.display = 'none';
          }
        });
      }
      const any = Object.keys(getChanges()).length > 0;
      applyButton.disabled = !any;
      applyButton.textContent = any ? _T('apply-changes') : _T('no-changes');
    };

    const applyChanges = async () => {
      const changes = getChanges();
      if (c.create) {
        if (changes.name === undefined) {
          return false;
        }
        c.collection = await this.#main.sendRPC('createCollection', changes.name.trim());
      } else if (changes.name !== undefined) {
        await this.#main.sendRPC('renameCollection', c.collection, changes.name.trim());
      }

      const perms = {
        canAdd: c.isOwner ? permAdd.checked : c.canAdd,
        canCopy: c.isOwner ? permCopy.checked : c.canCopy,
        canShare: c.isOwner ? permShare.checked : c.canShare,
      };

      if (changes.shared === true || changes.add !== undefined) {
        await this.#main.sendRPC('shareCollection', c.collection, perms, changes.add || []);
      }

      if (changes.shared === false) {
        await this.#main.sendRPC('unshareCollection', c.collection);
      }

      if (changes.remove !== undefined) {
        await this.#main.sendRPC('removeMembers', c.collection, changes.remove);
      }

      if (changes.canAdd !== undefined || changes.canCopy !== undefined || changes.canShare !== undefined) {
        await this.#main.sendRPC('updatePermissions', c.collection, perms);
      }

      if (changes.coverCode !== undefined) {
        await this.#main.sendRPC('changeCover', c.collection, changes.coverCode);
      }
      close();
      this.getUpdates_()
      .then(() => {
        this.switchView(c);
      });
    };

    if (!c.create) {
      const deleteButton = UI.create('button', {id:'collection-properties-delete',text:'🗑',parent:content});
      EL.add(deleteButton, 'click', () => {
        if (c.isOwner) {
          this.deleteCollection_(c.collection).then(() => close());
        } else {
          this.leaveCollection_(c.collection).then(() => close());
        }
      });
    }
    const coverLabel = UI.create('div', {id:'collection-properties-cover-label',text:_T('cover'),parent:content});

    const coverInput = UI.create('div');
    const imgDiv = UI.create('div', {id:'collection-properties-thumbdiv',draggable:false,parent:coverInput});
    EL.add(imgDiv, 'dragstart', event => {
      event.stopPropagation();
      event.preventDefault();
    }, true);
    const sz = UI.px_(150);
    if (cover) {
      imgDiv.style.width = sz;
      imgDiv.style.height = sz;
    }

    const setImage = url => {
      if (c.create) {
        return;
      }
      UI.clearElement_(imgDiv);
      const img = new Image();
      img.id = 'collection-properties-thumb';
      img.draggable = false;
      img.src = url ? url : 'clear.png';
      img.style.height = sz;
      imgDiv.appendChild(img);
    };
    setImage(cover?.url);

    let coverSelect;
    if (c.isOwner) {
      coverSelect = UI.create('select', {id:'collection-properties-cover'});
      const opts = [
        {code:'', label:_T('cover-latest')},
        {code:'__b__', label:_T('cover-blank')},
      ];
      if (cover !== null && cover.code !== '' && cover.code !== '__b__') {
        opts.push({code:cover.code, label:_T('cover-selected')});
      }
      let currCode = cover ? cover.code : '';
      for (let opt of opts) {
        UI.create('option', {value:opt.code,text:opt.label,selected:currCode === opt.code,parent:coverSelect});
      }
      EL.add(coverSelect, 'change', () => {
        if (c.collection) {
          this.#main.sendRPC('getCover', c.collection, coverSelect.options[coverSelect.options.selectedIndex].value)
          .then(cover => setImage(cover.url));
          onChange();
        }
      });
      coverInput.appendChild(coverSelect);
    }
    content.appendChild(coverInput);

    const nameLabel = UI.create('div', {id:'collection-properties-name-label', text:_T('name'), parent:content});

    let name;
    if (c.isOwner) {
      name = UI.create('input', {id:'collection-properties-name', type:'text', value:c.name, parent:content});
      EL.add(name, 'keydown', onChange);
      if (c.create) name.focus();
    } else {
      name = UI.create('div', {id:'collection-properties-name', text:c.name, parent:content});
    }

    const sharedLabel = UI.create('div', {id:'collection-properties-shared-label', text:_T('shared'), parent:content});

    let shared;
    if (c.isOwner) {
      shared = UI.create('input', {id:'collection-properties-shared', type:'checkbox', checked:c.isShared, parent:content});
      EL.add(shared, 'change', onChange);
    } else {
      const sharedDiv = UI.create('div', {id:'collection-properties-shared-div', text:c.isShared ? _T('yes') : _T('no'), parent:content});
    }

    const permLabel = UI.create('div', {id:'collection-properties-perm-label',className:'sharing-setting',text:_T('permissions'), parent:content});
    permLabel.style.display = c.isShared ? '' : 'none';

    const permDiv = UI.create('div', {id:'collection-properties-perm',className:'sharing-setting',parent:content});
    permDiv.style.display = c.isShared ? '' : 'none';

    const permAdd = UI.create('input', {id:'collection-properties-perm-add', type:'checkbox', checked:c.canAdd, disabled:!c.isOwner, parent:permDiv});
    EL.add(permAdd, 'change', onChange);
    const permAddLabel = UI.create('label', {text:_T('perm-add'),htmlFor:'collection-properties-perm-add', parent:permDiv});

    const permCopy = UI.create('input', {id:'collection-properties-perm-copy', type:'checkbox', checked:c.canCopy, disabled:!c.isOwner, parent:permDiv});
    EL.add(permCopy, 'change', onChange);
    const permCopyLabel = UI.create('label', {text:_T('perm-copy'),htmlFor:'collection-properties-perm-copy',parent:permDiv});

    const permShare = UI.create('input', {id:'collection-properties-perm-share', type:'checkbox', checked:c.canShare, disabled:!c.isOwner, parent:permDiv});
    EL.add(permShare, 'change', onChange);
    const permShareLabel = UI.create('label', {text:_T('perm-share'),htmlFor:'collection-properties-perm-share', parent:permDiv});

    const membersLabel = UI.create('div', {id:'collection-properties-members-label', className:'sharing-setting', text:_T('members'), parent: content});
    membersLabel.style.display = c.isShared ? '' : 'none';

    const membersDiv = UI.create('div', {id:'collection-properties-members', className:'sharing-setting', parent:content});
    membersDiv.style.display = c.isShared ? '' : 'none';

    const applyButton = UI.create('button', {id:'collection-properties-apply-button', text:_T('no-changes'), disabled: true, parent:content});
    EL.add(applyButton, 'click', applyChanges);

    const deleteMember = i => {
      UI.removeChild_(membersDiv, members[i].elem);
      delete members[i].elem;
      members.splice(i, 1);
      refreshMembers();
      onChange();
    };

    const refreshMembers = () => {
      UI.clearElement_(membersDiv);
      UI.sortBy(members, 'email');
      if (c.isOwner || c.canShare) {
        const list = UI.create('datalist', {id:'collection-properties-members-contacts', parent:membersDiv});
        for (let i = 0; i < contacts.length; i++) {
          if (members.some(m => m.userId === contacts[i].userId)) {
            continue;
          }
          UI.create('option', {value:contacts[i].email, parent:list});
        }
        membersDiv.appendChild(list);

        const input = UI.create('input', {id:'collection-properties-members-input', type:'search', placeholder:_T('contact-email'), list:'collection-properties-members-contacts', parent:membersDiv});

        const addButton = UI.create('button', {id:'collection-properties-members-add-button', text:_T('add-member'), parent:membersDiv});
        const addFunc = () => {
          const c = contacts.find(e => e.email === input.value);
          if (c) {
            input.value = '';
            members.push(c);
            refreshMembers();
            onChange();
            return;
          }
          addButton.disabled = true;
          input.readonly = true;
          this.#main.sendRPC('getContact', input.value)
          .then(cc => {
            input.value = '';
            contacts.push(cc);
            UI.sortBy(contacts, 'email');
            members.push({userId: cc.userId, email: cc.email});
            refreshMembers();
            onChange();
          })
          .catch(err => {
            if (err !== 'nok') {
              this.popupMessage(err);
            }
          })
          .finally(() => {
            addButton.disabled = false;
            input.readonly = false;
          });
        };
        EL.add(input, 'keydown', e => {
          if (e.key === 'Enter') {
            e.stopPropagation();
            addFunc();
          }
        });
        EL.add(input, 'change', addFunc);
        EL.add(addButton, 'click', addFunc);
      }
      if (members.length === 0) {
        UI.create('div', {html:'<i>'+_T('none')+'</i>', parent:membersDiv});
      }
      for (let i = 0; i < members.length; i++) {
        const div = UI.create('div', {parent:membersDiv});
        if (c.isOwner) {
          const del = UI.create('button', {text:'✖', parent:div});
          del.style.cursor = 'pointer';
          EL.add(del, 'click', () => deleteMember(i));
        }
        const name = UI.create('span', {text:members[i].email,className:'email',parent:div});
        members[i].elem = div;
      }
    };
    refreshMembers();
  }

  formatDuration_(d) {
    const min = Math.floor(d / 60);
    const sec = d % 60;
    return '' + min + ':' + ('00'+sec).slice(-2);
  }

  formatSize_(s) {
    if (s >= 1024*1024*1024) return _T('GiB', Math.floor(s * 100 / 1024 / 1024 / 1024) / 100);
    if (s >= 1024*1024) return _T('MiB', Math.floor(s * 100 / 1024 / 1024) / 100);
    if (s >= 1024) return _T('KiB', Math.floor(s * 100 / 1024) / 100);
    return _T('B', s);
  }

  formatSizeMB_(s) {
    if (s >= 1024*1024) return _T('TiB', Math.floor(s * 100 / 1024 / 1024) / 100);
    if (s >= 1024) return _T('GiB', Math.floor(s * 100 / 1024) / 100);
    return _T('MiB', s);
  }

  showThumbnailProgress_() {
    const sz = (this.thumbnailQueue_ ? this.thumbnailQueue_.length : 0 ) + (this.thumbnailQueueNumWorkers_ ? this.thumbnailQueueNumWorkers_ : 0);
    const info = _T('thumbnail-progress', sz);
    const e = document.querySelector('#thumbnail-progress-data');
    if (e) {
      e.textContent = info;
      if (sz === 0 && e.remove) {
        setTimeout(e.remove, 2000);
      }
    } else {
      const msg = UI.create('div', {className:'progress-div'});
      const span = UI.create('span', {id:'thumbnail-progress-data', text:info, parent:msg});
      const r = this.popupMessage(msg, 'progress', {sticky: sz > 0});
      if (sz > 0) {
        span.remove = () => {
          r();
          delete span.remove;
        };
      }
    }
  }

  showUploadProgress(progress) {
    const EL = new EventListeners();
    const info = _T('upload-progress', `${progress.numFilesDone}/${progress.numFiles} [${Math.floor(progress.numBytesDone / progress.numBytes * 100)}%]`);
    const e = document.querySelector('#upload-progress-data');
    if (e) {
      e.textContent = info;
      if (progress.done || progress.err) {
        document.querySelector('#upload-progress-cancel-button').style.display = 'none';
        e.clear = setTimeout(e.remove, 2000);
      } else if (e.clear) {
        clearTimeout(e.clear);
        delete e.clear;
      }
    } else {
      const msg = UI.create('div', {className:'progress-div'});
      const span = UI.create('span', {id:'upload-progress-data', text:info, parent:msg});
      const button = UI.create('button', {id:'upload-progress-cancel-button', text: _T('cancel'), parent:msg});
      EL.add(button, 'click', () => {
        button.disabled = true;
        this.cancelDropUploads_();
        this.#main.sendRPC('cancelUpload');
      });
      const r = this.popupMessage(msg, 'progress', {sticky: !progress.done});
      if (!progress.done) {
        span.remove = () => {
          r();
          EL.clear();
          delete span.clear;
          delete span.remove;
        };
      }
    }
  }

  showDownloadProgress(progress) {
    let info = _T('download-progress', `${progress.count}/${progress.total}`);
    if (progress.err) {
      info = progress.err;
    }
    const e = document.querySelector('#download-progress-data');
    if (e) {
      if (progress.err || progress.total > 0) {
        e.textContent = info;
      }
      if (progress.done) {
        e.clear = setTimeout(e.remove, 2000);
      } else if (e.clear) {
        clearTimeout(e.clear);
        delete e.clear;
      }
    } else {
      const msg = UI.create('div', {className:'progress-div'});
      const span = UI.create('span', {id:'download-progress-data', text:info, parent:msg});
      const r = this.popupMessage(msg, 'progress', {sticky: !progress.done});
      if (!progress.done) {
        span.remove = () => {
          r();
          delete span.clear;
          delete span.remove;
        };
      }
    }
  }

  async showUploadView_() {
    let collectionName = '';
    let members = [];

    for (let i in this.collections_) {
      if (!this.collections_.hasOwnProperty(i)) {
        continue;
      }
      const c = this.collections_[i];
      if (this.galleryState_.collection === c.collection) {
        collectionName = c.name;
        members = c.members;
        break;
      }
    }
    const {EL, popup, content, close} = this.commonPopup_({
      title: _T('upload:', collectionName),
      className: 'popup upload',
    });

    UI.create('h1', {text:_T('collection:', collectionName),parent:content});
    if (members?.length > 0) {
      UI.sortBy(members, 'email');
      UI.create('div', {text:_T('shared-with', members.map(m => m.email).join(', ')), parent:content});
    }

    const list = UI.create('div', {id:'upload-file-list', parent:content});

    let files = [];
    const processFiles = newFiles => {
      let p = [];
      for (let i = 0; i < newFiles.length; i++) {
        const f = newFiles[i];
        const elem = UI.create('div', {className:'upload-item-div'});
        const img = new Image();
        img.src = 'clear.png';
        img.className = 'upload-thumbnail';
        elem.appendChild(img);
        const div = UI.create('div', {className:'upload-item-attrs', parent:elem});
        UI.create('span', {text:_T('name:', f.name), parent:div});
        UI.create('span', {text:_T('size:', this.formatSize_(f.size)), parent:div});
        const errSpan = UI.create('span', {text:_T('status:', '...'), parent:div});
        const removeButton = UI.create('button', {className:'upload-item-remove-button', disabled:true, text:_T('remove'), parent:div});
        EL.add(removeButton, 'click', () => {
          files = files.filter(f => f.elem !== elem);
          processFiles([]);
        });
        div.appendChild(removeButton);
        const ff = {
          file: f,
          elem: elem,
        };
        files.push(ff);
        p.push(this.makeThumbnail_(f)
          .then(([data,duration]) => {
            img.src = data;
            ff.thumbnail = data;
            ff.duration = duration;
            errSpan.textContent = _T('status:', _T('ready'));
          })
          .catch(err => {
            console.log('Thumbnail error', err);
            errSpan.textContent = _T('status:', _T('error'));
            ff.err = err;
            return Promise.reject(err);
          })
          .finally(() => {
            removeButton.disabled = false;
          })
        );
      }
      const list = document.querySelector('#upload-file-list');
      UI.clearElement_(list);
      if (files.length > 0) {
        const uploadButton = UI.create('button', {className:'upload-file-list-upload-button', text:_T('upload'), disabled:true, parent:list});
        EL.add(uploadButton, 'click', () => {
          let toUpload = [];
          for (let i = 0; i < files.length; i++) {
            if (files[i].err) {
              continue;
            }
            toUpload.push({
              file: files[i].file,
              thumbnail: files[i].thumbnail,
              duration: files[i].duration,
            });
          }
          uploadButton.disabled = true;
          uploadButton.textContent = _T('uploading');
          this.#main.sendRPC('upload', this.galleryState_.collection, toUpload)
          .then(() => {
            close();
            this.refresh_();
          })
          .catch(e => {
            this.showError_(e);
          })
          .finally(() => {
            uploadButton.disabled = false;
            uploadButton.textContent = _T('upload');
          });
        });
        Promise.allSettled(p).then(() => {
          uploadButton.disabled = false;
        });
      }
      for (let i = 0; i < files.length; i++) {
        const f = files[i];
        list.appendChild(f.elem);
      }
    };
    const fileInputs = UI.create('div', {id:'upload-files-div', parent:content});

    UI.create('label', {forHtml:'files', text:_T('select-upload'), parent:fileInputs});
    const input = UI.create('input', {id:'upload-file-input', type:'file', name:'files', multiple:true, parent:fileInputs});
    EL.add(input, 'change', e => {
      processFiles(e.target.files);
    });

    EL.add(popup, 'drop', e => {
      e.preventDefault();
      e.stopPropagation();
      let files = [];
      if (e.dataTransfer.items) {
        for (let i = 0; i < e.dataTransfer.items.length; i++) {
          if (e.dataTransfer.items[i].kind === 'file') {
            files.push(e.dataTransfer.items[i].getAsFile());
          }
        }
      } else {
        for (let i = 0; i < e.dataTransfer.files.length; i++) {
          files.push(e.dataTransfer.files[i]);
        }
      }
      processFiles(files);
    });
    EL.add(popup, 'dragover', e => {
      e.preventDefault();
    });
  }

  async makeThumbnail_(file) {
    return new Promise((resolve, reject) => {
      if (!this.thumbnailQueue_) {
        this.thumbnailQueue_ = [];
        this.thumbnailQueueNumWorkers_ = 0;
      }
      this.thumbnailQueue_.push({file, resolve, reject});
      this.showThumbnailProgress_();
      if (this.thumbnailQueueNumWorkers_ < 10) {
        this.thumbnailQueueNumWorkers_ += 1;
        this.processThumbnailQueue_();
      }
    });
  }

  async cancelQueuedThumbnailRequests_() {
    while (this.thumbnailQueue_.length > 0) {
      const item = this.thumbnailQueue_.shift();
      item.reject('canceled');
    }
  }

  async processThumbnailQueue_() {
    try {
      while (this.thumbnailQueue_.length > 0) {
        const item = this.thumbnailQueue_.shift();
        await this.makeThumbnailNow_(item.file)
          .then(item.resolve)
          .catch(err => {
            console.log('Thumbnail error, trying generic image', err);
            item.resolve(this.makeGenericThumbnail_(item.file));
          });
        this.showThumbnailProgress_();
      }
    } catch (err) {
      console.error('processThumbnailQueue_', err);
    }
    this.thumbnailQueueNumWorkers_ -= 1;
    this.showThumbnailProgress_();
  }

  async makeThumbnailNow_(file) {
    const canvas = document.createElement("canvas", {willReadFrequently: true});
    const ctx = canvas.getContext('2d');
    if (file.type.startsWith('image/')) {
      return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => {
          const img = new Image();
          img.onload = () => {
            if (img.width > img.height) {
              canvas.width = 320;
              canvas.height = 240;
            } else {
              canvas.width = 240;
              canvas.height = 320;
            }
            let sx = 0;
            let sy = 0;
            let sw = img.width;
            let sh = img.height;
            if (sw / sh > canvas.width / canvas.height) {
              sw = Math.floor(canvas.width / canvas.height * sh);
              sx = Math.floor((img.width - sw) / 2);
            } else {
              sh = Math.floor(canvas.height / canvas.width * sw);
              sy = Math.floor((img.height - sh) / 2);
            }
            ctx.drawImage(img, sx, sy, sw, sh, 0, 0, canvas.width, canvas.height);
            return resolve([canvas.toDataURL('image/png'),0]);
          };
          img.onerror = err => reject(err);
          try {
            img.src = reader.result;
          } catch (err) {
            reject(err);
          }
        };
        reader.onerror = err => reject(err);
        reader.readAsDataURL(file);
      });
    } else if (file.type.startsWith('video/')) {
      return new Promise(resolve => {
        const video = document.createElement('video');
        video.muted = true;
        video.src = URL.createObjectURL(file);
        video.addEventListener('loadeddata', () => {
          setTimeout(() => {
            video.currentTime = Math.floor(Math.min(video.duration/2, 5));
          }, 100);
          video.addEventListener('seeked', () => {
            if (video.videoWidth > video.videoHeight) {
              canvas.width = 320;
              canvas.height = 240;
            } else {
              canvas.width = 240;
              canvas.height = 320;
            }
            let sx = 0;
            let sy = 0;
            let sw = video.videoWidth;
            let sh = video.videoHeight;
            if (sw / sh > canvas.width / canvas.height) {
              sw = Math.floor(canvas.width / canvas.height * sh);
              sx = Math.floor((video.videoWidth - sw) / 2);
            } else {
              sh = Math.floor(canvas.height / canvas.width * sw);
              sy = Math.floor((video.videoHeight - sh) / 2);
            }
            ctx.drawImage(video, sx, sy, sw, sh, 0, 0, canvas.width, canvas.height);
            video.pause();
            return resolve([canvas.toDataURL('image/png'),video.duration]);
          }, {once: true});
        }, {once: true});
      });
    } else {
      return this.makeGenericThumbnail_(file);
    }
  }

  makeGenericThumbnail_(file) {
    const canvas = document.createElement("canvas", {willReadFrequently: true});
    const ctx = canvas.getContext('2d');
    canvas.width = 100;
    canvas.height = 100;
    ctx.font = '10px monospace';
    let name = file.name;
    let row = 1;
    while (name !== '') {
      const n = name.length > 12 ? name.substring(0, 12) : name;
      name = name.slice(n.length);
      ctx.fillText(n, 10, 10 * row);
      row++;
    }
    return [canvas.toDataURL('image/png'), 0];
  }

  async showProfile_() {
    const curr = await this.#main.sendRPC('mfaStatus');

    const {EL, content, close} = this.commonPopup_({
      title: _T('profile'),
      className: 'popup profile-popup',
    });
    content.id = 'profile-content';

    const onchange = () => {
      let changed = false;
      if (email.value !== this.accountEmail_) {
        changed = true;
      }
      if (newPass.value !== '' && newPass.value === newPass2.value) {
        changed = true;
      }
      if (mfa.checked !== curr.mfaEnabled) {
        changed = true;
      }
      if (passkey.checked !== curr.passKey) {
        changed = true;
      }
      if (otp.checked !== curr.otpEnabled && (!otp.checked || form.querySelector('#profile-form-otp-code').value !== '')) {
        changed = true;
      }
      for (let k of Object.keys(keyList)) {
        if (keyList[k].input.value !== keyList[k].key.name || keyList[k].deleted) {
          changed = true;
        }
      }
      newPass.placeholder = newPass.value !== '' || newPass2.value !== '' ? _T('required') : _T('optional');
      newPass2.placeholder = newPass.placeholder;
      button.disabled = !changed;
      button.textContent = changed ? _T('apply-changes') : _T('no-changes');
      addSkButton.textContent = passkey.checked ? _T('add-passkey') : _T('add-security-key');
    };

    const form = UI.create('div', {id:'profile-form'});

    UI.create('label', {forHtml:'profile-form-email', text:_T('form-email'), parent:form});
    const email = UI.create('input', {id:'profile-form-email', type:'email', value:this.accountEmail_, parent:form});
    EL.add(email, 'keydown', onchange);
    EL.add(email, 'change', onchange);

    UI.create('label', {forHtml:'profile-form-new-password', text:_T('form-new-password'), parent:form});
    const newPass = UI.create('input', {id:'profile-form-new-password', type:'password', placeholder:_T('optional'), autocomplete:'new-password', parent:form});
    EL.add(newPass, 'keydown', onchange);
    EL.add(newPass, 'change', onchange);

    UI.create('label', {forHtml:'profile-form-new-password2', text:_T('form-confirm-password'), parent:form});
    const newPass2 = UI.create('input', {id:'profile-form-new-password2', type:'password', placeholder:_T('optional'), autocomplete:'new-password', parent:form});
    EL.add(newPass2, 'keydown', onchange);
    EL.add(newPass2, 'change', onchange);

    UI.create('label', {forHtml:'profile-form-enable-mfa', text:_T('enable-mfa?'), parent:form});
    const mfaDiv = UI.create('div', {id:'profile-form-enable-mfa-div', parent:form});
    const mfa = UI.create('input', {id:'profile-form-enable-mfa', type:'checkbox', checked:curr.mfaEnabled, parent:mfaDiv});
    EL.add(mfa, 'change', () => {
      form.querySelectorAll('.hide-no-mfa').forEach(e => e.style.display = mfa.checked ? '' : 'none');
      onchange();
    });
    const testButton = UI.create('button', {id:'profile-form-test-mfa', className:'hide-no-mfa', text:_T('test'), parent:mfaDiv});
    EL.add(testButton, 'click', () => {
      testButton.disabled = true;
      this.#main.sendRPC('mfaCheck', passkey.checked).finally(() => {
        testButton.disabled = false;
      })
      .finally(() => {
        testButton.focus();
      });
    });

    let otpKey = '';
    UI.create('label', {className:'hide-no-mfa', forHtml:'profile-form-enable-otp', text:_T('enable-otp?'), parent:form});
    const otpDiv = UI.create('div', {id:'profile-form-enable-otp-div', className:'hide-no-mfa', parent:form});
    const otp = UI.create('input', {id:'profile-form-enable-otp', type:'checkbox', checked:curr.otpEnabled, parent:otpDiv});
    EL.add(otp, 'change', () => {
      if (otp.checked && !curr.otpEnabled) {
        otp.disabled = true;
        this.#main.sendRPC('generateOTP')
        .then(({key, img}) => {
          otpKey = key;
          const image = new Image();
          image.src = img;
          otpDiv.appendChild(image);
          const keyDiv = UI.create('div', {id:'profile-form-otp-key', text:'KEY: ' + key, parent:otpDiv});
          const code = UI.create('input', {id:'profile-form-otp-code', type:'text', placeholder:_T('enter-code'), parent:otpDiv});
          EL.add(code, 'keydown', onchange);
          EL.add(code, 'change', onchange);
        })
        .finally(() => {
          otp.disabled = false;
          onchange();
        });
      } else {
        otpKey = '';
        UI.clearElement_(otpDiv);
        otpDiv.appendChild(otp);
        onchange();
      }
    });

    UI.create('label', {className:'hide-no-mfa', forHtml:'profile-form-enable-passkey', text:_T('enable-passkey?'), parent:form});
    const passkeyDiv = UI.create('div', {id:'profile-form-enable-passkey-div', className:'hide-no-mfa', parent:form});
    const passkey = UI.create('input', {id:'profile-form-enable-passkey', className:'hide-no-mfa', type:'checkbox', checked:curr.passKey, parent:passkeyDiv});
    EL.add(passkey, 'change', () => {
      updateKeyList();
      onchange();
    });

    UI.create('label', {className:'hide-no-mfa', forHtml:'profile-form-add-security-key-button', text:_T('security-keys:'), parent:form});
    const skDiv = UI.create('div', {id:'profile-form-security-keys-div', className:'hide-no-mfa', parent:form});

    const addSkButton = UI.create('button', {id:'profile-form-add-security-key-button', text:passkey.checked ? _T('add-passkey') : _T('add-security-key'), parent:skDiv});
    EL.add(addSkButton, 'click', () => {
      addSkButton.disabled = true;
      this.getCurrentPassword()
      .then(pw => this.#main.addSecurityKey(pw, passkey.checked))
      .finally(() => {
        addSkButton.disabled = false;
        addSkButton.focus();
        updateKeyList();
      });
    });

    const skList = UI.create('div', {id:'profile-form-security-key-list', parent:skDiv});

    let keyList = {};
    const updateKeyList = () => {
      this.#main.sendRPC('listSecurityKeys')
      .then(keys => {
        UI.clearElement_(skList);
        keys = keys.filter(k => !passkey.checked || k.discoverable);
        keyList = {};
        if (keys.length > 0) {
          skList.innerHTML = `<div class="profile-form-security-key-list-header">${_T('name')}</div><div class="profile-form-security-key-list-header">${_T('added')}</div><div></div>`;
        }
        for (let k of keys) {
          let input = UI.create('input', {type:'text', className:'profile-form-security-key-list-item', value:k.name, parent:skList});
          EL.add(input, 'change', onchange);
          EL.add(input, 'keydown', onchange);
          let t = UI.create('div', {text:(new Date(k.createdAt)).toLocaleDateString(undefined, {year: 'numeric', month: 'short', day: 'numeric'}), parent:skList});
          let del = UI.create('button', {text:'✖', parent:skList});
          del.style.cursor = 'pointer';
          EL.add(del, 'click', () => {
            keyList[k.id].deleted = !keyList[k.id].deleted;
            input.disabled = keyList[k.id].deleted;
            input.classList.toggle('deleted-key');
            t.classList.toggle('deleted-key');
            del.classList.toggle('deleted-key');
            onchange();
          });
          keyList[k.id] = {key: k, input: input};
        }
      });
    };
    updateKeyList();

    const button = UI.create('button', {id:'profile-form-button', text:_T('no-changes'), disabled:true, parent:form});
    EL.add(button, 'click', async () => {
      if ((newPass.value !== '' || newPass2.value !== '') && newPass.value !== newPass2.value) {
        this.popupMessage(_T('new-pass-doesnt-match'));
        return;
      }
      const code = form.querySelector('#profile-form-otp-code');
      if (otp.checked && !curr.otpEnabled && !code?.value) {
        this.popupMessage(_T('otp-code-required'));
        return;
      }
      if (mfa.checked) {
        if (!otp.checked && Object.keys(keyList).filter(k => (!passkey.checked || keyList[k].key.discoverable) && !keyList[k].deleted).length === 0) {
          this.popupMessage(_T('otp-or-sk-required'));
          return;
        }
      }
      email.disabled = true;
      newPass.disabled = true;
      newPass2.disabled = true;
      mfa.disabled = true;
      otp.disabled = true;
      passkey.disabled = true;
      button.disabled = true;
      button.textContent = _T('updating');
      delButton.disabled = true;

      const keyChanges = [];
      for (let k of Object.keys(keyList)) {
        if (keyList[k].deleted) {
          keyChanges.push({id:k, deleted:true});
        } else if (keyList[k].input.value !== keyList[k].key.name) {
          keyChanges.push({id:k, name:keyList[k].input.value});
        }
      }
      this.getCurrentPassword()
      .then(pw => this.#main.sendRPC('updateProfile', {
        email: email.value,
        password: pw,
        newPassword: newPass.value,
        setMFA: mfa.checked,
        passKey: passkey.checked,
        setOTP: otp.checked,
        otpKey: otpKey,
        otpCode: code ? code.value : '',
        keyChanges: keyChanges,
      }))
      .then(() => {
        this.accountEmail_ = email.value;
        close();
      })
      .catch(err => {
        console.log('updateProfile error', err);
      })
      .finally(() => {
        email.disabled = false;
        newPass.disabled = false;
        newPass2.disabled = false;
        mfa.disabled = false;
        otp.disabled = false;
        passkey.disabled = false;
        button.disabled = false;
        button.textContent = _T('no-changes');
        delButton.disabled = false;
        onchange();
        button.focus();
      });
    });
    form.querySelectorAll('.hide-no-mfa').forEach(e => e.style.display = mfa.checked ? '' : 'none');
    content.appendChild(form);

    UI.create('div', {html:'<hr>' + _T('delete-warning'), parent:content});

    const delButton = UI.create('button', {id:'profile-form-delete-button', text:_T('delete-account'), parent:content});
    EL.add(delButton, 'click', () => {
      email.disabled = true;
      newPass.disabled = true;
      newPass2.disabled = true;
      button.disabled = true;
      delButton.disabled = true;
      this.prompt({message: _T('confirm-delete-account'), getValue: true, password: true})
      .then(pw => this.#main.sendRPC('deleteAccount', pw))
      .then(() => {
        window.localStorage.removeItem('_');
        window.localStorage.removeItem('salt');
        this.#main.lock();
      })
      .finally(() => {
        email.disabled = false;
        newPass.disabled = false;
        newPass2.disabled = false;
        button.disabled = false;
        delButton.disabled = false;
        onchange();
      });
    });
  }

  async showBackupPhrase_() {
    const {EL, content, close} = this.commonPopup_({
      title: _T('key-backup'),
    });
    content.id = 'backup-phrase-content';
    let keyBackupEnabled = await this.#main.sendRPC('keyBackupEnabled');

    const warning = UI.create('div', {id:'backup-phrase-warning', className:'warning', html:_T('key-backup-warning'), parent:content});
    const phrase = UI.create('div', {id:'backup-phrase-value', parent:content});

    const button = UI.create('button', {id:'backup-phrase-show-button', text:_T('show-backup-phrase'), parent:content});
    EL.add(button, 'click', () => {
      if (phrase.textContent === '') {
        button.disabled = true;
        button.textContent = _T('checking-password');
        this.getCurrentPassword()
        .then(pw => this.#main.sendRPC('backupPhrase', pw))
        .then(v => {
          phrase.textContent = v;
          button.textContent = _T('hide-backup-phrase');
        })
        .catch(err => {
          button.textContent = _T('show-backup-phrase');
          this.popupMessage(err);
        })
        .finally(() => {
          button.disabled = false;
        });
      } else {
        phrase.textContent = '';
        button.textContent = _T('show-backup-phrase');
      }
    });

    const warning2 = UI.create('div', {id:'backup-phrase-warning2', className:'warning', html:'<hr>' + _T('key-backup-warning2'), parent:content});

    const changeBackup = choice => {
      inputYes.disabled = true;
      inputNo.disabled = true;
      this.getCurrentPassword()
      .then(pw => this.#main.sendRPC('changeKeyBackup', pw, choice))
      .then(() => {
        keyBackupEnabled = choice;
        this.popupMessage(choice ? _T('enabled') : _T('disabled'), 'info');
      })
      .catch(err => {
        inputYes.checked = keyBackupEnabled;
        inputNo.checked = !keyBackupEnabled;
        this.popupMessage(err);
      })
      .finally(() => {
        inputYes.disabled = false;
        inputNo.disabled = false;
      });
    };
    const divYes = UI.create('div', {className:'key-backup-option', parent:content});
    const inputYes = UI.create('input', {id:'choose-key-backup-yes', type:'radio', name:'do-backup', checked:keyBackupEnabled, parent:divYes});
    EL.add(inputYes, 'change', () => changeBackup(true));
    UI.create('label', {htmlFor:'choose-key-backup-yes', text:_T('opt-keep-backup'), parent:divYes});

    const divNo = UI.create('div', {className:'key-backup-option', parent:content});
    const inputNo = UI.create('input', {id:'choose-key-backup-no', type:'radio', name:'do-backup', checked:!keyBackupEnabled, parent:divNo});
    EL.add(inputNo, 'change', () => changeBackup(false));
    const labelNo = UI.create('label', {htmlFor:'choose-key-backup-no', text:_T('opt-dont-keep-backup'), parent:divNo});
  }

  async showPreferences_() {
    const EL = new EventListeners();
    const content = UI.create('div', {id:'preferences-content'});
    const text = UI.create('div', {id:'preferences-cache-text', html:_T('choose-cache-pref'), parent:content});

    const current = await this.#main.sendRPC('cachePreference');
    const choices = [
      {
        value: 'encrypted',
        label: _T('opt-encrypted'),
      },
      {
        value: 'no-store',
        label: _T('opt-no-store'),
      },
      {
        value: 'private',
        label: _T('opt-private'),
      },
    ];

    const changeCachePref = (mode) => {
      const all = allthumbs.checked;
      const mob = mobile.checked;
      const size = Math.max(parseInt(cacheSize.value), 1);
      choices.forEach(c => {
        c.input.disabled = true;
      });
      mobile.disabled = true;
      cacheSize.disabled = true;
      this.#main.sendRPC('setCachePreference', {mode:mode,allthumbs:all,mobile:mob,maxSize:size})
      .then(() => {
        current.mode = mode;
        current.allthumbs = all;
        current.mobile = mob;
        current.maxSize = size;
        this.popupMessage(_T('saved'), 'info');
      })
      .catch(err => {
        this.popupMessage(err);
      })
      .finally(() => {
        choices.forEach(c => {
          c.input.disabled = false;
          if (current.mode === c.value) {
            c.input.checked = true;
            c.input.focus();
          }
        });
        allthumbs.checked = current.allthumbs;
        allthumbs.disabled = current.mode !== 'encrypted';
        mobile.checked = current.mobile;
        mobile.disabled = current.mode !== 'encrypted';
        cacheSize.value = current.maxSize;
        cacheSize.disabled = current.mode !== 'encrypted';
      });
    };

    let allthumbs;
    let mobile;
    let cacheSize;
    const opts = UI.create('div', {id:'preferences-cache-choices', parent:content});
    choices.forEach(choice => {
      const input = UI.create('input', {id:`preferences-cache-${choice.value}`, type:'radio', name:'preferences-cache-option', checked:current.mode === choice.value, parent:opts});
      EL.add(input, 'change', () => changeCachePref(choice.value));
      choice.input = input;
      const label = UI.create('label', {htmlFor:`preferences-cache-${choice.value}`, html:choice.label, parent: opts});
      if (choice.value === 'encrypted') {
        const adiv = UI.create('div', {parent:label});
        UI.create('label', {htmlFor:'preferences-cache-allthumbs', text:_T('prefetch-all-thumbnails'), parent:adiv});
        allthumbs = UI.create('input', {id:'preferences-cache-allthumbs', type:'checkbox', checked:current.allthumbs, disabled:current.mode !== 'encrypted', parent:adiv});
        EL.add(allthumbs, 'change', () => changeCachePref(choice.value));

        const mdiv = UI.create('div', {parent:label});
        UI.create('label', {htmlFor:'preferences-cache-mobile', text:_T('use-mobile'), parent:mdiv});
        mobile = UI.create('input', {id:'preferences-cache-mobile', type:'checkbox', checked:current.mobile, disabled:current.mode !== 'encrypted', parent:mdiv});
        EL.add(mobile, 'change', () => changeCachePref(choice.value));

        const sdiv = UI.create('div', {parent:label});
        UI.create('label', {htmlFor:'preferences-cache-size', text:_T('max-cache-size'), parent:sdiv});
        cacheSize = UI.create('input', {id:'preferences-cache-size', type:'number', min:'1', value:current.maxSize, disabled:current.mode !== 'encrypted', parent:sdiv});
        EL.add(cacheSize, 'change', () => changeCachePref(choice.value));
        const usage = UI.create('div', {text:_T('cache-usage', current.usage), parent:sdiv});
      }
    });

    navigator.serviceWorker.ready
    .then(registration => {
      if (!registration.pushManager || !registration.pushManager.getSubscription) {
        return;
      }
      UI.create('div', {id:'preferences-notifications-text', html:_T('choose-notifications-pref'), parent:content});
      const notifopt = UI.create('div', {id:'preferences-notifications-choices', parent:content});
      const input = UI.create('input', {id:'preferences-notifications-checkbox', type:'checkbox', name:'preferences-notifications-checkbox', parent:notifopt});
      registration.pushManager.getSubscription()
      .then(sub => {
        input.checked = sub !== null;
      });
      EL.add(input, 'change', async () => {
        if (window.Notification && window.Notification.permission !== 'granted' && input.checked) {
          const p = await window.Notification.requestPermission();
          input.checked = p === 'granted';
        }
        this.#main.sendRPC('enableNotifications', input.checked)
        .then(v => {
          input.checked = v;
          this.enableNotifications = v;
          localStorage.setItem('enableNotifications', v ? 'yes' : 'no');
        })
        .catch(() => {
          input.checked = false;
          this.enableNotifications = false;
        });
      });
      UI.create('label', {htmlFor:'preferences-notifications-checkbox', html:_T('opt-enable-notifications'), parent:notifopt});
    });

    const {close} = this.commonPopup_({
      title: _T('prefs'),
      content: content,
      EL: EL,
      onclose: () => {
        this.refreshGallery_(false);
      },
    });
  }

  async showAdminConsole_() {
    const data = await this.#main.sendRPC('adminUsers');
    const {EL, content} = this.commonPopup_({
      title: _T('admin-console'),
      className: 'popup admin-console-popup',
    });
    return this.showAdminConsoleData_(EL, content, data);
  }

  async showAdminConsoleData_(EL, content, data) {
    UI.clearElement_(content);
    const changes = () => {
      const out = {};
      for (let p of Object.keys(data).filter(k => k.startsWith('_'))) {
        out[p.substring(1)] = data[p];
      }
      out.users = [];
      for (let u of data.users) {
        const keys = Object.keys(u).filter(k => k.startsWith('_'));
        if (keys.length === 0) {
          continue;
        }
        const n = {
          userId: u.userId,
        };
        for (let p of keys) {
          n[p.substring(1)] = u[p];
        }
        out.users.push(n);
      }
      if (out.users.length === 0) {
        delete out.users;
      }
      if (Object.keys(out).length === 0) {
        return null;
      }
      out.tag = data.tag;
      return out;
    };
    const onchange = () => {
      saveButton.disabled = changes() === null;
      saveButton.textContent = saveButton.disabled ? _T('no-changes') : _T('apply-changes');
    };

    const defQuotaDiv = UI.create('div', {id:'admin-console-default-quota-div', parent:content});
    UI.create('label', {htmlFor:'admin-console-default-quota-value', text:'Default quota:', parent:defQuotaDiv});
    const defQuotaValue = UI.create('input', {id:'admin-console-default-quota-value', type:'number', size:5, value:data.defaultQuota, parent:defQuotaDiv});
    EL.add(defQuotaValue, 'change', () => {
      const v = parseInt(defQuotaValue.value);
      if (v === data.defaultQuota) {
        delete data._defaultQuota;
        defQuotaValue.classList.remove('changed');
      } else {
        data._defaultQuota = v;
        defQuotaValue.classList.add('changed');
      }
      onchange();
    });
    const defQuotaUnit = UI.create('select', {parent:defQuotaDiv});
    for (let u of ['','MB','GB','TB']) {
      UI.create('option', {value:u, text:u === '' ? '' : _T(u), selected:u === data.defaultQuotaUnit, parent:defQuotaUnit});
    }
    EL.add(defQuotaUnit, 'change', () => {
      const v = defQuotaUnit.options[defQuotaUnit.options.selectedIndex].value;
      if (v === data.defaultQuotaUnit || (v === '' && data.defaultQuotaUnit === undefined)) {
        delete data._defaultQuotaUnit;
        defQuotaUnit.classList.remove('changed');
      } else {
        data._defaultQuotaUnit = v;
        defQuotaUnit.classList.add('changed');
      }
      onchange();
    });

    const filter = UI.create('input', {id:'admin-console-filter', type:'search', placeholder:_T('filter'), parent:content});
    EL.add(filter, 'keydown', () => {
      showUsers();
    });
    EL.add(filter, 'input', () => {
      showUsers();
    });

    const view = {};
    for (let user of data.users) {
      view[user.email] = [];

      const email = UI.create('div', {text:user.email});
      view[user.email].push(email);

      const lockedDiv = UI.create('div');
      const locked = UI.create('input', {type:'checkbox', checked:user.locked, parent:lockedDiv});
      EL.add(locked, 'change', () => {
        const v = locked.checked;
        if (v === user.locked) {
          delete user._locked;
          locked.classList.remove('changed');
        } else {
          user._locked = v;
          locked.classList.add('changed');
        }
        onchange();
      });
      view[user.email].push(lockedDiv);

      const approvedDiv = UI.create('div');
      const approved = UI.create('input', {type:'checkbox', checked:user.approved, parent:approvedDiv});
      EL.add(approved, 'change', () => {
        const v = approved.checked;
        if (v === user.approved) {
          delete user._approved;
          approved.classList.remove('changed');
        } else {
          user._approved = v;
          approved.classList.add('changed');
        }
        onchange();
      });
      view[user.email].push(approvedDiv);

      const adminDiv = UI.create('div');
      const admin = UI.create('input', {type:'checkbox', checked:user.admin, parent:adminDiv});
      EL.add(admin, 'change', () => {
        const v = admin.checked;
        if (v === user.admin) {
          delete user._admin;
          admin.classList.remove('changed');
        } else {
          user._admin = v;
          admin.classList.add('changed');
        }
        onchange();
      });
      view[user.email].push(adminDiv);

      const quotaDiv = UI.create('div', {className:'quota-cell'});
      const quotaValue = UI.create('input', {type:'number', size:5, value:user.quota, parent:quotaDiv});
      EL.add(quotaValue, 'change', () => {
        const v = parseInt(quotaValue.value);
        if ((quotaValue.value === '' && user.quota === undefined) || v === user.quota) {
          delete user._quota;
          quotaValue.classList.remove('changed');
        } else {
          user._quota = quotaValue.value === '' ? -1 : v;
          quotaValue.classList.add('changed');
        }
        onchange();
      });
      const quotaUnit = UI.create('select');
      for (let u of ['','MB','GB','TB']) {
        UI.create('option', {value:u, text:u === '' ? '' : _T(u), selected:u === user.quotaUnit, parent:quotaUnit});
      }
      EL.add(quotaUnit, 'change', () => {
        const v = quotaUnit.options[quotaUnit.options.selectedIndex].value;
        if (v === user.quotaUnit || (v === '' && user.quotaUnit === undefined)) {
          delete user._quotaUnit;
          quotaUnit.classList.remove('changed');
        } else {
          user._quotaUnit = v;
          quotaUnit.classList.add('changed');
        }
        onchange();
      });
      quotaDiv.appendChild(quotaUnit);
      view[user.email].push(quotaDiv);
    }

    const table = UI.create('div', {id:'admin-console-table', parent:content});

    const saveButton = UI.create('button', {id:'admin-console-save-button', text:_T('no-changes'), disabled:true, parent:content});
    EL.add(saveButton, 'click', () => {
      const c = changes();
      content.querySelectorAll('input,select').forEach(elem => {
        elem.disabled = true;
      });
      this.#main.sendRPC('adminUsers', c)
      .then(data => {
        this.popupMessage(_T('data-updated'), 'info');
        return this.showAdminConsoleData_(EL, content, data);
      })
      .finally(() => {
        content.querySelectorAll('input,select').forEach(elem => {
          elem.disabled = false;
        });
      });
    });

    const showUsers = () => {
      while(table.firstChild) {
        table.removeChild(table.firstChild);
      }
      table.innerHTML = `<div class="row"><div>${_T('email')}</div><div>${_T('locked')}</div><div>${_T('approved')}</div><div>${_T('admin')}</div><div>${_T('quota')}</div></div>`;
      for (let user of data.users) {
        if (filter.value === '' || user.email.includes(filter.value) || Object.keys(user).filter(k => k.startsWith('_')).length > 0) {
          const row = UI.create('div', {className:'row', parent:table});
          view[user.email].forEach(e => row.appendChild(e));
        }
      }
    };
    showUsers();
  }
}

let EL_added = 0;
let EL_removed = 0;
class EventListeners {
  constructor() {
    this.list_ = [];
  }
  add(elem, type, listener, options) {
    EL_added++;
    elem.addEventListener(type, listener, options);
    this.list_.push({elem, type, listener, options});
  }
  clear() {
    for (const it of this.list_) {
      EL_removed++;
      it.elem.removeEventListener(it.type, it.listener, it.options);
      delete it.elem;
      delete it.listener;
    }
    this.list_ = [];
    //console.log(`XXX EL clear ${EL_added - EL_removed} added=${EL_added} removed=${EL_removed}`);
  }
}

class CollectionThumb {
  constructor(c) {
    const div = UI.create('div', {className:'collectionThumbdiv', tabindex:'0', role:'link', title:_T('collection-title', c.name)});
    if (!c.isOwner) {
      div.classList.add('not-owner');
    }
    const img = new Image();
    img.alt = c.name;
    if (c.cover) {
      img.src = c.cover;
    } else {
      img.src = 'clear.png';
    }
    const imgdiv = UI.create('div');
    img.style.gridArea = '1 / 1 / 2 / 2';
    imgdiv.style.display = 'grid';
    imgdiv.appendChild(img);
    if (c.isOffline) {
      const check = UI.create('div', {text: '☑', className:'collectionThumbOfflineCheck', parent:imgdiv});
      check.style.display = 'none';
      check.style.gridArea = '1 / 1 / 2 / 2';
      check.style.justifySelf = 'end';
      check.style.alignSelf = 'start';
      check.style.opacity = '0.75';
    }
    div.appendChild(imgdiv);
    const n = UI.create('div', {text:c.name, className:'collectionThumbLabel', parent:div});

    this.setCurrent = isCurrent => {
      const sz = isCurrent ? UI.px_(150) : UI.px_(120);
      img.style.height = sz;
      img.style.width = sz;
      imgdiv.style.height = sz;
      imgdiv.style.width = sz;
      n.style.width = sz;
    };
    this.setCacheMode = mode => {
      const e = this.div.querySelector('.collectionThumbOfflineCheck');
      if (e) {
        e.style.display = mode === 'encrypted' ? 'block' : 'none';
      }
    };
    this.div = div;
  }
}

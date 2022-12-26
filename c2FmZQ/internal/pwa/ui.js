/*
 * Copyright 2021-2022 TTBT Enterprises LLC
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

const SHOW_ITEMS_INCREMENT = 10;

class UI {
  constructor() {
    this.uiStarted_ = false;
    this.promptingForPassphrase_ = false;
    this.addingFiles_ = false;
    this.popupZindex_ = 1000;
    this.galleryState_ = {
      collection: main.getHash('collection', 'gallery'),
      files: [],
      lastDate: '',
      shown: SHOW_ITEMS_INCREMENT,
    };
    this.enableNotifications = window.Notification?.permission === 'granted';

    _T = Lang.text;

    const langSelect = document.querySelector('#language');
    while (langSelect.firstChild) {
      langSelect.removeChild(langSelect.firstChild);
    }
    const languages = Lang.languages();
    for (let l of Object.keys(languages)) {
      const opt = document.createElement('option');
      opt.value = l;
      opt.textContent = languages[l];
      if (l === Lang.current) {
        opt.selected = true;
      }
      langSelect.appendChild(opt);
    }
    langSelect.addEventListener('change', () => {
      localStorage.setItem('lang', langSelect.options[langSelect.options.selectedIndex].value);
      window.location.reload();
    });

    this.title_ = document.querySelector('title');
    this.passphraseInput_ = document.querySelector('#passphrase-input');
    this.setPassphraseButton_ = document.querySelector('#set-passphrase-button');
    this.showPassphraseButton_ = document.querySelector('#show-passphrase-button');
    this.skipPassphraseButton_ = document.querySelector('#skip-passphrase-button');
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
    this.loggedInAccount_ = document.querySelector('#loggedin-account');

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

    this.passphraseInput_.addEventListener('keyup', e => {
      if (e.key === 'Enter') {
        this.setPassphrase_();
      }
    });
    this.setPassphraseButton_.addEventListener('click', this.setPassphrase_.bind(this));
    this.showPassphraseButton_.addEventListener('click', () => {
      if (this.passphraseInput_.type === 'text') {
        this.passphraseInput_.type = 'password';
        this.showPassphraseButton_.textContent = _T('show');
      } else {
        this.passphraseInput_.type = 'text';
        this.showPassphraseButton_.textContent = _T('hide');
      }
    });
    this.skipPassphraseButton_.addEventListener('click', () => {
      this.prompt({message: _T('skip-passphrase-warning')})
      .then(() => {
        const passphrase = btoa(String.fromCharCode(...window.crypto.getRandomValues(new Uint8Array(64))));
        localStorage.setItem('_', passphrase);
        this.passphraseInput_.value = passphrase;
        this.setPassphrase_();
      })
      .catch(err => {
        console.log(err);
      });
    });
    this.resetDbButton_.addEventListener('click', main.resetServiceWorker.bind(main));
  }

  promptForPassphrase() {
    const p = localStorage.getItem('_');
    if (p) {
      this.passphraseInput_.value = p;
      this.setPassphrase_();
      return;
    }
    this.promptingForPassphrase_ = true;
    this.setPassphraseButton_.textContent = 'Set';
    this.setPassphraseButton_.disabled = false;
    this.passphraseInput_.disabled = false;
    this.showPassphraseBox_();
  }

  promptForBackupPhrase_() {
    return ui.prompt({
      message: _T('enter-backup-phrase'),
      getValue: true,
    })
    .then(v => {
      return main.sendRPC('restoreSecretKey', v);
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

  setPassphrase_() {
    if (!this.passphraseInput_.value) {
      return;
    }
    this.promptingForPassphrase_ = false;
    this.setPassphraseButton_.textContent = 'Setting';
    this.setPassphraseButton_.disabled = true;
    this.passphraseInput_.disabled = true;

    main.setPassphrase(this.passphraseInput_.value);
    this.passphraseInput_.value = '';

    setTimeout(() => {
      if (!this.uiStarted_ && !this.promptingForPassphrase_) {
        window.location.reload();
      }
    }, 3000);
  }

  wrongPassphrase(err) {
    if (localStorage.getItem('_')) {
      main.resetServiceWorker();
      return;
    }
    this.resetDbButton_.className = 'resetdb-button';
    this.popupMessage(err);
  }

  serverHash_(n) {
    this.serverUrl_ = n;
    let e = document.querySelector('#server-fingerprint');
    if (!e) {
      e = document.createElement('div');
      e.id = 'server-fingerprint';
      document.querySelector('body').appendChild(e);
    }
    main.calcServerFingerPrint(n)
    .then(fp => {
      e.textContent = fp;
    });
  }

  startUI() {
    console.log('Start UI');
    if (this.uiStarted_) {
      return;
    }
    this.uiStarted_ = true;

    if (SAMEORIGIN) {
      this.serverHash_(window.location.href.replace(/^(.*\/)[^\/]*/, '$1'));
    } else {
      document.querySelector('label[for=server]').style.display = '';
      this.serverInput_.style.display = '';
      this.serverInput_.value = main.getHash('server') || '';
      this.serverHash_(this.serverInput_.value);
    }

    window.addEventListener('scroll', this.onScroll_.bind(this));
    window.addEventListener('resize', this.onScroll_.bind(this));
    window.addEventListener('hashchange', () => {
      const c = main.getHash('collection');
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
    this.refreshButton_.addEventListener('click', this.refresh_.bind(this));
    this.emailInput_.addEventListener('keyup', e => {
      if (e.key === 'Enter') {
        this.passwordInput_.focus();
      }
    });
    this.passwordInput_.addEventListener('keyup', e => {
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
    this.passwordInput2_.addEventListener('keyup', e => {
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

    this.serverInput_.addEventListener('keyup', () => {
      this.serverHash_(this.serverInput_.value);
    });
    this.serverInput_.addEventListener('change', () => {
      this.serverHash_(this.serverInput_.value);
    });
    this.loggedInAccount_.addEventListener('click', this.showAccountMenu_.bind(this));
    this.loggedInAccount_.addEventListener('contextmenu', this.showAccountMenu_.bind(this));

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
    }

    main.sendRPC('isLoggedIn')
    .then(({account, isAdmin, needKey}) => {
      if (account !== '') {
        this.accountEmail_ = account;
        this.isAdmin_ = isAdmin;
        this.loggedInAccount_.textContent = account;
        this.showLoggedIn_();
        if (needKey) {
          return this.promptForBackupPhrase_();
        }
        return main.sendRPC('getUpdates')
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
    main.sendRPC('quota')
    .then(({usage, quota}) => {
      const pct = Math.floor(100 * usage / quota) + '%';
      document.querySelector('#quota').textContent = this.formatSizeMB_(usage) + ' / ' + this.formatSizeMB_(quota) + ' (' + pct + ')';
    });
  }

  showAccountMenu_(event) {
    event.preventDefault();
    const params = {
      x: event.x,
      y: event.y,
      items: [
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
    params.items.push({});
    params.items.push({
      text: _T('logout'),
      onclick: this.logout_.bind(this),
      id: 'account-menu-logout',
    });
    this.contextMenu_(params);
  }

  popupMessage(message, className, opt) {
    const div = document.createElement('div');
    div.className = className || 'error';
    div.style.position = 'relative';
    div.style.zIndex = this.popupZindex_++;
    const v = document.createElement('span');
    v.textContent = '✖';
    v.style = 'float: right;';
    const m = document.createElement('div');
    m.className = 'popup-message';
    if (message instanceof Element) {
      m.appendChild(message);
    } else {
      m.textContent = message;
    }
    div.appendChild(v);
    div.appendChild(m);

    const container = document.querySelector('#popup-messages');
    const remove = () => {
      div.style.animation = '1s linear slideOut';
      div.addEventListener('animationend', () => {
        try {
          container.removeChild(div);
        } catch(e) {}
      });
    };
    div.addEventListener('click', remove);
    container.appendChild(div);
    m.remove = remove;

    if (!opt || !opt.sticky) {
      setTimeout(remove, 5000);
    }
    return remove;
  }

  showError_(e) {
    console.log('Show Error', e);
    console.trace();
    this.popupMessage(e.toString());
  }

  clearElement_(id) {
    const e = document.getElementById(id);
    while (e && e.firstChild) {
      e.removeChild(e.firstChild);
    }
  }
  
  clearView_() {
    this.clearElement_('gallery');
    this.clearElement_('popup-name');
    this.clearElement_('popup-content');
  }

  showPassphraseBox_() {
    this.clearView_();
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
    this.clearView_();
  }

  showLoggedOut_() {
    this.title_.textContent = _T('login');
    this.clearView_();
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
    const body = document.querySelector('body');
    const div = document.createElement('div');
    div.id = 'bit-scroller';
    const a = document.createElement('div');
    const b = document.createElement('div');
    const c = document.createElement('div');
    div.appendChild(a);
    div.appendChild(b);
    div.appendChild(c);
    body.appendChild(div);

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
      body.removeChild(div);
    };
  }

  async login_() {
    if (this.selectedTab_ !== 'login' && this.passwordInput_.value !== this.passwordInput2_.value) {
      this.popupMessage(_T('new-pass-doesnt-match'));
      return;
    }
    if (!SAMEORIGIN) {
      try {
        this.serverInput_.value = new URL(this.serverInput_.value).toString();
      } catch (err) {
        return Promise.reject(err);
      }
      this.serverHash_(this.serverInput_.value);
      main.setHash('server', this.serverInput_.value);
    }
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
    return main.sendRPC(this.tabs_[this.selectedTab_].rpc, args).finally(done)
    .then(({isAdmin, needKey}) => {
      this.accountEmail_ = this.emailInput_.value;
      this.isAdmin_ = isAdmin;
      document.querySelector('#loggedin-account').textContent = this.emailInput_.value;
      this.passwordInput_.value = '';
      this.passwordInput2_.value = '';
      this.backupPhraseInput_.value = '';
      this.showLoggedIn_();
      if (needKey) {
        return this.promptForBackupPhrase_();
      }
      return main.sendRPC('getUpdates')
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
    return main.sendRPC('logout')
    .then(() => {
      this.showLoggedOut_();
    });
  }

  async refresh_() {
    this.refreshButton_.disabled = true;
    return main.sendRPC('getUpdates')
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
        .then(() => {
          this.addingFiles_ = false;
        });
      });
    }
  }

  switchView(c) {
    if (this.galleryState_.collection !== c.collection) {
      this.galleryState_.collection = c.collection;
      this.galleryState_.shown = SHOW_ITEMS_INCREMENT;
      main.setHash('collection', c.collection);
      this.refreshGallery_(true);
    } else {
      this.refreshGallery_(false);
    }
  }

  showAddMenu_(event) {
    event.preventDefault();
    const params = {
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
    const collections = await main.sendRPC('getCollections');
    this.galleryState_.content = await main.sendRPC('getFiles', this.galleryState_.collection);
    if (!this.galleryState_.content) {
      this.galleryState_.content = {'total': 0, 'files': []};
    }
    const oldScrollLeft = document.querySelector('#collections')?.scrollLeft;
    const oldScrollTop = scrollToTop ? 0 : document.documentElement.scrollTop;

    const cd = document.querySelector('#collections');
    while (cd.firstChild) {
      cd.removeChild(cd.firstChild);
    }

    let g = document.querySelector('#gallery');
    while (g.firstChild) {
      g.removeChild(g.firstChild);
    }

    let collectionName = '';
    let members = [];
    let scrollTo = null;
    let isOwner = false;
    let canAdd = false;

    const showContextMenu = (event, c) => {
      let params = {
        x: event.x,
        y: event.y,
        items: [],
      };
      if (this.galleryState_.collection !== c.collection) {
        params.items.push({
          text: _T('open'),
          onclick: () => this.switchView({collection: c.collection}),
        });
      }
      if (c.collection !== 'trash' && c.collection !== 'gallery') {
        params.items.push({
          text: _T('properties'),
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
              onclick: () => this.moveFiles_({file: f.file, collection: f.collection, move: false}, c.collection),
            });
          }
          params.items.push({
            text: _T('move-selected'),
            onclick: () => this.moveFiles_({file: f.file, collection: f.collection, move: true}, c.collection),
          });
        }
      }
      if (params.items.length > 0) {
        this.contextMenu_(params);
      }
    };

    let currentCollection;
    for (let i in collections) {
      if (!collections.hasOwnProperty(i)) {
        continue;
      }
      const c = collections[i];
      if (c.collection === 'trash' && this.galleryState_.collection !== c.collection) {
        continue;
      }
      if (!currentCollection) {
        currentCollection = c;
      }
      if (c.name === 'gallery' || c.name === 'trash') {
        c.name = _T(c.name);
      }
      const div = document.createElement('div');
      div.className = 'collectionThumbdiv';
      if (!c.isOwner) {
        div.classList.add('not-owner');
      }
      if (c.isOwner || c.canAdd) {
        div.addEventListener('dragover', event => {
          event.preventDefault();
        });
        div.addEventListener('drop', event => {
          event.preventDefault();
          event.stopPropagation();
          this.handleCollectionDropEvent_(c.collection, event);
        });
      }
      div.addEventListener('contextmenu', event => {
        event.preventDefault();
        showContextMenu(event, c);
      });
      div.addEventListener('click', () => {
        this.switchView(c);
      });
      if (this.galleryState_.collection === c.collection) {
        currentCollection = c;
        this.title_.textContent = c.name;
        scrollTo = div;
        this.galleryState_.canDrag = c.isOwner || c.canCopy;
        this.galleryState_.isOwner = c.isOwner;
      }
      const img = new Image();
      img.alt = c.name;
      if (c.cover) {
        img.src = c.cover;
      } else {
        img.src = 'clear.png';
      }
      const sz = this.galleryState_.collection === c.collection ? UI.px_(150) : UI.px_(120);
      const imgdiv = document.createElement('div');
      img.style.height = sz;
      img.style.width = sz;
      imgdiv.style.height = sz;
      imgdiv.style.width = sz;
      imgdiv.appendChild(img);
      div.appendChild(imgdiv);
      const n = document.createElement('div');
      n.className = 'collectionThumbLabel';
      n.style.width = sz;
      n.textContent = c.name;
      div.appendChild(n);
      cd.appendChild(div);
    }

    if (this.galleryState_.collection !== 'gallery' && this.galleryState_.collection !== 'trash') {
      const settingsButton = document.createElement('button');
      settingsButton.id = 'settings-button';
      settingsButton.textContent = '⚙';
      settingsButton.addEventListener('click', () => {
        this.collectionProperties_(currentCollection);
      });
      g.appendChild(settingsButton);
    }

    if (currentCollection.isOwner || currentCollection.canAdd) {
      const addDiv = document.createElement('div');
      addDiv.id = 'add-button';
      addDiv.textContent = '＋';
      addDiv.addEventListener('click', this.showAddMenu_.bind(this));
      addDiv.addEventListener('contextmenu', this.showAddMenu_.bind(this));
      g.appendChild(addDiv);
    }

    const br = document.createElement('br');
    br.clear = 'all';
    g.appendChild(br);
    const h1 = document.createElement('h1');
    h1.textContent = _T('collection:', currentCollection.name);
    if (this.galleryState_.collection === 'trash') {
      const button = document.createElement('button');
      button.className = 'empty-trash';
      button.textContent = _T('empty');
      button.addEventListener('click', e => {
        this.emptyTrash_(e.target);
      });
      h1.appendChild(button);
    }
    g.appendChild(h1);
    if (currentCollection?.members?.length > 0) {
      UI.sortBy(currentCollection.members, 'email');
      const div = document.createElement('div');
      div.textContent = _T('shared-with', currentCollection.members.map(m => m.email).join(', '));
      g.appendChild(div);
    }

    this.galleryState_.lastDate = '';
    const n = Math.max(this.galleryState_.shown, SHOW_ITEMS_INCREMENT);
    this.galleryState_.shown = 0;
    this.showMoreFiles_(n);
    if (scrollTo) {
      if (oldScrollLeft) {
        cd.scrollLeft = oldScrollLeft;
      }
      setTimeout(() => {
        if (oldScrollLeft) cd.scrollLeft = oldScrollLeft;
        const left = Math.max(scrollTo.offsetLeft - (cd.offsetWidth - scrollTo.offsetWidth)/2, 0);
        cd.scrollTo({behavior: 'smooth', left: left});
        document.documentElement.scrollTo({top: oldScrollTop, behavior: 'smooth'});
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
    return main.sendRPC('getFiles', this.galleryState_.collection, this.galleryState_.content.files.length)
      .then(ff => {
        this.galleryState_.content.files.push(...ff.files);
        return this.galleryState_.content.files.length;
      });
  }

  async showMoreFiles_(n) {
    if (!this.galleryState_.content) {
      return;
    }
    const max = await this.maybeGetMoreFiles_(this.galleryState_.shown + n);
    const g = document.querySelector('#gallery');

    const showContextMenu = (event, i) => {
      const f = this.galleryState_.content.files[i];
      let params = {
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
          onclick: () => this.showEdit_(f),
        });
      }
      params.items.push({
        text: f.selected ? _T('unselect') : _T('select'),
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

    const last = Math.min(this.galleryState_.shown + n, max);
    for (let i = this.galleryState_.shown; i < last; i++) {
      this.galleryState_.shown++;
      const f = this.galleryState_.content.files[i];
      const date = (new Date(f.dateCreated)).toLocaleDateString(undefined, {weekday: 'long', year: 'numeric', month: 'long', day: 'numeric'});
      if (date !== this.galleryState_.lastDate) {
        this.galleryState_.lastDate = date;
        const span = document.createElement('span');
        span.className = 'date';
        span.innerHTML = '<br clear="all" />'+date+'<br clear="all" />';
        g.appendChild(span);
      }
      const img = new Image();
      img.alt = f.fileName;
      img.src = f.thumbUrl;
      img.style.height = UI.px_(320);

      const d = document.createElement('div');
      d.className = 'thumbdiv';
      f.elem = d;
      f.select = () => selectItem(i);

      d.appendChild(img);
      if (f.isVideo) {
        const div = document.createElement('div');
        div.className = 'duration';
        const dur = document.createElement('span');
        dur.className = 'duration';
        dur.textContent = this.formatDuration_(f.duration);
        div.appendChild(dur);
        d.appendChild(div);
      }

      d.draggable = true;
      d.addEventListener('click', e => click(i, e));
      d.addEventListener('dragstart', e => dragStart(f, e, img));
      d.addEventListener('dragend', e => dragEnd(f, e));
      d.addEventListener('contextmenu', e => contextMenu(i, e));

      g.appendChild(d);
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
    this.popupMessage(_T('drop-received'), 'upload-progress');
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
        return main.sendRPC('upload', collection, toUpload)
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
    return main.sendRPC('moveFiles', file.collection, collection, files, file.move)
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
    return main.sendRPC('deleteFiles', files)
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
    main.sendRPC('emptyTrash')
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
    return main.sendRPC('changeCover', collection, cover)
      .then(() => {
        this.refresh_();
      })
      .catch(e => {
        this.showError_(e);
      });
  }

  async leaveCollection_(collection) {
    return this.prompt({message: _T('confirm-leave')})
    .then(() => main.sendRPC('leaveCollection', collection))
    .then(() => {
      if (this.galleryState_.collection === collection) {
        this.switchView({collection: 'gallery'});
      }
      this.refresh_();
    });
  }

  async deleteCollection_(collection) {
    return this.prompt({message: _T('confirm-delete')})
    .then(() => main.sendRPC('deleteCollection', collection))
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
    const menu = document.createElement('div');
    menu.className = params.className || 'context-menu';
    let x = params.x || 10;
    let y = params.y || 10;
    menu.addEventListener('contextmenu', event => {
      event.preventDefault();
    });
    const point = document.createElement('img');
    point.style.position = 'absolute';
    menu.appendChild(point);

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
      this.setGlobalEventHandlers([
        ['keyup', handleEscape],
        ['click', handleClickOutside, true],
      ]);
    });

    const body = document.querySelector('body');
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
      this.setGlobalEventHandlers(null);
      menu.style.animation = 'fadeOut 0.25s';
      menu.addEventListener('animationend', () => {
        try {
          body.removeChild(menu);
        } catch (e) {}
      });
      closeMenu = () => null;
    };

    for (let i = 0; i < params.items.length; i++) {
      if (!params.items[i].text) {
        const space = document.createElement('div');
        space.className = 'context-menu-space';
        space.innerHTML = '&nbsp;';
        menu.appendChild(space);
        continue;
      }
      const item = document.createElement(params.items[i].onclick ? 'button' : 'div');
      item.className = 'context-menu-item';
      item.textContent = params.items[i].text;
      if (params.items[i].id) {
        item.id = params.items[i].id;
      }
      if (params.items[i].onclick) {
        item.addEventListener('click', e => {
          closeMenu();
          if (params.items[i].onclick) {
            params.items[i].onclick();
          }
        });
      }
      menu.appendChild(item);
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

  setGlobalEventHandlers(handlers) {
    if (!this.windowHandlers) {
      this.windowHandlers = [];
    }
    const add = h => {
      h.forEach(args => document.addEventListener(...args));
    };
    const remove = h => {
      h.forEach(args => document.removeEventListener(...args));
    };
    if (handlers) {
      // Add events.
      if (this.windowHandlers.length) {
        remove(this.windowHandlers[0]);
      }
      this.windowHandlers.unshift(handlers);
      add(handlers);
    } else {
      // Remove events.
      remove(this.windowHandlers.shift());
      if (this.windowHandlers.length) {
        add(this.windowHandlers[0]);
      }
    }
  }

  async prompt(params) {
    const win = document.createElement('div');
    win.className = 'prompt-div';
    const bg = document.createElement('div');
    bg.className = 'prompt-bg';

    const text = document.createElement('div');
    text.className = 'prompt-text';
    text.textContent = params.message;
    win.appendChild(text);

    let input;
    if (params.getValue) {
      if (params.password) {
        input = document.createElement('input');
        input.type = 'password';
      } else {
        input = document.createElement('textarea');
      }
      input.className = 'prompt-input';
      if (params.defaultValue) {
        input.value = params.defaultValue;
      }
      win.appendChild(input);
    }

    const buttons = document.createElement('div');
    buttons.className = 'prompt-button-row';
    const conf = document.createElement('button');
    conf.className = 'prompt-confirm-button';
    conf.textContent = params.confirmText || _T('confirm');
    buttons.appendChild(conf);
    const canc = document.createElement('button');
    canc.className = 'prompt-cancel-button';
    canc.textContent = params.cancelText || _T('cancel');
    buttons.appendChild(canc);
    win.appendChild(buttons);

    const body = document.querySelector('body');
    body.appendChild(bg);
    body.appendChild(win);
    if (input) input.focus();

    const waiting = body.classList.contains('waiting');
    if (waiting) {
      body.classList.remove('waiting');
    }
    const p = new Promise((resolve, reject) => {
      const handleEscape = e => {
        if (e.key === 'Escape') {
          close();
          reject(_T('canceled'));
        }
        if (e.key === 'Enter') {
          close();
          resolve(params.getValue ? input.value.trim() : true);
        }
      };
      this.setGlobalEventHandlers([
        ['keyup', handleEscape],
      ]);
      const close = () => {
        this.setGlobalEventHandlers(null);
        body.removeChild(bg);
        body.removeChild(win);
        if (waiting) {
          body.classList.add('waiting');
        }
      };
      conf.addEventListener('click', () => {
        close();
        resolve(params.getValue ? input.value.trim() : true);
      });
      canc.addEventListener('click', () => {
        close();
        reject(_T('canceled'));
      });
    });
    return p;
  }

  freeze(params) {
    const win = document.createElement('div');
    win.className = 'prompt-div';
    const bg = document.createElement('div');
    bg.className = 'prompt-bg';

    const text = document.createElement('div');
    text.className = 'prompt-text';
    text.textContent = params.message;
    win.appendChild(text);

    const body = document.querySelector('body');
    body.appendChild(bg);
    body.appendChild(win);

    const waiting = body.classList.contains('waiting');
    if (waiting) {
      body.classList.remove('waiting');
    }
    this.setGlobalEventHandlers([]);
    return () => {
      this.setGlobalEventHandlers(null);
      body.removeChild(bg);
      body.removeChild(win);
      if (waiting) {
        body.classList.add('waiting');
      }
    };
  }

  async getCurrentPassword() {
    return this.prompt({message: _T('enter-current-password'), getValue: true, password: true});
  }

  commonPopup_(params) {
    const popup = document.createElement('div');
    const popupHeader = document.createElement('div');
    const popupName = document.createElement('div');
    const popupClose = document.createElement('div');
    const popupContent = document.createElement('div');
    const popupInfo = document.createElement('div');

    popupContent.className = 'popup-content';
    if (params.content) {
      popupContent.appendChild(params.content);
    }
    popup.className = params.className || 'popup';
    popupHeader.className = 'popup-header';
    popupName.className = 'popup-name';
    popupName.textContent = params.title || 'Title';
    popupInfo.className = 'popup-info';
    popupInfo.textContent = 'ⓘ';
    popupClose.className = 'popup-close';
    popupClose.textContent = '✖';

    popupHeader.appendChild(popupName);
    if (params.showInfo) {
      popupHeader.appendChild(popupInfo);
    }
    popupHeader.appendChild(popupClose);
    popup.appendChild(popupHeader);
    popup.appendChild(popupContent);

    const handleClickClose = () => {
      closePopup();
    };
    const handleEscape = e => {
      if (e.key === 'Escape') {
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
    popupClose.addEventListener('click', handleClickClose);
    setTimeout(() => {
      const handlers = params.handlers || [];
      handlers.push(['keyup', handleEscape]);
      handlers.push(['click', handleClickOutside, true]);
      this.setGlobalEventHandlers(handlers);
    });

    const body = document.querySelector('body');
    const g = document.querySelector('#gallery');

    if (!this.popupBlur_ || this.popupBlur_.depth <= 0) {
      this.popupBlur_ = {
        depth: 0,
        elem: document.createElement('div'),
      };
      this.popupBlur_.elem.className = 'blur';
      g.appendChild(this.popupBlur_.elem);
    }
    this.popupBlur_.depth++;

    const closePopup = opt_slide => {
      // Remove handlers.
      popupClose.removeEventListener('click', handleClickClose);
      this.setGlobalEventHandlers(null);
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
        g.removeChild(popup);
        if (--this.popupBlur_.depth <= 0) {
          const elem = this.popupBlur_.elem;
          this.popupBlur_.elem = null;
          elem.style.animation = 'fadeOut 0.25s';
          elem.addEventListener('animationend', () => {
            g.removeChild(elem);
          });
        }
      });
      if (params.onclose) {
        params.onclose();
      }
      body.classList.remove('noscroll');
    };
    body.classList.add('noscroll');
    g.appendChild(popup);
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
    });
    return {popup: popup, content: popupContent, close: closePopup, info: popupInfo};
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
        const i = this.recentImages_.byTime.findIndex(u => u === url);
        const j = this.recentImages_.byTime.length-1;
        this.recentImages_.byTime[i] = this.recentImages_.byTime[j];
        this.recentImages_.byTime[j] = url;
      }
      return this.recentImages_.byUrl[url];
    }
    const img = new Image();
    img.decoding = 'async';
    img.src = url;
    img.decode().catch(() => {
      delete this.recentImages_.byUrl[url];
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
    const onkeyup = e => {
      if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') {
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
      event.stopPropagation();
      event.preventDefault();
      const dx = event.changedTouches[0].clientX - touchX;
      const dy = event.changedTouches[0].clientY - touchY;
      if (Math.abs(dx) > Math.abs(dy) && Math.abs(dx) > Math.min(content.offsetWidth / 4, 200)) {
        touchX = null;
        touchY = null;
        if (dx < 0) goRight();
        if (dx > 0) goLeft();
      }
    };
    const params = {
      title: f.fileName,
      showInfo: true,
      slide: opt_slide,
      handlers: [
        ['keyup', onkeyup],
        ['touchstart', ontouchstart, {capture:true,passive:false}],
        ['touchmove', ontouchmove, {capture:true,passive:false}],
      ],
    };
    for (let j = i - 1; j <= i + 1; j++) {
      if (j >= 0 && j < max && this.galleryState_.content.files[j].isImage) {
        this.preload_(this.galleryState_.content.files[j].url);
      }
    }
    let img;
    if (f.isImage) {
      img = this.preload_(f.url);
      img.className = 'popup-media';
      img.alt = f.fileName;
      params.content = img;
    } else if (f.isVideo) {
      const video = document.createElement('video');
      video.className = 'popup-media';
      video.src = f.url;
      video.poster = f.thumbUrl;
      video.controls = 'controls';
      params.content = video;
    } else {
      const anchor = document.createElement('a');
      anchor.href = f.url;
      anchor.target = '_blank';
      anchor.textContent = _T('open-doc');
      params.content = anchor;
    }
    const {content, close, info} = this.commonPopup_(params);
    content.draggable = false;
    content.addEventListener('dragstart', event => {
      event.stopPropagation();
      event.preventDefault();
    }, true);
    if (i > 0) {
      const leftButton = document.createElement('div');
      leftButton.className = 'arrow left';
      leftButton.textContent = '⬅️';
      leftButton.addEventListener('click', goLeft);
      content.appendChild(leftButton);
    }
    if (i+1 < max) {
      const rightButton = document.createElement('div');
      rightButton.className = 'arrow right';
      rightButton.textContent = '➡️';
      rightButton.addEventListener('click', goRight);
      content.appendChild(rightButton);
    }
    if (f.isImage) {
      content.classList.add('image-popup');
      let exifData;
      info.addEventListener('click', () => {
        if (exifData === undefined) {
          exifData = null;
          const me = this;
          EXIF.getData(img, function() {
            exifData = EXIF.getAllTags(this);
            const div = document.createElement('div');
            div.className = 'exif-data';
            div.style.maxHeight = '' + Math.floor(0.9*content.offsetHeight) + 'px';
            div.style.maxWidth = '' + Math.floor(0.9*content.offsetWidth) + 'px';
            me.formatExif_(div, exifData);
            content.appendChild(div);
          });
        } else {
          const e = content.querySelector('.exif-data');
          if (e) e.classList.toggle('hidden');
        }
      });
    }
    if (!f.isImage && !f.isVideo) {
      content.classList.add('popup-download');
    }
  }

  formatExif_(div, data) {
    const flat = [];
    for (let n of Object.keys(data).sort()) {
      if (n === 'thumbnail') {
        continue;
      }
      if (Array.isArray(data[n])) {
        for (let i in data[n]) {
          if (data[n].hasOwnProperty(i)) {
            flat.push({key: `${n}[${i}]`, value: data[n][i]});
          }
        }
      } else {
          flat.push({key: n, value: data[n]});
      }
    }
    if (flat.length === 0) {
      div.textContent = '∅';
      return;
    }
    const out = [];
    for (let {key,value} of flat) {
      if (value instanceof Number) {
        out.push(`${key}: ${value} [${value.numerator} / ${value.denominator}]`);
      } else {
        out.push(`${key}: ${JSON.stringify(value)}`);
      }
    }
    const makeModel = document.createElement('div');
    if (data.Make) {
      makeModel.textContent = `${data.Make} ${data.Model}`;
    }
    div.appendChild(makeModel);
    const pos = document.createElement('div');
    if (data.GPSLatitudeRef) {
      const lat = `${data.GPSLatitudeRef} ${data.GPSLatitude[0].toFixed(0)}° ${data.GPSLatitude[1].toFixed(0)}' ${data.GPSLatitude[2].toFixed(3)}"`;
      const lon = `${data.GPSLongitudeRef} ${data.GPSLongitude[0].toFixed(0)}° ${data.GPSLongitude[1].toFixed(0)}' ${data.GPSLongitude[2].toFixed(3)}"`;
      pos.textContent = `${lat} ${lon}`;
    }
    div.appendChild(pos);
    const more = document.createElement('div');
    more.textContent = '➕';
    more.className = 'exif-more-details';
    let expanded = false;
    more.addEventListener('click', () => {
      details.classList.toggle('hidden');
      expanded = !expanded;
      more.textContent = expanded ? '➖' : '➕';
    });
    div.appendChild(more);
    const details = document.createElement('div');
    details.className = 'exif-details hidden';
    details.textContent = out.join('\n');
    div.appendChild(details);
  }

  showEdit_(f) {
    if (!f.isImage) {
      return;
    }
    const {content} = this.commonPopup_({
      title: _T('edit:', f.fileName),
      className: 'popup photo-editor-popup',
      disableClickOutsideClose: true,
      onclose: () => editor.terminate(),
    });

    const editor = new FilerobotImageEditor(content, {
      source: f.url,
      onSave: (img, state) => {
        console.log('saving', img.fullName);
        const binary = atob(img.imageBase64.split(',')[1]);
        const array = [];
        for (let i = 0; i < binary.length; i++) {
          array.push(binary.charCodeAt(i));
        }
        const blob = new Blob([new Uint8Array(array)], { type: img.mimeType });
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
          return main.sendRPC('upload', f.collection, up);
        })
        .then(() => {
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
      cover = await main.sendRPC('getCover', c.collection);
    }
    const contacts = await main.sendRPC('getContacts');
    const {content, close} = this.commonPopup_({
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
        c.collection = await main.sendRPC('createCollection', changes.name.trim());
      } else if (changes.name !== undefined) {
        await main.sendRPC('renameCollection', c.collection, changes.name.trim());
      }

      const perms = {
        canAdd: c.isOwner ? permAdd.checked : c.canAdd,
        canCopy: c.isOwner ? permCopy.checked : c.canCopy,
        canShare: c.isOwner ? permShare.checked : c.canShare,
      };

      if (changes.shared === true || changes.add !== undefined) {
        await main.sendRPC('shareCollection', c.collection, perms, changes.add || []);
      }

      if (changes.shared === false) {
        await main.sendRPC('unshareCollection', c.collection);
      }

      if (changes.remove !== undefined) {
        await main.sendRPC('removeMembers', c.collection, changes.remove);
      }

      if (changes.canAdd !== undefined || changes.canCopy !== undefined || changes.canShare !== undefined) {
        await main.sendRPC('updatePermissions', c.collection, perms);
      }

      if (changes.coverCode !== undefined) {
        await main.sendRPC('changeCover', c.collection, changes.coverCode);
      }
      close();
      main.sendRPC('getUpdates')
      .then(() => {
        this.switchView(c);
      });
    };

    if (!c.create) {
      const deleteButton = document.createElement('button');
      deleteButton.id = 'collection-properties-delete';
      deleteButton.textContent = '🗑';
      deleteButton.addEventListener('click', () => {
        if (c.isOwner) {
          this.deleteCollection_(c.collection).then(() => close());
        } else {
          this.leaveCollection_(c.collection).then(() => close());
        }
      });
      content.appendChild(deleteButton);
    }

    const coverLabel = document.createElement('div');
    coverLabel.id = 'collection-properties-cover-label';
    coverLabel.textContent = _T('cover');
    content.appendChild(coverLabel);

    const coverInput = document.createElement('div');
    const imgDiv = document.createElement('div');
    imgDiv.id = 'collection-properties-thumbdiv';
    imgDiv.draggable = false;
    imgDiv.addEventListener('dragstart', event => {
      event.stopPropagation();
      event.preventDefault();
    }, true);
    const sz = UI.px_(150);
    if (cover) {
      imgDiv.style.width = sz;
      imgDiv.style.height = sz;
    }
    coverInput.appendChild(imgDiv);
    const setImage = url => {
      if (c.create) {
        return;
      }
      while (imgDiv.firstChild) {
        imgDiv.removeChild(imgDiv.firstChild);
      }
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
      coverSelect = document.createElement('select');
      coverSelect.id = 'collection-properties-cover';
      const opts = [
        {code:'', label:_T('cover-latest')},
        {code:'__b__', label:_T('cover-blank')},
      ];
      if (cover !== null && cover.code !== '' && cover.code !== '__b__') {
        opts.push({code:cover.code, label:_T('cover-selected')});
      }
      let currCode = cover ? cover.code : '';
      for (let opt of opts) {
        let e = document.createElement('option');
        e.value = opt.code;
        e.textContent = opt.label;
        e.selected = currCode === e.value;
        coverSelect.appendChild(e);
      }
      coverSelect.addEventListener('change', () => {
        if (c.collection) {
          main.sendRPC('getCover', c.collection, coverSelect.options[coverSelect.options.selectedIndex].value)
          .then(cover => setImage(cover.url));
          onChange();
        }
      });
      coverInput.appendChild(coverSelect);
    }
    content.appendChild(coverInput);

    const nameLabel = document.createElement('div');
    nameLabel.id = 'collection-properties-name-label';
    nameLabel.textContent = _T('name');
    content.appendChild(nameLabel);

    let name;
    if (c.isOwner) {
      name = document.createElement('input');
      name.id = 'collection-properties-name';
      name.type = 'text';
      name.value = c.name;
      name.addEventListener('keyup', onChange);
      content.appendChild(name);
      if (c.create) name.focus();
    } else {
      name = document.createElement('div');
      name.id = 'collection-properties-name';
      name.textContent = c.name;
      content.appendChild(name);
    }

    const sharedLabel = document.createElement('div');
    sharedLabel.id = 'collection-properties-shared-label';
    sharedLabel.textContent = _T('shared');
    content.appendChild(sharedLabel);

    let shared;
    if (c.isOwner) {
      shared = document.createElement('input');
      shared.id = 'collection-properties-shared';
      shared.type = 'checkbox';
      shared.checked = c.isShared;
      shared.addEventListener('change', onChange);
      content.appendChild(shared);
    } else {
      const sharedDiv = document.createElement('div');
      sharedDiv.id = 'collection-properties-shared-div';
      sharedDiv.textContent = c.isShared ? _T('yes') : _T('no');
      content.appendChild(sharedDiv);
    }

    const permLabel = document.createElement('div');
    permLabel.id = 'collection-properties-perm-label';
    permLabel.className = 'sharing-setting';
    permLabel.style.display = c.isShared ? '' : 'none';
    permLabel.textContent = _T('permissions');
    content.appendChild(permLabel);

    const permDiv = document.createElement('div');
    permDiv.id = 'collection-properties-perm';
    permDiv.className = 'sharing-setting';
    permDiv.style.display = c.isShared ? '' : 'none';

    const permAdd = document.createElement('input');
    permAdd.id = 'collection-properties-perm-add';
    permAdd.type = 'checkbox';
    permAdd.checked = c.canAdd;
    permAdd.disabled = !c.isOwner;
    permAdd.addEventListener('change', onChange);
    const permAddLabel = document.createElement('label');
    permAddLabel.textContent = _T('perm-add');
    permAddLabel.htmlFor = 'collection-properties-perm-add';
    permDiv.appendChild(permAdd);
    permDiv.appendChild(permAddLabel);

    const permCopy = document.createElement('input');
    permCopy.id = 'collection-properties-perm-copy';
    permCopy.type = 'checkbox';
    permCopy.checked = c.canCopy;
    permCopy.disabled = !c.isOwner;
    permCopy.addEventListener('change', onChange);
    const permCopyLabel = document.createElement('label');
    permCopyLabel.textContent = _T('perm-copy');
    permCopyLabel.htmlFor = 'collection-properties-perm-copy';
    permDiv.appendChild(permCopy);
    permDiv.appendChild(permCopyLabel);

    const permShare = document.createElement('input');
    permShare.id = 'collection-properties-perm-share';
    permShare.type = 'checkbox';
    permShare.checked = c.canShare;
    permShare.disabled = !c.isOwner;
    permShare.addEventListener('change', onChange);
    const permShareLabel = document.createElement('label');
    permShareLabel.textContent = _T('perm-share');
    permShareLabel.htmlFor = 'collection-properties-perm-share';
    permDiv.appendChild(permShare);
    permDiv.appendChild(permShareLabel);

    content.appendChild(permDiv);

    const membersLabel = document.createElement('div');
    membersLabel.id = 'collection-properties-members-label';
    membersLabel.className = 'sharing-setting';
    membersLabel.style.display = c.isShared ? '' : 'none';
    membersLabel.textContent = _T('members');
    content.appendChild(membersLabel);

    const membersDiv = document.createElement('div');
    membersDiv.id = 'collection-properties-members';
    membersDiv.className = 'sharing-setting';
    membersDiv.style.display = c.isShared ? '' : 'none';
    content.appendChild(membersDiv);

    const applyButton = document.createElement('button');
    applyButton.id = 'collection-properties-apply-button';
    applyButton.textContent = _T('no-changes');
    applyButton.disabled = true;
    applyButton.addEventListener('click', applyChanges);
    content.appendChild(applyButton);

    const deleteMember = i => {
      membersDiv.removeChild(members[i].elem);
      members.splice(i, 1);
      refreshMembers();
      onChange();
    };

    const refreshMembers = () => {
      while (membersDiv.firstChild) {
        membersDiv.removeChild(membersDiv.firstChild);
      }
      UI.sortBy(members, 'email');
      if (c.isOwner || c.canShare) {
        const list = document.createElement('datalist');
        list.id = 'collection-properties-members-contacts';
        for (let i = 0; i < contacts.length; i++) {
          if (members.some(m => m.userId === contacts[i].userId)) {
            continue;
          }
          const opt = document.createElement('option');
          opt.value = contacts[i].email;
          list.appendChild(opt);
        }
        membersDiv.appendChild(list);

        const input = document.createElement('input');
        input.type = 'search';
        input.id = 'collection-properties-members-input';
        input.placeholder = _T('contact-email');
        input.setAttribute('list', 'collection-properties-members-contacts');
        membersDiv.appendChild(input);

        const addButton = document.createElement('button');
        addButton.id = 'collection-properties-members-add-button';
        addButton.textContent = _T('add-member');
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
          main.sendRPC('getContact', input.value)
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
        input.addEventListener('keyup', e => {
          if (e.key === 'Enter') {
            addFunc();
          }
        });
        input.addEventListener('change', addFunc);
        addButton.addEventListener('click', addFunc);
        membersDiv.appendChild(addButton);
      }
      if (members.length === 0) {
        const div = document.createElement('div');
        div.innerHTML = '<i>'+_T('none')+'</i>';
        membersDiv.appendChild(div);
      }
      for (let i = 0; i < members.length; i++) {
        const div = document.createElement('div');
        if (c.isOwner) {
          const del = document.createElement('button');
          del.textContent = '✖';
          del.style.cursor = 'pointer';
          del.addEventListener('click', () => deleteMember(i));
          div.appendChild(del);
        }
        const name = document.createElement('span');
        name.textContent = members[i].email;
        div.appendChild(name);
        membersDiv.appendChild(div);
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
      if (sz === 0) {
        setTimeout(e.remove, 2000);
      }
    } else {
      const msg = document.createElement('div');
      msg.className = 'thumbnail-progress-div';
      const span = document.createElement('span');
      span.id = 'thumbnail-progress-data';
      span.textContent = info;
      msg.appendChild(span);
      span.remove = this.popupMessage(msg, 'upload-progress', {sticky: sz > 0});
    }
  }

  showUploadProgress(progress) {
    const info = _T('upload-progress', `${progress.numFilesDone}/${progress.numFiles} [${Math.floor(progress.numBytesDone / progress.numBytes * 100)}%]`);
    const e = document.querySelector('#upload-progress-data');
    if (e) {
      e.textContent = info;
      if (progress.done || progress.err) {
        document.querySelector('#upload-progress-cancel-button').style.display = 'none';
        setTimeout(e.remove, 2000);
      }
    } else {
      const msg = document.createElement('div');
      msg.className = 'upload-progress-div';
      const span = document.createElement('span');
      span.id = 'upload-progress-data';
      span.textContent = info;
      msg.appendChild(span);
      const button = document.createElement('button');
      button.id = 'upload-progress-cancel-button';
      button.textContent = _T('cancel');
      button.addEventListener('click', () => {
        button.disabled = true;
        this.cancelDropUploads_();
        main.sendRPC('cancelUpload');
      });
      msg.appendChild(button);
      span.remove = this.popupMessage(msg, 'upload-progress', {sticky: !progress.done});
    }
  }

  async showUploadView_() {
    const collections = await main.sendRPC('getCollections');

    let collectionName = '';
    let members = [];

    for (let i in collections) {
      if (!collections.hasOwnProperty(i)) {
        continue;
      }
      const c = collections[i];
      if (this.galleryState_.collection === c.collection) {
        collectionName = c.name;
        members = c.members;
        break;
      }
    }
    const {popup, content, close} = this.commonPopup_({
      title: _T('upload:', collectionName),
      className: 'popup upload',
    });

    const h1 = document.createElement('h1');
    h1.textContent = _T('collection:', collectionName);
    content.appendChild(h1);
    if (members?.length > 0) {
      UI.sortBy(members, 'email');
      const div = document.createElement('div');
      div.textContent = _T('shared-with', members.map(m => m.email).join(', '));
      content.appendChild(div);
    }

    const list = document.createElement('div');
    list.id = 'upload-file-list';
    content.appendChild(list);

    let files = [];
    const processFiles = newFiles => {
      let p = [];
      for (let i = 0; i < newFiles.length; i++) {
        const f = newFiles[i];
        const elem = document.createElement('div');
        elem.className = 'upload-item-div';
        const img = new Image();
        img.src = 'clear.png';
        img.className = 'upload-thumbnail';
        elem.appendChild(img);
        const div = document.createElement('div');
        div.className = 'upload-item-attrs';
        elem.appendChild(div);
        const nameSpan = document.createElement('span');
        nameSpan.textContent = _T('name:', f.name);
        div.appendChild(nameSpan);
        const sizeSpan = document.createElement('span');
        sizeSpan.textContent = _T('size:', this.formatSize_(f.size));
        div.appendChild(sizeSpan);
        const errSpan = document.createElement('span');
        errSpan.textContent = _T('status:', '...');
        div.appendChild(errSpan);
        const removeButton = document.createElement('button');
        removeButton.className = 'upload-item-remove-button';
        removeButton.disabled = true;
        removeButton.textContent = _T('remove');
        removeButton.addEventListener('click', () => {
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
      while (list.firstChild) {
        list.removeChild(list.firstChild);
      }
      if (files.length > 0) {
        const uploadButton = document.createElement('button');
        uploadButton.className = 'upload-file-list-upload-button';
        uploadButton.textContent = _T('upload');
        uploadButton.disabled = true;
        uploadButton.addEventListener('click', () => {
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
          main.sendRPC('upload', this.galleryState_.collection, toUpload)
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
        list.appendChild(uploadButton);
        Promise.allSettled(p).then(() => {
          uploadButton.disabled = false;
        });
      }
      for (let i = 0; i < files.length; i++) {
        const f = files[i];
        list.appendChild(f.elem);
      }
    };
    const fileInputs = document.createElement('div');
    fileInputs.id = 'upload-files-div';
    content.appendChild(fileInputs);

    const label = document.createElement('label');
    label.for = 'files';
    label.textContent = _T('select-upload');
    fileInputs.appendChild(label);
    const input = document.createElement('input');
    input.id = 'upload-file-input';
    input.type = 'file';
    input.name = 'files';
    input.multiple = true;
    input.addEventListener('change', e => {
      processFiles(e.target.files);
    });
    fileInputs.appendChild(input);

    popup.addEventListener('drop', e => {
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
    popup.addEventListener('dragover', e => {
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
            return resolve([canvas.toDataURL(file.type),0]);
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
          });
        });
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
    const curr = await main.sendRPC('mfaStatus');

    const {content, close} = this.commonPopup_({
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

    const form = document.createElement('div');
    form.id = 'profile-form';

    const emailLabel = document.createElement('label');
    emailLabel.forHtml = 'profile-form-email';
    emailLabel.textContent = _T('form-email');
    const email = document.createElement('input');
    email.id = 'profile-form-email';
    email.type = 'email';
    email.value = this.accountEmail_;
    email.addEventListener('keyup', onchange);
    email.addEventListener('change', onchange);
    form.appendChild(emailLabel);
    form.appendChild(email);

    const newPassLabel = document.createElement('label');
    newPassLabel.forHtml = 'profile-form-new-password';
    newPassLabel.textContent = _T('form-new-password');
    const newPass = document.createElement('input');
    newPass.id = 'profile-form-new-password';
    newPass.type = 'password';
    newPass.placeholder = _T('optional');
    newPass.autocomplete = 'new-password';
    newPass.addEventListener('keyup', onchange);
    newPass.addEventListener('change', onchange);
    form.appendChild(newPassLabel);
    form.appendChild(newPass);

    const newPass2Label = document.createElement('label');
    newPass2Label.forHtml = 'profile-form-new-password2';
    newPass2Label.textContent = _T('form-confirm-password');
    const newPass2 = document.createElement('input');
    newPass2.id = 'profile-form-new-password2';
    newPass2.type = 'password';
    newPass2.placeholder = _T('optional');
    newPass2.autocomplete = 'new-password';
    newPass2.addEventListener('keyup', onchange);
    newPass2.addEventListener('change', onchange);
    form.appendChild(newPass2Label);
    form.appendChild(newPass2);

    const mfaLabel = document.createElement('label');
    mfaLabel.forHtml = 'profile-form-enable-mfa';
    mfaLabel.textContent = _T('enable-mfa?');
    const mfaDiv = document.createElement('div');
    mfaDiv.id = 'profile-form-enable-mfa-div';
    const mfa = document.createElement('input');
    mfa.id = 'profile-form-enable-mfa';
    mfa.type = 'checkbox';
    mfa.checked = curr.mfaEnabled;
    mfa.addEventListener('change', () => {
      form.querySelectorAll('.hide-no-mfa').forEach(e => e.style.display = mfa.checked ? '' : 'none');
      onchange();
    });
    mfaDiv.appendChild(mfa);
    const testButton = document.createElement('button');
    testButton.id = 'profile-form-test-mfa';
    testButton.className = 'hide-no-mfa';
    testButton.textContent = _T('test');
    testButton.addEventListener('click', () => {
      testButton.disabled = true;
      main.sendRPC('mfaCheck', passkey.checked).finally(() => {
        testButton.disabled = false;
      });
    });
    mfaDiv.appendChild(testButton);
    form.appendChild(mfaLabel);
    form.appendChild(mfaDiv);

    let otpKey = '';
    const otpLabel = document.createElement('label');
    otpLabel.className = 'hide-no-mfa';
    otpLabel.forHtml = 'profile-form-enable-otp';
    otpLabel.textContent = _T('enable-otp?');
    const otpDiv = document.createElement('div');
    otpDiv.id = 'profile-form-enable-otp-div';
    otpDiv.className = 'hide-no-mfa';
    const otp = document.createElement('input');
    otp.id = 'profile-form-enable-otp';
    otp.type = 'checkbox';
    otp.checked = curr.otpEnabled;
    otp.addEventListener('change', () => {
      if (otp.checked && !curr.otpEnabled) {
        otp.disabled = true;
        main.sendRPC('generateOTP')
        .then(({key, img}) => {
          otpKey = key;
          const image = new Image();
          image.src = img;
          const keyDiv = document.createElement('div');
          keyDiv.id = 'profile-form-otp-key';
          keyDiv.textContent = 'KEY: ' + key;
          const code = document.createElement('input');
          code.id = 'profile-form-otp-code';
          code.type = 'text';
          code.placeholder = _T('enter-code');
          code.addEventListener('keyup', onchange);
          code.addEventListener('change', onchange);
          otpDiv.appendChild(image);
          otpDiv.appendChild(keyDiv);
          otpDiv.appendChild(code);
        })
        .finally(() => {
          otp.disabled = false;
          onchange();
        });
      } else {
        otpKey = '';
        while (otpDiv.firstChild) {
          otpDiv.removeChild(otpDiv.firstChild);
        }
        otpDiv.appendChild(otp);
        onchange();
      }
    });
    otpDiv.appendChild(otp);
    form.appendChild(otpLabel);
    form.appendChild(otpDiv);

    const passkeyLabel = document.createElement('label');
    passkeyLabel.className = 'hide-no-mfa';
    passkeyLabel.forHtml = 'profile-form-enable-passkey';
    passkeyLabel.textContent = _T('enable-passkey?');
    const passkeyDiv = document.createElement('div');
    passkeyDiv.id = 'profile-form-enable-passkey-div';
    passkeyDiv.className = 'hide-no-mfa';
    const passkey = document.createElement('input');
    passkey.id = 'profile-form-enable-passkey';
    passkey.className = 'hide-no-mfa';
    passkey.type = 'checkbox';
    passkey.checked = curr.passKey;
    passkey.addEventListener('change', () => {
      updateKeyList();
      onchange();
    });
    passkeyDiv.appendChild(passkey);
    form.appendChild(passkeyLabel);
    form.appendChild(passkeyDiv);

    const skLabel = document.createElement('label');
    skLabel.className = 'hide-no-mfa';
    skLabel.forHtml = 'profile-form-add-security-key-button';
    skLabel.textContent = _T('security-keys:');
    const skDiv = document.createElement('div');
    skDiv.id = 'profile-form-security-keys-div';
    skDiv.className = 'hide-no-mfa';

    const addSkButton = document.createElement('button');
    addSkButton.id = 'profile-form-add-security-key-button';
    addSkButton.textContent = passkey.checked ? _T('add-passkey') : _T('add-security-key');
    addSkButton.addEventListener('click', () => {
      addSkButton.disabled = true;
      this.getCurrentPassword()
      .then(pw => main.addSecurityKey(pw, passkey.checked))
      .finally(() => {
        addSkButton.disabled = false;
        updateKeyList();
      });
    });
    skDiv.appendChild(addSkButton);

    const skList = document.createElement('div');
    skList.id = 'profile-form-security-key-list';
    skDiv.appendChild(skList);

    form.appendChild(skLabel);
    form.appendChild(skDiv);

    let keyList = {};
    const updateKeyList = () => {
      main.sendRPC('listSecurityKeys')
      .then(keys => {
        while (skList.firstChild) {
          skList.removeChild(skList.firstChild);
        }
        keys = keys.filter(k => !passkey.checked || k.discoverable);
        keyList = {};
        if (keys.length > 0) {
          skList.innerHTML = `<div class="profile-form-security-key-list-header">${_T('name')}</div><div class="profile-form-security-key-list-header">${_T('added')}</div><div></div>`;
        }
        for (let k of keys) {
          let input = document.createElement('input');
          input.type = 'text';
          input.className = 'profile-form-security-key-list-item';
          input.value = k.name;
          input.addEventListener('change', onchange);
          input.addEventListener('keyup', onchange);
          let t = document.createElement('div');
          t.textContent = (new Date(k.createdAt)).toLocaleDateString(undefined, {year: 'numeric', month: 'short', day: 'numeric'});
          let del = document.createElement('button');
          del.textContent = '✖';
          del.style.cursor = 'pointer';
          del.addEventListener('click', () => {
            keyList[k.id].deleted = !keyList[k.id].deleted;
            input.disabled = keyList[k.id].deleted;
            input.classList.toggle('deleted-key');
            t.classList.toggle('deleted-key');
            del.classList.toggle('deleted-key');
            onchange();
          });
          skList.appendChild(input);
          skList.appendChild(t);
          skList.appendChild(del);
          keyList[k.id] = {key: k, input: input};
        }
      });
    };
    updateKeyList();

    const button = document.createElement('button');
    button.id = 'profile-form-button';
    button.textContent = _T('no-changes');
    button.disabled = true;
    button.addEventListener('click', async () => {
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
      .then(pw => main.sendRPC('updateProfile', {
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
        this.loggedInAccount_.textContent = email.value;
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
      });
    });
    form.appendChild(button);
    form.querySelectorAll('.hide-no-mfa').forEach(e => e.style.display = mfa.checked ? '' : 'none');
    content.appendChild(form);

    const deleteMsg = document.createElement('div');
    deleteMsg.innerHTML = '<hr>' + _T('delete-warning');
    content.appendChild(deleteMsg);

    const delButton = document.createElement('button');
    delButton.id = 'profile-form-delete-button';
    delButton.textContent = _T('delete-account');
    delButton.addEventListener('click', () => {
      email.disabled = true;
      newPass.disabled = true;
      newPass2.disabled = true;
      button.disabled = true;
      delButton.disabled = true;
      this.prompt({message: _T('confirm-delete-account'), getValue: true, password: true})
      .then(pw => main.sendRPC('deleteAccount', pw))
      .then(() => {
        window.location.reload();
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
    content.appendChild(delButton);
  }

  async showBackupPhrase_() {
    const {content, close} = this.commonPopup_({
      title: _T('key-backup'),
    });
    content.id = 'backup-phrase-content';
    let keyBackupEnabled = await main.sendRPC('keyBackupEnabled');

    const warning = document.createElement('div');
    warning.id = 'backup-phrase-warning';
    warning.className = 'warning';
    warning.innerHTML = _T('key-backup-warning');
    content.appendChild(warning);

    const phrase = document.createElement('div');
    phrase.id = 'backup-phrase-value';
    content.appendChild(phrase);

    const button = document.createElement('button');
    button.id = 'backup-phrase-show-button';
    button.textContent = _T('show-backup-phrase');
    content.appendChild(button);
    button.addEventListener('click', () => {
      if (phrase.textContent === '') {
        button.disabled = true;
        button.textContent = _T('checking-password');
        this.getCurrentPassword()
        .then(pw => main.sendRPC('backupPhrase', pw))
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

    const warning2 = document.createElement('div');
    warning2.id = 'backup-phrase-warning2';
    warning2.className = 'warning';
    warning2.innerHTML = '<hr>' + _T('key-backup-warning2');
    content.appendChild(warning2);

    const changeBackup = choice => {
      inputYes.disabled = true;
      inputNo.disabled = true;
      this.getCurrentPassword()
      .then(pw => main.sendRPC('changeKeyBackup', pw, choice))
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
    const divYes = document.createElement('div');
    divYes.className = 'key-backup-option';
    const inputYes = document.createElement('input');
    inputYes.type = 'radio';
    inputYes.id = 'choose-key-backup-yes';
    inputYes.name = 'do-backup';
    inputYes.checked = keyBackupEnabled;
    inputYes.addEventListener('change', () => changeBackup(true));
    const labelYes = document.createElement('label');
    labelYes.htmlFor = 'choose-key-backup-yes';
    labelYes.textContent = _T('opt-keep-backup');
    divYes.appendChild(inputYes);
    divYes.appendChild(labelYes);

    const divNo = document.createElement('div');
    divNo.className = 'key-backup-option';
    const inputNo = document.createElement('input');
    inputNo.type = 'radio';
    inputNo.id = 'choose-key-backup-no';
    inputNo.name = 'do-backup';
    inputNo.checked = !keyBackupEnabled;
    inputNo.addEventListener('change', () => changeBackup(false));
    const labelNo = document.createElement('label');
    labelNo.htmlFor = 'choose-key-backup-no';
    labelNo.textContent = _T('opt-dont-keep-backup');
    divNo.appendChild(inputNo);
    divNo.appendChild(labelNo);

    content.appendChild(divYes);
    content.appendChild(divNo);
  }

  async showPreferences_() {
    const {content, close} = this.commonPopup_({
      title: _T('prefs'),
    });
    content.id = 'preferences-content';

    const text = document.createElement('div');
    text.id = 'preferences-cache-text';
    text.innerHTML = _T('choose-cache-pref');
    content.appendChild(text);

    const current = await main.sendRPC('cachePreference');
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

    const changeCachePref = choice => {
      choices.forEach(c => {
        c.input.disabled = true;
      });
      main.sendRPC('setCachePreference', choice)
      .then(() => {
        this.popupMessage(_T('saved'), 'info');
      })
      .catch(err => {
        choices.forEach(c => {
          c.input.checked = current === c.value;
        });
        this.popupMessage(err);
      })
      .finally(() => {
        choices.forEach(c => {
          c.input.disabled = false;
        });
      });
    };

    const opts = document.createElement('div');
    opts.id = 'preferences-cache-choices';
    choices.forEach(choice => {
      const input = document.createElement('input');
      input.type = 'radio';
      input.id = `preferences-cache-${choice.value}`;
      input.name = 'preferences-cache-option';
      input.checked = current === choice.value;
      input.addEventListener('change', () => changeCachePref(choice.value));
      choice.input = input;
      const label = document.createElement('label');
      label.htmlFor = `preferences-cache-${choice.value}`;
      label.innerHTML = choice.label;
      opts.appendChild(input);
      opts.appendChild(label);
    });
    content.appendChild(opts);


    navigator.serviceWorker.ready
    .then(registration => {
      if (!registration.pushManager || !registration.pushManager.getSubscription) {
        return;
      }
      const notif = document.createElement('div');
      notif.id = 'preferences-notifications-text';
      notif.innerHTML = _T('choose-notifications-pref');
      content.appendChild(notif);
      const notifopt = document.createElement('div');
      notifopt.id = 'preferences-notifications-choices';
      const input = document.createElement('input');
      input.type = 'checkbox';
      input.id = 'preferences-notifications-checkbox';
      input.name = 'preferences-notifications-checkbox';
      registration.pushManager.getSubscription()
      .then(sub => {
        input.checked = sub !== null;
      });
      input.addEventListener('change', async () => {
        if (window.Notification && window.Notification.permission !== 'granted' && input.checked) {
          const p = await window.Notification.requestPermission();
          input.checked = p === 'granted';
        }
        main.sendRPC('enableNotifications', input.checked)
        .then(v => {
          input.checked = v;
          this.enableNotifications = v;
        })
        .catch(() => {
          input.checked = false;
          this.enableNotifications = false;
        });
      });
      const label = document.createElement('label');
      label.htmlFor = `preferences-notifications-checkbox`;
      label.innerHTML = _T('opt-enable-notifications');
      notifopt.appendChild(input);
      notifopt.appendChild(label);
      content.appendChild(notifopt);
    });
  }

  async showAdminConsole_() {
    const data = await main.sendRPC('adminUsers');
    const {content} = this.commonPopup_({
      title: _T('admin-console'),
      className: 'popup admin-console-popup',
    });
    return this.showAdminConsoleData_(content, data);
  }

  async showAdminConsoleData_(content, data) {
    while (content.firstChild) {
      content.removeChild(content.firstChild);
    }
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
    const saveButton = document.createElement('button');
    saveButton.id = 'admin-console-save-button';
    saveButton.textContent = _T('no-changes');
    saveButton.disabled = true;
    saveButton.addEventListener('click', () => {
      const c = changes();
      content.querySelectorAll('input,select').forEach(elem => {
        elem.disabled = true;
      });
      main.sendRPC('adminUsers', c)
      .then(data => {
        this.popupMessage(_T('data-updated'), 'info');
        return this.showAdminConsoleData_(content, data);
      })
      .finally(() => {
        content.querySelectorAll('input,select').forEach(elem => {
          elem.disabled = false;
        });
      });
    });
    content.appendChild(saveButton);

    const defQuotaDiv = document.createElement('div');
    defQuotaDiv.id = 'admin-console-default-quota-div';
    const defQuotaLabel = document.createElement('label');
    defQuotaLabel.htmlFor = 'admin-console-default-quota-value';
    defQuotaLabel.textContent = 'Default quota:';
    defQuotaDiv.appendChild(defQuotaLabel);
    const defQuotaValue = document.createElement('input');
    defQuotaValue.id = 'admin-console-default-quota-value';
    defQuotaValue.type = 'number';
    defQuotaValue.size = 5;
    defQuotaValue.value = data.defaultQuota;
    defQuotaValue.addEventListener('change', () => {
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
    defQuotaDiv.appendChild(defQuotaValue);
    const defQuotaUnit = document.createElement('select');
    for (let u of ['','MB','GB','TB']) {
      const opt = document.createElement('option');
      opt.value = u;
      opt.textContent = u === '' ? '' : _T(u);
      opt.selected = u === data.defaultQuotaUnit;
      defQuotaUnit.appendChild(opt);
    }
    defQuotaUnit.addEventListener('change', () => {
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
    defQuotaDiv.appendChild(defQuotaUnit);
    content.appendChild(defQuotaDiv);

    const filter = document.createElement('input');
    filter.id = 'admin-console-filter';
    filter.type = 'search';
    filter.placeholder = _T('filter');
    filter.addEventListener('keyup', () => {
      showUsers();
    });
    content.appendChild(filter);

    const view = {};
    for (let user of data.users) {
      view[user.email] = [];

      const email = document.createElement('div');
      email.textContent = user.email;
      view[user.email].push(email);

      const lockedDiv = document.createElement('div');
      const locked = document.createElement('input');
      locked.type = 'checkbox';
      locked.checked = user.locked;
      locked.addEventListener('change', () => {
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
      lockedDiv.appendChild(locked);
      view[user.email].push(lockedDiv);

      const approvedDiv = document.createElement('div');
      const approved = document.createElement('input');
      approved.type = 'checkbox';
      approved.checked = user.approved;
      approved.addEventListener('change', () => {
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
      approvedDiv.appendChild(approved);
      view[user.email].push(approvedDiv);

      const adminDiv = document.createElement('div');
      const admin = document.createElement('input');
      admin.type = 'checkbox';
      admin.checked = user.admin;
      admin.addEventListener('change', () => {
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
      adminDiv.appendChild(admin);
      view[user.email].push(adminDiv);

      const quotaDiv = document.createElement('div');
      quotaDiv.className = 'quota-cell';
      const quotaValue = document.createElement('input');
      quotaValue.type = 'number';
      quotaValue.size = 5;
      quotaValue.value = user.quota;
      quotaValue.addEventListener('change', () => {
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
      quotaDiv.appendChild(quotaValue);
      const quotaUnit = document.createElement('select');
      for (let u of ['','MB','GB','TB']) {
        const opt = document.createElement('option');
        opt.value = u;
        opt.textContent = u === '' ? '' : _T(u);
        opt.selected = u === user.quotaUnit;
        quotaUnit.appendChild(opt);
      }
      quotaUnit.addEventListener('change', () => {
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

    const table = document.createElement('div');
    table.id = 'admin-console-table';
    content.appendChild(table);

    const showUsers = () => {
      while (table.firstChild) {
        table.removeChild(table.firstChild);
      }
      table.innerHTML = `<div>${_T('email')}</div><div>${_T('locked')}</div><div>${_T('approved')}</div><div>${_T('admin')}</div><div>${_T('quota')}</div>`;
      for (let user of data.users) {
        if (filter.value === '' || user.email.includes(filter.value) || Object.keys(user).filter(k => k.startsWith('_')).length > 0) {
          view[user.email].forEach(e => table.appendChild(e));
        }
      }
    };
    showUsers();
  }
}

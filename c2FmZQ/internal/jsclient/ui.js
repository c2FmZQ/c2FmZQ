/*
 * Copyright 2021-2022 Robin Thellend
 *
 * This file is part of c2FmZQ.
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
'use strict';

class UI {
  static SHOW_ITEMS_INCREMENT = 10;

  constructor() {
    this.uiStarted_ = false;
    this.promptingForPassphrase_ = false;
    this.addingFiles_ = false;
    this.popupZ_ = 1000;
    this.galleryState_ = {
      collection: main.getHash('collection', 'gallery'),
      files: [],
      lastDate: '',
      shown: UI.SHOW_ITEMS_INCREMENT,
    };

    this.passphraseInput_ = document.getElementById('passphrase-input');
    this.setPassphraseButton_ = document.getElementById('set-passphrase-button');
    this.showPassphraseButton_ = document.getElementById('show-passphrase-button');
    this.resetDbButton_ = document.getElementById('resetdb-button');

    this.emailInput_ = document.getElementById('email-input');
    this.passwordInput_ = document.getElementById('password-input');
    this.loginButton_ = document.getElementById('login-button');
    this.refreshButton_ = document.getElementById('refresh-button');
    this.trashButton_ = document.getElementById('trash-button');
    this.logoutButton_ = document.getElementById('logout-button');

    this.passphraseInput_.addEventListener('keyup', e => {
      if (e.key === 'Enter') {
        this.setPassphrase_();
      }
    });
    this.setPassphraseButton_.addEventListener('click', this.setPassphrase_.bind(this));
    this.showPassphraseButton_.addEventListener('click', e => {
      if (this.passphraseInput_.type === 'text') {
        this.passphraseInput_.type = 'password';
        this.showPassphraseButton_.textContent = 'Show';
      } else {
        this.passphraseInput_.type = 'text';
        this.showPassphraseButton_.textContent = 'Hide';
      }
    });
    this.resetDbButton_.addEventListener('click', main.resetServiceWorker.bind(main));
  }

  promptForPassphrase() {
    this.promptingForPassphrase_ = true;
    this.setPassphraseButton_.textContent = 'Set';
    this.setPassphraseButton_.disabled = false;
    this.passphraseInput_.disabled = false;
    this.showPassphraseBox_();
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
    this.resetDbButton_.className = 'resetdb-button';
    this.popupMessage('Error', err, 'error');
  }

  startUI() {
    console.log('Start UI');
    if (this.uiStarted_) {
      return;
    }
    this.uiStarted_ = true;

    window.addEventListener('scroll', this.onScroll_.bind(this));
    window.addEventListener('resize', this.onScroll_.bind(this));
    this.trashButton_.addEventListener('click', () => {
      this.switchView_({collection: 'trash'});
    });

    this.loginButton_.addEventListener('click', this.login_.bind(this));
    this.refreshButton_.addEventListener('click', this.refresh_.bind(this));
    this.logoutButton_.addEventListener('click', this.logout_.bind(this));
    this.emailInput_.addEventListener('keyup', e => {
      if (e.key === 'Enter') {
        this.passwordInput_.focus();
      }
    });
    this.passwordInput_.addEventListener('keyup', e => {
      if (e.key === 'Enter') {
        this.login_();
      }
    });

    main.sendRPC('isLoggedIn')
    .then(account => {
      if (account !== '') {
        document.getElementById('loggedin-account').textContent = account;
        this.showLoggedIn_();
        main.sendRPC('getUpdates')
          .catch(this.showError_.bind(this))
          .finally(this.refreshGallery_.bind(this));
      } else {
        this.showLoggedOut_();
      }
    })
    .catch(this.showLoggedOut_.bind(this));
  }

  popupMessage(title, message, className) {
    const div = document.createElement('div');
    div.className = className;
    div.style.zIndex = this.popupZ_++;
    const v = document.createElement('span');
    v.textContent = '✖';
    v.style = 'float: right;';
    const t = document.createElement('div');
    t.textContent = title;
    const m = document.createElement('div');
    m.textContent = message;
    div.appendChild(v);
    div.appendChild(t);
    div.appendChild(m);

    const remove = () => {
      div.style.animation = 'slideOut 1s';
      div.addEventListener('animationend', () => {
        body.removeChild(div);
      });
    };
    const body = document.getElementsByTagName('body')[0];
    div.addEventListener('click', remove);
    body.appendChild(div);

    setTimeout(remove, 5000);
  }

  showError_(e) {
    console.log('Show Error', e);
    console.trace();
    this.popupMessage('ERROR', e.toString(), 'error');
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
    document.getElementById('loggedout-div').className = 'hidden';
    document.getElementById('loggedin-div').className = 'hidden';
    document.getElementById('passphrase-div').className = '';
    this.passphraseInput_.focus();
  }

  showLoggedIn_() {
    document.getElementById('loggedout-div').className = 'hidden';
    document.getElementById('passphrase-div').className = 'hidden';
    document.getElementById('loggedin-div').className = '';
    this.clearView_();
  }

  showLoggedOut_() {
    this.clearView_();
    document.getElementById('loggedin-div').className = 'hidden';
    document.getElementById('passphrase-div').className = 'hidden';
    document.getElementById('loggedout-div').className = '';
    this.emailInput_.focus();
  }

  async login_() {
    let old = this.loginButton_.textContent;
    this.loginButton_.textContent = 'Logging in';
    this.loginButton_.disabled = true;
    this.emailInput_.disabled = true;
    this.passwordInput_.disabled = true;
    return main.sendRPC('login', this.emailInput_.value, this.passwordInput_.value)
    .then(() => {
      document.getElementById('loggedin-account').textContent = this.emailInput_.value;
      this.passwordInput_.value = '';
      this.showLoggedIn_();
      return main.sendRPC('getUpdates');
    })
    .then(() => {
      this.refreshGallery_();
    })
    .catch(e => {
      if (e !== 'nok') {
        this.showError_(e);
      }
    })
    .finally(() => {
      this.loginButton_.textContent = old;
      this.loginButton_.disabled = false;
      this.emailInput_.disabled = false;
      this.passwordInput_.disabled = false;
    });
  }

  async logout_() {
    let old = this.logoutButton_.textContent;
    this.logoutButton_.textContent = 'Logging out';
    this.logoutButton_.disabled = true;
    return main.sendRPC('logout')
    .then(() => {
      this.showLoggedOut_();
    })
    .finally(() => {
      this.logoutButton_.textContent = old;
      this.logoutButton_.disabled = false;
    });
  }

  async refresh_() {
    this.refreshButton_.disabled = true;
    this.refreshButton_.textContent = 'Refreshing';
    return main.sendRPC('getUpdates')
      .then(this.refreshGallery_.bind(this))
      .catch(this.showError_.bind(this))
      .finally(() => {
        this.refreshButton_.textContent = 'Refresh';
        this.refreshButton_.disabled = false;
      });
  }

  onScroll_() {
    const distanceToBottom = Math.floor(document.documentElement.scrollHeight - document.documentElement.scrollTop - window.innerHeight);
    if (distanceToBottom < 200 && !this.addingFiles_) {
      this.addingFiles_ = true;
      window.requestAnimationFrame(() => {
        this.showMoreFiles_(UI.SHOW_ITEMS_INCREMENT)
        .then(() => {
          this.addingFiles_ = false;
        });
      });
    }
  }

  switchView_(c) {
    this.galleryState_.collection = c.collection;
    this.galleryState_.shown = UI.SHOW_ITEMS_INCREMENT;
    main.setHash('collection', c.collection);
    this.refreshGallery_();
  }

  async refreshGallery_() {
    const collections = await main.sendRPC('getCollections');
    this.galleryState_.content = await main.sendRPC('getFiles', this.galleryState_.collection);
    if (!this.galleryState_.content) {
      this.galleryState_.content = {'total': 0, 'files': []};
    }
    const oldScrollLeft = document.getElementById('collections')?.scrollLeft;

    let g = document.getElementById('gallery');
    while (g.firstChild) {
      g.removeChild(g.firstChild);
    }
    const collectionDiv = document.createElement('div');
    collectionDiv.id = 'collections';
    g.appendChild(collectionDiv);

    let collectionName = '';
    let members = [];
    let scrollTo = null;

    for (let i in collections) {
      if (!collections.hasOwnProperty(i)) {
        continue;
      }
      const div = document.createElement('div');
      div.className = 'collectionThumbdiv';
      const c = collections[i];
      if (c.collection === 'trash' && this.galleryState_.collection !== c.collection) {
        continue;
      }
      if (this.galleryState_.collection === c.collection) {
        collectionName = c.name;
        members = c.members;
        scrollTo = div;
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
      img.addEventListener('click', e => {
        this.switchView_(c);
      });
      imgdiv.appendChild(img);
      div.appendChild(imgdiv);
      const n = document.createElement('div');
      n.className = 'collectionThumbLabel';
      n.style.width = sz;
      n.textContent = c.name;
      div.appendChild(n);
      collectionDiv.appendChild(div);
    }

    const br = document.createElement('br');
    br.clear = 'all';
    g.appendChild(br);
    const h1 = document.createElement('h1');
    h1.textContent = 'Collection: ' + collectionName;
    g.appendChild(h1);
    if (members?.length > 0) {
      const div = document.createElement('div');
      div.textContent = 'Shared with ' + members.join(', ');
      g.appendChild(div);
    }

    this.galleryState_.lastDate = '';
    const n = this.galleryState_.shown;
    this.galleryState_.shown = 0;
    this.showMoreFiles_(n);
    if (scrollTo) {
      if (oldScrollLeft) {
        collectionDiv.scrollLeft = oldScrollLeft;
      }
      setTimeout(() => {
        if (oldScrollLeft) collectionDiv.scrollLeft = oldScrollLeft;
        scrollTo.scrollIntoView({behavior: 'smooth', block: 'end', inline: 'center'});
      });
    }
  }

  static px_(n) {
    return ''+Math.floor(n / window.devicePixelRatio)+'px';
  }

  async showMoreFiles_(n) {
    if (!this.galleryState_.content) {
      return;
    }
    const g = document.getElementById('gallery');
    const max = Math.min(this.galleryState_.shown + n, this.galleryState_.content.total);

    if (max > this.galleryState_.content.files.length) {
      let ff = await main.sendRPC('getFiles', this.galleryState_.collection, this.galleryState_.content.files.length);
      if (ff) {
        this.galleryState_.content.files.push(...ff.files);
      }
    }

    for (let i = this.galleryState_.shown; i < this.galleryState_.content.files.length && i < max; i++) {
      this.galleryState_.shown++;
      const f = this.galleryState_.content.files[i];
      const date = (new Date(f.dateCreated)).toLocaleDateString();
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
      d.addEventListener('click', () => {
        this.setUpPopup_(f);
      });
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
      g.appendChild(d);
    }
  }

  setUpPopup_(f) {
    const popup = document.createElement('div');
    const popupBlur = document.createElement('div');
    const popupHeader = document.createElement('div');
    const popupName = document.createElement('div');
    const popupClose = document.createElement('div');
    const popupContent = document.createElement('div');
    popup.className = 'popup';
    popupBlur.className = 'blur';
    popupHeader.className = 'popup-header';
    popupName.className = 'popup-name';
    popupName.textContent = f.fileName;
    popupClose.className = 'popup-close';
    popupClose.textContent = '✖';

    popupHeader.appendChild(popupName);
    popupHeader.appendChild(popupClose);
    popup.appendChild(popupHeader);
    popup.appendChild(popupContent);

    if (f.isImage) {
      const img = new Image();
      img.className = 'popup-media';
      img.alt = f.fileName;
      img.src = f.url;
      popupContent.appendChild(img);
    }
    if (f.isVideo) {
      const video = document.createElement('video');
      video.className = 'popup-media';
      video.src = f.url;
      video.poster = f.thumbUrl;
      video.controls = 'controls';
      popupContent.appendChild(video);
    }

    let closePopup;
    const handleClickClose = () => {
      closePopup();
    };
    const handleEscape = e => {
      if (e.key === 'Escape') {
        closePopup();
      }
    };
    const handleClickOutside = e => {
      if (!e.composedPath().includes(popup)) {
        e.stopPropagation();
        closePopup();
      }
    };
    // Add handlers.
    popupClose.addEventListener('click', handleClickClose);
    document.addEventListener('keyup', handleEscape);
    setTimeout(() => {
      document.addEventListener('click', handleClickOutside, true);
    });

    const g = document.getElementById('gallery');
    closePopup = () => {
      // Remove handlers.
      popupClose.removeEventListener('click', handleClickClose);
      document.removeEventListener('keyup', handleEscape);
      document.removeEventListener('click', handleClickOutside, true);
      popup.style.animation = 'fadeOut 0.25s';
      popup.addEventListener('animationend', () => {
        g.removeChild(popup);
      });
      popupBlur.style.animation = 'fadeOut 0.25s';
      popupBlur.addEventListener('animationend', () => {
        g.removeChild(popupBlur);
      });
    };
    g.appendChild(popupBlur);
    g.appendChild(popup);
  }

  formatDuration_(d) {
    const min = Math.floor(d / 60);
    const sec = d % 60;
    return '' + min + ':' + ('00'+sec).slice(-2);
  }
}

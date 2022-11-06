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
'use strict';

window.Lang = {
  init: () => {
    for (let lang of navigator.languages) {
      if (Lang.dict[lang]) {
        Lang.current = lang;
      }
    }
    const saved = localStorage.getItem('lang');
    if (saved && Lang.dict[saved] !== undefined) {
      Lang.current = saved;
    }
  },

  current: 'en-US',

  languages: () => {
    let out = {};
    Object.keys(Lang.dict).forEach(x => {
      out[x] = Lang.dict[x].LANG;
    });
    return out;
  },

  text: (key, ...args) => {
    if (Lang.dict[Lang.current]) {
      let v = Lang.dict[Lang.current][key];
      if (v) return Lang.sub(v, args);
    }
    const lang = Lang.current.split('-')[0];
    if (Lang.dict[lang]) {
      let v = Lang.dict[lang][key];
      if (v) return Lang.sub(v, args);
    }
    console.log('Missing lang key', Lang.current, key);
    let v = Lang.dict.en[key];
    if (v) return Lang.sub(v, args);
    console.log('Missing lang key', 'en', key);
    return `::${key}:${args.join(' ')}::`;
  },

  sub: (s, args) => {
    for (let i = 0; i < args.length; i++) {
      s = s.replace(`$${i+1}`, args[i]);
    }
    return s;
  },

  dict: {
    'en': {
      'LANG': 'English',
      'login': 'Login',
      'logging-in': 'Logging in',
      'login-failed': 'Login failed',
      'register': 'Register',
      'create-account': 'Create Account',
      'creating-account': 'Creating Account',
      'create-account-failed': 'Account creation failed',
      'recover': 'Recover',
      'recover-account': 'Recover Account',
      'recovering-account': 'Recovering Account',
      'recover-account-failed': 'Account recovery failed',
      'form-email': 'Email:',
      'form-password': 'Password:',
      'form-current-password': 'Current password:',
      'form-new-password': 'New password:',
      'form-confirm-password': 'Confirm password:',
      'form-otp-code': 'OTP code:',
      'form-backup-phrase': 'Backup phrase:',
      'form-backup-keys?': 'Backup keys?',
      'form-server': 'Server:',
      'server-placeholder': 'https://your-server-name/',
      'show': 'Show',
      'hide': 'Hide',
      'skip-passphrase-warning': 'Skipping the passphrase is less secure. Never used this option on a public or shared computer. Continue?',
      'enter-backup-phrase': 'Enter backup phrase:',
      'B': '$1 B',
      'KiB': '$1 KiB',
      'MiB': '$1 MiB',
      'GiB': '$1 GiB',
      'TiB': '$1 TiB',
      'MB': 'MB',
      'GB': 'GB',
      'TB': 'TB',
      'gallery': 'gallery',
      'trash': 'trash',
      'cancel': 'Cancel',
      'done': 'Done',
      'ready': 'Ready',
      'drop-received': 'Processing new files',
      'upload:': 'Upload: $1',
      'collection:': 'Collection: $1',
      'shared-with': 'Shared with $1',
      'empty': 'Empty',
      'name': 'Name',
      'name:': 'Name: $1',
      'size:': 'Size: $1',
      'remove': 'Remove',
      'thumbnail-progress': 'Preparing: $1',
      'status:': 'Status: $1',
      'upload-progress': 'Upload: $1',
      'upload-files': 'Upload files',
      'upload': 'Upload',
      'uploading': 'Uploading',
      'select-upload': 'Select files to upload (or drag & drop files anywhere):',
      'profile': 'Profile',
      'required': 'required',
      'optional': 'optional',
      'enable-otp?': 'Enable OTP?',
      'enter-code': 'Enter code',
      'update': 'Update',
      'updating': 'Updating',
      'updated': 'Updated',
      'error': 'Error',
      'current-password-required': 'Current password is required',
      'new-pass-doesnt-match': 'New password doesn\'t match',
      'otp-code-required': 'OTP code required',
      'delete-warning': '<p>If you delete your account, all your data will be permanently deleted.</p>',
      'delete-account': 'Delete my account',
      'confirm-delete-account': 'Are you sure you want to permanently delete your account?\nThe operation is not reversible.',
      'key-backup': 'Key backup',
      'enter-current-password': 'Enter you account password:',
      'key-backup-warning': '<p>The backup phrase is your <b>unencrypted</b> secret key.</p><p>It is the most sensitive information in your account. It can be used to recover your account and your data if your secret key is not backed up on the server, or if you forget your password.</p><p>It can also be used to <b>TAKE OVER</b> your account.</p><p>It MUST be kept secret. Write it down on a piece of paper and store it in a safe.</p>',
      'show-backup-phrase': 'Show backup phrase',
      'hide-backup-phrase': 'Hide backup phrase',
      'checking-password': 'Checking password',
      'key-backup-warning2': '<p>Choose whether to keep an encrypted backup of your secret key on the server.<p><p>If you do NOT keep a backup, you will need to enter your backup phrase every time you login to your account.</p>',
      'enabled': 'Enabled',
      'disabled': 'Disabled',
      'opt-keep-backup': 'Keep a backup on the server (RECOMMENDED)',
      'opt-dont-keep-backup': 'Do NOT keep a backup on the server',
      'prefs': 'Preferences',
      'choose-cache-pref': '<h1>Choose your cache preference:</h1>',
      'opt-encrypted': 'Store thumbnails in a local encrypted database. Thumbnails are stored securely on the local system, along with the App\'s metadata. The other files, e.g. photos and videos, are not cached. This option might lead to quota issues. The cache is cleared on logout or when another option is selected. (DEFAULT)',
      'opt-no-store': 'Disable caching. All files, including thumbnails, are fetched and decrypted each time they are accessed. This is the slowest option.',
      'opt-private': 'Use the default browser cache. Decrypted files are stored in the browser\'s cache. This is the fastest option, but also the most likely the leak information.',
      'cache-pref': 'Cache preference',
      'saved': 'saved',
      'admin-console': 'Admin console',
      'save-changes': 'Save changes',
      'email': 'Email',
      'locked': 'Locked',
      'approved': 'Approved',
      'admin': 'Admin',
      'quota': 'Quota',
      'open': 'Open',
      'open-doc': 'Open document',
      'copy-selected': 'Copy selected files',
      'move-selected': 'Move selected files',
      'edit': 'Edit',
      'edit:': 'Edit: $1',
      'select': 'Select',
      'unselect': 'Unselect',
      'unselect-all': 'Unselect all',
      'move-to-trash': 'Move to trash',
      'confirm-move-to-trash': 'Move to trash?',
      'move-to-gallery': 'Move to gallery',
      'confirm-move-to-gallery': 'Move to gallery?',
      'delete-perm': 'Delete permanently',
      'confirm-delete-perm': 'Delete permanently?',
      'use-as-cover': 'Use as cover',
      'move-to-gallery-only': 'Must move from trash to gallery',
      'moved-one-file': 'Moved 1 file',
      'moved-many-files': 'Moved $1 files',
      'copied-one-file': 'Copied 1 file',
      'copied-many-files': 'Copied $1 files',
      'deleted-one-file': 'Deleted 1 file',
      'deleted-many-file': 'Deleted $1 files',
      'leave': 'Leave',
      'confirm-leave': 'Are you sure you want to leave this collection?',
      'delete': 'Delete',
      'confirm-delete': 'Are you sure you want to delete this collection?',
      'properties': 'Properties',
      'properties:': 'Properties: $1',
      'create-collection': 'Create collection',
      'new-collection': 'NEW COLLECTION',
      'apply-changes': 'Apply changes',
      'no-changes': 'No changes',
      'shared': 'Shared',
      'yes': 'Yes',
      'no': 'No',
      'permissions': 'Permissions',
      'perm-add': 'Add',
      'perm-share': 'Share',
      'perm-copy': 'Copy',
      'members': 'Members',
      'contact-email': 'contact email',
      'add-member': 'Add',
      'none': 'none',
      'logout': 'Logout',
      'default-cover': 'Default cover',
      'no-cover': 'No cover',
      'data-updated': 'Data updated',
      'network-error': 'offline?',
      'filter': 'filter',
    },
//    'fr': {
//      'LANG': 'Français',
//    },
  },
};

Lang.init();

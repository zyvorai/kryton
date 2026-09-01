// Copyright 2026 Kryton contributors
// SPDX-License-Identifier: Apache-2.0

const cfg = JSON.parse(document.getElementById('console-config').textContent);
const { machine, project, bootError } = cfg;
const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
const url = `${proto}//${location.host}/api/v1/machines/${encodeURIComponent(machine)}/vnc?project=${encodeURIComponent(project)}`;

const err = document.getElementById('err');
const statusEl = document.getElementById('status');
const btnPaste = document.getElementById('btnPaste');
const btnCad = document.getElementById('btnCad');
const screen = document.getElementById('viewport') || document.getElementById('screen');

const show = (title, detail) => {
  err.innerHTML = `<strong>${title}</strong>${detail}`;
  err.classList.add('show');
};
const setStatus = (t) => { statusEl.textContent = t; };
const enableKeys = (on) => {
  btnPaste.disabled = !on;
  btnCad.disabled = !on;
};

let rfb = null;
enableKeys(false);

btnPaste.addEventListener('click', async () => {
  if (!rfb) return;
  try {
    const text = await navigator.clipboard.readText();
    if (!text) {
      setStatus('Clipboard empty');
      return;
    }
    rfb.focus();
    rfb.clipboardPasteFrom(text);
    setStatus(`Pasted ${text.length} chars into guest`);
  } catch {
    setStatus('Clipboard blocked — allow paste permission, or focus the screen and type');
  }
});

btnCad.addEventListener('click', () => {
  if (!rfb) return;
  rfb.focus();
  rfb.sendCtrlAltDel();
  setStatus('Sent Ctrl+Alt+Del');
});

if (bootError) {
  show('Console unavailable', `${bootError} Reload this page when the machine is Running.`);
  setStatus('Guest not ready');
} else {
  setStatus('Loading viewer…');
  let RFB;
  try {
    RFB = (await import('/novnc-rfb.js')).default;
  } catch (e) {
    show('Viewer failed to load', `Could not load the embedded noVNC client (${e && e.message ? e.message : e}). Reload, or check that /novnc-rfb.js is served by krytond.`);
    setStatus('Viewer load failed');
  }
  if (RFB) {
    try {
      setStatus('Connecting to guest VNC…');
      rfb = new RFB(screen, url, { shared: true });
      rfb.scaleViewport = true;
      rfb.resizeSession = true;
      rfb.focusOnClick = true;
      rfb.addEventListener('connect', () => {
        setStatus('Connected — click screen to type');
        enableKeys(true);
        err.classList.remove('show');
        rfb.focus();
      });
      rfb.addEventListener('disconnect', (e) => {
        enableKeys(false);
        const clean = e.detail && e.detail.clean;
        if (!clean) {
          show('Console disconnected', 'The guest VNC session ended. Confirm the VM is Running, then reload.');
        }
        setStatus('Disconnected');
      });
      rfb.addEventListener('securityfailure', () => {
        enableKeys(false);
        show('Console authentication failed', 'KubeVirt rejected the VNC session. Check RBAC for the Kryton service account.');
        setStatus('Auth failed');
      });
      rfb.addEventListener('credentialsrequired', () => {
        // KubeVirt VNC normally needs no password; ignore / empty.
        try { rfb.sendCredentials({ password: '' }); } catch { /* ignore */ }
      });
      rfb.addEventListener('clipboard', (ev) => {
        const text = ev.detail && ev.detail.text;
        if (!text) return;
        navigator.clipboard.writeText(text)
          .then(() => setStatus('Copied from guest to clipboard'))
          .catch(() => setStatus('Guest clipboard updated (browser blocked copy)'));
      });
      // Failsafe: if WS is open but connect is slow, still allow CAD attempts after a short wait.
      setTimeout(() => {
        if (rfb && btnCad.disabled && statusEl.textContent.includes('Connecting')) {
          setStatus('Still connecting… try Reload if this persists');
        }
      }, 8000);
    } catch (e) {
      show('Console failed to start', String(e && e.message || e));
      setStatus('Failed');
    }
  }
}

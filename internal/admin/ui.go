package admin

// setupHTML is the first-run bootstrap page. %s receives the error message.
const setupHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Tunnd — Set up admin</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg:#0b0c0e;--bg2:#111316;--bg3:#181b1f;
  --border:rgba(255,255,255,.08);--border2:rgba(255,255,255,.18);
  --text:#e8e6e0;--muted:#6b6f78;--muted2:#9499a4;
  --accent:#3dffc0;--red:#ff6b6b;--amber:#fbbf24;
  --mono:'Berkeley Mono','Fira Code',monospace;
}
html,body{height:100%}
body{font-family:var(--mono);background:var(--bg);color:var(--text);font-size:13px;display:flex;flex-direction:column;align-items:center;justify-content:center;min-height:100vh;-webkit-font-smoothing:antialiased}
.card{width:380px;max-width:92vw;background:var(--bg2);border:1px solid var(--border2);border-radius:14px;padding:36px 32px}
.logo{display:flex;align-items:center;gap:10px;margin-bottom:28px}
.logo-mark{width:32px;height:32px;background:var(--accent);border-radius:8px;display:flex;align-items:center;justify-content:center;flex-shrink:0}
.logo-mark svg{width:16px;height:16px}
.logo-text{font-size:17px;font-weight:700;color:var(--text)}
.logo-sub{font-size:11px;color:var(--muted);margin-top:1px}
.first-run{display:inline-flex;align-items:center;gap:6px;background:rgba(61,255,192,.08);border:1px solid rgba(61,255,192,.2);border-radius:100px;padding:3px 12px;font-size:11px;color:var(--accent);margin-bottom:18px;letter-spacing:.04em}
h1{font-size:15px;font-weight:700;margin-bottom:6px}
.sub{font-size:12px;color:var(--muted);margin-bottom:24px;line-height:1.6}
.field{margin-bottom:16px}
.field label{display:block;font-size:11px;color:var(--muted);margin-bottom:7px;letter-spacing:.06em;text-transform:uppercase;font-weight:500}
.field input{width:100%;background:var(--bg3);border:1px solid var(--border);border-radius:8px;padding:10px 13px;color:var(--text);font-family:var(--mono);font-size:13px;outline:none;transition:.15s;-webkit-appearance:none}
.field input:focus{border-color:var(--accent);box-shadow:0 0 0 3px rgba(61,255,192,.08)}
.field input::placeholder{color:var(--muted)}
.hint{font-size:11px;color:var(--muted);margin-top:5px}
.err{background:rgba(255,107,107,.08);border:1px solid rgba(255,107,107,.2);border-radius:7px;padding:10px 13px;color:var(--red);font-size:12px;margin-bottom:16px}
.err:empty{display:none}
.btn{width:100%;padding:11px;border-radius:8px;font-size:13px;font-weight:700;border:none;background:var(--accent);color:#0b0c0e;font-family:var(--mono);cursor:pointer;transition:.15s;letter-spacing:.02em}
.btn:hover{background:#5fffcf}
.btn:active{transform:scale(.98)}
.btn:disabled{opacity:.5;cursor:not-allowed}
</style>
</head>
<body>
<div class="card">
  <div class="logo">
    <div class="logo-mark">
      <svg viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
        <path d="M3 12L8 4L13 12" stroke="#0b0c0e" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
    </div>
    <div>
      <div class="logo-text">Tunnd</div>
      <div class="logo-sub">Admin Dashboard</div>
    </div>
  </div>
  <div class="first-run">✦ First run</div>
  <h1>Create admin account</h1>
  <p class="sub">Set a password to secure your dashboard. This is a one-time step — you can change it later from the dashboard settings.</p>
  <div class="err" id="err-msg">%s</div>
  <form method="POST" action="/setup" onsubmit="onSubmit(event)">
    <div class="field">
      <label>Admin Password</label>
      <input type="password" name="password" id="pw" placeholder="••••••••••••" autocomplete="new-password" autofocus required minlength="12">
      <div class="hint">Minimum 12 characters. Store it somewhere safe.</div>
    </div>
    <button class="btn" type="submit" id="btn">Create account →</button>
  </form>
</div>
<script>
function onSubmit(e) {
  const pw = document.getElementById('pw').value;
  if (pw.length < 12) {
    e.preventDefault();
    document.getElementById('err-msg').textContent = 'Password must be at least 12 characters.';
    return;
  }
  const btn = document.getElementById('btn');
  btn.disabled = true;
  btn.textContent = 'Setting up…';
}
const err = document.getElementById('err-msg');
if (err && err.textContent.trim()) {
  setTimeout(() => { err.style.opacity='0'; err.style.transition='opacity .4s'; }, 4000);
}
</script>
</body>
</html>`

// loginHTML is the login page template. %s receives the error message (empty string = no error).
const loginHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Tunnd — Sign in</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg:#0b0c0e;--bg2:#111316;--bg3:#181b1f;
  --border:rgba(255,255,255,.08);--border2:rgba(255,255,255,.18);
  --text:#e8e6e0;--muted:#6b6f78;--muted2:#9499a4;
  --accent:#3dffc0;--red:#ff6b6b;
  --mono:'Berkeley Mono','Fira Code',monospace;
}
html,body{height:100%}
body{font-family:var(--mono);background:var(--bg);color:var(--text);font-size:13px;display:flex;flex-direction:column;align-items:center;justify-content:center;min-height:100vh;-webkit-font-smoothing:antialiased}
.card{width:360px;max-width:92vw;background:var(--bg2);border:1px solid var(--border2);border-radius:14px;padding:36px 32px}
.logo{display:flex;align-items:center;gap:10px;margin-bottom:28px}
.logo-mark{width:32px;height:32px;background:var(--accent);border-radius:8px;display:flex;align-items:center;justify-content:center;flex-shrink:0}
.logo-mark svg{width:16px;height:16px}
.logo-text{font-size:17px;font-weight:700;color:var(--text)}
.logo-sub{font-size:11px;color:var(--muted);margin-top:1px}
h1{font-size:15px;font-weight:700;margin-bottom:6px}
.sub{font-size:12px;color:var(--muted);margin-bottom:24px;line-height:1.5}
.field{margin-bottom:18px}
.field label{display:block;font-size:11px;color:var(--muted);margin-bottom:7px;letter-spacing:.06em;text-transform:uppercase;font-weight:500}
.field input{width:100%;background:var(--bg3);border:1px solid var(--border);border-radius:8px;padding:10px 13px;color:var(--text);font-family:var(--mono);font-size:13px;outline:none;transition:.15s;-webkit-appearance:none}
.field input:focus{border-color:var(--accent);box-shadow:0 0 0 3px rgba(61,255,192,.08)}
.field input::placeholder{color:var(--muted)}
.err{background:rgba(255,107,107,.08);border:1px solid rgba(255,107,107,.2);border-radius:7px;padding:10px 13px;color:var(--red);font-size:12px;margin-bottom:18px;display:flex;align-items:center;gap:8px}
.err:empty{display:none}
.btn{width:100%;padding:11px;border-radius:8px;font-size:13px;font-weight:700;border:none;background:var(--accent);color:#0b0c0e;font-family:var(--mono);cursor:pointer;transition:.15s;letter-spacing:.02em}
.btn:hover{background:#5fffcf}
.btn:active{transform:scale(.98)}
.btn:disabled{opacity:.5;cursor:not-allowed}
.footer{margin-top:14px;text-align:center;font-size:11px;color:var(--muted)}
</style>
</head>
<body>
<div class="card">
  <div class="logo">
    <div class="logo-mark">
      <svg viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
        <path d="M3 12L8 4L13 12" stroke="#0b0c0e" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
    </div>
    <div>
      <div class="logo-text">Tunnd</div>
      <div class="logo-sub">Admin Dashboard</div>
    </div>
  </div>
  <h1>Sign in</h1>
  <p class="sub">Enter your admin password to access the dashboard.</p>
  <div class="err" id="err-msg">%s</div>
  <form method="POST" action="/login" onsubmit="onSubmit(event)">
    <div class="field">
      <label>Password</label>
      <input type="password" name="password" id="pw" placeholder="••••••••••••" autocomplete="current-password" autofocus required>
    </div>
    <button class="btn" type="submit" id="btn">Sign in</button>
  </form>
  <p class="footer">Tunnd self-hosted tunnel server</p>
</div>
<script>
function onSubmit(e) {
  const btn = document.getElementById('btn');
  btn.disabled = true;
  btn.textContent = 'Signing in…';
}
const err = document.getElementById('err-msg');
if (err && err.textContent.trim()) {
  setTimeout(() => { err.style.opacity='0'; err.style.transition='opacity .4s'; }, 4000);
}
</script>
</body>
</html>`

// dashboardHTML is the full admin SPA served after login.
const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Tunnd Admin</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
:root{
  --bg:#0b0c0e;--bg2:#111316;--bg3:#181b1f;
  --border:rgba(255,255,255,.08);--border2:rgba(255,255,255,.14);
  --text:#e8e6e0;--muted:#6b6f78;--muted2:#9499a4;
  --accent:#3dffc0;--red:#ff6b6b;--amber:#fbbf24;--blue:#60a5fa;
  --mono:'Berkeley Mono','Fira Code',monospace;
}
html,body{height:100%}
body{font-family:var(--mono);background:var(--bg);color:var(--text);font-size:13px;min-height:100vh;-webkit-font-smoothing:antialiased}
a{color:var(--accent);text-decoration:none}
button{font-family:var(--mono);cursor:pointer}
/* ── Nav ── */
nav{display:flex;align-items:center;gap:0;padding:0 20px;height:52px;border-bottom:1px solid var(--border);background:rgba(11,12,14,.95);backdrop-filter:blur(12px);position:sticky;top:0;z-index:100}
.logo{display:flex;align-items:center;gap:8px;flex-shrink:0;text-decoration:none}
.logo-mark{width:26px;height:26px;background:var(--accent);border-radius:6px;display:flex;align-items:center;justify-content:center;flex-shrink:0}
.logo-mark svg{width:14px;height:14px}
.logo-text{color:var(--accent);font-weight:700;font-size:14px;letter-spacing:-.01em}
.nav-divider{width:1px;height:18px;background:var(--border2);margin:0 16px;flex-shrink:0}
.nav-links{display:flex;gap:2px}
.nav-link{padding:5px 13px;color:var(--muted2);border-radius:6px;font-size:13px;cursor:pointer;border:none;background:none;transition:.15s;white-space:nowrap}
.nav-link:hover{color:var(--text);background:var(--bg3)}
.nav-link.active{color:var(--accent);background:rgba(61,255,192,.07)}
.nav-right{margin-left:auto;display:flex;align-items:center;gap:10px;flex-shrink:0}
.pill{font-size:12px;color:var(--muted);background:var(--bg3);border:1px solid var(--border);border-radius:100px;padding:3px 11px;white-space:nowrap}
.pill b{color:var(--accent)}
.dot{width:7px;height:7px;border-radius:50%;background:var(--accent);animation:pulse 2s infinite;flex-shrink:0}
@keyframes pulse{0%,100%{opacity:1;transform:scale(1)}50%{opacity:.3;transform:scale(.75)}}
.btn-logout{padding:5px 12px;border-radius:6px;font-size:12px;font-weight:600;border:1px solid var(--border);background:none;color:var(--muted2);transition:.15s;white-space:nowrap}
.btn-logout:hover{border-color:var(--red);color:var(--red);background:rgba(255,107,107,.06)}
/* ── Hamburger (mobile only, hidden on desktop) ── */
.hamburger{display:none;align-items:center;justify-content:center;width:36px;height:36px;background:none;border:1px solid var(--border);border-radius:7px;cursor:pointer;flex-direction:column;gap:4px;padding:0;flex-shrink:0;transition:.15s}
.hamburger:hover{border-color:var(--border2);background:var(--bg3)}
.hamburger span{display:block;width:14px;height:1.5px;background:var(--muted2);border-radius:2px;transition:.25s}
.hamburger.open span:nth-child(1){transform:translateY(5.5px) rotate(45deg)}
.hamburger.open span:nth-child(2){opacity:0;transform:scaleX(0)}
.hamburger.open span:nth-child(3){transform:translateY(-5.5px) rotate(-45deg)}

/* ── Mobile dropdown menu ── */
.mobile-menu{display:none;position:fixed;top:52px;left:0;right:0;z-index:99;background:rgba(11,12,14,.98);backdrop-filter:blur(16px);border-bottom:1px solid var(--border2);padding:10px 16px 14px;flex-direction:column;gap:4px}
.mobile-menu.open{display:flex}
.mobile-menu-link{display:block;width:100%;padding:12px 14px;border-radius:8px;font-size:14px;font-family:var(--mono);color:var(--muted2);background:none;border:none;text-align:left;cursor:pointer;transition:.15s}
.mobile-menu-link:hover{background:var(--bg3);color:var(--text)}
.mobile-menu-link.active{color:var(--accent);background:rgba(61,255,192,.07)}
.mobile-menu-divider{height:1px;background:var(--border);margin:6px 0}
.mobile-menu-signout{display:block;width:100%;padding:12px 14px;border-radius:8px;font-size:14px;font-family:var(--mono);color:var(--red);background:none;border:none;text-align:left;cursor:pointer;transition:.15s}
.mobile-menu-signout:hover{background:rgba(255,107,107,.08)}
/* ── Layout ── */
main{padding:26px 28px;max-width:1140px;margin:0 auto}
.view{display:none}.view.active{display:block}
.ph{display:flex;align-items:flex-start;justify-content:space-between;margin-bottom:22px}
.ph-title{font-size:17px;font-weight:700;letter-spacing:-.02em}
.ph-sub{font-size:12px;color:var(--muted);margin-top:4px}

/* ── Cards ── */
.cards{display:grid;grid-template-columns:repeat(3,1fr);gap:12px;margin-bottom:26px}
.card{background:var(--bg2);border:1px solid var(--border);border-radius:11px;padding:20px 22px}
.card-n{font-size:30px;font-weight:700;letter-spacing:-.04em;color:var(--text)}
.card-n.accent{color:var(--accent)}
.card-l{font-size:12px;color:var(--muted);margin-top:5px}

/* ── Section titles ── */
.section-title{font-size:11px;color:var(--muted);letter-spacing:.07em;text-transform:uppercase;margin-bottom:10px}

/* ── Tables ── */
.tw{background:var(--bg2);border:1px solid var(--border);border-radius:11px;overflow:hidden;margin-bottom:14px}
.tw.scroll{max-height:540px;overflow:auto}
table{width:100%;border-collapse:collapse}
th{text-align:left;color:var(--muted);font-size:11px;letter-spacing:.06em;text-transform:uppercase;padding:10px 16px;border-bottom:1px solid var(--border);font-weight:500;white-space:nowrap;background:var(--bg2);position:sticky;top:0;z-index:1}
td{padding:11px 16px;border-bottom:1px solid var(--border);color:var(--muted2);vertical-align:middle}
tr:last-child td{border-bottom:none}
tr:hover td{background:rgba(255,255,255,.012)}
td.p{color:var(--text);font-weight:500}
td.mono{font-size:12px}
.er td{text-align:center;color:var(--muted);padding:38px;font-size:12px}
.empty{display:flex;flex-direction:column;align-items:center;gap:6px;padding:48px 24px;color:var(--muted);text-align:center}
.empty-title{color:var(--muted2);font-size:13px;font-weight:500}
.empty-sub{font-size:12px;line-height:1.6;max-width:380px}
.empty code{background:var(--bg3);border:1px solid var(--border);border-radius:5px;padding:1px 6px;color:var(--accent)}

/* ── Pagination ── */
.pg{display:flex;align-items:center;gap:10px;flex-wrap:wrap;margin:6px 2px 26px;font-size:12px;color:var(--muted)}
.pg .pg-info{flex:1;min-width:140px}
.pg .pg-spacer{width:6px}
.pg select{background:var(--bg3);border:1px solid var(--border);color:var(--text);font-family:var(--mono);font-size:12px;padding:5px 8px;border-radius:6px;outline:none;cursor:pointer;transition:.15s}
.pg select:hover{border-color:var(--border2)}
.pg select:focus{border-color:var(--accent);box-shadow:0 0 0 3px rgba(61,255,192,.08)}
.pg-btn{padding:5px 11px;font-size:12px;border-radius:6px;border:1px solid var(--border);background:var(--bg3);color:var(--muted2);cursor:pointer;transition:.15s;font-family:var(--mono)}
.pg-btn:hover:not(:disabled){border-color:var(--border2);color:var(--text)}
.pg-btn:disabled{opacity:.4;cursor:not-allowed}

/* ── Toolbars (search + actions above a table) ── */
.tb{display:flex;align-items:center;gap:10px;margin-bottom:10px;flex-wrap:wrap}
.tb-search{position:relative;flex:1;max-width:320px;min-width:200px}
.tb-search input{width:100%;background:var(--bg3);border:1px solid var(--border);border-radius:7px;padding:7px 12px 7px 32px;color:var(--text);font-family:var(--mono);font-size:13px;outline:none;transition:.15s}
.tb-search input:focus{border-color:var(--accent);box-shadow:0 0 0 3px rgba(61,255,192,.08)}
.tb-search input::placeholder{color:var(--muted)}
.tb-search svg{position:absolute;left:10px;top:50%;transform:translateY(-50%);width:14px;height:14px;color:var(--muted);pointer-events:none}
/* ── Badges ── */
.badge{display:inline-flex;align-items:center;font-size:11px;font-weight:700;padding:2px 8px;border-radius:4px;white-space:nowrap}
.bg{background:rgba(61,255,192,.1);color:var(--accent)}
.br{background:rgba(255,107,107,.1);color:var(--red)}
.bb{background:rgba(96,165,250,.1);color:var(--blue)}
.bm{background:rgba(148,153,164,.1);color:var(--muted2)}

/* ── Buttons ── */
.btn{padding:7px 14px;border-radius:7px;font-size:13px;font-weight:600;border:none;transition:.15s;display:inline-flex;align-items:center;gap:6px;cursor:pointer;white-space:nowrap}
.btn-p{background:var(--accent);color:#0b0c0e}.btn-p:hover{background:#5fffcf}
.btn-g{background:transparent;color:var(--muted2);border:1px solid var(--border)}.btn-g:hover{border-color:var(--border2);color:var(--text);background:var(--bg3)}
.btn-d{background:rgba(255,107,107,.08);color:var(--red);border:1px solid rgba(255,107,107,.2)}.btn-d:hover{background:rgba(255,107,107,.16)}
.btn-sm{padding:4px 10px;font-size:12px}

/* ── Modals ── */
.mb{position:fixed;inset:0;background:rgba(0,0,0,.65);display:none;align-items:center;justify-content:center;z-index:200;backdrop-filter:blur(6px)}
.mb.open{display:flex}
.md{background:var(--bg2);border:1px solid var(--border2);border-radius:13px;padding:28px 30px;width:440px;max-width:94vw}
.md h3{font-size:16px;font-weight:700;margin-bottom:5px}
.md p{font-size:13px;color:var(--muted2);margin-bottom:22px;line-height:1.65}
.field{margin-bottom:16px}
.field label{display:block;font-size:11px;color:var(--muted);margin-bottom:7px;letter-spacing:.05em;text-transform:uppercase}
.field input{width:100%;background:var(--bg3);border:1px solid var(--border);border-radius:7px;padding:9px 12px;color:var(--text);font-family:var(--mono);font-size:13px;outline:none;transition:.15s}
.field input:focus{border-color:var(--accent);box-shadow:0 0 0 3px rgba(61,255,192,.08)}
.ma{display:flex;justify-content:flex-end;gap:10px;margin-top:22px}

/* ── Token result box ── */
.tok-box{background:var(--bg);border:1px solid rgba(61,255,192,.2);border-radius:9px;padding:14px 16px;margin:16px 0}
.tok-label{font-size:11px;color:var(--muted);margin-bottom:6px}
.tok-val{color:var(--accent);word-break:break-all;font-weight:700;font-size:12px;line-height:1.75;cursor:pointer;transition:.15s}
.tok-val:hover{opacity:.8}
.tok-copy{font-size:11px;color:var(--muted);margin-top:6px;cursor:pointer}
.tok-copy:hover{color:var(--accent)}
.tok-warn{color:var(--amber);font-size:11px;margin-top:8px;display:flex;align-items:center;gap:5px}

/* ── Responsive ── */
@media(max-width:680px){
  .cards{grid-template-columns:1fr}
  .pill{display:none}
  .dot{display:none}
  .nav-links{display:none}
  .nav-divider{display:none}
  .btn-logout{display:none}
  .hamburger{display:flex}
  main{padding:16px 14px}
}
</style>
</head>
<body>

<!-- ── Mobile dropdown (sits just below nav) ── -->
<div class="mobile-menu" id="mobile-menu">
  <button class="mobile-menu-link active" id="mm-tunnels" onclick="mobileNav('tunnels')">Tunnels</button>
  <button class="mobile-menu-link" id="mm-tokens" onclick="mobileNav('tokens')">Tokens</button>
  <div class="mobile-menu-divider"></div>
  <form method="POST" action="/logout" style="margin:0">
    <button class="mobile-menu-signout" type="submit">Sign out</button>
  </form>
</div>

<nav>
  <a class="logo" href="/">
    <div class="logo-mark">
      <svg viewBox="0 0 16 16" fill="none" xmlns="http://www.w3.org/2000/svg">
        <path d="M3 12L8 4L13 12" stroke="#0b0c0e" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
      </svg>
    </div>
    <span class="logo-text">Tunnd</span>
  </a>
  <div class="nav-divider"></div>
  <div class="nav-links">
    <button class="nav-link active" onclick="showView('tunnels',this)">Tunnels</button>
    <button class="nav-link" onclick="showView('tokens',this)">Tokens</button>
  </div>
  <div class="nav-right">
    <span class="pill">Active <b id="na">—</b></span>
    <span class="pill">Tokens <b id="nt">—</b></span>
    <div class="dot"></div>
    <form method="POST" action="/logout" style="margin:0">
      <button class="btn-logout" type="submit">Sign out</button>
    </form>
    <!-- hamburger: only visible on mobile via CSS -->
    <button class="hamburger" id="hamburger" onclick="toggleMenu()" aria-label="Menu">
      <span></span><span></span><span></span>
    </button>
  </div>
</nav>

<main>

<!-- ── Tunnels view ── -->
<div class="view active" id="view-tunnels">
  <div class="ph">
    <div>
      <div class="ph-title">Tunnels</div>
      <div class="ph-sub">Live sessions and history</div>
    </div>
    <button class="btn btn-g btn-sm" onclick="loadTunnels()">↻ Refresh</button>
  </div>
  <div class="cards">
    <div class="card"><div class="card-n accent" id="s-active">—</div><div class="card-l">Active tunnels</div></div>
    <div class="card"><div class="card-n" id="s-tokens">—</div><div class="card-l">Total tokens</div></div>
    <div class="card"><div class="card-n" id="s-time" style="font-size:18px">—</div><div class="card-l">Server time (UTC)</div></div>
  </div>
  <div class="section-title">Active</div>
  <div class="tw"><table>
    <thead><tr><th>Subdomain</th><th>Protocol</th><th>Public URL</th><th>Status</th></tr></thead>
    <tbody id="tb-active"><tr class="er"><td colspan="4">Loading…</td></tr></tbody>
  </table></div>
  <div class="section-title">History</div>
  <div class="tw scroll"><table>
    <thead><tr><th>Subdomain</th><th>Protocol</th><th>Opened</th><th>Closed</th><th>Status</th></tr></thead>
    <tbody id="tb-history"><tr class="er"><td colspan="5">Loading…</td></tr></tbody>
  </table></div>
  <div class="pg" id="pg-history">
    <div class="pg-info" id="pg-info">—</div>
    <label style="font-size:11px;color:var(--muted)">Per page</label>
    <select id="pg-size">
      <option value="50">50</option>
      <option value="100">100</option>
      <option value="250">250</option>
    </select>
    <div class="pg-spacer"></div>
    <button class="pg-btn" id="pg-prev" onclick="historyPage(-1)">← Prev</button>
    <button class="pg-btn" id="pg-next" onclick="historyPage(1)">Next →</button>
  </div>
</div>

<!-- ── Tokens view ── -->
<div class="view" id="view-tokens">
  <div class="ph">
    <div>
      <div class="ph-title">Auth Tokens</div>
      <div class="ph-sub">Manage client authentication tokens</div>
    </div>
    <button class="btn btn-p" onclick="openCreate()">+ New Token</button>
  </div>
  <div class="tb">
    <div class="tb-search">
      <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="7" cy="7" r="5"/><path d="M11 11l3 3"/></svg>
      <input id="tk-search" type="text" placeholder="Filter by label or token…" oninput="filterTokens()" autocomplete="off">
    </div>
    <button class="btn btn-g btn-sm" onclick="loadTokens()">↻ Refresh</button>
  </div>
  <div class="tw scroll"><table>
    <thead><tr><th>Label</th><th>Token</th><th>Max Tunnels</th><th>Status</th><th></th></tr></thead>
    <tbody id="tb-tokens"><tr class="er"><td colspan="5">Loading…</td></tr></tbody>
  </table></div>
  <p style="font-size:11px;color:var(--muted);margin-top:10px">
    Tokens are shown only at creation time and never stored in plain text on disk. The hints above are the first 16 characters — if you've lost a token, revoke it and create a new one.
  </p>
</div>

</main>

<!-- ── Create token modal ── -->
<div class="mb" id="m-create">
  <div class="md">
    <h3>Create Auth Token</h3>
    <p>Tokens authenticate client tunnel connections. Copy the value immediately — it won't be shown again.</p>
    <div class="field"><label>Label</label><input id="f-label" placeholder="e.g. my-laptop" autocomplete="off"></div>
    <div class="field">
      <label>Max Tunnels <span style="color:var(--muted);font-size:11px;text-transform:none">(0 = unlimited)</span></label>
      <input id="f-max" type="number" value="0" min="0">
    </div>
    <div id="tok-result" style="display:none">
      <div class="tok-box">
        <div class="tok-label">Your new token — save this now:</div>
        <div class="tok-val" id="tok-val" onclick="copyToken()" title="Click to copy"></div>
        <div class="tok-copy" onclick="copyToken()" id="copy-hint">Click to copy</div>
        <div class="tok-warn">⚠ This value will not be shown again.</div>
      </div>
    </div>
    <div class="ma">
      <button class="btn btn-g" onclick="closeCreate()">Close</button>
      <button class="btn btn-p" id="btn-create" onclick="doCreate()">Create Token</button>
    </div>
  </div>
</div>

<!-- ── Revoke token modal ── -->
<div class="mb" id="m-revoke">
  <div class="md">
    <h3>Revoke Token?</h3>
    <p>This will immediately invalidate the token. Any client currently using it will be disconnected.</p>
    <div class="ma">
      <button class="btn btn-g" onclick="closeRevoke()">Cancel</button>
      <button class="btn btn-d" id="btn-revoke" onclick="doRevoke()">Revoke Token</button>
    </div>
  </div>
</div>

<script>
let revokeId = null;
let menuOpen = false;
let activeView = 'tunnels';

// Pagination state for the tunnel history table.
const histState = {
  offset: 0,
  size: 50,
  total: 0,
};

// Cache of the latest tokens fetched from the server, used for the
// client-side search filter on the Tokens view.
let tokensCache = [];

// Timers for auto-refresh. Cleared on view switch so we don't keep
// hammering an endpoint the user isn't looking at.
let activeRefreshTimer = null;
let statsRefreshTimer = null;

function toggleMenu() {
  menuOpen = !menuOpen;
  document.getElementById('mobile-menu').classList.toggle('open', menuOpen);
  document.getElementById('hamburger').classList.toggle('open', menuOpen);
}

function closeMenu() {
  menuOpen = false;
  document.getElementById('mobile-menu').classList.remove('open');
  document.getElementById('hamburger').classList.remove('open');
}

function mobileNav(name) {
  closeMenu();
  document.getElementById('mm-tunnels').classList.toggle('active', name === 'tunnels');
  document.getElementById('mm-tokens').classList.toggle('active', name === 'tokens');
  const desktopBtns = document.querySelectorAll('.nav-link');
  desktopBtns.forEach((b, i) => b.classList.toggle('active', (i === 0 && name === 'tunnels') || (i === 1 && name === 'tokens')));
  switchView(name);
}

function showView(name, btn) {
  document.querySelectorAll('.nav-link').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  document.getElementById('mm-tunnels').classList.toggle('active', name === 'tunnels');
  document.getElementById('mm-tokens').classList.toggle('active', name === 'tokens');
  switchView(name);
}

function switchView(name) {
  document.querySelectorAll('.view').forEach(v => v.classList.remove('active'));
  document.getElementById('view-' + name).classList.add('active');
  activeView = name;
  stopRefresh();
  if (name === 'tunnels') {
    loadTunnels();
    startTunnelsRefresh();
  } else {
    loadTokens();
  }
}

document.addEventListener('click', e => {
  if (menuOpen && !document.getElementById('mobile-menu').contains(e.target) && !document.getElementById('hamburger').contains(e.target)) {
    closeMenu();
  }
});

// Stop visibility-change pump while the tab is hidden so we don't keep
// polling in the background.
document.addEventListener('visibilitychange', () => {
  if (document.hidden) {
    stopRefresh();
  } else if (activeView === 'tunnels') {
    startTunnelsRefresh();
  }
});

function startTunnelsRefresh() {
  stopRefresh();
  // Active tunnels + stats refresh every 5s — feels live without
  // being noisy.
  activeRefreshTimer = setInterval(() => {
    refreshActiveTunnels();
    loadStats();
  }, 5000);
  statsRefreshTimer = null;
}

function stopRefresh() {
  if (activeRefreshTimer) clearInterval(activeRefreshTimer);
  if (statsRefreshTimer) clearInterval(statsRefreshTimer);
  activeRefreshTimer = null;
  statsRefreshTimer = null;
}

async function api(path, opts = {}) {
  const res = await fetch('/api' + path, {
    headers: {'Content-Type': 'application/json'},
    credentials: 'same-origin',
    ...opts
  });
  if (res.status === 401) { window.location.href = '/login'; return {}; }
  return res.json();
}

async function loadStats() {
  try {
    const d = await api('/stats');
    if (!d.server_time) return;
    document.getElementById('s-active').textContent = d.active_tunnels ?? '—';
    document.getElementById('s-tokens').textContent = d.total_tokens ?? '—';
    document.getElementById('s-time').textContent = new Date(d.server_time).toLocaleTimeString();
    document.getElementById('na').textContent = d.active_tunnels ?? '—';
    document.getElementById('nt').textContent = d.total_tokens ?? '—';
  } catch(_) {}
}

function emptyRow(cols, title, sub) {
  const subHtml = sub ? '<div class="empty-sub">'+sub+'</div>' : '';
  return '<tr class="er"><td colspan="'+cols+'"><div class="empty"><div class="empty-title">'+title+'</div>'+subHtml+'</div></td></tr>';
}

function renderActiveTunnels(tunnels) {
  const tb = document.getElementById('tb-active');
  if (!tunnels || !tunnels.length) {
    tb.innerHTML = emptyRow(4,
      'No active tunnels right now',
      'Open one from a client with <code>tunnd http &lt;port&gt;</code> or <code>tunnd tcp &lt;port&gt;</code>.');
    return;
  }
  tb.innerHTML = tunnels.map(t =>
    '<tr><td class="p mono">'+esc(t.subdomain)+'</td>' +
    '<td><span class="badge bb">'+esc(t.protocol.toUpperCase())+'</span></td>' +
    '<td><a href="'+esc(t.public_url)+'" target="_blank" rel="noopener" style="color:var(--blue);font-size:12px">'+esc(t.public_url)+'</a></td>' +
    '<td><span class="badge bg">● Live</span></td></tr>').join('');
}

async function refreshActiveTunnels() {
  try {
    const d = await api('/tunnels/active');
    renderActiveTunnels(d.tunnels);
  } catch(_) {}
}

async function loadTunnels() {
  await loadStats();
  await refreshActiveTunnels();
  await loadHistory();
}

async function loadHistory() {
  try {
    const q = '?limit=' + histState.size + '&offset=' + histState.offset;
    const d = await api('/tunnels' + q);
    histState.total = d.total ?? 0;
    const tb = document.getElementById('tb-history');
    if (!d.tunnels || !d.tunnels.length) {
      tb.innerHTML = emptyRow(5,
        'No tunnel history yet',
        'Closed tunnels will appear here. Open one from a client to get started.');
    } else {
      tb.innerHTML = d.tunnels.map(t => {
        const o = new Date(t.opened_at).toLocaleString();
        const c = t.closed_at ? new Date(t.closed_at).toLocaleString() : '—';
        const badge = t.closed_at ? '<span class="badge bm">Closed</span>' : '<span class="badge bg">● Live</span>';
        return '<tr><td class="p mono">'+esc(t.subdomain)+'</td>' +
          '<td><span class="badge bb">'+esc(t.protocol.toUpperCase())+'</span></td>' +
          '<td class="mono">'+o+'</td><td class="mono">'+c+'</td><td>'+badge+'</td></tr>';
      }).join('');
    }
    updatePaginationControls();
  } catch(_) {}
}

function updatePaginationControls() {
  const total = histState.total;
  const start = total === 0 ? 0 : histState.offset + 1;
  const end = Math.min(histState.offset + histState.size, total);
  document.getElementById('pg-info').textContent = total === 0
    ? 'No history rows yet'
    : 'Showing ' + start + '–' + end + ' of ' + total;
  document.getElementById('pg-prev').disabled = histState.offset <= 0;
  document.getElementById('pg-next').disabled = end >= total;
}

function historyPage(dir) {
  const next = histState.offset + dir * histState.size;
  if (next < 0 || next >= histState.total) return;
  histState.offset = next;
  loadHistory();
}

document.getElementById('pg-size').addEventListener('change', e => {
  histState.size = parseInt(e.target.value, 10) || 50;
  histState.offset = 0;
  loadHistory();
});

async function loadTokens() {
  try {
    const d = await api('/tokens');
    tokensCache = d.tokens || [];
    renderTokens();
  } catch(_) {}
}

function renderTokens() {
  const q = (document.getElementById('tk-search').value || '').trim().toLowerCase();
  const list = q
    ? tokensCache.filter(t =>
        (t.label || '').toLowerCase().includes(q) ||
        (t.value_hint || '').toLowerCase().includes(q))
    : tokensCache;
  const tb = document.getElementById('tb-tokens');
  if (!tokensCache.length) {
    tb.innerHTML = emptyRow(5,
      'No tokens yet',
      'Click <b>+ New Token</b> above to issue your first one.');
    return;
  }
  if (!list.length) {
    tb.innerHTML = emptyRow(5,
      'No tokens match your filter',
      'Try a different label or token prefix.');
    return;
  }
  tb.innerHTML = list.map(t =>
    '<tr><td class="p">'+esc(t.label)+'</td>' +
    '<td class="mono" style="color:var(--accent)">'+esc(t.value_hint)+'</td>' +
    '<td>'+(t.max_tunnels===0?'<span style="color:var(--muted)">Unlimited</span>':t.max_tunnels)+'</td>' +
    '<td>'+(t.enabled?'<span class="badge bg">Active</span>':'<span class="badge br">Revoked</span>')+'</td>' +
    '<td style="text-align:right">'+(t.enabled?'<button class="btn btn-d btn-sm" onclick="openRevoke(\''+esc(t.id)+'\')">Revoke</button>':'')+'</td></tr>'
  ).join('');
}

function filterTokens() {
  renderTokens();
}

function openCreate() {
  document.getElementById('f-label').value = '';
  document.getElementById('f-max').value = '0';
  document.getElementById('tok-result').style.display = 'none';
  document.getElementById('btn-create').style.display = '';
  document.getElementById('btn-create').disabled = false;
  document.getElementById('btn-create').textContent = 'Create Token';
  document.getElementById('m-create').classList.add('open');
  setTimeout(() => document.getElementById('f-label').focus(), 60);
}
function closeCreate() { document.getElementById('m-create').classList.remove('open'); loadTokens(); }
async function doCreate() {
  const label = document.getElementById('f-label').value.trim() || 'unnamed';
  const max = parseInt(document.getElementById('f-max').value) || 0;
  const btn = document.getElementById('btn-create');
  btn.textContent = 'Creating…'; btn.disabled = true;
  try {
    const d = await api('/tokens', {method:'POST', body:JSON.stringify({label, max_tunnels:max})});
    if (d.value) {
      document.getElementById('tok-val').textContent = d.value;
      document.getElementById('tok-result').style.display = 'block';
      document.getElementById('copy-hint').textContent = 'Click to copy';
      btn.style.display = 'none';
    }
  } catch(e) { btn.textContent = 'Create Token'; btn.disabled = false; }
}
function copyToken() {
  const val = document.getElementById('tok-val').textContent;
  navigator.clipboard.writeText(val).then(() => {
    document.getElementById('copy-hint').textContent = '✓ Copied!';
    setTimeout(() => { document.getElementById('copy-hint').textContent = 'Click to copy'; }, 2000);
  });
}

function openRevoke(id) {
  revokeId = id;
  document.getElementById('btn-revoke').textContent = 'Revoke Token';
  document.getElementById('btn-revoke').disabled = false;
  document.getElementById('m-revoke').classList.add('open');
}
function closeRevoke() { revokeId = null; document.getElementById('m-revoke').classList.remove('open'); }
async function doRevoke() {
  if (!revokeId) return;
  const btn = document.getElementById('btn-revoke');
  btn.textContent = 'Revoking…'; btn.disabled = true;
  try { await api('/tokens/'+revokeId, {method:'DELETE'}); } catch(_) {}
  closeRevoke(); loadTokens();
}

document.getElementById('m-create').addEventListener('click', e => { if(e.target===document.getElementById('m-create')) closeCreate(); });
document.getElementById('m-revoke').addEventListener('click', e => { if(e.target===document.getElementById('m-revoke')) closeRevoke(); });

function esc(s) {
  return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;').replace(/'/g,'&#39;');
}

loadTunnels();
startTunnelsRefresh();
</script>
</body>
</html>`

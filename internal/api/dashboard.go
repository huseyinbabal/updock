package api

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Updock Dashboard</title>
    <style>
        :root {
            --bg: #0f172a;
            --surface: #1e293b;
            --surface-hover: #334155;
            --border: #334155;
            --text: #e2e8f0;
            --text-muted: #94a3b8;
            --primary: #6366f1;
            --primary-hover: #4f46e5;
            --success: #22c55e;
            --warning: #f59e0b;
            --danger: #ef4444;
            --cyan: #06b6d4;
            --radius: 8px;
        }
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: var(--bg); color: var(--text); min-height: 100vh; }
        .header { background: var(--surface); border-bottom: 1px solid var(--border); padding: 16px 24px; display: flex; align-items: center; justify-content: space-between; }
        .header h1 { font-size: 22px; font-weight: 700; display: flex; align-items: center; gap: 10px; }
        .header-actions { display: flex; gap: 12px; align-items: center; }
        .badge { display: inline-flex; align-items: center; padding: 4px 10px; border-radius: 12px; font-size: 12px; font-weight: 600; }
        .badge-healthy { background: rgba(34,197,94,0.15); color: var(--success); }
        .badge-unhealthy { background: rgba(239,68,68,0.15); color: var(--danger); }
        .badge-info { background: rgba(99,102,241,0.15); color: var(--primary); }
        .badge-warn { background: rgba(245,158,11,0.15); color: var(--warning); }
        .btn { padding: 8px 16px; border-radius: var(--radius); border: none; cursor: pointer; font-size: 13px; font-weight: 500; transition: all 0.15s; }
        .btn-primary { background: var(--primary); color: white; }
        .btn-primary:hover { background: var(--primary-hover); }
        .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }
        .btn-sm { padding: 4px 10px; font-size: 12px; }
        .container { max-width: 1280px; margin: 0 auto; padding: 24px; }
        .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 14px; margin-bottom: 24px; }
        .stat-card { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); padding: 18px; }
        .stat-card .label { font-size: 11px; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.5px; }
        .stat-card .value { font-size: 28px; font-weight: 700; margin-top: 4px; }
        .tabs { display: flex; gap: 2px; margin-bottom: 16px; background: var(--surface); border-radius: var(--radius); padding: 4px; width: fit-content; }
        .tab { padding: 8px 16px; border-radius: 6px; border: none; background: transparent; color: var(--text-muted); cursor: pointer; font-size: 13px; font-weight: 500; transition: all 0.15s; }
        .tab.active { background: var(--primary); color: white; }
        .tab:hover:not(.active) { color: var(--text); }
        .section { margin-bottom: 24px; }
        table { width: 100%; border-collapse: collapse; background: var(--surface); border-radius: var(--radius); overflow: hidden; border: 1px solid var(--border); }
        th { text-align: left; padding: 10px 14px; font-size: 11px; font-weight: 600; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.5px; border-bottom: 1px solid var(--border); }
        td { padding: 10px 14px; font-size: 13px; border-bottom: 1px solid var(--border); }
        tr:last-child td { border-bottom: none; }
        tr:hover td { background: var(--surface-hover); }
        .status { display: inline-flex; align-items: center; gap: 6px; }
        .dot { width: 8px; height: 8px; border-radius: 50%; display: inline-block; }
        .dot-green { background: var(--success); }
        .dot-red { background: var(--danger); }
        .dot-yellow { background: var(--warning); }
        .dot-blue { background: var(--cyan); }
        .mono { font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace; font-size: 12px; }
        .text-primary { color: var(--primary); }
        .text-muted { color: var(--text-muted); }
        .text-success { color: var(--success); }
        .text-danger { color: var(--danger); }
        .text-warning { color: var(--warning); }
        .text-cyan { color: var(--cyan); }
        .empty { text-align: center; padding: 40px 24px; color: var(--text-muted); }
        .spinner { display: inline-block; width: 14px; height: 14px; border: 2px solid rgba(255,255,255,0.3); border-radius: 50%; border-top-color: white; animation: spin 0.6s linear infinite; }
        @keyframes spin { to { transform: rotate(360deg); } }
        .toast { position: fixed; bottom: 24px; right: 24px; padding: 12px 20px; border-radius: var(--radius); font-size: 13px; font-weight: 500; z-index: 100; animation: slideIn 0.3s ease; }
        .toast-success { background: var(--success); color: white; }
        .toast-error { background: var(--danger); color: white; }
        @keyframes slideIn { from { transform: translateY(20px); opacity: 0; } to { transform: translateY(0); opacity: 1; } }
        .event-type { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 11px; font-weight: 600; }
        .event-type-update { background: rgba(34,197,94,0.15); color: var(--success); }
        .event-type-rollback { background: rgba(239,68,68,0.15); color: var(--danger); }
        .event-type-approval { background: rgba(245,158,11,0.15); color: var(--warning); }
        .event-type-skip { background: rgba(148,163,184,0.15); color: var(--text-muted); }
        .policy-tag { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 11px; font-weight: 600; background: rgba(6,182,212,0.15); color: var(--cyan); }
    </style>
</head>
<body>
    <div class="header">
        <h1>&#x1F433; Updock</h1>
        <div class="header-actions">
            <span id="health-badge" class="badge badge-healthy">Healthy</span>
            <span id="version-badge" class="badge badge-info">v-</span>
            <button id="update-btn" class="btn btn-primary" onclick="triggerUpdate()">Check for Updates</button>
        </div>
    </div>
    <div class="container">
        <div class="stats">
            <div class="stat-card"><div class="label">Containers</div><div class="value" id="stat-containers">-</div></div>
            <div class="stat-card"><div class="label">Updates Applied</div><div class="value text-success" id="stat-updates">-</div></div>
            <div class="stat-card"><div class="label">Pending Approval</div><div class="value text-warning" id="stat-pending">-</div></div>
            <div class="stat-card"><div class="label">Errors</div><div class="value text-danger" id="stat-errors">-</div></div>
            <div class="stat-card"><div class="label">Last Check</div><div class="value" id="stat-lastcheck" style="font-size:15px;">-</div></div>
        </div>

        <div class="tabs">
            <button class="tab active" onclick="switchTab('containers')">Containers</button>
            <button class="tab" onclick="switchTab('audit')">Audit Log</button>
            <button class="tab" onclick="switchTab('policies')">Policies</button>
            <button class="tab" onclick="switchTab('history')">Update History</button>
        </div>

        <div id="tab-containers" class="section">
            <table><thead><tr><th>Name</th><th>Image</th><th>Policy</th><th>Status</th><th>ID</th></tr></thead>
            <tbody id="containers-body"><tr><td colspan="5" class="empty">Loading...</td></tr></tbody></table>
        </div>

        <div id="tab-audit" class="section" style="display:none;">
            <table><thead><tr><th>Time</th><th>Type</th><th>Container</th><th>Actor</th><th>Message</th></tr></thead>
            <tbody id="audit-body"><tr><td colspan="5" class="empty">Loading...</td></tr></tbody></table>
        </div>

        <div id="tab-policies" class="section" style="display:none;">
            <h3 style="margin-bottom:12px;font-size:15px;color:var(--text-muted)">Defined Policies</h3>
            <table><thead><tr><th>Name</th><th>Strategy</th><th>Approve</th><th>Rollback</th><th>Health Timeout</th></tr></thead>
            <tbody id="policies-body"><tr><td colspan="5" class="empty">Loading...</td></tr></tbody></table>
            <h3 style="margin:20px 0 12px;font-size:15px;color:var(--text-muted)">Container Assignments</h3>
            <table><thead><tr><th>Container</th><th>Policy</th><th>Schedule</th><th>Ignored</th></tr></thead>
            <tbody id="assignments-body"><tr><td colspan="4" class="empty">No overrides</td></tr></tbody></table>
            <h3 style="margin:20px 0 12px;font-size:15px;color:var(--text-muted)">Groups</h3>
            <table><thead><tr><th>Group</th><th>Members</th><th>Strategy</th><th>Order</th></tr></thead>
            <tbody id="groups-body"><tr><td colspan="4" class="empty">No groups</td></tr></tbody></table>
        </div>

        <div id="tab-history" class="section" style="display:none;">
            <table><thead><tr><th>Container</th><th>Image</th><th>Updated</th><th>Error</th><th>Time</th></tr></thead>
            <tbody id="history-body"><tr><td colspan="5" class="empty">Loading...</td></tr></tbody></table>
        </div>
    </div>

    <script>
        const API = window.location.origin;
        const TOKEN = new URLSearchParams(window.location.search).get('token') || localStorage.getItem('updock_token') || '';
        if (TOKEN) localStorage.setItem('updock_token', TOKEN);
        let policiesData = null;

        function authHeaders() {
            const h = {};
            if (TOKEN) h['Authorization'] = 'Bearer ' + TOKEN;
            return h;
        }
        function apiFetch(path, opts) {
            opts = opts || {};
            opts.headers = Object.assign(authHeaders(), opts.headers || {});
            return fetch(API + path, opts).then(r => {
                if (r.status === 401) {
                    const t = prompt('API token required:');
                    if (t) { localStorage.setItem('updock_token', t); location.reload(); }
                    throw new Error('Unauthorized');
                }
                return r;
            });
        }

        function switchTab(tab) {
            document.querySelectorAll('.tab').forEach((t,i) => { t.classList.remove('active'); if(t.textContent.toLowerCase().replace(/ /g,'').includes(tab)) t.classList.add('active'); });
            ['containers','audit','policies','history'].forEach(t => { document.getElementById('tab-'+t).style.display = t===tab ? 'block' : 'none'; });
            if (tab==='audit') loadAudit();
            if (tab==='policies') loadPolicies();
            if (tab==='history') loadHistory();
        }

        function showToast(msg, type) { const t = document.createElement('div'); t.className='toast toast-'+type; t.textContent=msg; document.body.appendChild(t); setTimeout(()=>t.remove(),3000); }
        function timeAgo(d) { const s=Math.floor((new Date()-new Date(d))/1000); if(s<60)return s+'s ago'; if(s<3600)return Math.floor(s/60)+'m ago'; if(s<86400)return Math.floor(s/3600)+'h ago'; return Math.floor(s/86400)+'d ago'; }
        function esc(s) { const d=document.createElement('div'); d.textContent=s; return d.innerHTML; }

        function eventClass(type) {
            if(!type) return 'event-type-skip';
            if(type.startsWith('update.applied')||type.startsWith('update.pulled')) return 'event-type-update';
            if(type.startsWith('rollback')) return 'event-type-rollback';
            if(type.startsWith('approval')) return 'event-type-approval';
            return 'event-type-skip';
        }

        function getPolicyForContainer(name) {
            if(!policiesData) return '-';
            const c = policiesData.Containers || policiesData.containers || {};
            if(c[name] && c[name].policy) return c[name].policy;
            return 'default';
        }

        async function loadContainers() {
            try {
                const [cr, pr] = await Promise.all([apiFetch('/api/containers'), apiFetch('/api/policies')]);
                const containers = await cr.json();
                policiesData = await pr.json();
                document.getElementById('stat-containers').textContent = containers.length||0;
                const tbody = document.getElementById('containers-body');
                if(!containers||!containers.length) { tbody.innerHTML='<tr><td colspan="5" class="empty">No containers</td></tr>'; return; }
                tbody.innerHTML = containers.map(c => {
                    const st = c.State||'unknown';
                    const dot = st==='running'?'dot-green':st==='exited'?'dot-red':'dot-yellow';
                    const pol = getPolicyForContainer(c.Name||'');
                    return '<tr><td><strong>'+esc(c.Name||'-')+'</strong></td><td><span class="mono text-primary">'+esc(c.Image||'-')+'</span></td><td><span class="policy-tag">'+esc(pol)+'</span></td><td><span class="status"><span class="dot '+dot+'"></span>'+esc(c.Status||st)+'</span></td><td><span class="mono text-muted">'+(c.ID||'').substring(0,12)+'</span></td></tr>';
                }).join('');
            } catch(e) { console.error(e); }
        }

        async function loadAudit() {
            try {
                const r = await apiFetch('/api/audit?limit=100');
                const entries = await r.json();
                let pending=0;
                if(entries) entries.forEach(e=>{ if(e.type==='approval.pending')pending++; });
                document.getElementById('stat-pending').textContent = pending;
                const tbody = document.getElementById('audit-body');
                if(!entries||!entries.length) { tbody.innerHTML='<tr><td colspan="5" class="empty">No audit entries</td></tr>'; return; }
                tbody.innerHTML = entries.map(e => '<tr><td class="text-muted" style="font-size:12px;">'+(e.timestamp?timeAgo(e.timestamp):'-')+'</td><td><span class="event-type '+eventClass(e.type)+'">'+esc(e.type||'-')+'</span></td><td><strong>'+esc(e.container_name||'-')+'</strong></td><td class="text-muted">'+esc(e.actor||'-')+'</td><td style="font-size:12px;">'+esc(e.message||'-')+'</td></tr>').join('');
            } catch(e) { console.error(e); }
        }

        async function loadPolicies() {
            try {
                const r = await apiFetch('/api/policies');
                const data = await r.json();
                policiesData = data;
                const policies = data.Policies||data.policies||{};
                const containers = data.Containers||data.containers||{};
                const groups = data.Groups||data.groups||{};

                const pb = document.getElementById('policies-body');
                const pEntries = Object.entries(policies);
                pb.innerHTML = pEntries.length ? pEntries.map(([k,v])=>'<tr><td><strong>'+esc(k)+'</strong></td><td><span class="policy-tag">'+esc(v.strategy||v.Strategy||'all')+'</span></td><td>'+esc(v.approve||v.Approve||'auto')+'</td><td>'+esc(v.rollback||v.Rollback||'on-failure')+'</td><td class="mono text-muted">'+(v.health_timeout||v.HealthTimeout||'30s')+'</td></tr>').join('') : '<tr><td colspan="5" class="empty">No policies defined</td></tr>';

                const ab = document.getElementById('assignments-body');
                const aEntries = Object.entries(containers);
                ab.innerHTML = aEntries.length ? aEntries.map(([k,v])=>'<tr><td><strong>'+esc(k)+'</strong></td><td><span class="policy-tag">'+esc(v.policy||v.Policy||'default')+'</span></td><td class="mono text-muted">'+esc(v.schedule||v.Schedule||'-')+'</td><td>'+(v.ignore||v.Ignore?'<span class="text-danger">Yes</span>':'-')+'</td></tr>').join('') : '<tr><td colspan="4" class="empty">No container overrides</td></tr>';

                const gb = document.getElementById('groups-body');
                const gEntries = Object.entries(groups);
                gb.innerHTML = gEntries.length ? gEntries.map(([k,v])=>'<tr><td><strong>'+esc(k)+'</strong></td><td>'+esc((v.members||v.Members||[]).join(', '))+'</td><td>'+esc(v.strategy||v.Strategy||'-')+'</td><td class="mono text-muted">'+esc((v.order||v.Order||[]).join(' → '))+'</td></tr>').join('') : '<tr><td colspan="4" class="empty">No groups defined</td></tr>';
            } catch(e) { console.error(e); }
        }

        async function loadHistory() {
            try {
                const r = await apiFetch('/api/history');
                const history = await r.json();
                let updates=0, errors=0;
                if(history) history.forEach(h=>{ if(h.updated)updates++; if(h.error)errors++; });
                document.getElementById('stat-updates').textContent = updates;
                document.getElementById('stat-errors').textContent = errors;
                if(history&&history.length>0) document.getElementById('stat-lastcheck').textContent = timeAgo(history[history.length-1].checked_at);
                const tbody = document.getElementById('history-body');
                if(!history||!history.length) { tbody.innerHTML='<tr><td colspan="5" class="empty">No history</td></tr>'; return; }
                tbody.innerHTML = [...history].reverse().map(h=>'<tr><td><strong>'+esc(h.container_name||'-')+'</strong></td><td><span class="mono text-primary">'+esc(h.image||'-')+'</span></td><td>'+(h.updated?'<span class="text-success">Yes</span>':'No')+'</td><td class="text-danger" style="font-size:12px;">'+esc(h.error||'-')+'</td><td class="text-muted">'+timeAgo(h.checked_at)+'</td></tr>').join('');
            } catch(e) { console.error(e); }
        }

        async function loadHealth() {
            try {
                const r = await apiFetch('/api/health');
                const d = await r.json();
                const b = document.getElementById('health-badge');
                b.className = 'badge badge-'+(d.status==='healthy'?'healthy':'unhealthy');
                b.textContent = d.status==='healthy'?'Healthy':'Unhealthy';
            } catch(e) { const b=document.getElementById('health-badge'); b.className='badge badge-unhealthy'; b.textContent='Disconnected'; }
        }

        async function loadInfo() {
            try { const r=await apiFetch('/api/info'); const d=await r.json(); document.getElementById('version-badge').textContent='v'+(d.version||'-'); } catch(e){}
        }

        async function triggerUpdate() {
            const btn=document.getElementById('update-btn'); btn.disabled=true; btn.innerHTML='<span class="spinner"></span> Checking...';
            try {
                const r=await apiFetch('/api/update',{method:'POST'}); const d=await r.json();
                if(d.results){const u=d.results.filter(r=>r.updated).length; showToast(d.results.length+' checked, '+u+' updated','success');}
                else showToast('Check completed','success');
                loadContainers(); loadHistory(); loadAudit();
            } catch(e){ showToast('Failed: '+e.message,'error'); }
            finally { btn.disabled=false; btn.innerHTML='Check for Updates'; }
        }

        loadContainers(); loadHistory(); loadHealth(); loadInfo(); loadAudit();
        setInterval(()=>{ loadContainers(); loadHealth(); },30000);
        setInterval(()=>{ loadHistory(); loadAudit(); },60000);
    </script>
</body>
</html>` + ""

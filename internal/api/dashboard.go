package api

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Updock - Container Update Dashboard</title>
    <style>
        :root {
            --bg: #0f172a;
            --surface: #1e293b;
            --surface-hover: #334155;
            --border: #334155;
            --text: #e2e8f0;
            --text-muted: #94a3b8;
            --primary: #3b82f6;
            --primary-hover: #2563eb;
            --success: #22c55e;
            --warning: #f59e0b;
            --danger: #ef4444;
            --radius: 8px;
        }
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: var(--bg);
            color: var(--text);
            min-height: 100vh;
        }
        .header {
            background: var(--surface);
            border-bottom: 1px solid var(--border);
            padding: 16px 24px;
            display: flex;
            align-items: center;
            justify-content: space-between;
        }
        .header h1 {
            font-size: 24px;
            font-weight: 700;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        .header h1 .logo {
            font-size: 28px;
        }
        .header-actions {
            display: flex;
            gap: 12px;
            align-items: center;
        }
        .badge {
            display: inline-flex;
            align-items: center;
            padding: 4px 10px;
            border-radius: 12px;
            font-size: 12px;
            font-weight: 600;
        }
        .badge-healthy { background: rgba(34,197,94,0.15); color: var(--success); }
        .badge-unhealthy { background: rgba(239,68,68,0.15); color: var(--danger); }
        .btn {
            padding: 8px 16px;
            border-radius: var(--radius);
            border: none;
            cursor: pointer;
            font-size: 14px;
            font-weight: 500;
            transition: all 0.15s;
        }
        .btn-primary {
            background: var(--primary);
            color: white;
        }
        .btn-primary:hover { background: var(--primary-hover); }
        .btn-primary:disabled {
            opacity: 0.5;
            cursor: not-allowed;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            padding: 24px;
        }
        .stats {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 16px;
            margin-bottom: 24px;
        }
        .stat-card {
            background: var(--surface);
            border: 1px solid var(--border);
            border-radius: var(--radius);
            padding: 20px;
        }
        .stat-card .label {
            font-size: 13px;
            color: var(--text-muted);
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        .stat-card .value {
            font-size: 32px;
            font-weight: 700;
            margin-top: 4px;
        }
        .section {
            margin-bottom: 24px;
        }
        .section h2 {
            font-size: 18px;
            font-weight: 600;
            margin-bottom: 12px;
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .tabs {
            display: flex;
            gap: 2px;
            margin-bottom: 16px;
            background: var(--surface);
            border-radius: var(--radius);
            padding: 4px;
            width: fit-content;
        }
        .tab {
            padding: 8px 16px;
            border-radius: 6px;
            border: none;
            background: transparent;
            color: var(--text-muted);
            cursor: pointer;
            font-size: 14px;
            font-weight: 500;
            transition: all 0.15s;
        }
        .tab.active {
            background: var(--primary);
            color: white;
        }
        .tab:hover:not(.active) {
            color: var(--text);
        }
        table {
            width: 100%;
            border-collapse: collapse;
            background: var(--surface);
            border-radius: var(--radius);
            overflow: hidden;
            border: 1px solid var(--border);
        }
        th {
            text-align: left;
            padding: 12px 16px;
            font-size: 12px;
            font-weight: 600;
            color: var(--text-muted);
            text-transform: uppercase;
            letter-spacing: 0.5px;
            border-bottom: 1px solid var(--border);
        }
        td {
            padding: 12px 16px;
            font-size: 14px;
            border-bottom: 1px solid var(--border);
        }
        tr:last-child td { border-bottom: none; }
        tr:hover td { background: var(--surface-hover); }
        .status {
            display: inline-flex;
            align-items: center;
            gap: 6px;
        }
        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
        }
        .status-running .status-dot { background: var(--success); }
        .status-exited .status-dot { background: var(--danger); }
        .status-paused .status-dot { background: var(--warning); }
        .image-name {
            font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
            font-size: 13px;
            color: var(--primary);
        }
        .container-id {
            font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
            font-size: 12px;
            color: var(--text-muted);
        }
        .empty-state {
            text-align: center;
            padding: 48px 24px;
            color: var(--text-muted);
        }
        .spinner {
            display: inline-block;
            width: 16px;
            height: 16px;
            border: 2px solid rgba(255,255,255,0.3);
            border-radius: 50%;
            border-top-color: white;
            animation: spin 0.6s linear infinite;
        }
        @keyframes spin { to { transform: rotate(360deg); } }
        .toast {
            position: fixed;
            bottom: 24px;
            right: 24px;
            padding: 12px 20px;
            border-radius: var(--radius);
            font-size: 14px;
            font-weight: 500;
            z-index: 100;
            animation: slideIn 0.3s ease;
        }
        .toast-success { background: var(--success); color: white; }
        .toast-error { background: var(--danger); color: white; }
        @keyframes slideIn {
            from { transform: translateY(20px); opacity: 0; }
            to { transform: translateY(0); opacity: 1; }
        }
        .info-bar {
            background: var(--surface);
            border: 1px solid var(--border);
            border-radius: var(--radius);
            padding: 12px 16px;
            margin-bottom: 24px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            font-size: 13px;
            color: var(--text-muted);
        }
    </style>
</head>
<body>
    <div class="header">
        <h1><span class="logo">&#x1F433;</span> Updock</h1>
        <div class="header-actions">
            <span id="health-badge" class="badge badge-healthy">Healthy</span>
            <span id="version-badge" class="badge" style="background:rgba(59,130,246,0.15);color:var(--primary);">v-</span>
            <button id="update-btn" class="btn btn-primary" onclick="triggerUpdate()">
                Check for Updates
            </button>
        </div>
    </div>

    <div class="container">
        <div class="stats">
            <div class="stat-card">
                <div class="label">Containers</div>
                <div class="value" id="stat-containers">-</div>
            </div>
            <div class="stat-card">
                <div class="label">Updates Applied</div>
                <div class="value" id="stat-updates" style="color:var(--success);">-</div>
            </div>
            <div class="stat-card">
                <div class="label">Errors</div>
                <div class="value" id="stat-errors" style="color:var(--danger);">-</div>
            </div>
            <div class="stat-card">
                <div class="label">Last Check</div>
                <div class="value" id="stat-lastcheck" style="font-size:16px;">-</div>
            </div>
        </div>

        <div class="tabs">
            <button class="tab active" onclick="switchTab('containers', this)">Containers</button>
            <button class="tab" onclick="switchTab('history', this)">Update History</button>
        </div>

        <div id="tab-containers" class="section">
            <table>
                <thead>
                    <tr>
                        <th>Name</th>
                        <th>Image</th>
                        <th>Status</th>
                        <th>ID</th>
                        <th>Created</th>
                    </tr>
                </thead>
                <tbody id="containers-body">
                    <tr><td colspan="5" class="empty-state">Loading containers...</td></tr>
                </tbody>
            </table>
        </div>

        <div id="tab-history" class="section" style="display:none;">
            <table>
                <thead>
                    <tr>
                        <th>Container</th>
                        <th>Image</th>
                        <th>Updated</th>
                        <th>Error</th>
                        <th>Time</th>
                    </tr>
                </thead>
                <tbody id="history-body">
                    <tr><td colspan="5" class="empty-state">Loading history...</td></tr>
                </tbody>
            </table>
        </div>
    </div>

    <script>
        const API = window.location.origin;

        function switchTab(tab, el) {
            document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
            el.classList.add('active');
            document.getElementById('tab-containers').style.display = tab === 'containers' ? 'block' : 'none';
            document.getElementById('tab-history').style.display = tab === 'history' ? 'block' : 'none';
            if (tab === 'history') loadHistory();
        }

        function showToast(msg, type) {
            const toast = document.createElement('div');
            toast.className = 'toast toast-' + type;
            toast.textContent = msg;
            document.body.appendChild(toast);
            setTimeout(() => toast.remove(), 3000);
        }

        function timeAgo(dateStr) {
            const date = new Date(dateStr);
            const now = new Date();
            const seconds = Math.floor((now - date) / 1000);
            if (seconds < 60) return seconds + 's ago';
            if (seconds < 3600) return Math.floor(seconds / 60) + 'm ago';
            if (seconds < 86400) return Math.floor(seconds / 3600) + 'h ago';
            return Math.floor(seconds / 86400) + 'd ago';
        }

        async function loadContainers() {
            try {
                const resp = await fetch(API + '/api/containers');
                const containers = await resp.json();

                document.getElementById('stat-containers').textContent = containers.length || 0;

                const tbody = document.getElementById('containers-body');
                if (!containers || containers.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="5" class="empty-state">No containers found</td></tr>';
                    return;
                }

                tbody.innerHTML = containers.map(c => {
                    const statusClass = 'status-' + (c.State || 'unknown');
                    return '<tr>' +
                        '<td><strong>' + (c.Name || '-') + '</strong></td>' +
                        '<td><span class="image-name">' + (c.Image || '-') + '</span></td>' +
                        '<td><span class="status ' + statusClass + '"><span class="status-dot"></span>' + (c.Status || c.State || '-') + '</span></td>' +
                        '<td><span class="container-id">' + (c.ID || '').substring(0, 12) + '</span></td>' +
                        '<td>' + (c.Created ? timeAgo(c.Created) : '-') + '</td>' +
                        '</tr>';
                }).join('');
            } catch (e) {
                console.error('Failed to load containers:', e);
            }
        }

        async function loadHistory() {
            try {
                const resp = await fetch(API + '/api/history');
                const history = await resp.json();

                let updates = 0, errors = 0;
                if (history) {
                    history.forEach(h => {
                        if (h.updated) updates++;
                        if (h.error) errors++;
                    });
                }

                document.getElementById('stat-updates').textContent = updates;
                document.getElementById('stat-errors').textContent = errors;

                if (history && history.length > 0) {
                    document.getElementById('stat-lastcheck').textContent = timeAgo(history[history.length - 1].checked_at);
                }

                const tbody = document.getElementById('history-body');
                if (!history || history.length === 0) {
                    tbody.innerHTML = '<tr><td colspan="5" class="empty-state">No update history yet</td></tr>';
                    return;
                }

                const reversed = [...history].reverse();
                tbody.innerHTML = reversed.map(h => {
                    return '<tr>' +
                        '<td><strong>' + (h.container_name || '-') + '</strong></td>' +
                        '<td><span class="image-name">' + (h.image || '-') + '</span></td>' +
                        '<td>' + (h.updated ? '<span style="color:var(--success)">Yes</span>' : 'No') + '</td>' +
                        '<td style="color:var(--danger);font-size:12px;">' + (h.error || '-') + '</td>' +
                        '<td>' + (h.checked_at ? timeAgo(h.checked_at) : '-') + '</td>' +
                        '</tr>';
                }).join('');
            } catch (e) {
                console.error('Failed to load history:', e);
            }
        }

        async function loadHealth() {
            try {
                const resp = await fetch(API + '/api/health');
                const data = await resp.json();
                const badge = document.getElementById('health-badge');
                if (data.status === 'healthy') {
                    badge.className = 'badge badge-healthy';
                    badge.textContent = 'Healthy';
                } else {
                    badge.className = 'badge badge-unhealthy';
                    badge.textContent = 'Unhealthy';
                }
            } catch (e) {
                const badge = document.getElementById('health-badge');
                badge.className = 'badge badge-unhealthy';
                badge.textContent = 'Disconnected';
            }
        }

        async function loadInfo() {
            try {
                const resp = await fetch(API + '/api/info');
                const data = await resp.json();
                document.getElementById('version-badge').textContent = 'v' + (data.version || '-');
            } catch (e) {}
        }

        async function triggerUpdate() {
            const btn = document.getElementById('update-btn');
            btn.disabled = true;
            btn.innerHTML = '<span class="spinner"></span> Checking...';

            try {
                const resp = await fetch(API + '/api/update', { method: 'POST' });
                const data = await resp.json();

                if (data.results) {
                    const updated = data.results.filter(r => r.updated).length;
                    showToast('Check complete: ' + data.results.length + ' checked, ' + updated + ' updated', 'success');
                } else {
                    showToast('Update check completed', 'success');
                }

                loadContainers();
                loadHistory();
            } catch (e) {
                showToast('Update check failed: ' + e.message, 'error');
            } finally {
                btn.disabled = false;
                btn.innerHTML = 'Check for Updates';
            }
        }

        // Initial load
        loadContainers();
        loadHistory();
        loadHealth();
        loadInfo();

        // Auto-refresh every 30 seconds
        setInterval(() => {
            loadContainers();
            loadHealth();
        }, 30000);

        // Refresh history every 60 seconds
        setInterval(loadHistory, 60000);
    </script>
</body>
</html>`

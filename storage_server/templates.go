// ============================================================
//  【电脑端软件】运行目标: Windows / macOS / Linux / NAS 服务器
//  编译工具链: Go 1.21+ (跨平台单二进制, 不依赖 ESP32)
//  运行方式: 直接运行二进制 或 Docker 容器 / systemd 服务
// ============================================================
//
// Package main - Web 画廊模板渲染 (多语言)
package main

import (
	"html/template"
	"log/slog"
	"net/http"

	"bio-growth-recorder/i18n"
	"bio-growth-recorder/storage"
)

// galleryData 画廊页面数据
type galleryData struct {
	Lang          string
	LangName      string
	Languages     []string
	LangNames     map[string]string
	Devices       []storage.DeviceInfo
	TotalPhotos   int
	TotalSize     int64
	DeviceCount   int
	ServerTime    string
	CSRFToken     string // CSRF token (供 AJAX 请求使用)
	RetentionDays int    // 保留天数
	MaxStorageMB  int64  // 最大存储 (MB)
	RetentionInfo string // 保留策略描述
}

// ======================= 画廊页面 =======================

const galleryTpl = `<!DOCTYPE html>
<html lang="{{.Lang}}">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>{{T "title"}} - {{T "subtitle"}}</title>
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		:root {
			--bg: #0f1117;
			--card: #1a1d27;
			--border: #2a2d3a;
			--text: #e0e0e0;
			--text-dim: #888;
			--accent: #4fc3f7;
			--accent-hover: #29b6f6;
			--danger: #ef5350;
			--success: #66bb6a;
		}
		body {
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Noto Sans CJK SC', 'Noto Sans CJK JP', 'Noto Sans CJK TC', sans-serif;
			background: var(--bg);
			color: var(--text);
			min-height: 100vh;
		}
		.header {
			background: var(--card);
			border-bottom: 1px solid var(--border);
			padding: 16px 24px;
			display: flex;
			align-items: center;
			justify-content: space-between;
			flex-wrap: wrap;
			gap: 12px;
		}
		.header h1 { font-size: 20px; font-weight: 600; }
		.header h1 span { color: var(--accent); }
		.header-right { display: flex; align-items: center; gap: 16px; flex-wrap: wrap; }
		.lang-select {
			background: var(--bg);
			color: var(--text);
			border: 1px solid var(--border);
			border-radius: 6px;
			padding: 6px 10px;
			font-size: 13px;
			cursor: pointer;
		}
		.stats-bar {
			display: flex;
			gap: 24px;
			padding: 12px 24px;
			background: var(--card);
			border-bottom: 1px solid var(--border);
			font-size: 14px;
			flex-wrap: wrap;
		}
		.stat-item { display: flex; align-items: center; gap: 6px; }
		.stat-value { color: var(--accent); font-weight: 600; font-size: 16px; }
		.stat-label { color: var(--text-dim); }
		.btn {
			background: var(--accent);
			color: #000;
			border: none;
			border-radius: 6px;
			padding: 8px 16px;
			font-size: 13px;
			cursor: pointer;
			text-decoration: none;
			display: inline-flex;
			align-items: center;
			gap: 4px;
			transition: background 0.2s;
		}
		.btn:hover { background: var(--accent-hover); }
		.btn-sm { padding: 4px 10px; font-size: 12px; }
		.btn-danger { background: var(--danger); color: #fff; }
		.btn-danger:hover { background: #d32f2f; }
		.btn-ghost {
			background: transparent;
			border: 1px solid var(--border);
			color: var(--text);
		}
		.btn-ghost:hover { border-color: var(--accent); }
		.container { max-width: 1400px; margin: 0 auto; padding: 24px; }
		.device-section {
			background: var(--card);
			border: 1px solid var(--border);
			border-radius: 12px;
			margin-bottom: 24px;
			overflow: hidden;
		}
		.device-header {
			padding: 16px 20px;
			border-bottom: 1px solid var(--border);
			display: flex;
			align-items: center;
			justify-content: space-between;
		}
		.device-header h2 { font-size: 16px; }
		.device-header .device-stats { color: var(--text-dim); font-size: 13px; }
		.date-group { border-bottom: 1px solid var(--border); }
		.date-group:last-child { border-bottom: none; }
		.date-header {
			padding: 12px 20px;
			background: rgba(255,255,255,0.02);
			display: flex;
			align-items: center;
			justify-content: space-between;
		}
		.date-header .date-label { font-size: 14px; font-weight: 500; }
		.date-header .date-actions { display: flex; gap: 8px; }
		.photo-grid {
			display: grid;
			grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));
			gap: 8px;
			padding: 16px 20px;
		}
		.photo-card {
			position: relative;
			border-radius: 8px;
			overflow: hidden;
			cursor: pointer;
			aspect-ratio: 4/3;
			background: var(--bg);
			transition: transform 0.2s;
		}
		.photo-card:hover { transform: scale(1.03); }
		.photo-card img {
			width: 100%;
			height: 100%;
			object-fit: cover;
			display: block;
		}
		.photo-card .time-label {
			position: absolute;
			bottom: 0;
			left: 0;
			right: 0;
			background: linear-gradient(transparent, rgba(0,0,0,0.8));
			color: #fff;
			font-size: 11px;
			padding: 16px 8px 6px;
			text-align: center;
		}
		.empty-state {
			text-align: center;
			padding: 60px 20px;
			color: var(--text-dim);
		}
		.empty-state .icon { font-size: 48px; margin-bottom: 16px; }
		.empty-state p { font-size: 16px; margin-bottom: 8px; }
		.empty-state .hint { font-size: 13px; }

		/* Modal */
		.modal-overlay {
			display: none;
			position: fixed;
			top: 0; left: 0; right: 0; bottom: 0;
			background: rgba(0,0,0,0.9);
			z-index: 1000;
			justify-content: center;
			align-items: center;
		}
		.modal-overlay.active { display: flex; }
		.modal-content {
			max-width: 90vw;
			max-height: 90vh;
			position: relative;
		}
		.modal-content img, .modal-content video {
			max-width: 90vw;
			max-height: 85vh;
			border-radius: 8px;
		}
		.modal-close {
			position: absolute;
			top: -40px;
			right: 0;
			background: none;
			border: none;
			color: #fff;
			font-size: 28px;
			cursor: pointer;
		}
		.modal-nav {
			position: absolute;
			top: 50%;
			width: 100%;
			display: flex;
			justify-content: space-between;
			transform: translateY(-50%);
			pointer-events: none;
		}
		.modal-nav button {
			background: rgba(0,0,0,0.6);
			border: none;
			color: #fff;
			font-size: 24px;
			padding: 16px 12px;
			cursor: pointer;
			border-radius: 4px;
			pointer-events: all;
		}
		.modal-nav button:hover { background: rgba(0,0,0,0.8); }

		/* Toast */
		.toast {
			position: fixed;
			bottom: 24px;
			left: 50%;
			transform: translateX(-50%);
			background: var(--card);
			border: 1px solid var(--border);
			color: var(--text);
			padding: 12px 24px;
			border-radius: 8px;
			font-size: 14px;
			z-index: 2000;
			opacity: 0;
			transition: opacity 0.3s;
		}
		.toast.show { opacity: 1; }
		.toast.success { border-color: var(--success); }
		.toast.error { border-color: var(--danger); }

		@media (max-width: 600px) {
			.header { padding: 12px 16px; }
			.header h1 { font-size: 16px; }
			.stats-bar { padding: 10px 16px; gap: 12px; }
			.container { padding: 12px; }
			.photo-grid { grid-template-columns: repeat(auto-fill, minmax(110px, 1fr)); gap: 6px; padding: 12px; }
		}
	</style>
</head>
<body>
	<div class="header">
		<h1>&#127793; <span>{{T "title"}}</span> &middot; {{T "subtitle"}}</h1>
		<div class="header-right">
			<select class="lang-select" onchange="window.location.href='?lang='+this.value">
				{{range .Languages}}
				<option value="{{.}}" {{if eq . $.Lang}}selected{{end}}>{{index $.LangNames .}}</option>
				{{end}}
			</select>
			<a href="/devices" class="btn btn-ghost btn-sm">&#128187; {{T "devices"}}</a>
			<a href="/photo-mode" class="btn btn-ghost btn-sm">&#128247; {{T "photo_mode"}}</a>
			<button class="btn btn-ghost btn-sm" onclick="location.reload()">&#8635; {{T "refresh"}}</button>
			<a href="/logout" class="btn btn-ghost btn-sm">{{T "logout"}}</a>
		</div>
	</div>

	<div class="stats-bar">
		<div class="stat-item">
			<span class="stat-value">{{.TotalPhotos}}</span>
			<span class="stat-label">{{T "total_photos"}}</span>
		</div>
		<div class="stat-item">
			<span class="stat-value">{{.DeviceCount}}</span>
			<span class="stat-label">{{T "device_count"}}</span>
		</div>
		<div class="stat-item">
			<span class="stat-value">{{.TotalSize}}</span>
			<span class="stat-label">{{T "total_size"}}</span>
		</div>
		<div class="stat-item">
			<span class="stat-value">{{.ServerTime}}</span>
			<span class="stat-label">{{T "server_time"}}</span>
		</div>
		<div class="stat-item">
			<span class="stat-value">{{.RetentionInfo}}</span>
			<span class="stat-label">{{T "retention"}}</span>
		</div>
	</div>

	<div class="container">
		{{if eq (len .Devices) 0}}
		<div class="empty-state">
			<div class="icon">&#128247;</div>
			<p>{{T "no_photos"}}</p>
			<p class="hint">{{T "waiting_photos"}}</p>
		</div>
		{{else}}
		{{range .Devices}}
		{{$deviceID := .DeviceID}}
		<div class="device-section">
			<div class="device-header">
				<h2>&#128225; {{.DeviceID}}</h2>
				<span class="device-stats">{{.Total}} {{T "photos"}}</span>
			</div>
			{{range .Days}}
			<div class="date-group">
				<div class="date-header">
					<span class="date-label">&#128197; {{.Date}} &middot; {{.Count}} {{T "photos"}}</span>
					<div class="date-actions">
						<button class="btn btn-sm" onclick="playTimelapse('{{$deviceID}}', '{{.Date}}')">
							&#9654; {{T "play_timelapse"}}
						</button>
						<button class="btn btn-sm btn-ghost" onclick="downloadMP4('{{$deviceID}}', '{{.Date}}')">
							&#11015; {{T "download_mp4"}}
						</button>
						<button class="btn btn-sm btn-danger" onclick="deleteDay('{{$deviceID}}', '{{.Date}}')">
							&#128465; {{T "delete"}}
						</button>
					</div>
				</div>
				<div class="photo-grid">
					{{range .Photos}}
					<div class="photo-card" onclick="openPhoto('{{.Path}}', this)">
						<img src="{{.Path}}" loading="lazy" alt="{{.Filename}}">
						<div class="time-label">{{.Time}}</div>
					</div>
					{{end}}
				</div>
			</div>
			{{end}}
		</div>
		{{end}}
		{{end}}
	</div>

	<!-- Photo Modal -->
	<div class="modal-overlay" id="photoModal" onclick="closeModal(event)">
		<div class="modal-content">
			<button class="modal-close" onclick="closeModal()">&#10005;</button>
			<div class="modal-nav">
				<button onclick="navPhoto(-1)">&#8249;</button>
				<button onclick="navPhoto(1)">&#8250;</button>
			</div>
			<img id="modalImg" src="">
		</div>
	</div>

	<!-- Timelapse Modal -->
	<div class="modal-overlay" id="timelapseModal" onclick="closeTimelapse(event)">
		<div class="modal-content">
			<button class="modal-close" onclick="closeTimelapse()">&#10005;</button>
			<img id="timelapseImg" src="">
		</div>
	</div>

	<!-- Toast -->
	<div class="toast" id="toast"></div>

	<script>
		var currentPhotoEl = null;
		var csrfToken = "{{.CSRFToken}}";

		function openPhoto(path, el) {
			currentPhotoEl = el;
			document.getElementById('modalImg').src = path;
			document.getElementById('photoModal').classList.add('active');
		}

		function closeModal(e) {
			if (e && e.target !== e.currentTarget) return;
			document.getElementById('photoModal').classList.remove('active');
		}

		function navPhoto(dir) {
			if (!currentPhotoEl) return;
			var cards = Array.from(document.querySelectorAll('.photo-card'));
			var idx = cards.indexOf(currentPhotoEl);
			var next = idx + dir;
			if (next < 0) next = cards.length - 1;
			if (next >= cards.length) next = 0;
			currentPhotoEl = cards[next];
			document.getElementById('modalImg').src = cards[next].querySelector('img').src;
		}

		function playTimelapse(deviceID, date) {
			var src = '/timelapse/' + deviceID + '/' + date;
			document.getElementById('timelapseImg').src = src;
			document.getElementById('timelapseModal').classList.add('active');
		}

		function closeTimelapse(e) {
			if (e && e.target !== e.currentTarget) return;
			document.getElementById('timelapseImg').src = '';
			document.getElementById('timelapseModal').classList.remove('active');
		}

		function downloadMP4(deviceID, date) {
			showToast('Generating MP4...', 'info');
			window.location.href = '/timelapse/mp4/' + deviceID + '/' + date;
		}

		function deleteDay(deviceID, date) {
			if (!confirm('Delete all photos for ' + date + '?')) return;
			fetch('/api/v1/delete/' + deviceID + '/' + date, {
				method: 'DELETE',
				headers: { 'X-CSRF-Token': csrfToken }
			})
				.then(r => r.json())
				.then(data => {
					if (data.deleted !== undefined) {
						showToast('Deleted ' + data.deleted + ' photos', 'success');
						setTimeout(() => location.reload(), 1000);
					} else {
						showToast('Delete failed', 'error');
					}
				})
				.catch(() => showToast('Network error', 'error'));
		}

		function showToast(msg, type) {
			var t = document.getElementById('toast');
			t.textContent = msg;
			t.className = 'toast show ' + (type || '');
			setTimeout(() => t.classList.remove('show'), 3000);
		}

		document.addEventListener('keydown', function(e) {
			var photoModal = document.getElementById('photoModal');
			var tlModal = document.getElementById('timelapseModal');
			if (photoModal.classList.contains('active')) {
				if (e.key === 'ArrowLeft') navPhoto(-1);
				if (e.key === 'ArrowRight') navPhoto(1);
				if (e.key === 'Escape') closeModal();
			}
			if (tlModal.classList.contains('active') && e.key === 'Escape') {
				closeTimelapse();
			}
		});
	</script>
</body>
</html>`

// ======================= Login Page =======================

const loginTpl = `<!DOCTYPE html>
<html lang="{{.Lang}}">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>{{T "login"}} - {{T "title"}}</title>
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		:root {
			--bg: #0f1117;
			--card: #1a1d27;
			--border: #2a2d3a;
			--text: #e0e0e0;
			--text-dim: #888;
			--accent: #4fc3f7;
			--danger: #ef5350;
		}
		body {
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Noto Sans CJK SC', 'Noto Sans CJK JP', 'Noto Sans CJK TC', sans-serif;
			background: var(--bg);
			color: var(--text);
			min-height: 100vh;
			display: flex;
			align-items: center;
			justify-content: center;
		}
		.login-card {
			background: var(--card);
			border: 1px solid var(--border);
			border-radius: 16px;
			padding: 40px;
			width: 360px;
			max-width: 90vw;
		}
		.login-card h1 {
			font-size: 22px;
			text-align: center;
			margin-bottom: 8px;
		}
		.login-card h1 span { color: var(--accent); }
		.login-card .subtitle {
			text-align: center;
			color: var(--text-dim);
			font-size: 14px;
			margin-bottom: 32px;
		}
		.form-group { margin-bottom: 20px; }
		.form-group label {
			display: block;
			font-size: 13px;
			color: var(--text-dim);
			margin-bottom: 6px;
		}
		.form-group input {
			width: 100%;
			background: var(--bg);
			border: 1px solid var(--border);
			border-radius: 8px;
			padding: 12px 14px;
			color: var(--text);
			font-size: 15px;
			outline: none;
			transition: border-color 0.2s;
		}
		.form-group input:focus { border-color: var(--accent); }
		.btn-login {
			width: 100%;
			background: var(--accent);
			color: #000;
			border: none;
			border-radius: 8px;
			padding: 12px;
			font-size: 15px;
			font-weight: 600;
			cursor: pointer;
			transition: background 0.2s;
		}
		.btn-login:hover { background: #29b6f6; }
		.error-msg {
			color: var(--danger);
			font-size: 13px;
			text-align: center;
			margin-bottom: 16px;
		}
		.lang-select {
			position: absolute;
			top: 16px;
			right: 16px;
			background: var(--card);
			color: var(--text);
			border: 1px solid var(--border);
			border-radius: 6px;
			padding: 6px 10px;
			font-size: 13px;
			cursor: pointer;
		}
	</style>
</head>
<body>
	<select class="lang-select" onchange="window.location.href='/login?lang='+this.value">
		{{range .Languages}}
		<option value="{{.}}" {{if eq . $.Lang}}selected{{end}}>{{index $.LangNames .}}</option>
		{{end}}
	</select>

	<div class="login-card">
		<h1>&#127793; <span>{{T "title"}}</span></h1>
		<p class="subtitle">{{T "login_required"}}</p>

		{{if .Error}}
		<p class="error-msg">{{T "wrong_password"}}</p>
		{{end}}

		<form method="POST" action="/login">
			<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
			<div class="form-group">
				<label>{{T "password"}}</label>
				<input type="password" name="password" autofocus required>
			</div>
			<button type="submit" class="btn-login">{{T "login"}}</button>
		</form>
	</div>
</body>
</html>`

// ======================= Render Functions =======================

// devicesTpl 设备管理页面模板
const devicesTpl = `<!DOCTYPE html>
<html lang="{{.Lang}}">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>{{T "devices"}} - {{T "title"}}</title>
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		:root {
			--bg: #0f1117;
			--card: #1a1d27;
			--border: #2a2d3a;
			--text: #e0e0e0;
			--text-dim: #888;
			--accent: #4fc3f7;
			--accent-hover: #29b6f6;
			--danger: #ef5350;
			--success: #66bb6a;
		}
		body {
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Noto Sans CJK SC', 'Noto Sans CJK JP', 'Noto Sans CJK TC', sans-serif;
			background: var(--bg);
			color: var(--text);
			min-height: 100vh;
		}
		.header {
			background: var(--card);
			border-bottom: 1px solid var(--border);
			padding: 16px 24px;
			display: flex;
			align-items: center;
			justify-content: space-between;
			flex-wrap: wrap;
			gap: 12px;
		}
		.header h1 { font-size: 20px; font-weight: 600; }
		.header h1 span { color: var(--accent); }
		.header-right { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
		.lang-select, input, select {
			background: var(--bg);
			color: var(--text);
			border: 1px solid var(--border);
			border-radius: 6px;
			padding: 8px 10px;
			font-size: 13px;
		}
		.btn {
			background: var(--accent);
			color: #000;
			border: none;
			border-radius: 6px;
			padding: 8px 16px;
			font-size: 13px;
			cursor: pointer;
			text-decoration: none;
			display: inline-flex;
			align-items: center;
			gap: 4px;
			transition: background 0.2s;
		}
		.btn:hover { background: var(--accent-hover); }
		.btn-sm { padding: 4px 10px; font-size: 12px; }
		.btn-danger { background: var(--danger); color: #fff; }
		.btn-danger:hover { background: #d32f2f; }
		.btn-ghost {
			background: transparent;
			border: 1px solid var(--border);
			color: var(--text);
		}
		.btn-ghost:hover { border-color: var(--accent); }
		.container { max-width: 1200px; margin: 0 auto; padding: 24px; }
		.section {
			background: var(--card);
			border: 1px solid var(--border);
			border-radius: 12px;
			margin-bottom: 24px;
			overflow: hidden;
		}
		.section-header {
			padding: 16px 20px;
			border-bottom: 1px solid var(--border);
			font-size: 16px;
			font-weight: 600;
		}
		table { width: 100%; border-collapse: collapse; }
		th, td {
			padding: 12px 20px;
			text-align: left;
			border-bottom: 1px solid var(--border);
			font-size: 13px;
			word-break: break-all;
		}
		th { color: var(--text-dim); font-weight: 500; }
		.mono { font-family: 'SF Mono', 'Monaco', 'Consolas', monospace; font-size: 12px; }
		.form-row {
			display: flex;
			gap: 12px;
			padding: 16px 20px;
			flex-wrap: wrap;
			align-items: flex-end;
		}
		.form-group { display: flex; flex-direction: column; gap: 6px; }
		.form-group label { font-size: 12px; color: var(--text-dim); }
		.form-group input { min-width: 180px; }
		.toast {
			position: fixed;
			bottom: 24px;
			left: 50%;
			transform: translateX(-50%);
			background: var(--card);
			border: 1px solid var(--border);
			color: var(--text);
			padding: 12px 24px;
			border-radius: 8px;
			font-size: 14px;
			z-index: 2000;
			opacity: 0;
			transition: opacity 0.3s;
		}
		.toast.show { opacity: 1; }
		.toast.success { border-color: var(--success); }
		.toast.error { border-color: var(--danger); }
		.empty { padding: 40px 20px; text-align: center; color: var(--text-dim); }
	</style>
</head>
<body>
	<div class="header">
		<h1>&#128187; <span>{{T "devices"}}</span></h1>
		<div class="header-right">
			<select class="lang-select" onchange="window.location.href='?lang='+this.value">
				{{range .Languages}}
				<option value="{{.}}" {{if eq . $.Lang}}selected{{end}}>{{index $.LangNames .}}</option>
				{{end}}
			</select>
			<a href="/" class="btn btn-ghost btn-sm">&#127793; {{T "title"}}</a>
			<a href="/logout" class="btn btn-ghost btn-sm">{{T "logout"}}</a>
		</div>
	</div>

	<div class="container">
		<div class="section">
			<div class="section-header">{{T "register_device"}}</div>
			<form class="form-row" id="registerForm">
				<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
				<div class="form-group">
					<label>{{T "device_id"}}</label>
					<input type="text" id="dev_id" placeholder="bgr-000002" required>
				</div>
				<div class="form-group">
					<label>{{T "device_name"}}</label>
					<input type="text" id="dev_name" placeholder="">
				</div>
				<div class="form-group">
					<label>{{T "device_secret"}}</label>
					<input type="text" id="dev_secret" placeholder="(auto-generate if empty)">
				</div>
				<button type="submit" class="btn">+ {{T "register_device"}}</button>
			</form>
		</div>

		<div class="section">
			<div class="section-header">{{T "devices"}} ({{len .Devices}})</div>
			{{if eq (len .Devices) 0}}
			<div class="empty">{{T "no_photos"}}</div>
			{{else}}
			<table>
				<thead>
					<tr>
						<th>{{T "device_id"}}</th>
						<th>{{T "device_name"}}</th>
						<th>{{T "device_secret"}}</th>
						<th>{{T "status"}}</th>
						<th>{{T "last_seen"}}</th>
						<th>{{T "total_photos"}}</th>
						<th></th>
					</tr>
				</thead>
				<tbody>
					{{range .Devices}}
					<tr>
						<td class="mono">{{.DeviceID}}</td>
						<td>{{.DeviceName}}</td>
						<td class="mono">{{.SecretHex}}</td>
						<td>{{.Status}}</td>
						<td>{{.LastSeen}}</td>
						<td>{{.PhotoCount}}</td>
						<td><button class="btn btn-sm btn-danger" onclick="deleteDevice('{{.DeviceID}}')">{{T "delete"}}</button></td>
					</tr>
					{{end}}
				</tbody>
			</table>
			{{end}}
		</div>
	</div>

	<div class="toast" id="toast"></div>

	<script>
		var csrfToken = "{{.CSRFToken}}";

		function showToast(msg, type) {
			var t = document.getElementById('toast');
			t.textContent = msg;
			t.className = 'toast show ' + (type || '');
			setTimeout(() => t.classList.remove('show'), 3000);
		}

		document.getElementById('registerForm').addEventListener('submit', function(e) {
			e.preventDefault();
			var body = {
				device_id: document.getElementById('dev_id').value.trim(),
				device_name: document.getElementById('dev_name').value.trim(),
				secret_hex: document.getElementById('dev_secret').value.trim()
			};
			fetch('/api/v1/devices', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'X-CSRF-Token': csrfToken
				},
				body: JSON.stringify(body)
			})
				.then(r => r.json())
				.then(data => {
					if (data.error) {
						showToast(data.error, 'error');
					} else {
						showToast('OK: ' + data.device_id, 'success');
						setTimeout(() => location.reload(), 1000);
					}
				})
				.catch(() => showToast('Network error', 'error'));
		});

		function deleteDevice(id) {
			if (!confirm('Delete device ' + id + '?')) return;
			fetch('/api/v1/devices/' + id, {
				method: 'DELETE',
				headers: { 'X-CSRF-Token': csrfToken }
			})
				.then(r => r.json())
				.then(data => {
					if (data.error) {
						showToast(data.error, 'error');
					} else {
						showToast('Deleted', 'success');
						setTimeout(() => location.reload(), 1000);
					}
				})
				.catch(() => showToast('Network error', 'error'));
		}
	</script>
</body>
</html>`

// ======================= Render Functions =======================

// loginData login page data
type loginData struct {
	Lang      string
	Languages []string
	LangNames map[string]string
	Error     bool
	CSRFToken string
}

// devicesPageData 设备管理页面数据
type devicesPageData struct {
	Lang      string
	LangName  string
	Languages []string
	LangNames map[string]string
	Devices   []deviceView
	CSRFToken string
}

// deviceView 设备管理页面的设备视图
type deviceView struct {
	DeviceID   string
	DeviceName string
	SecretHex  string
	Status     string
	LastSeen   string
	PhotoCount int64
}

// makeFuncMap creates a template FuncMap with translation support for the given language
func makeFuncMap(lang string) template.FuncMap {
	return template.FuncMap{
		"T": func(key string) string {
			return i18n.Translate(lang, i18n.Key(key))
		},
	}
}

// renderGallery renders the gallery page
func renderGallery(w http.ResponseWriter, data galleryData) {
	tpl := template.Must(template.New("gallery").Funcs(makeFuncMap(data.Lang)).Parse(galleryTpl))
	if err := tpl.Execute(w, data); err != nil {
		slog.Error("模板: 画廊渲染错误", "error", err)
	}
}

// renderLogin renders the login page
func renderLogin(w http.ResponseWriter, lang, csrfToken string, hasError bool) {
	data := loginData{
		Lang:      lang,
		Languages: i18n.SupportedLanguages,
		LangNames: i18n.LanguageNames,
		Error:     hasError,
		CSRFToken: csrfToken,
	}
	tpl := template.Must(template.New("login").Funcs(makeFuncMap(lang)).Parse(loginTpl))
	if err := tpl.Execute(w, data); err != nil {
		slog.Error("模板: 登录渲染错误", "error", err)
	}
}

// renderDevicesPage renders the device management page
func renderDevicesPage(w http.ResponseWriter, data devicesPageData) {
	tpl := template.Must(template.New("devices").Funcs(makeFuncMap(data.Lang)).Parse(devicesTpl))
	if err := tpl.Execute(w, data); err != nil {
		slog.Error("模板: 设备管理页渲染错误", "error", err)
	}
}

// ======================= 拍照模式设置页面 =======================

// photoModePageData 拍照模式设置页面数据
type photoModePageData struct {
	Lang       string
	LangName   string
	Languages  []string
	LangNames  map[string]string
	Devices    []photoModeDeviceView
	SaveFolder string
	CustomDir  string
	CSRFToken  string
}

// photoModeDeviceView 拍照模式设备视图
type photoModeDeviceView struct {
	DeviceID      string
	DeviceName    string
	PhotoInterval int
	Minutes       int
	Seconds       int
}

// photoModeTpl 拍照模式设置页面模板
const photoModeTpl = `<!DOCTYPE html>
<html lang="{{.Lang}}">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>{{T "photo_mode_settings"}} - {{T "title"}}</title>
	<style>
		* { margin: 0; padding: 0; box-sizing: border-box; }
		:root {
			--bg: #0f1117;
			--card: #1a1d27;
			--border: #2a2d3a;
			--text: #e0e0e0;
			--text-dim: #888;
			--accent: #4fc3f7;
			--accent-hover: #29b6f6;
			--danger: #ef5350;
			--success: #66bb6a;
		}
		body {
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'Noto Sans CJK SC', 'Noto Sans CJK JP', 'Noto Sans CJK TC', sans-serif;
			background: var(--bg);
			color: var(--text);
			min-height: 100vh;
		}
		.header {
			background: var(--card);
			border-bottom: 1px solid var(--border);
			padding: 16px 24px;
			display: flex;
			align-items: center;
			justify-content: space-between;
			flex-wrap: wrap;
			gap: 12px;
		}
		.header h1 { font-size: 20px; font-weight: 600; }
		.header h1 span { color: var(--accent); }
		.header-right { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; }
		.lang-select, input, select {
			background: var(--bg);
			color: var(--text);
			border: 1px solid var(--border);
			border-radius: 6px;
			padding: 8px 10px;
			font-size: 13px;
		}
		.btn {
			background: var(--accent);
			color: #000;
			border: none;
			border-radius: 6px;
			padding: 8px 16px;
			font-size: 13px;
			cursor: pointer;
			text-decoration: none;
			display: inline-flex;
			align-items: center;
			gap: 4px;
			transition: background 0.2s;
		}
		.btn:hover { background: var(--accent-hover); }
		.btn-sm { padding: 4px 10px; font-size: 12px; }
		.btn-ghost {
			background: transparent;
			border: 1px solid var(--border);
			color: var(--text);
		}
		.btn-ghost:hover { border-color: var(--accent); }
		.container { max-width: 900px; margin: 0 auto; padding: 24px; }
		.section {
			background: var(--card);
			border: 1px solid var(--border);
			border-radius: 12px;
			margin-bottom: 24px;
			overflow: hidden;
		}
		.section-header {
			padding: 16px 20px;
			border-bottom: 1px solid var(--border);
			font-size: 16px;
			font-weight: 600;
			display: flex;
			align-items: center;
			gap: 8px;
		}
		.section-desc {
			padding: 12px 20px;
			color: var(--text-dim);
			font-size: 13px;
			border-bottom: 1px solid var(--border);
		}
		.section-body { padding: 20px; }

		/* 拍照间隔设置 */
		.interval-row {
			display: flex;
			align-items: center;
			gap: 12px;
			flex-wrap: wrap;
			margin-bottom: 16px;
		}
		.interval-input-group {
			display: flex;
			align-items: center;
			gap: 6px;
		}
		.interval-input-group input {
			width: 80px;
			text-align: center;
			font-size: 18px;
			font-weight: 600;
			padding: 10px;
		}
		.interval-input-group label {
			font-size: 14px;
			color: var(--text-dim);
		}
		.interval-hint {
			color: var(--text-dim);
			font-size: 13px;
			margin-top: 8px;
			line-height: 1.5;
		}
		.checkbox-row {
			display: flex;
			align-items: center;
			gap: 8px;
			margin-bottom: 16px;
			cursor: pointer;
		}
		.checkbox-row input { width: auto; }
		.checkbox-row label { font-size: 14px; cursor: pointer; }

		/* 保存文件夹设置 */
		.folder-row {
			display: flex;
			gap: 8px;
			align-items: center;
			flex-wrap: wrap;
		}
		.folder-input {
			flex: 1;
			min-width: 300px;
			font-family: 'SF Mono', 'Monaco', 'Consolas', monospace;
			font-size: 13px;
		}
		.current-folder {
			background: var(--bg);
			border: 1px solid var(--border);
			border-radius: 6px;
			padding: 12px 16px;
			margin-bottom: 12px;
			font-size: 13px;
			color: var(--text-dim);
		}
		.current-folder strong { color: var(--accent); }

		/* 设备列表 */
		table { width: 100%; border-collapse: collapse; }
		th, td {
			padding: 12px 20px;
			text-align: left;
			border-bottom: 1px solid var(--border);
			font-size: 13px;
		}
		th { color: var(--text-dim); font-weight: 500; }
		.mono { font-family: 'SF Mono', 'Monaco', 'Consolas', monospace; font-size: 12px; }
		.interval-display {
			display: inline-flex;
			align-items: center;
			gap: 4px;
			background: var(--bg);
			padding: 4px 10px;
			border-radius: 6px;
			font-weight: 600;
			color: var(--accent);
		}

		/* 保存按钮 */
		.save-bar {
			padding: 16px 20px;
			border-top: 1px solid var(--border);
			display: flex;
			justify-content: flex-end;
		}
		.btn-save {
			background: var(--success);
			color: #fff;
			border: none;
			border-radius: 6px;
			padding: 10px 24px;
			font-size: 14px;
			font-weight: 600;
			cursor: pointer;
			transition: background 0.2s;
		}
		.btn-save:hover { background: #4caf50; }

		/* Toast */
		.toast {
			position: fixed;
			bottom: 24px;
			left: 50%;
			transform: translateX(-50%);
			background: var(--card);
			border: 1px solid var(--border);
			color: var(--text);
			padding: 12px 24px;
			border-radius: 8px;
			font-size: 14px;
			z-index: 2000;
			opacity: 0;
			transition: opacity 0.3s;
		}
		.toast.show { opacity: 1; }
		.toast.success { border-color: var(--success); }
		.toast.error { border-color: var(--danger); }
		.empty { padding: 40px 20px; text-align: center; color: var(--text-dim); }

		@media (max-width: 600px) {
			.header { padding: 12px 16px; }
			.container { padding: 12px; }
			.interval-input-group input { width: 60px; font-size: 16px; }
			.folder-input { min-width: 200px; }
		}
	</style>
</head>
<body>
	<div class="header">
		<h1>&#128247; <span>{{T "photo_mode_settings"}}</span></h1>
		<div class="header-right">
			<select class="lang-select" onchange="window.location.href='?lang='+this.value">
				{{range .Languages}}
				<option value="{{.}}" {{if eq . $.Lang}}selected{{end}}>{{index $.LangNames .}}</option>
				{{end}}
			</select>
			<a href="/" class="btn btn-ghost btn-sm">&#127793; {{T "title"}}</a>
			<a href="/devices" class="btn btn-ghost btn-sm">&#128187; {{T "devices"}}</a>
			<a href="/logout" class="btn btn-ghost btn-sm">{{T "logout"}}</a>
		</div>
	</div>

	<div class="container">
		<!-- 拍照间隔设置 -->
		<div class="section">
			<div class="section-header">&#9201; {{T "photo_interval"}}</div>
			<div class="section-desc">{{T "settings_desc"}}</div>
			<div class="section-body">
				<div class="interval-row">
					<div class="interval-input-group">
						<input type="number" id="minutes" min="0" max="1440" value="{{if gt (len .Devices) 0}}{{(index .Devices 0).Minutes}}{{else}}1{{end}}" onchange="updateTotal()">
						<label>{{T "minutes"}}</label>
					</div>
					<div class="interval-input-group">
						<input type="number" id="seconds" min="0" max="59" value="{{if gt (len .Devices) 0}}{{(index .Devices 0).Seconds}}{{else}}0{{end}}" onchange="updateTotal()">
						<label>{{T "seconds"}}</label>
					</div>
					<div class="interval-display" id="totalDisplay">= 60s</div>
				</div>
				<p class="interval-hint">{{T "interval_hint"}}</p>
				<div class="checkbox-row">
					<input type="checkbox" id="applyToAll" checked>
					<label for="applyToAll">{{T "apply_to_all"}}</label>
				</div>
			</div>
		</div>

		<!-- 保存文件夹设置 -->
		<div class="section">
			<div class="section-header">&#128193; {{T "save_folder"}}</div>
			<div class="section-desc">{{T "folder_hint"}}</div>
			<div class="section-body">
				<div class="current-folder">
					{{T "current_interval"}}: <strong>{{.SaveFolder}}</strong>
				</div>
				<div class="folder-row">
					<input type="text" class="folder-input" id="saveFolder" placeholder="C:\BioRecorder\Photos or /home/user/photos" value="{{.CustomDir}}">
					<button class="btn btn-ghost btn-sm" onclick="selectFolder()">&#128194; {{T "browse"}}</button>
				</div>
				<p class="interval-hint">{{T "default_folder"}}: ./captures</p>
			</div>
		</div>

		<!-- 设备列表 -->
		<div class="section">
			<div class="section-header">&#128225; {{T "devices"}} ({{len .Devices}})</div>
			{{if eq (len .Devices) 0}}
			<div class="empty">{{T "no_photos"}}</div>
			{{else}}
			<table>
				<thead>
					<tr>
						<th>{{T "device_id"}}</th>
						<th>{{T "device_name"}}</th>
						<th>{{T "current_interval"}}</th>
					</tr>
				</thead>
				<tbody>
					{{range .Devices}}
					<tr>
						<td class="mono">{{.DeviceID}}</td>
						<td>{{.DeviceName}}</td>
						<td>
							<span class="interval-display">{{.Minutes}} {{T "minutes"}} {{.Seconds}} {{T "seconds"}} ({{.PhotoInterval}}s)</span>
						</td>
					</tr>
					{{end}}
				</tbody>
			</table>
			{{end}}
			<div class="save-bar">
				<button class="btn-save" onclick="saveSettings()">&#10003; {{T "save_settings"}}</button>
			</div>
		</div>
	</div>

	<div class="toast" id="toast"></div>

	<script>
		var csrfToken = "{{.CSRFToken}}";

		function updateTotal() {
			var min = parseInt(document.getElementById('minutes').value) || 0;
			var sec = parseInt(document.getElementById('seconds').value) || 0;
			var total = min * 60 + sec;
			document.getElementById('totalDisplay').textContent = '= ' + total + 's';
		}

		function selectFolder() {
			// Web 环境无法直接访问文件系统, 用户手动输入路径
			// 提示用户输入路径
			var current = document.getElementById('saveFolder').value;
			var path = prompt('{{T "save_folder"}}:', current || 'C:\\BioRecorder\\Photos');
			if (path !== null) {
				document.getElementById('saveFolder').value = path;
			}
		}

		function saveSettings() {
			var minutes = parseInt(document.getElementById('minutes').value) || 0;
			var seconds = parseInt(document.getElementById('seconds').value) || 0;
			var applyToAll = document.getElementById('applyToAll').checked;
			var saveFolder = document.getElementById('saveFolder').value.trim();

			if (minutes === 0 && seconds === 0) {
				showToast('Error: interval must be at least 1 second', 'error');
				return;
			}

			var body = {
				minutes: minutes,
				seconds: seconds,
				apply_to_all: applyToAll,
				save_folder: saveFolder
			};

			fetch('/api/v1/photo-mode', {
				method: 'PUT',
				headers: {
					'Content-Type': 'application/json',
					'X-CSRF-Token': csrfToken
				},
				body: JSON.stringify(body)
			})
				.then(r => r.json())
				.then(data => {
					if (data.error) {
						showToast(data.error, 'error');
					} else {
						showToast('{{T "settings_saved"}}', 'success');
						setTimeout(() => location.reload(), 1500);
					}
				})
				.catch(() => showToast('Network error', 'error'));
		}

		function showToast(msg, type) {
			var t = document.getElementById('toast');
			t.textContent = msg;
			t.className = 'toast show ' + (type || '');
			setTimeout(() => t.classList.remove('show'), 3000);
		}

		// 初始化显示
		updateTotal();
	</script>
</body>
</html>`

// renderPhotoModePage renders the photo mode settings page
func renderPhotoModePage(w http.ResponseWriter, data photoModePageData) {
	tpl := template.Must(template.New("photoMode").Funcs(makeFuncMap(data.Lang)).Parse(photoModeTpl))
	if err := tpl.Execute(w, data); err != nil {
		slog.Error("模板: 拍照模式页渲染错误", "error", err)
	}
}

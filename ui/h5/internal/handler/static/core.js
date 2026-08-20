        const searchInput = document.getElementById('searchInput');
        function hasContentKeyword() {
            const el = document.getElementById('contentInput');
            return !!(el && el.value.trim());
        }
        const resultsBody = document.getElementById('resultsBody');
        const resultsCount = document.getElementById('resultsCount');
        const exportButtons = document.getElementById('exportButtons');
        const searchHistoryContainer = document.getElementById('searchHistoryContainer');
        const searchHistoryItems = document.getElementById('searchHistoryItems');
        
        let debounceTimer = null;
        let debounceDelay = 300; // 按键防抖延迟(ms)，可在设置中调整
        let currentResults = [];
        let currentMatches = [];
        let currentSort = { field: null, display: null, direction: 'asc' };
        let openEnabled = true;   // set from server status (open_supported)
        let activePanel = null;
        let currentPage = 1;
        let lastQuery = '';
        let currentTotal = 0;
        
        // Initialize
        document.addEventListener('DOMContentLoaded', () => {
            applyLang();
            loadFavorites();
            saveFavorites();
            loadSearchHistory();
            loadSettings();
            initSortHandlers();
            initColResizers();
            refreshStatus();
        });

        // ---- Resizable table columns ----
        // Each header cell gets a drag handle on its right edge; dragging it
        // resizes that column (px), and the fixed-layout table re-flows the
        // remaining percentage columns automatically. Widths persist in
        // localStorage so the user's layout survives reloads.
        let colResizing = null;
        function initColResizers() {
            const ths = document.querySelectorAll('#resultsTable th');
            ths.forEach((th, idx) => {
                const grip = document.createElement('span');
                grip.className = 'col-resizer';
                grip.dataset.col = idx;
                grip.addEventListener('mousedown', colResizeStart);
                grip.addEventListener('click', e => e.stopPropagation()); // don't trigger sort
                grip.addEventListener('touchstart', colResizeStartTouch);
                th.appendChild(grip);
            });
            try {
                const saved = JSON.parse(localStorage.getItem('golocateColWidths') || '{}');
                // On small windows the CSS breakpoints manage the layout; only
                // apply persisted pixel widths on wide (desktop) windows.
                if ((window.innerWidth || 0) >= 900) {
                    ths.forEach((th, idx) => {
                        const w = Number(saved[idx]);
                        if (w >= 70 && w <= 1800) th.style.width = w + 'px';
                    });
                }
            } catch (e) { /* ignore corrupt storage */ }
        }
        function colResizeStart(e) {
            e.preventDefault();
            e.stopPropagation();
            const th = e.target.parentElement;
            colResizing = {
                idx: +e.target.dataset.col,
                startX: e.clientX,
                startW: th.getBoundingClientRect().width,
                th: th
            };
            document.body.classList.add('col-resizing');
        }
        function colResizeStartTouch(e) {
            const t = e.touches && e.touches[0];
            if (!t) return;
            e.preventDefault();
            e.stopPropagation();
            const th = e.target.parentElement;
            colResizing = {
                idx: +e.target.dataset.col,
                startX: t.clientX,
                startW: th.getBoundingClientRect().width,
                th: th,
                touchId: t.identifier
            };
            document.body.classList.add('col-resizing');
            document.addEventListener('touchmove', colResizeMove, { passive: false });
            document.addEventListener('touchend', colResizeEnd);
        }
        function colResizeMove(e) {
            if (!colResizing) return;
            const pt = (e.touches && e.touches[0]) || null;
            const x = pt ? pt.clientX : e.clientX;
            const delta = x - colResizing.startX;
            const w = Math.max(70, colResizing.startW + delta);
            colResizing.th.style.width = w + 'px';
        }
        function colResizeEnd() {
            if (!colResizing) return;
            document.body.classList.remove('col-resizing');
            document.removeEventListener('touchmove', colResizeMove);
            document.removeEventListener('touchend', colResizeEnd);
            const finalW = colResizing.th.getBoundingClientRect().width;
            try {
                const saved = JSON.parse(localStorage.getItem('golocateColWidths') || '{}');
                saved[colResizing.idx] = finalW;
                localStorage.setItem('golocateColWidths', JSON.stringify(saved));
            } catch (e) { /* ignore */ }
            colResizing = null;
        }
        // mouse drag: global move/up
        document.addEventListener('mousemove', colResizeMove);
        document.addEventListener('mouseup', colResizeEnd);

        // ---- Right-click context menu on result rows ----
        function hideCtxMenu() {
            const m = document.getElementById('ctxMenu');
            if (m) m.style.display = 'none';
        }
        function showRowContextMenu(i, x, y) {
            const menu = document.getElementById('ctxMenu');
            if (!menu) return;
            const path = resultPathAt(i) || '';
            let items = '';
            if (openEnabled) {
                items += '<button class="ctx-item" data-act="open">' + t('open') + '</button>';
                items += '<button class="ctx-item" data-act="opendir">' + t('openDir') + '</button>';
            }
            items += '<button class="ctx-item" data-act="copy">' + t('copy') + '</button>';
            const favNow = typeof isFavorite === 'function' && isFavorite(path);
            items += '<button class="ctx-item" data-act="fav">' + (favNow ? '★ ' + t('unfav') : '☆ ' + t('favorite')) + '</button>';
            menu.innerHTML = items;
            menu.style.display = 'block';
            const w = menu.offsetWidth, h = menu.offsetHeight;
            menu.style.left = Math.max(4, Math.min(x, window.innerWidth - w - 8)) + 'px';
            menu.style.top = Math.max(4, Math.min(y, window.innerHeight - h - 8)) + 'px';
            menu.dataset.index = i;
        }
        document.addEventListener('contextmenu', (e) => {
            const tr = e.target && e.target.closest ? e.target.closest('tr[data-index]') : null;
            if (!tr || !resultsBody.contains(tr)) return;
            e.preventDefault();
            showRowContextMenu(parseInt(tr.dataset.index, 10), e.clientX, e.clientY);
        });
        document.addEventListener('click', (e) => {
            if (!e.target || !e.target.closest || !e.target.closest('#ctxMenu')) hideCtxMenu();
        }, true);
        window.addEventListener('scroll', hideCtxMenu, true);
        document.addEventListener('keydown', (e) => { if (e.key === 'Escape') hideCtxMenu(); });
        const ctxMenuEl = document.getElementById('ctxMenu');
        if (ctxMenuEl) {
            ctxMenuEl.addEventListener('click', (e) => {
                const act = e.target && e.target.dataset ? e.target.dataset.act : null;
                if (!act) return;
                const idx = parseInt(ctxMenuEl.dataset.index, 10);
                if (act === 'open') openResult(idx);
                else if (act === 'opendir') openDirResult(idx);
                else if (act === 'copy') copyResult(idx);
                else if (act === 'fav') toggleFavorite(idx);
                hideCtxMenu();
            });
        }

        
        // Server Status & Index Rebuild
        async function refreshStatus() {
            const bar = document.getElementById('statusBar');
            const text = document.getElementById('statusText');
            try {
                const r = await fetch('/api/status');
                const data = await r.json();
                bar.style.display = 'flex';
                if (data.error) {
                    bar.innerHTML = '<span class="dot off"></span><span>' + t('offline') + ': ' + escapeHtml(data.error) + '</span>';
                    return;
                }
                if (data && data.open_supported !== undefined) openEnabled = !!data.open_supported;
                const dotClass = data.is_building ? 'building' : (data.running ? 'on' : 'off');
                let building = data.is_building ? t('statusBuilding') : '';
                if (data.is_building) {
                    if (data.build_scanned) building += t('statusScanned') + ' ' + data.build_scanned;
                    building += '...';
                }
                text.innerHTML = '<span class="dot ' + dotClass + '"></span>' +
                    '<span class="stat">' + t('statusRunning') + ': <b>' + (data.running ? t('yes') : t('no')) + '</b></span>' +
                    '<span class="stat">' + t('statusIndexFiles') + ': <b>' + (data.index_size || 0) + '</b></span>' +
                    '<span class="stat">' + t('statusIndexed') + ': <b>' + (data.indexed_file_count || 0) + '</b></span>' +
                    '<span class="stat">' + t('statusLastBuild') + ': <b>' + escapeHtml(data.last_build_time ? data.last_build_time.replace('T', ' ').slice(0, 19) : '-') + '</b></span>' +
                    '<span class="stat">' + t('statusUptime') + ': <b>' + escapeHtml(data.uptime || '-') + '</b></span>' + building;

                // Detail panel (status tab)
                const body = document.getElementById('statusPanelBody');
                if (body) {
                    const stats = data.stats || {};
                    body.innerHTML =
                        row(t('statusRunning'), (data.running ? t('yes') : t('no')) + (data.is_building ? ' · ' + t('statusBuilding') : '')) +
                        row(t('statusIndexFiles'), data.index_size || 0) +
                        row(t('statusIndexed'), data.indexed_file_count || 0) +
                        row(t('statusLastBuild'), data.last_build_time ? data.last_build_time.replace('T', ' ').slice(0, 19) : '-') +
                        row(t('statusUptime'), data.uptime || '-') +
                        row(t('statusProtocol'), data.protocol_version || '-') +
                        row(t('statusPid'), data.pid || '-') +
                        (data.is_building ? row(t('statusBuilding'), (data.build_scanned || 0) + (data.build_history ? ' · ' + t('metaBuilds') + ': ' + (data.build_history || 0) : '')) : '') +
                        (stats.searches ? row(t('statusSearches'), stats.searches) : '') +
                        (stats.content_searches ? row(t('statusContentSearches'), stats.content_searches) : '') +
                        (stats.builds ? row(t('statusBuilds'), stats.builds) : '') +
                        (stats.opens ? row(t('statusOpens'), stats.opens) : '');
                    function row(k, v) { return '<div class="stat-row"><span>' + escapeHtml(String(k)) + '</span><b>' + escapeHtml(String(v)) + '</b></div>'; }
                }
            } catch (err) {
                bar.style.display = 'flex';
                bar.innerHTML = '<span class="dot off"></span><span>' + t('statusFail') + ': ' + escapeHtml(err.message) + '</span>';
            }
        }
        
        async function rebuildIndex() {
            try {
                const r = await fetch('/api/build', { method: 'POST' });
                const data = await r.json();
                if (data.status === 'build started') {
                    alert('索引重建已开始');
                    setTimeout(refreshStatus, 1000);
                } else {
                    alert('重建失败: ' + (data.error || r.statusText));
                }
            } catch (err) {
                alert('重建失败: ' + err.message);
            }
        }
        
        // Settings Functions
        // Top tabs: at most one panel is visible; the active tab is highlighted.
        // Clicking the active tab again closes the panel.
        function switchPanel(id) {
            const panel = document.getElementById(id);
            if (!panel) return;
            const alreadyOpen = activePanel === id && panel.style.display === 'block';
            ['favPanel', 'settingsPanel', 'serverPanel', 'statusPanel'].forEach(pid => {
                const el = document.getElementById(pid);
                if (el && el !== panel) el.style.display = 'none';
            });
            document.querySelectorAll('.header-buttons .tab').forEach(b => b.classList.remove('active'));
            if (alreadyOpen) {
                panel.style.display = 'none';
                activePanel = null;
                return;
            }
            panel.style.display = 'block';
            activePanel = id;
            const tb = document.querySelector('.header-buttons .tab[data-target="' + id + '"]');
            if (tb) tb.classList.add('active');
            if (id === 'favPanel') { renderFavorites(); renderRecents(); }
            if (id === 'statusPanel') { refreshStatus(); }
        }
        function toggleSettings() {
            const panel = document.getElementById('settingsPanel');
            panel.style.display = panel.style.display === 'none' ? 'block' : 'none';
        }
        
        function saveSettings() {
            const settings = {
                defaultLimit: document.getElementById('defaultLimit').value,
                defaultIgnoreCase: document.getElementById('defaultIgnoreCase').checked,
                theme: document.getElementById('themeSelect').value,
                debounceDelay: document.getElementById('debounceDelay').value
            };
            localStorage.setItem('golocateSettings', JSON.stringify(settings));
            alert('设置已保存');
            applySettings(settings);
        }
        
        function loadSettings() {
            const settings = JSON.parse(localStorage.getItem('golocateSettings') || '{}');
            if (settings.defaultLimit) {
                document.getElementById('defaultLimit').value = settings.defaultLimit;
                document.getElementById('maxResults').value = settings.defaultLimit;
            }
            if (settings.defaultIgnoreCase !== undefined) {
                document.getElementById('defaultIgnoreCase').checked = settings.defaultIgnoreCase;
                document.getElementById('ignoreCase').checked = settings.defaultIgnoreCase;
            }
            if (settings.theme) {
                document.getElementById('themeSelect').value = settings.theme;
                applySettings(settings);
            }
            if (settings.debounceDelay) {
                document.getElementById('debounceDelay').value = settings.debounceDelay;
                debounceDelay = parseInt(settings.debounceDelay) || 2500;
            }
        }
        
        function applySettings(settings) {
            if (settings.theme === 'light') {
                document.body.classList.add('light-theme');
            } else {
                document.body.classList.remove('light-theme');
            }
        }
        
        // Sort Functions
        // Header clicks re-run the search with server-side sort parameters,
        // so sorting applies across the full result set, not just the page.
        const SORT_FIELD_MAP = { Name: 'name', Path: 'path', Size: 'size', ModTime: 'time' };

        function initSortHandlers() {
            const headers = document.querySelectorAll('th.sortable');
            headers.forEach(th => {
                th.addEventListener('click', () => {
                    const field = th.dataset.sort;
                    sortResults(field);
                });
            });
        }
        
        function sortResults(displayField) {
            const serverField = SORT_FIELD_MAP[displayField] || displayField.toLowerCase();
            if (currentSort.field === serverField) {
                currentSort.direction = currentSort.direction === 'asc' ? 'desc' : 'asc';
            } else {
                currentSort.field = serverField;
                currentSort.direction = 'asc';
            }
            currentSort.display = displayField;
            // Content-search matches keep file order; only path searches sort.
            updateSortIndicators();
            if (!hasContentKeyword()) {
                search(); // re-run with server-side sort (applies across all pages)
            }
        }
        
        function updateSortIndicators() {
            // Remove all sort classes
            document.querySelectorAll('th.sortable').forEach(th => {
                th.classList.remove('sort-asc', 'sort-desc');
            });
            
            // Add current sort class
            if (currentSort.field) {
                const th = document.querySelector(`th[data-sort="${currentSort.display}"]`);
                if (th) {
                    th.classList.add(`sort-${currentSort.direction}`);
                }
            }
        }
        
        // Keyboard Shortcuts
        document.addEventListener('keydown', (e) => {
            const target = e.target;
            const typing = target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable);
            if (typing) {
                if (e.key === 'Escape') {
                    target.blur();
                    searchInput.focus();
                    e.preventDefault();
                }
                return;
            }
            switch (e.key) {
                case '/':
                    searchInput.focus();
                    e.preventDefault();
                    break;
                case 'PageDown':
                    if (!document.getElementById('nextPageBtn').disabled) nextPage();
                    e.preventDefault();
                    break;
                case 'PageUp':
                    if (!document.getElementById('prevPageBtn').disabled) prevPage();
                    e.preventDefault();
                    break;
                case 'Home':
                    if (!document.getElementById('firstPageBtn').disabled) goFirstPage();
                    break;
                case 'End':
                    if (!document.getElementById('lastPageBtn').disabled) goLastPage();
                    break;
                case 'g':
                    document.getElementById('pageJump').focus();
                    break;
            }
        });
        
        // Search Functions
        searchInput.addEventListener('input', () => {
            clearTimeout(debounceTimer);
            debounceTimer = setTimeout(() => {
                search();
            }, debounceDelay);
        });
        
        searchInput.addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                clearTimeout(debounceTimer);
                search();
            }
        });
        
        async function search() {
            const query = searchInput.value.trim();
            if (!query && !hasContentKeyword()) {
                resultsBody.innerHTML = '<tr><td colspan="6" class="empty-state">' + t('emptyStart') + '</td></tr>';
                resultsCount.textContent = '';
                exportButtons.style.display = 'none';
                document.getElementById('paginationControls').style.display = 'none';
                currentResults = [];
                currentMatches = [];
                currentPage = 1;
                return;
            }
            
            // New query resets to page 1 and clears the server sort; the same
            // query (paging, re-run) keeps its server-side sort + indicators.
            if (query !== lastQuery) {
                lastQuery = query;
                currentPage = 1;
                if (currentSort.field) {
                    currentSort = { field: null, display: null, direction: 'asc' };
                    updateSortIndicators();
                }
            }
            
            resultsBody.innerHTML = '<tr><td colspan="6" class="status">' + t('searching') + '</td></tr>';
            
            const ignoreCase = document.getElementById('ignoreCase').checked;
            const basename = document.getElementById('basenameMode').checked;
            const patternMode = document.getElementById('patternMode').value;
            const regexMode = patternMode === 'regex';
            const content = (() => { const el = document.getElementById('contentInput'); return el ? el.value.trim() : ''; })();
            const contentMode = content !== '';
            const dedupe = document.getElementById('dedupeMode').checked;
            const scope = document.getElementById('scopeInput').value.trim();
            const types = document.getElementById('typeInput').value.trim();
            const exclude = document.getElementById('excludeInput').value.trim();
            const minSize = document.getElementById('minSizeInput').value;
            const maxSize = document.getElementById('maxSizeInput').value;
            const maxResults = document.getElementById('maxResults').value;
            const mtimeAfter = document.getElementById('mtimeAfter') ? document.getElementById('mtimeAfter').value.trim() : '';
            const mtimeBefore = document.getElementById('mtimeBefore') ? document.getElementById('mtimeBefore').value.trim() : '';
            const pageSize = parseInt(maxResults) || 100;
            const offset = (currentPage - 1) * pageSize;
            
            try {
                let url = '/api/search?q=' + encodeURIComponent(query) + 
                    '&ignore_case=' + ignoreCase + '&regex=' + regexMode +
                    '&pattern_mode=' + encodeURIComponent(patternMode) +
                    '&basename=' + basename +
                    '&dedupe=' + dedupe +
                    '&scope=' + encodeURIComponent(scope) +
                    '&type=' + encodeURIComponent(types) +
                    '&exclude=' + encodeURIComponent(exclude) +
                    '&min_size=' + minSize + '&max_size=' + maxSize +
                    '&mtime_after=' + encodeURIComponent(mtimeAfter) + '&mtime_before=' + encodeURIComponent(mtimeBefore) +
                    '&limit=' + pageSize + '&offset=' + offset;
                if (contentMode) {
                    url += '&content=' + encodeURIComponent(content);
                }
                if (currentSort.field && !content) {
                    url += '&sort_field=' + currentSort.field + '&sort_order=' + currentSort.direction;
                }
                const response = await fetch(url);
                const data = await response.json();
                
                if (data.error) {
                    resultsBody.innerHTML = '<tr><td colspan="6" class="error">' + t('errPrefix') + ': ' + escapeHtml(data.error) + '</td></tr>';
                    resultsCount.textContent = '';
                    exportButtons.style.display = 'none';
                    currentResults = [];
                    currentMatches = [];
                    return;
                }
                
                if ((!data.results || data.results.length === 0) && (!data.matches || data.matches.length === 0)) {
                    resultsBody.innerHTML = '<tr><td colspan="6" class="empty-state">' + t('noResults') + '</td></tr>';
                    resultsCount.textContent = '无结果';
                    exportButtons.style.display = 'none';
                    document.getElementById('paginationControls').style.display = 'none';
                    currentResults = [];
                    currentMatches = [];
                    return;
                }
                
                // Save to search history
                saveSearchHistory(query);
                
                // Store results for export and sorting
                currentResults = data.results || [];
                currentMatches = data.matches || [];
                currentTotal = data.total || currentResults.length || currentMatches.length;
                
                // Sort is server-side and already reflected in `currentSort`;
                // keep it across pages. Just refresh the arrow indicators.
                updateSortIndicators();
                
                // Render results
                renderResults();
                const shown = Math.max(currentResults.length, currentMatches.length);
                resultsCount.textContent = t('resultsFound') + ' ' + shown + ' (' + t('resultsTotal') + ' ' + currentTotal + ')';
                exportButtons.style.display = 'flex';
                updatePagination();
            } catch (err) {
                resultsBody.innerHTML = '<tr><td colspan="6" class="error">' + t('errPrefix') + ': ' + escapeHtml(err.message) + '</td></tr>';
                resultsCount.textContent = '';
                exportButtons.style.display = 'none';
                currentResults = [];
                currentMatches = [];
            }
        }
        
        // Pagination Functions
        function updatePagination() {
            const pageSize = parseInt(document.getElementById('maxResults').value) || 100;
            const totalPages = currentTotal > 0 ? Math.ceil(currentTotal / pageSize) : 1;
            const controls = document.getElementById('paginationControls');
            if (currentTotal === 0) {
                controls.style.display = 'none';
                return;
            }
            controls.style.display = 'flex';
            document.getElementById('pageInfo').textContent = t('pageInfo') + ' ' + currentPage + '/' + totalPages;
            document.getElementById('firstPageBtn').disabled = currentPage <= 1;
            document.getElementById('prevPageBtn').disabled = currentPage <= 1;
            document.getElementById('nextPageBtn').disabled = currentPage >= totalPages;
            document.getElementById('lastPageBtn').disabled = currentPage >= totalPages;
        }
        
        function goFirstPage() {
            if (currentPage <= 1) return;
            currentPage = 1;
            search();
        }
        
        function goLastPage() {
            const pageSize = parseInt(document.getElementById('maxResults').value) || 100;
            const totalPages = currentTotal > 0 ? Math.ceil(currentTotal / pageSize) : 1;
            if (currentPage >= totalPages) return;
            currentPage = totalPages;
            search();
        }
        
        function jumpToPage() {
            const pageSize = parseInt(document.getElementById('maxResults').value) || 100;
            const totalPages = currentTotal > 0 ? Math.ceil(currentTotal / pageSize) : 1;
            const n = parseInt(document.getElementById('pageJump').value);
            if (!n || n < 1 || n > totalPages) return;
            if (n === currentPage) return;
            currentPage = n;
            search();
        }
        
        function prevPage() {
            if (currentPage <= 1) return;
            currentPage--;
            search();
        }
        
        function nextPage() {
            currentPage++;
            search();
        }
        
        function renderResults() {
            if ((!currentResults || currentResults.length === 0) && (!currentMatches || currentMatches.length === 0)) return;
            
            const contentMode = currentMatches && currentMatches.length > 0;
            const count = Math.max(currentResults.length, currentMatches.length);
            
            let html = '';
            for (let i = 0; i < count; i++) {
                const result = currentResults[i] || {};
                const match = contentMode ? (currentMatches[i] || {}) : null;
                // In content mode the row's file columns come from the match
                // (the server now returns Name/Size/ModTime alongside each
                // match); fall back to the path/name search result.
                const row = (contentMode && match && match.Path) ? match : result;
                const modTime = row.ModTime ? formatDate(row.ModTime) : '-';
                const size = (row.Size !== undefined && row.Size !== null) ? formatSize(row.Size) : '-';
                const dispPath = row.Path || '';
                let dispName = row.Name || result.Name || '';
                if (!dispName && dispPath.indexOf('/') >= 0) dispName = dispPath.split('/').pop();
                
                let matchCell = '-';
                let matchTitle = '';
                if (match) {
                    const lineNo = t('linePrefix') + (match.LineNum || '') + ': ';
                    matchCell = lineNo + highlightMatch(match.Line || '', match.Match);
                    const ctx = [];
                    (match.Before || []).forEach(l => ctx.push('↑ ' + l));
                    ctx.push(match.Line || '');
                    (match.After || []).forEach(l => ctx.push('↓ ' + l));
                    matchTitle = ctx.join('\n');
                }
                
                html += '<tr data-index="' + i + '">';
                html += '<td class="col-name" title="' + escapeHtml(dispName) + '">' + escapeHtml(dispName || '-') + '</td>';
                html += '<td class="col-path" title="' + escapeHtml(dispPath) + '">' + highlightMatch(dispPath || '-', searchInput.value.trim()) + '</td>';
                html += '<td class="col-size">' + size + '</td>';
                html += '<td class="col-modtime">' + modTime + '</td>';
                html += '<td class="col-match" title="' + escapeHtml(matchTitle || matchCell) + '">' + matchCell + '</td>';
                const openBtns = openEnabled ?
                    '<button onclick="openResult(' + i + ')">' + t('open') + '</button>' +
                    '<button onclick="openDirResult(' + i + ')">' + t('openDir') + '</button>' : '';
                html += '<td class="col-actions">' +
                    openBtns +
                    '<button onclick="copyResult(' + i + ')">' + t('copy') + '</button>' +
                    '<button onclick="toggleFavorite(' + i + ')" id="fav-' + i + '" title="' + t('favorite') + '">☆</button>' +
                    '</td>';
                html += '</tr>';
            }
            
            resultsBody.innerHTML = html;
        }
        
        // Path actions: open via the daemon, or copy to clipboard (browser).
        function resultPathAt(i) {
            const result = currentResults[i] || {};
            const match = currentMatches && currentMatches[i] || {};
            return result.Path || match.Path || '';
        }

        function dirnameOf(path) {
            const i = path.lastIndexOf('/');
            return i > 0 ? path.slice(0, i) : '';
        }

        async function openDirResult(i) {
            const dir = dirnameOf(resultPathAt(i));
            if (!dir) { alert(t('noDirToOpen')); return; }
            try {
                const r = await fetch('/api/open', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ path: dir })
                });
                if (!r.ok) { alert(t('openFail') + ': ' + (await r.text())); return; }
                const data = await r.json();
                alert(t('opened') + ': ' + data.path);
            } catch (err) {
                alert(t('openFail') + ': ' + err.message);
            }
        }

        async function openResult(i) {
            const path = resultPathAt(i);
            if (!path) { alert('无路径可打开'); return; }
            try {
                const r = await fetch('/api/open', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ path: path })
                });
                if (!r.ok) {
                    alert('打开失败: ' + (await r.text()));
                    return;
                }
                const data = await r.json();
                recordRecent(data.path);
                alert(t('opened') + ': ' + data.path);
            } catch (err) {
                alert('打开失败: ' + err.message);
            }
        }

        async function copyResult(i) {
            const path = resultPathAt(i);
            if (!path) { alert('无路径可复制'); return; }
            try {
                await navigator.clipboard.writeText(path);
                alert('已复制路径: ' + path);
            } catch (err) {
                // Clipboard API unavailable (e.g. non-secure context): fall back
                // to a temporary textarea + execCommand.
                try {
                    const ta = document.createElement('textarea');
                    ta.value = path;
                    document.body.appendChild(ta);
                    ta.select();
                    document.execCommand('copy');
                    document.body.removeChild(ta);
                    alert('已复制路径: ' + path);
                } catch (err2) {
                    alert('复制失败: ' + err2.message);
                }
            }
        }
        

        // Search History Functions
        function saveSearchHistory(query) {
            const history = JSON.parse(localStorage.getItem('searchHistory') || '[]');
            const filtered = history.filter(item => item.query !== query);
            filtered.unshift({ query: query, timestamp: Date.now() });
            const trimmed = filtered.slice(0, 20);
            localStorage.setItem('searchHistory', JSON.stringify(trimmed));
            loadSearchHistory();
        }
        
        function loadSearchHistory() {
            const history = JSON.parse(localStorage.getItem('searchHistory') || '[]');
            
            if (history.length === 0) {
                searchHistoryContainer.style.display = 'none';
                return;
            }
            
            searchHistoryContainer.style.display = 'block';
            searchHistoryItems.innerHTML = '';
            
            history.slice(0, 10).forEach(item => {
                const div = document.createElement('div');
                div.className = 'search-history-item';
                div.textContent = item.query;
                div.onclick = () => {
                    searchInput.value = item.query;
                    search();
                };
                searchHistoryItems.appendChild(div);
            });
        }
        
        // Export Functions
        // Exports fetch the FULL result set (up to 100000) so the file is not
        // limited to the currently displayed page.
        async function exportResults(format) {
            const query = lastQuery;
            if (!query && currentResults.length === 0 && currentMatches.length === 0) {
                alert('无结果可导出');
                return;
            }
            // Re-run the search without pagination to get everything.
            const ignoreCase = document.getElementById('ignoreCase').checked;
            const basename = document.getElementById('basenameMode').checked;
            const patternMode = document.getElementById('patternMode').value;
            const regexMode = patternMode === 'regex';
            const contentKw = (() => { const el = document.getElementById('contentInput'); return el ? el.value.trim() : ''; })();
            const contentMode = contentKw !== '';
            const dedupe = document.getElementById('dedupeMode').checked;
            const scope = document.getElementById('scopeInput').value.trim();
            const types = document.getElementById('typeInput').value.trim();
            const exclude = document.getElementById('excludeInput').value.trim();
            const minSize = document.getElementById('minSizeInput').value;
            const maxSize = document.getElementById('maxSizeInput').value;

            const base = '/api/search?q=' + encodeURIComponent(query) +
                '&ignore_case=' + ignoreCase + '&regex=' + regexMode +
                '&pattern_mode=' + encodeURIComponent(patternMode) +
                '&basename=' + basename + '&dedupe=' + dedupe +
                    '&scope=' + encodeURIComponent(scope) +
                    '&type=' + encodeURIComponent(types) +
                    '&exclude=' + encodeURIComponent(exclude) +
                    '&min_size=' + minSize + '&max_size=' + maxSize +
                    '&mtime_after=' + encodeURIComponent(mtimeAfter) + '&mtime_before=' + encodeURIComponent(mtimeBefore) +
                    (contentMode ? '&content=' + encodeURIComponent(contentKw) : '');

            // Stream the export page-by-page (paths only; content search is
            // served as a single page), so huge result sets don't arrive as
            // one giant response / JSON object. (docs/PERFORMANCE.md H2)
            const EX_PAGE = 5000;
            let allResults = [], allMatches = [], total = null, offset = 0;
            try {
                for (;;) {
                    const page = contentMode ? (EX_PAGE * 20) : EX_PAGE;
                    const r = await fetch(base + '&limit=' + page + '&offset=' + offset);
                    const d = await r.json();
                    if (d.error) { alert('导出失败: ' + d.error); return; }
                    const rs = d.results || [];
                    const ms = d.matches || [];
                    allResults = allResults.concat(rs);
                    allMatches = allMatches.concat(ms);
                    if (total === null) total = d.total;
                    if (contentMode) break;
                    offset += EX_PAGE;
                    if (total !== null && offset >= total) break;
                    if (rs.length < EX_PAGE) break;
                }
            } catch (err) {
                alert('导出失败: ' + err.message);
                return;
            }

            const useMatches = allMatches.length > 0;
            if (allResults.length === 0 && !useMatches) {
                alert('无结果可导出');
                return;
            }
            
            let content, filename, mimeType;
            
            if (useMatches) {
                switch (format) {
                    case 'json':
                        content = JSON.stringify(allMatches, null, 2);
                        filename = 'search_results.json';
                        mimeType = 'application/json';
                        break;
                    case 'csv':
                        content = '文件名,路径,行号,匹配内容\n' + allMatches.map(m => 
                            `"${(m.Path || '').split('/').pop()}","${m.Path || ''}","${m.LineNum || ''}","${(m.Line || '').replace(/"/g, '""')}"`
                        ).join('\n');
                        filename = 'search_results.csv';
                        mimeType = 'text/csv';
                        break;
                    case 'txt':
                        content = allMatches.map(m => `${m.Path}:${m.LineNum}:${m.Line}`).join('\n');
                        filename = 'search_results.txt';
                        mimeType = 'text/plain';
                        break;
                }
            } else {
                switch (format) {
                    case 'json':
                        content = JSON.stringify(allResults, null, 2);
                        filename = 'search_results.json';
                        mimeType = 'application/json';
                        break;
                    case 'csv':
                        content = '文件名,路径,大小,修改时间\n' + allResults.map(r => 
                            `"${r.Name || ''}","${r.Path || ''}","${formatSize(r.Size)}","${formatDate(r.ModTime)}"`
                        ).join('\n');
                        filename = 'search_results.csv';
                        mimeType = 'text/csv';
                        break;
                    case 'txt':
                        content = allResults.map(r => r.Path).join('\n');
                        filename = 'search_results.txt';
                        mimeType = 'text/plain';
                        break;
                }
            }
            
            const blob = new Blob([content], { type: mimeType });
            const urlObj = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = urlObj;
            a.download = filename;
            a.click();
            URL.revokeObjectURL(urlObj);
        }
        
        // Utility Functions
        function formatSize(bytes) {
            if (!bytes || bytes === 0) return '0 B';
            if (bytes < 1024) return bytes + ' B';
            if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
            if (bytes < 1024 * 1024 * 1024) return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
            return (bytes / (1024 * 1024 * 1024)).toFixed(1) + ' GB';
        }
        
        function formatDate(dateStr) {
            if (!dateStr) return '-';
            try {
                const date = new Date(dateStr);
                if (isNaN(date.getTime())) return '-';
                // 零值时间（文件不存在或 stat 失败时 ModTime 为 0001 年）显示为 -
                if (date.getFullYear() < 1000) return '-';
                
                const year = date.getFullYear();
                const month = String(date.getMonth() + 1).padStart(2, '0');
                const day = String(date.getDate()).padStart(2, '0');
                const hour = String(date.getHours()).padStart(2, '0');
                const minute = String(date.getMinutes()).padStart(2, '0');
                
                return `${year}-${month}-${day} ${hour}:${minute}`;
            } catch (e) {
                return '-';
            }
        }
        
        function escapeHtml(text) {
            if (!text) return '';
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

// Toggle the collapsed advanced search options.
function toggleAdvanced() {
    const box = document.getElementById("advancedOptions");
    const btn = document.getElementById("advToggleBtn");
    const open = box.style.display !== "block";
    box.style.display = open ? "block" : "none";
    if (btn) btn.textContent = open ? t("btnAdvancedHide") : t("btnAdvanced");
}

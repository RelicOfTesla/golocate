        // ---- Favorites (localStorage bookmarks) ----
        let favoritePaths = [];
        function loadFavorites() {
            try { favoritePaths = JSON.parse(localStorage.getItem('golocateFavorites') || '[]'); }
            catch (e) { favoritePaths = []; }
        }
        function saveFavorites() {
            localStorage.setItem('golocateFavorites', JSON.stringify(favoritePaths));
            const favBtn = document.getElementById('favBtn');
            if (favBtn) favBtn.textContent = '⭐ ' + t('favorites') + (favoritePaths.length ? ' (' + favoritePaths.length + ')' : '');
        }
        function isFavorite(path) { return favoritePaths.some(f => f.path === path); }
        function toggleFavorite(i) {
            const path = resultPathAt(i);
            if (!path) return;
            loadFavorites();
            const idx = favoritePaths.findIndex(f => f.path === path);
            if (idx >= 0) { favoritePaths.splice(idx, 1); } else {
                favoritePaths.unshift({ path: path, name: (currentResults[i] || {}).Name || path.split('/').pop(), time: Date.now() });
            }
            saveFavorites();
            const btn = document.getElementById('fav-' + i);
            if (btn) btn.textContent = idx >= 0 ? '☆' : '★';
        }
        function renderFavorites() {
            loadFavorites();
            const list = document.getElementById('favList');
            list.innerHTML = '';
            if (favoritePaths.length === 0) {
                list.innerHTML = '<div style="color:#888; font-size:0.85em;">' + t('noFavorites') + '</div>';
                return;
            }
            favoritePaths.forEach((f, i) => {
                const row = document.createElement('div');
                row.style.cssText = 'display:flex; justify-content:space-between; align-items:center; gap:8px; background:#1a1a1a; border:1px solid #333; border-radius:4px; padding:6px 10px; margin-bottom:4px;';
                const span = document.createElement('span');
                span.textContent = f.name + ' — ' + f.path;
                span.style.cssText = 'font-family:monospace; font-size:12px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; flex:1; cursor:pointer;';
                span.title = f.path;
                span.onclick = () => openPath(f.path);
                const del = document.createElement('button');
                del.textContent = t('delete');
                del.style.cssText = 'padding:2px 8px; font-size:0.8em; background:#c0392b;';
                del.onclick = () => { favoritePaths.splice(i, 1); saveFavorites(); renderFavorites(); };
                row.appendChild(span);
                row.appendChild(del);
                list.appendChild(row);
            });
        }
        function toggleFavPanel() {
            const panel = document.getElementById('favPanel');
            panel.style.display = panel.style.display === 'none' ? 'block' : 'none';
            if (panel.style.display === 'block') { renderFavorites(); renderRecents(); }
        }
        // ---- Recent files (last opened) ----
        function recordRecent(path) {
            if (!path) return;
            let recents = [];
            try { recents = JSON.parse(localStorage.getItem('golocateRecent') || '[]'); } catch (e) { recents = []; }
            recents = recents.filter(r => r.path !== path);
            recents.unshift({ path: path, name: path.split('/').pop(), time: Date.now() });
            recents = recents.slice(0, 10);
            localStorage.setItem('golocateRecent', JSON.stringify(recents));
        }
        function renderRecents() {
            const list = document.getElementById('recentList');
            list.innerHTML = '';
            let recents = [];
            try { recents = JSON.parse(localStorage.getItem('golocateRecent') || '[]'); } catch (e) { recents = []; }
            if (recents.length === 0) {
                list.innerHTML = '<div style="color:#888; font-size:0.85em;">' + t('noRecent') + '</div>';
                return;
            }
            recents.forEach((f, i) => {
                const row = document.createElement('div');
                row.style.cssText = 'display:flex; justify-content:space-between; align-items:center; gap:8px; background:#1a1a1a; border:1px solid #333; border-radius:4px; padding:6px 10px; margin-bottom:4px;';
                const span = document.createElement('span');
                span.textContent = f.name + ' — ' + f.path;
                span.style.cssText = 'font-family:monospace; font-size:12px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; flex:1; cursor:pointer;';
                span.title = f.path;
                span.onclick = () => openPath(f.path);
                const del = document.createElement('button');
                del.textContent = '×';
                del.style.cssText = 'padding:2px 8px; font-size:0.8em; background:#555;';
                del.onclick = () => { recents.splice(i, 1); localStorage.setItem('golocateRecent', JSON.stringify(recents)); renderRecents(); };
                row.appendChild(span);
                row.appendChild(del);
                list.appendChild(row);
            });
        }
        async function openPath(path) {
            if (!path) { alert(t('noDirToOpen')); return; }
            try {
                const r = await fetch('/api/open', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ path: path })
                });
                if (!r.ok) { alert(t('openFail') + ': ' + (await r.text())); return; }
            } catch (err) {
                alert(t('openFail') + ': ' + err.message);
            }
        }
        
        // highlightMatch escapes text and wraps each occurrence of `match` in <mark>.
        function highlightMatch(text, match) {
            if (!match) return escapeHtml(text);
            const parts = String(text).split(match);
            if (parts.length === 1) return escapeHtml(text);
            return parts.map((p, i) =>
                i < parts.length - 1 ? escapeHtml(p) + '<mark>' + escapeHtml(match) + '</mark>' : escapeHtml(p)
            ).join('');
        }
        

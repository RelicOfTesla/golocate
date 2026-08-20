        // Server configuration edit
        function toSimpleYaml(obj) {
            let out = '';
            for (const k in obj) {
                const v = obj[k];
                if (Array.isArray(v)) {
                    out += k + ':';
                    if (v.length === 0) { out += ' []\n'; continue; }
                    out += '\n';
                    v.forEach(x => { out += '  - ' + x + '\n'; });
                } else {
                    out += k + ': ' + v + '\n';
                }
            }
            return out;
        }
        
        async function loadServerConfig() {
            try {
                const r = await fetch('/api/config');
                const cfg = await r.json();
                document.getElementById('serverConfigArea').value = toSimpleYaml(cfg);
                renderDirList(cfg.directories || []);
            } catch (err) {
                alert('加载配置失败: ' + err.message);
            }
        }

        // ---- Index directory management ----
        let currentDirs = [];

        function renderDirList(dirs) {
            currentDirs = dirs.slice();
            const box = document.getElementById('dirList');
            box.innerHTML = '';
            if (dirs.length === 0) {
                box.innerHTML = '<div style="color:#888; font-size:0.85em;">' + t('noDirs') + '</div>';
                return;
            }
            dirs.forEach((d, i) => {
                const row = document.createElement('div');
                row.style.cssText = 'display:flex; justify-content:space-between; align-items:center; gap:8px; background:#1a1a1a; border:1px solid #333; border-radius:4px; padding:6px 10px; margin-bottom:4px;';
                const span = document.createElement('span');
                span.textContent = d;
                span.style.cssText = 'font-family:monospace; font-size:12px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; flex:1;';
                const del = document.createElement('button');
                del.textContent = t('delete');
                del.style.cssText = 'padding:2px 8px; font-size:0.8em; background:#c0392b;';
                del.onclick = () => { currentDirs.splice(i, 1); renderDirList(currentDirs); };
                row.appendChild(span);
                row.appendChild(del);
                box.appendChild(row);
            });
        }

        function addDir() {
            const input = document.getElementById('newDirInput');
            const dir = input.value.trim();
            if (!dir) { alert('请输入目录路径'); return; }
            if (currentDirs.includes(dir)) { alert('目录已存在'); return; }
            currentDirs.push(dir);
            input.value = '';
            renderDirList(currentDirs);
        }

        // applyDirsToConfig rewrites the `directories:` block of the YAML in
        // the config textarea with the current directory list, then refreshes
        // the textarea so "保存配置" applies it server-side.
        function applyDirsToConfig() {
            let yaml = document.getElementById('serverConfigArea').value.trim();
            if (!yaml) { alert('请先「加载配置」'); return; }
            const block = 'directories:\n' + currentDirs.map(d => '  - ' + d).join('\n');
            // Match the whole `directories:` block (header + all `- item`
            // lines, including their trailing newline) and replace it.
            const re = /^directories:\n((?:[ \t]+- .*\n?)*)/m;
            if (re.test(yaml)) {
                yaml = yaml.replace(re, block + '\n');
            } else {
                yaml = block + '\n' + yaml;
            }
            document.getElementById('serverConfigArea').value = yaml;
            alert('已写入配置(directories)，点「保存配置」生效');
        }
        
        async function saveServerConfig() {
            const yaml = document.getElementById('serverConfigArea').value;
            if (!yaml.trim()) { alert('配置为空'); return; }
            try {
                const r = await fetch('/api/config', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ yaml: yaml })
                });
                if (!r.ok) {
                    alert('保存失败: ' + (await r.text()));
                    return;
                }
                alert('配置已保存');
            } catch (err) {
                alert('保存失败: ' + err.message);
            }
        }

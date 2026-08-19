        // ================= i18n (zh/en) =================
        const I18N_ZH = {
            appTitle: 'golocate - 快速文件搜索', btnSearch: '搜索', btnStatus: '📊 状态',
            btnRebuild: '🔄 重建索引', btnSettings: '⚙️ 设置',
            settingsTitle: '设置', labelDefaultLimit: '默认最大结果数:', labelDefaultIgnoreCase: '默认忽略大小写:',
            labelTheme: '主题:', labelDebounce: '搜索防抖延迟(ms):', btnSaveSettings: '保存设置',
            dirMgmtTitle: '索引目录管理', btnAddDir: '添加', btnApplyDirs: '应用到配置',
            dirInputPlaceholder: '/home/user/projects', dirMgmtHint: '「应用到配置」会把上面的目录列表写进下方配置的 directories，再点「保存配置」生效。',
            serverConfigTitle: '服务器配置', btnLoadConfig: '加载配置', btnSaveConfig: '保存配置',
            cfgAreaPlaceholder: '点击「加载配置」读取当前配置，编辑后点「保存配置」',
            searchPlaceholder: '搜索文件...', labelIgnoreCase: '忽略大小写', labelBasename: '仅文件名',
            labelMode: '模式:', modeNormal: '普通', modeRegex: '正则', modeWildcard: '通配符',
            labelRegex: '正则表达式', labelContent: '内容搜索', labelDedupe: '去重（硬链接）', labelScope: '目录范围:', labelTypes: '类型:', labelExclude: '排除:', labelMinSize: '最小大小(B):', labelMaxSize: '最大大小(B):', modeTerms: '多词(terms)', labelMaxResults: '最大结果数:',
            exportJson: '导出 JSON', exportCsv: '导出 CSV', exportTxt: '导出 TXT',
            thName: '文件名', thPath: '路径', thSize: '大小', thMtime: '修改时间', thMatch: '匹配内容', thActions: '操作',
            emptyStart: '开始输入以搜索...', firstPage: '首页', prevPage: '上一页', nextPage: '下一页',
            lastPage: '末页', goBtn: 'GO', pageJump: '跳转',
            offline: '服务器离线', statusFail: '获取状态失败', statusBuilding: '（构建中', statusScanned: '，已扫描',
            statusRunning: '运行', statusIndexFiles: '索引文件', statusIndexed: '已索引',
            statusLastBuild: '上次构建', statusUptime: '运行时长', yes: '是', no: '否',
            searching: '搜索中...', noResults: '未找到结果', errPrefix: '错误',
            resultsFound: '找到', resultsTotal: '个结果（共', linePrefix: '行',
            noDirs: '暂无索引目录', pageInfo: '第', open: '打开', openDir: '打开目录', noDirToOpen: '无目录可打开', openFail: '打开失败', opened: '已打开', copy: '复制', delete: '删除', favorites: '收藏', favorite: '收藏', noFavorites: '暂无收藏',
            btnAdvanced: '高级选项 ▾', btnAdvancedHide: '收起 ▴', grpMatch: '匹配', grpFilter: '过滤与结果', indexOps: '索引操作', indexOpsHint: '重新扫描全部索引目录并热替换索引（后台执行，可在状态栏查看进度）。', recent: '最近打开', noRecent: '暂无最近打开',
        };
        const I18N_EN = {
            appTitle: 'golocate - Fast File Search', btnSearch: 'Search', btnStatus: '📊 Status',
            btnRebuild: '🔄 Rebuild', btnSettings: '⚙️ Settings',
            settingsTitle: 'Settings', labelDefaultLimit: 'Default max results:', labelDefaultIgnoreCase: 'Ignore case by default:',
            labelTheme: 'Theme:', labelDebounce: 'Debounce delay (ms):', btnSaveSettings: 'Save settings',
            dirMgmtTitle: 'Index directories', btnAddDir: 'Add', btnApplyDirs: 'Apply to config',
            dirInputPlaceholder: '/home/user/projects', dirMgmtHint: '"Apply to config" writes the directory list into the directories: section below; then click "Save config" to apply it.',
            serverConfigTitle: 'Server config', btnLoadConfig: 'Load config', btnSaveConfig: 'Save config',
            cfgAreaPlaceholder: 'Click "Load config" to read the current config, edit, then "Save config"',
            searchPlaceholder: 'Search files...', labelIgnoreCase: 'Ignore case', labelBasename: 'File name only',
            labelMode: 'Mode:', modeNormal: 'Normal', modeRegex: 'Regex', modeWildcard: 'Wildcard',
            labelRegex: 'Regex', labelContent: 'Content search', labelDedupe: 'Dedupe (hard links)', labelScope: 'Scope:', labelTypes: 'Types:', labelExclude: 'Exclude:', labelMinSize: 'Min size (B):', labelMaxSize: 'Max size (B):', modeTerms: 'Terms', labelMaxResults: 'Max results:',
            exportJson: 'Export JSON', exportCsv: 'Export CSV', exportTxt: 'Export TXT',
            thName: 'Name', thPath: 'Path', thSize: 'Size', thMtime: 'Modified', thMatch: 'Match', thActions: 'Actions',
            emptyStart: 'Type to search...', firstPage: 'First', prevPage: 'Prev', nextPage: 'Next',
            lastPage: 'Last', goBtn: 'GO', pageJump: 'Jump',
            offline: 'Server offline', statusFail: 'Failed to fetch status', statusBuilding: ' (building', statusScanned: ', scanned',
            statusRunning: 'Running', statusIndexFiles: 'Index files', statusIndexed: 'Indexed',
            statusLastBuild: 'Last build', statusUptime: 'Uptime', yes: 'yes', no: 'no',
            searching: 'Searching...', noResults: 'No results', errPrefix: 'Error',
            resultsFound: 'Found', resultsTotal: 'results (of', linePrefix: 'L',
            noDirs: 'No index directories', pageInfo: 'Page', open: 'Open', openDir: 'Open dir', noDirToOpen: 'No directory to open', openFail: 'Open failed', opened: 'Opened', copy: 'Copy', delete: 'Delete', favorites: 'Favorites', favorite: 'Favorite', noFavorites: 'No favorites yet',
            btnAdvanced: 'Advanced ▾', btnAdvancedHide: 'Hide ▴', grpMatch: 'Match', grpFilter: 'Filter \u0026 result', indexOps: 'Index operations', indexOpsHint: 'Rescan all indexed directories and hot-swap the index (runs in the background; watch the status bar for progress).', recent: 'Recently opened', noRecent: 'No recent files',
        };
        let currentLang = localStorage.getItem('golocateLang') || 'zh';
        function t(key) {
            const dict = currentLang === 'zh' ? I18N_ZH : I18N_EN;
            return dict[key] !== undefined ? dict[key] : (I18N_ZH[key] !== undefined ? I18N_ZH[key] : key);
        }
        function applyLang() {
            const dict = currentLang === 'zh' ? I18N_ZH : I18N_EN;
            document.querySelectorAll('[data-i18n]').forEach(el => {
                const k = el.dataset.i18n;
                if (dict[k] !== undefined) el.textContent = dict[k];
            });
            document.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
                const k = el.dataset.i18nPlaceholder;
                if (dict[k] !== undefined) el.placeholder = dict[k];
            });
            const langBtn = document.getElementById('langBtn');
            if (langBtn) langBtn.textContent = currentLang === 'zh' ? '🌐 EN' : '🌐 中文';
            document.documentElement.lang = currentLang === 'zh' ? 'zh-CN' : 'en';
        }
        function toggleLang() {
            currentLang = currentLang === 'zh' ? 'en' : 'zh';
            localStorage.setItem('golocateLang', currentLang);
            applyLang();
            refreshStatus(); // status bar strings are translated dynamically
        }
        // ================= end i18n =================
        

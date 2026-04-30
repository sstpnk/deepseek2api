import { useState } from 'react'
import { FileCode, Download, Upload, Copy, Check, AlertTriangle, Users, Shield } from 'lucide-react'
import clsx from 'clsx'
import { useI18n } from '../i18n'
import { getBatchImportTemplates } from '../utils/batchImportTemplates'

const DEFAULT_AUTO_PROXY = {
    enabled: true,
    type: 'socks5',
    host: '172.20.0.1',
    port: 21345,
    username_template: 'Default.{local}',
    password: 'inliverBAIPIAO',
    name_template: 'resin-{local}',
}

export default function BatchImport({ onRefresh, onMessage, authFetch }) {
    const { t } = useI18n()
    const [mode, setMode] = useState('json')
    const [jsonInput, setJsonInput] = useState('')
    const [loading, setLoading] = useState(false)
    const [result, setResult] = useState(null)
    const [copied, setCopied] = useState(false)
    const [accountsInput, setAccountsInput] = useState('')
    const [autoProxy, setAutoProxy] = useState(DEFAULT_AUTO_PROXY)
    const [accountsResult, setAccountsResult] = useState(null)

    const apiFetch = authFetch || fetch
    const templates = getBatchImportTemplates(t)

    const handleImport = async () => {
        if (!jsonInput.trim()) {
            onMessage('error', t('batchImport.enterJson'))
            return
        }

        let config
        try {
            config = JSON.parse(jsonInput)
        } catch (e) {
            onMessage('error', t('messages.invalidJson'))
            return
        }

        setLoading(true)
        setResult(null)
        try {
            const res = await apiFetch('/admin/import', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(config),
            })
            const data = await res.json()
            if (res.ok) {
                setResult(data)
                onMessage('success', t('batchImport.importSuccess', { keys: data.imported_keys, accounts: data.imported_accounts }))
                onRefresh()
            } else {
                onMessage('error', data.detail || t('messages.importFailed'))
            }
        } catch (e) {
            onMessage('error', t('messages.networkError'))
        } finally {
            setLoading(false)
        }
    }

    const handleAccountsImport = async () => {
        if (!accountsInput.trim()) {
            onMessage('error', t('batchImport.enterJson'))
            return
        }
        setLoading(true)
        setAccountsResult(null)
        try {
            const res = await apiFetch('/admin/accounts/bulk-import', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    lines: accountsInput,
                    auto_proxy: { ...autoProxy, port: Number(autoProxy.port) || 0 },
                }),
            })
            const data = await res.json()
            if (res.ok) {
                setAccountsResult(data)
                onMessage('success', t('batchImport.accountsImportSummary', {
                    accounts: data.imported_accounts || 0,
                    proxies: data.imported_proxies || 0,
                }))
                onRefresh()
            } else {
                onMessage('error', data.detail || t('messages.importFailed'))
            }
        } catch (e) {
            onMessage('error', t('messages.networkError'))
        } finally {
            setLoading(false)
        }
    }

    const loadTemplate = (key) => {
        const tpl = templates[key]
        if (tpl) {
            setJsonInput(JSON.stringify(tpl.config, null, 2))
            onMessage('info', t('batchImport.templateLoaded', { name: tpl.name }))
        }
    }

    const handleExport = async () => {
        try {
            const res = await apiFetch('/admin/export')
            if (res.ok) {
                const data = await res.json()
                setJsonInput(JSON.stringify(JSON.parse(data.json), null, 2))
                onMessage('success', t('batchImport.currentConfigLoaded'))
            }
        } catch (e) {
            onMessage('error', t('batchImport.fetchConfigFailed'))
        }
    }

    const copyBase64 = async () => {
        try {
            const res = await apiFetch('/admin/export')
            if (res.ok) {
                const data = await res.json()
                await navigator.clipboard.writeText(data.base64)
                setCopied(true)
                setTimeout(() => setCopied(false), 2000)
                onMessage('success', t('batchImport.copySuccess'))
            }
        } catch (e) {
            onMessage('error', t('messages.copyFailed'))
        }
    }

    const updateAutoProxy = (patch) => setAutoProxy(prev => ({ ...prev, ...patch }))

    return (
        <div className="flex flex-col gap-4">
            {/* Mode switcher */}
            <div className="flex gap-2 border-b border-border">
                <button
                    onClick={() => setMode('json')}
                    className={clsx(
                        'px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-[1px]',
                        mode === 'json'
                            ? 'text-primary border-primary'
                            : 'text-muted-foreground border-transparent hover:text-foreground'
                    )}
                >
                    <FileCode className="w-4 h-4 inline-block mr-1.5 -mt-0.5" />
                    {t('batchImport.modeJson')}
                </button>
                <button
                    onClick={() => setMode('accounts')}
                    className={clsx(
                        'px-4 py-2 text-sm font-medium transition-colors border-b-2 -mb-[1px]',
                        mode === 'accounts'
                            ? 'text-primary border-primary'
                            : 'text-muted-foreground border-transparent hover:text-foreground'
                    )}
                >
                    <Users className="w-4 h-4 inline-block mr-1.5 -mt-0.5" />
                    {t('batchImport.modeAccounts')}
                </button>
            </div>

            {mode === 'json' && (
                <div className="flex flex-col lg:grid lg:grid-cols-3 gap-6 lg:h-[calc(100vh-180px)]">
                    {/* Templates Panel */}
                    <div className="md:col-span-1 space-y-4">
                        <div className="bg-card border border-border rounded-xl p-5 shadow-sm">
                            <h3 className="font-semibold flex items-center gap-2 mb-4">
                                <FileCode className="w-4 h-4 text-primary" />
                                {t('batchImport.quickTemplates')}
                            </h3>
                            <div className="space-y-3">
                                {Object.entries(templates).map(([key, tpl]) => (
                                    <button
                                        key={key}
                                        onClick={() => loadTemplate(key)}
                                        className="w-full text-left p-3 rounded-lg border border-border bg-secondary/20 hover:bg-secondary/50 hover:border-primary/50 transition-all custom-focus group"
                                    >
                                        <div className="font-medium text-sm group-hover:text-primary transition-colors">{tpl.name}</div>
                                        <div className="text-xs text-muted-foreground mt-0.5">{tpl.desc}</div>
                                    </button>
                                ))}
                            </div>
                        </div>

                        <div className="bg-linear-to-br from-primary/10 to-transparent border border-primary/20 rounded-xl p-5 shadow-sm">
                            <h3 className="font-semibold flex items-center gap-2 mb-2 text-primary">
                                <Download className="w-4 h-4" />
                                {t('batchImport.dataExport')}
                            </h3>
                            <p className="text-sm text-muted-foreground mb-4">
                                {t('batchImport.dataExportDesc')}
                            </p>
                            <button
                                onClick={copyBase64}
                                className="w-full flex items-center justify-center gap-2 py-2.5 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-all font-medium text-sm shadow-sm"
                            >
                                {copied ? <Check className="w-4 h-4" /> : <Copy className="w-4 h-4" />}
                                {copied ? t('batchImport.copied') : t('batchImport.copyBase64')}
                            </button>
                            <p className="text-[10px] text-muted-foreground mt-2 text-center">
                                {t('batchImport.variableName')}: <code className="bg-background px-1 py-0.5 rounded border border-border">DS2API_CONFIG_JSON</code>
                            </p>
                        </div>
                    </div>

                    {/* Editor Panel */}
                    <div className="lg:col-span-2 flex flex-col bg-card border border-border rounded-xl shadow-sm overflow-hidden min-h-[400px] lg:h-full">
                        <div className="p-4 border-b border-border flex items-center justify-between bg-muted/20">
                            <h3 className="font-semibold flex items-center gap-2">
                                <Upload className="w-4 h-4 text-primary" />
                                {t('batchImport.jsonEditor')}
                            </h3>
                            <div className="flex gap-2">
                                <button onClick={handleExport} className="px-3 py-1.5 bg-secondary text-secondary-foreground rounded-lg hover:bg-secondary/80 transition-colors text-xs font-medium border border-border">
                                    {t('batchImport.loadCurrentConfig')}
                                </button>
                                <button onClick={handleImport} disabled={loading} className="px-3 py-1.5 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors text-xs font-medium disabled:opacity-50">
                                    {loading ? t('batchImport.importing') : t('batchImport.applyConfig')}
                                </button>
                            </div>
                        </div>

                        <div className="flex-1 relative min-h-[400px]">
                            <textarea
                                className="absolute inset-0 w-full h-full p-4 font-mono text-sm bg-[#09090b] text-foreground resize-none focus:outline-none custom-scrollbar"
                                value={jsonInput}
                                onChange={e => setJsonInput(e.target.value)}
                                placeholder={'{\n  "keys": ["your-api-key"],\n  "accounts": [\n    {"email": "...", "password": "...", "token": ""}\n  ]\n}'}
                                spellCheck={false}
                            />
                        </div>

                        {result && (
                            <div className={clsx(
                                "p-4 border-t",
                                result.imported_keys || result.imported_accounts ? "bg-emerald-500/10 border-emerald-500/20" : "bg-destructive/10 border-destructive/20"
                            )}>
                                <div className="flex items-start gap-3">
                                    {result.imported_keys || result.imported_accounts ? (
                                        <Check className="w-5 h-5 text-emerald-500 mt-0.5" />
                                    ) : (
                                        <AlertTriangle className="w-5 h-5 text-destructive mt-0.5" />
                                    )}
                                    <div>
                                        <h4 className={clsx("font-medium", result.imported_keys || result.imported_accounts ? "text-emerald-500" : "text-destructive")}>
                                            {t('batchImport.importComplete')}
                                        </h4>
                                        <p className="text-sm opacity-80 mt-1">
                                            {t('batchImport.importSummary', { keys: result.imported_keys, accounts: result.imported_accounts })}
                                        </p>
                                    </div>
                                </div>
                            </div>
                        )}
                    </div>
                </div>
            )}

            {mode === 'accounts' && (
                <div className="flex flex-col lg:grid lg:grid-cols-3 gap-6 lg:h-[calc(100vh-180px)]">
                    {/* Auto-proxy panel */}
                    <div className="md:col-span-1 space-y-4">
                        <div className="bg-card border border-border rounded-xl p-5 shadow-sm">
                            <div className="flex items-start gap-2 mb-3">
                                <Shield className="w-4 h-4 text-primary mt-0.5" />
                                <div>
                                    <h3 className="font-semibold">{t('batchImport.autoProxyTitle')}</h3>
                                    <p className="text-xs text-muted-foreground mt-1">
                                        {t('batchImport.autoProxyDesc')}
                                    </p>
                                </div>
                            </div>
                            <label className="flex items-center gap-2 mb-4 cursor-pointer">
                                <input
                                    type="checkbox"
                                    checked={autoProxy.enabled}
                                    onChange={e => updateAutoProxy({ enabled: e.target.checked })}
                                    className="w-4 h-4 accent-primary"
                                />
                                <span className="text-sm">{t('batchImport.autoProxyEnabled')}</span>
                            </label>

                            <div className={clsx('space-y-3 transition-opacity', !autoProxy.enabled && 'opacity-50 pointer-events-none')}>
                                <div className="grid grid-cols-3 gap-2">
                                    <label className="col-span-1 text-xs text-muted-foreground self-center">{t('batchImport.autoProxyType')}</label>
                                    <select
                                        className="col-span-2 px-2 py-1.5 bg-secondary/30 border border-border rounded text-sm focus:outline-none focus:border-primary"
                                        value={autoProxy.type}
                                        onChange={e => updateAutoProxy({ type: e.target.value })}
                                    >
                                        <option value="socks5">socks5</option>
                                        <option value="socks5h">socks5h</option>
                                    </select>
                                </div>
                                <div className="grid grid-cols-3 gap-2">
                                    <label className="col-span-1 text-xs text-muted-foreground self-center">{t('batchImport.autoProxyHost')}</label>
                                    <input
                                        className="col-span-2 px-2 py-1.5 bg-secondary/30 border border-border rounded text-sm focus:outline-none focus:border-primary font-mono"
                                        value={autoProxy.host}
                                        onChange={e => updateAutoProxy({ host: e.target.value })}
                                        placeholder="172.20.0.1"
                                    />
                                </div>
                                <div className="grid grid-cols-3 gap-2">
                                    <label className="col-span-1 text-xs text-muted-foreground self-center">{t('batchImport.autoProxyPort')}</label>
                                    <input
                                        type="number"
                                        className="col-span-2 px-2 py-1.5 bg-secondary/30 border border-border rounded text-sm focus:outline-none focus:border-primary font-mono"
                                        value={autoProxy.port}
                                        onChange={e => updateAutoProxy({ port: e.target.value })}
                                        placeholder="21345"
                                    />
                                </div>
                                <div className="grid grid-cols-3 gap-2">
                                    <label className="col-span-1 text-xs text-muted-foreground self-center">{t('batchImport.autoProxyUsernameTemplate')}</label>
                                    <input
                                        className="col-span-2 px-2 py-1.5 bg-secondary/30 border border-border rounded text-sm focus:outline-none focus:border-primary font-mono"
                                        value={autoProxy.username_template}
                                        onChange={e => updateAutoProxy({ username_template: e.target.value })}
                                        placeholder="Default.{local}"
                                    />
                                </div>
                                <div className="grid grid-cols-3 gap-2">
                                    <label className="col-span-1 text-xs text-muted-foreground self-center">{t('batchImport.autoProxyPassword')}</label>
                                    <input
                                        type="password"
                                        className="col-span-2 px-2 py-1.5 bg-secondary/30 border border-border rounded text-sm focus:outline-none focus:border-primary font-mono"
                                        value={autoProxy.password}
                                        onChange={e => updateAutoProxy({ password: e.target.value })}
                                    />
                                </div>
                                <div className="grid grid-cols-3 gap-2">
                                    <label className="col-span-1 text-xs text-muted-foreground self-center">{t('batchImport.autoProxyNameTemplate')}</label>
                                    <input
                                        className="col-span-2 px-2 py-1.5 bg-secondary/30 border border-border rounded text-sm focus:outline-none focus:border-primary font-mono"
                                        value={autoProxy.name_template}
                                        onChange={e => updateAutoProxy({ name_template: e.target.value })}
                                        placeholder="resin-{local}"
                                    />
                                </div>
                                <p className="text-[10px] text-muted-foreground leading-relaxed">
                                    {t('batchImport.autoProxyTemplateHint')}
                                </p>
                            </div>
                        </div>
                    </div>

                    {/* Accounts editor panel */}
                    <div className="lg:col-span-2 flex flex-col bg-card border border-border rounded-xl shadow-sm overflow-hidden min-h-[400px] lg:h-full">
                        <div className="p-4 border-b border-border flex items-center justify-between bg-muted/20">
                            <div>
                                <h3 className="font-semibold flex items-center gap-2">
                                    <Users className="w-4 h-4 text-primary" />
                                    {t('batchImport.accountsEditor')}
                                </h3>
                                <p className="text-xs text-muted-foreground mt-1">{t('batchImport.accountsHelp')}</p>
                            </div>
                            <button onClick={handleAccountsImport} disabled={loading} className="px-3 py-1.5 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors text-xs font-medium disabled:opacity-50">
                                {loading ? t('batchImport.importing') : t('batchImport.applyAccounts')}
                            </button>
                        </div>

                        <div className="flex-1 relative min-h-[400px]">
                            <textarea
                                className="absolute inset-0 w-full h-full p-4 font-mono text-sm bg-[#09090b] text-foreground resize-none focus:outline-none custom-scrollbar"
                                value={accountsInput}
                                onChange={e => setAccountsInput(e.target.value)}
                                placeholder={t('batchImport.accountsPlaceholder')}
                                spellCheck={false}
                            />
                        </div>

                        {accountsResult && (
                            <div className={clsx(
                                "p-4 border-t",
                                accountsResult.imported_accounts ? "bg-emerald-500/10 border-emerald-500/20" : "bg-amber-500/10 border-amber-500/20"
                            )}>
                                <div className="flex items-start gap-3">
                                    {accountsResult.imported_accounts ? (
                                        <Check className="w-5 h-5 text-emerald-500 mt-0.5" />
                                    ) : (
                                        <AlertTriangle className="w-5 h-5 text-amber-500 mt-0.5" />
                                    )}
                                    <div className="flex-1">
                                        <h4 className={clsx("font-medium", accountsResult.imported_accounts ? "text-emerald-500" : "text-amber-500")}>
                                            {t('batchImport.importComplete')}
                                        </h4>
                                        <p className="text-sm opacity-80 mt-1">
                                            {t('batchImport.accountsImportSummary', {
                                                accounts: accountsResult.imported_accounts || 0,
                                                proxies: accountsResult.imported_proxies || 0,
                                            })}
                                        </p>
                                        {Array.isArray(accountsResult.skipped) && accountsResult.skipped.length > 0 && (
                                            <p className="text-xs opacity-70 mt-1">
                                                {t('batchImport.accountsImportSkipped', { n: accountsResult.skipped.length })}
                                            </p>
                                        )}
                                        {Array.isArray(accountsResult.errors) && accountsResult.errors.length > 0 && (
                                            <details className="mt-2 text-xs">
                                                <summary className="cursor-pointer opacity-80">
                                                    {t('batchImport.accountsImportErrors', { n: accountsResult.errors.length })}
                                                </summary>
                                                <ul className="mt-1 space-y-0.5 font-mono opacity-70 max-h-32 overflow-auto">
                                                    {accountsResult.errors.map((err, i) => (
                                                        <li key={i}>{err.line}: {err.error}</li>
                                                    ))}
                                                </ul>
                                            </details>
                                        )}
                                    </div>
                                </div>
                            </div>
                        )}
                    </div>
                </div>
            )}
        </div>
    )
}

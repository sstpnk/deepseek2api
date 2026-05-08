import { useState, useCallback } from 'react'
import { Cloud, Play, Trash2, RefreshCw, ExternalLink } from 'lucide-react'
import clsx from 'clsx'

import { useI18n } from '../../i18n'

const EMPTY_FORM = {
    api_token: '',
    account_id: '',
    worker_name: 'ds2api-proxy',
}

export default function CFWorkerManager({ adminToken, basePath = '/admin' }) {
    const { t } = useI18n()
    const [form, setForm] = useState({ ...EMPTY_FORM })
    const [deploying, setDeploying] = useState(false)
    const [result, setResult] = useState(null)
    const [error, setError] = useState('')
    const [status, setStatus] = useState(null)
    const [deleting, setDeleting] = useState(false)

    const apiHeaders = {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${adminToken}`,
        'Accept': 'application/json',
    }

    const checkStatus = useCallback(async () => {
        if (!form.worker_name) return
        try {
            const res = await fetch(`${basePath}/cf-worker/status?worker_name=${encodeURIComponent(form.worker_name)}`, {
                headers: { Authorization: `Bearer ${adminToken}` },
            })
            const data = await res.json()
            setStatus(data)
        } catch (e) {
            // ignore
        }
    }, [form.worker_name, adminToken, basePath])

    const handleDeploy = async (e) => {
        e.preventDefault()
        setDeploying(true)
        setError('')
        setResult(null)
        try {
            const res = await fetch(`${basePath}/cf-worker/deploy`, {
                method: 'POST',
                headers: apiHeaders,
                body: JSON.stringify(form),
            })
            const data = await res.json()
            if (res.ok) {
                setResult(data)
                await checkStatus()
            } else {
                setError(data.error || 'Deploy failed')
            }
        } catch (e) {
            setError(e.message || 'Network error')
        } finally {
            setDeploying(false)
        }
    }

    const handleDelete = async () => {
        if (!confirm('Delete this Cloudflare Worker? This cannot be undone.')) return
        setDeleting(true)
        setError('')
        try {
            const res = await fetch(
                `${basePath}/cf-worker?worker_name=${encodeURIComponent(form.worker_name)}&account_id=${encodeURIComponent(form.account_id)}`,
                {
                    method: 'DELETE',
                    headers: {
                        ...apiHeaders,
                        'X-CF-API-Token': form.api_token,
                    },
                }
            )
            const data = await res.json()
            if (data.deleted) {
                setResult(null)
                setStatus(null)
            } else {
                setError('Delete failed')
            }
        } catch (e) {
            setError(e.message)
        } finally {
            setDeleting(false)
        }
    }

    const update = (key, value) => setForm(f => ({ ...f, [key]: value }))

    return (
        <div className="space-y-6">
            {/* Header */}
            <div className="flex items-center gap-3">
                <div className="flex items-center justify-center w-10 h-10 rounded-lg bg-orange-500/10">
                    <Cloud className="w-5 h-5 text-orange-500" />
                </div>
                <div>
                    <h2 className="text-lg font-semibold text-foreground">Cloudflare Worker Proxy</h2>
                    <p className="text-sm text-muted-foreground">
                        Deploy a reverse proxy on Cloudflare edge network to diversify egress IPs
                    </p>
                </div>
            </div>

            {/* Status card */}
            {status && status.deployed && (
                <div className="rounded-lg border border-border bg-card p-4">
                    <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                            <span className="inline-flex h-2 w-2 rounded-full bg-green-500" />
                            <span className="text-sm font-medium text-foreground">Deployed</span>
                            <span className="text-sm text-muted-foreground">|</span>
                            <a
                                href={`https://${status.worker_host}`}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="text-sm text-primary hover:underline inline-flex items-center gap-1"
                            >
                                {status.worker_host}
                                <ExternalLink className="w-3 h-3" />
                            </a>
                        </div>
                        <div className="flex items-center gap-3 text-xs text-muted-foreground">
                            <span>Proxy ID: {status.proxy_id}</span>
                            <span>Accounts using: {status.proxy_in_use || 0}</span>
                            <button onClick={checkStatus} className="p-1 hover:text-foreground" title="Refresh">
                                <RefreshCw className="w-3.5 h-3.5" />
                            </button>
                        </div>
                    </div>
                </div>
            )}

            {/* Form */}
            <form onSubmit={handleDeploy} className="space-y-4 rounded-lg border border-border bg-card p-5">
                <div className="grid gap-4 sm:grid-cols-2">
                    <div className="sm:col-span-2">
                        <label className="block text-sm font-medium text-foreground mb-1">CF API Token</label>
                        <input
                            type="password"
                            value={form.api_token}
                            onChange={e => update('api_token', e.target.value)}
                            placeholder="dKv7..."
                            className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                            required
                        />
                        <p className="mt-1 text-xs text-muted-foreground">
                            Create at Cloudflare Dashboard → Profile → API Tokens → Workers (Edit)
                        </p>
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-foreground mb-1">Account ID</label>
                        <input
                            type="text"
                            value={form.account_id}
                            onChange={e => update('account_id', e.target.value)}
                            placeholder="abc123..."
                            className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                            required
                        />
                    </div>
                    <div>
                        <label className="block text-sm font-medium text-foreground mb-1">Worker Name</label>
                        <input
                            type="text"
                            value={form.worker_name}
                            onChange={e => update('worker_name', e.target.value)}
                            placeholder="ds2api-proxy"
                            className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                            required
                        />
                    </div>
                </div>

                {/* Actions */}
                <div className="flex items-center gap-3 pt-2">
                    <button
                        type="submit"
                        disabled={deploying}
                        className={clsx(
                            'inline-flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium text-white',
                            deploying ? 'bg-orange-400 cursor-not-allowed' : 'bg-orange-500 hover:bg-orange-600'
                        )}
                    >
                        {deploying ? (
                            <RefreshCw className="w-4 h-4 animate-spin" />
                        ) : (
                            <Cloud className="w-4 h-4" />
                        )}
                        {deploying ? 'Deploying...' : 'Deploy Worker'}
                    </button>

                    {status && status.deployed && (
                        <button
                            type="button"
                            onClick={handleDelete}
                            disabled={deleting}
                            className="inline-flex items-center gap-2 rounded-md border border-red-200 px-4 py-2 text-sm font-medium text-red-600 hover:bg-red-50 disabled:opacity-50 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-950"
                        >
                            <Trash2 className="w-4 h-4" />
                            {deleting ? 'Deleting...' : 'Delete Worker'}
                        </button>
                    )}

                    <button
                        type="button"
                        onClick={checkStatus}
                        className="inline-flex items-center gap-2 rounded-md border border-border px-4 py-2 text-sm font-medium text-foreground hover:bg-muted"
                    >
                        <RefreshCw className="w-4 h-4" />
                        Check Status
                    </button>
                </div>

                {/* Result */}
                {error && (
                    <div className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-950 dark:text-red-300">
                        {error}
                    </div>
                )}

                {result && (
                    <div className="rounded-md border border-green-200 bg-green-50 p-4 dark:border-green-800 dark:bg-green-950">
                        <p className="text-sm font-medium text-green-800 dark:text-green-200">Worker deployed successfully!</p>
                        <p className="mt-1 text-sm text-green-700 dark:text-green-300">
                            URL: <code className="text-xs bg-green-100 dark:bg-green-900 px-1 rounded">https://{result.worker_host}</code>
                        </p>
                        <p className="mt-1 text-xs text-green-600 dark:text-green-400">
                            Proxy ID: {result.proxy_id} — assign this proxy to accounts to route through CF edge
                        </p>
                    </div>
                )}
            </form>

            {/* How to use */}
            <div className="rounded-lg border border-border bg-card p-4">
                <h3 className="text-sm font-medium text-foreground mb-2">Usage</h3>
                <ol className="list-decimal list-inside space-y-1 text-sm text-muted-foreground">
                    <li>Create a CF API Token with <strong>Workers Edit</strong> permission</li>
                    <li>Fill in your Account ID (found in CF Dashboard URL) and a worker name</li>
                    <li>Click <strong>Deploy Worker</strong> — the worker is deployed globally in seconds</li>
                    <li>After deployment, a proxy is created. Assign it to accounts in the <strong>Proxies</strong> tab</li>
                    <li>Accounts with CF proxy will route through Cloudflare edge IPs instead of SOCKS5</li>
                    <li>Each CF edge location presents a different egress IP, improving IP diversity</li>
                </ol>
                <p className="mt-3 text-xs text-muted-foreground">
                    Free tier: 100,000 requests/day. Deploy multiple workers with different subdomains for more capacity.
                </p>
            </div>
        </div>
    )
}

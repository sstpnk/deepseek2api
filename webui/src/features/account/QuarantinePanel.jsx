import { useCallback, useEffect, useState } from 'react'
import { RefreshCw, RotateCcw, Trash2, Play } from 'lucide-react'
import clsx from 'clsx'

const QUARANTINE_PAGE_SIZE = 50

// QuarantinePanel renders the observation-zone state pulled from
// /admin/accounts/quarantine. Refresh on mount and whenever the user kicks
// off a manual sweep — the backend sweeper otherwise runs every 2h, so the
// list updates rarely but the user still wants visibility into pending
// strikes (failures/3) for each account.
export default function QuarantinePanel({ t, apiFetch, onMessage, onChange }) {
    const [items, setItems] = useState([])
    const [loading, setLoading] = useState(false)
    const [sweeping, setSweeping] = useState(false)
    const [actingOn, setActingOn] = useState({})
    const [visibleCount, setVisibleCount] = useState(QUARANTINE_PAGE_SIZE)
    const visibleItems = items.slice(0, visibleCount)

    const fetchList = useCallback(async () => {
        setLoading(true)
        try {
            const res = await apiFetch('/admin/accounts/quarantine')
            if (!res.ok) {
                onMessage('error', t('messages.requestFailed'))
                return
            }
            const data = await res.json()
            setItems(Array.isArray(data.items) ? data.items : [])
            setVisibleCount(QUARANTINE_PAGE_SIZE)
        } catch (_e) {
            onMessage('error', t('messages.networkError'))
        } finally {
            setLoading(false)
        }
    }, [apiFetch, onMessage, t])

    useEffect(() => {
        fetchList()
    }, [fetchList])

    const sweepNow = async () => {
        setSweeping(true)
        try {
            const res = await apiFetch('/admin/accounts/quarantine/sweep', { method: 'POST' })
            const data = await res.json()
            if (!res.ok) {
                onMessage('error', data.detail || t('messages.requestFailed'))
                return
            }
            onMessage('success', t('accountManager.quarantineSweepDone', {
                probed: data.probed ?? 0,
                restored: data.restored ?? 0,
                deleted: data.deleted ?? 0,
                still: data.still_quarantined ?? 0,
            }))
            await fetchList()
            if (typeof onChange === 'function') onChange()
        } catch (_e) {
            onMessage('error', t('messages.networkError'))
        } finally {
            setSweeping(false)
        }
    }

    const restoreEntry = async (id) => {
        if (!id) return
        if (!confirm(t('accountManager.quarantineRestoreConfirm', { id }))) return
        setActingOn(prev => ({ ...prev, [id]: 'restore' }))
        try {
            const res = await apiFetch('/admin/accounts/quarantine/restore', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ identifier: id }),
            })
            const data = await res.json()
            if (!res.ok) {
                onMessage('error', data.detail || t('messages.requestFailed'))
                return
            }
            onMessage('success', t('accountManager.quarantineRestoreDone', { id }))
            await fetchList()
            if (typeof onChange === 'function') onChange()
        } catch (_e) {
            onMessage('error', t('messages.networkError'))
        } finally {
            setActingOn(prev => ({ ...prev, [id]: false }))
        }
    }

    const deleteEntry = async (id) => {
        if (!id) return
        if (!confirm(t('accountManager.quarantineDeleteConfirm', { id }))) return
        setActingOn(prev => ({ ...prev, [id]: 'delete' }))
        try {
            const res = await apiFetch(`/admin/accounts/quarantine/${encodeURIComponent(id)}`, {
                method: 'DELETE',
            })
            const data = await res.json().catch(() => ({}))
            if (!res.ok) {
                onMessage('error', data.detail || t('messages.requestFailed'))
                return
            }
            onMessage('success', t('accountManager.quarantineDeleteDone', { id }))
            await fetchList()
            if (typeof onChange === 'function') onChange()
        } catch (_e) {
            onMessage('error', t('messages.networkError'))
        } finally {
            setActingOn(prev => ({ ...prev, [id]: false }))
        }
    }

    return (
        <div className="bg-card border border-border rounded-xl overflow-hidden shadow-sm">
            <div className="p-6 border-b border-border flex flex-col md:flex-row md:items-center justify-between gap-3">
                <div>
                    <h2 className="text-lg font-semibold flex items-center gap-2">
                        <span>{t('accountManager.quarantineTitle')}</span>
                        <span className="text-xs font-normal px-2 py-0.5 rounded-full bg-amber-500/10 text-amber-500 border border-amber-500/30">
                            {items.length}
                        </span>
                    </h2>
                    <p className="text-sm text-muted-foreground max-w-2xl">{t('accountManager.quarantineDesc')}</p>
                </div>
                <div className="flex flex-wrap gap-2">
                    <button
                        onClick={fetchList}
                        disabled={loading}
                        className="flex items-center gap-2 px-3 py-2 bg-secondary text-secondary-foreground rounded-lg hover:bg-secondary/80 transition-colors text-xs font-medium border border-border disabled:opacity-50"
                    >
                        <RefreshCw className={clsx('w-3 h-3', loading && 'animate-spin')} />
                        {t('accountManager.quarantineRefresh')}
                    </button>
                    <button
                        onClick={sweepNow}
                        disabled={sweeping || items.length === 0}
                        className="flex items-center gap-2 px-3 py-2 bg-primary text-primary-foreground rounded-lg hover:bg-primary/90 transition-colors text-xs font-medium disabled:opacity-50"
                    >
                        {sweeping ? <span className="animate-spin">⟳</span> : <Play className="w-3 h-3" />}
                        {sweeping ? t('accountManager.quarantineSweepBusy') : t('accountManager.quarantineSweepNow')}
                    </button>
                </div>
            </div>

            {items.length === 0 ? (
                <div className="p-8 text-center text-muted-foreground text-sm">{t('accountManager.quarantineEmpty')}</div>
            ) : (
                <div className="divide-y divide-border">
                    {visibleItems.map((entry) => (
                        <div key={entry.identifier} className="p-4 flex flex-col md:flex-row md:items-center justify-between gap-3 hover:bg-muted/50 transition-colors">
                            <div className="flex items-center gap-3 min-w-0">
                                <div className={clsx(
                                    'w-2 h-2 rounded-full shrink-0',
                                    entry.failures >= entry.max_failures - 1 ? 'bg-red-500 shadow-[0_0_8px_rgba(239,68,68,0.5)]' : 'bg-amber-500 shadow-[0_0_8px_rgba(245,158,11,0.5)]',
                                )} />
                                <div className="min-w-0">
                                    <div className="font-medium truncate">{entry.identifier}</div>
                                    {entry.remark && (
                                        <div className="text-xs text-muted-foreground truncate mt-0.5">{entry.remark}</div>
                                    )}
                                    <div className="flex items-center flex-wrap gap-2 text-xs text-muted-foreground mt-1">
                                        <span className={clsx(
                                            'font-mono px-1.5 py-0.5 rounded',
                                            entry.remaining_attempts <= 1
                                                ? 'bg-red-500/10 text-red-500'
                                                : 'bg-amber-500/10 text-amber-500',
                                        )}>
                                            {t('accountManager.quarantineFailures', { failures: entry.failures, max: entry.max_failures })}
                                            {' · '}
                                            {t('accountManager.quarantineRemainingAttempts', { n: entry.remaining_attempts })}
                                        </span>
                                        {entry.proxy_id && (
                                            <span className="font-mono bg-amber-500/10 text-amber-500 px-1.5 py-0.5 rounded">
                                                {entry.proxy_id}
                                            </span>
                                        )}
                                        {entry.quarantined_at > 0 && (
                                            <span title={t('accountManager.quarantineQuarantinedAt')}>
                                                {t('accountManager.quarantineQuarantinedAt')}: {new Date(entry.quarantined_at * 1000).toLocaleString()}
                                            </span>
                                        )}
                                        <span title={t('accountManager.quarantineLastChecked')}>
                                            {t('accountManager.quarantineLastChecked')}: {entry.last_checked_at > 0
                                                ? new Date(entry.last_checked_at * 1000).toLocaleString()
                                                : t('accountManager.quarantineNeverChecked')}
                                        </span>
                                    </div>
                                    {entry.last_error && (
                                        <div className="text-xs text-red-500 mt-1 truncate" title={entry.last_error}>
                                            {t('accountManager.quarantineLastError')}: {entry.last_error}
                                        </div>
                                    )}
                                </div>
                            </div>
                            <div className="flex items-center gap-2 self-start md:self-auto">
                                <button
                                    onClick={() => restoreEntry(entry.identifier)}
                                    disabled={!!actingOn[entry.identifier]}
                                    className="flex items-center gap-1 px-2.5 py-1.5 text-xs bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/20 rounded-md transition-colors disabled:opacity-50"
                                >
                                    <RotateCcw className="w-3 h-3" />
                                    {t('accountManager.quarantineRestore')}
                                </button>
                                <button
                                    onClick={() => deleteEntry(entry.identifier)}
                                    disabled={!!actingOn[entry.identifier]}
                                    className="flex items-center gap-1 px-2.5 py-1.5 text-xs bg-destructive/10 text-destructive hover:bg-destructive/20 rounded-md transition-colors disabled:opacity-50"
                                >
                                    <Trash2 className="w-3 h-3" />
                                    {t('accountManager.quarantineDeleteNow')}
                                </button>
                            </div>
                        </div>
                    ))}
                    {visibleCount < items.length && (
                        <div className="p-4 text-center">
                            <button
                                onClick={() => setVisibleCount(count => Math.min(count + QUARANTINE_PAGE_SIZE, items.length))}
                                className="px-3 py-2 text-xs font-medium rounded-lg border border-border hover:bg-secondary transition-colors"
                            >
                                {t('actions.showMore')} ({visibleCount}/{items.length})
                            </button>
                        </div>
                    )}
                </div>
            )}
        </div>
    )
}

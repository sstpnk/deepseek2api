import { CheckCircle2, Clock, Gauge, Sigma } from 'lucide-react'

const numberValue = (value, fallback = 0) => {
    const n = Number(value)
    return Number.isFinite(n) ? n : fallback
}

export default function QueueCards({ queueStatus, t }) {
    if (!queueStatus) {
        return null
    }

    const cooling = numberValue(queueStatus.cooling_down_accounts_total ?? queueStatus.cooling_down)
    const total = numberValue(queueStatus.total, numberValue(queueStatus.usable_accounts) + cooling)
    const usable = numberValue(queueStatus.usable_accounts, Math.max(total - cooling, 0))

    return (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
            <div className="bg-card border border-border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group">
                <div className="absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity">
                    <CheckCircle2 className="w-16 h-16" />
                </div>
                <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">{t('accountManager.available')}</p>
                <div className="mt-2 flex items-baseline gap-2">
                    <span className="text-3xl font-bold text-foreground">{usable}</span>
                    <span className="text-xs text-muted-foreground">{t('accountManager.accountsUnit')}</span>
                </div>
                <p className="mt-2 text-xs text-muted-foreground">
                    {t('accountManager.accountPoolFormula', { available: usable, cooling, total })}
                </p>
            </div>
            <div className="bg-card border border-border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group">
                <div className="absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity">
                    <Clock className="w-16 h-16" />
                </div>
                <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">{t('accountManager.coolingDown')}</p>
                <div className="mt-2 flex items-baseline gap-2">
                    <span className="text-3xl font-bold text-foreground">{cooling}</span>
                    <span className="text-xs text-muted-foreground">{t('accountManager.accountsUnit')}</span>
                </div>
            </div>
            <div className="bg-card border border-border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group">
                <div className="absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity">
                    <Gauge className="w-16 h-16" />
                </div>
                <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">{t('accountManager.rpm')}</p>
                <div className="mt-2 flex items-baseline gap-2">
                    <span className="text-3xl font-bold text-foreground">{queueStatus.rpm ?? 0}</span>
                    <span className="text-xs text-muted-foreground">{t('accountManager.requestsPerMinuteUnit')}</span>
                </div>
            </div>
            <div className="bg-card border border-border rounded-xl p-4 flex flex-col justify-between shadow-sm relative overflow-hidden group">
                <div className="absolute right-0 top-0 p-4 opacity-5 group-hover:opacity-10 transition-opacity">
                    <Sigma className="w-16 h-16" />
                </div>
                <p className="text-xs font-medium text-muted-foreground uppercase tracking-widest">{t('accountManager.totalRequests')}</p>
                <div className="mt-2 flex items-baseline gap-2">
                    <span className="text-3xl font-bold text-foreground">{queueStatus.total_requests ?? 0}</span>
                    <span className="text-xs text-muted-foreground">{t('accountManager.requestsUnit')}</span>
                </div>
            </div>
        </div>
    )
}

import { Button } from "antd";
import { useEffect, useState } from "react";
import { listPluginRuns, type PluginRunView } from "@/services/api/plugin-operations";
import { useUserStore } from "@/stores/use-user-store";
import { PluginRunCard } from "./plugin-run-card";

export function PluginRunHistory() {
    const userID = useUserStore((s) => s.user?.id);
    const [offset, setOffset] = useState(0);
    const [refresh, setRefresh] = useState(0);
    const [rows, setRows] = useState<PluginRunView[]>([]);
    const [selected, setSelected] = useState("");
    const [error, setError] = useState("");
    useEffect(() => {
        const abort = new AbortController();
        setRows([]);
        setError("");
        void listPluginRuns(offset, abort.signal)
            .then((v) => {
                if (!abort.signal.aborted && useUserStore.getState().user?.id === userID) setRows(v);
            })
            .catch((e) => {
                if (!abort.signal.aborted) setError(e.message);
            });
        return () => abort.abort();
    }, [userID, offset, refresh]);
    return (
        <details className="space-y-2 text-sm">
            <summary>我的插件运行（关闭页面后可在此继续）</summary>
            <Button size="small" onClick={() => setRefresh((v) => v + 1)}>
                刷新运行列表
            </Button>
            {rows.map((row) => (
                <div key={row.id}>
                    <Button type="text" onClick={() => setSelected(row.id)}>
                        {row.operation} · {row.status} · {row.id.slice(0, 8)}
                    </Button>
                </div>
            ))}
            {!rows.length && !error && <p className="text-muted-foreground">暂无运行记录</p>}
            {error && (
                <p role="alert" className="text-destructive">
                    {error}
                </p>
            )}
            <Button size="small" disabled={offset === 0} onClick={() => setOffset((v) => Math.max(0, v - 30))}>
                上一页
            </Button>
            <Button size="small" disabled={rows.length < 30} onClick={() => setOffset((v) => v + 30)}>
                下一页
            </Button>
            {selected && <PluginRunCard key={`${userID}:${selected}`} runId={selected} />}
        </details>
    );
}
